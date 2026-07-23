// SPDX-License-Identifier: Apache-2.0

// Package xreserve implements the TokenBridge adapter for tokens bridged by
// Circle xReserve (USDCx). It is an observer mechanism: Circle executes the
// bridge, so the adapter exposes no event sources and never submits anything
// for deposits — transfers are registered at initiation time via the relayer
// API and Step only tracks their progress.
//
// Deposit stage sequence:
//
//	"" -> awaiting_attestation -> awaiting_mint -> completed (stage "minted")
//
// Mint detection is balance-based: the recipient's instrument balance is
// snapshotted on the first step (before Ethereum finality, so before any mint
// for this deposit can exist) and the transfer completes once the balance has
// grown by the deposited amount. This assumes the transfer is registered
// promptly after the deposit transaction is sent, well within the ~15 minute
// attestation window.
package xreserve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// Mechanism is the TokenConfig mechanism value and registry bridge key.
const Mechanism = "xreserve"

// Deposit stages.
const (
	StageAwaitingAttestation = "awaiting_attestation"
	StageAwaitingMint        = "awaiting_mint"
	StageMinted              = "minted"
)

// Metadata keys accumulated on transfers.
const (
	metaBaselineBalance = "baseline_balance"
	metaAttestationID   = "attestation_id"
)

const (
	defaultAttestationPollInterval = time.Minute
	defaultMintPollInterval        = 15 * time.Second
	defaultHTTPTimeout             = 10 * time.Second
)

// tokenRuntime is one configured xreserve token with its attestation client.
type tokenRuntime struct {
	symbol string
	cfg    relayer.XReserveConfig
	circle AttestationClient
}

// Bridge is the xreserve TokenBridge adapter.
type Bridge struct {
	tokens   map[string]*tokenRuntime // keyed by token symbol
	holdings HoldingLister
	logger   *zap.Logger
}

// Option customizes the Bridge, primarily for tests.
type Option func(*options)

type options struct {
	httpClient  *http.Client
	newAttester func(t *tokenRuntime) (AttestationClient, error)
}

// WithHTTPClient overrides the HTTP client used for attestation calls.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.httpClient = c }
}

// WithAttestationClient overrides attestation-client construction (tests).
func WithAttestationClient(client AttestationClient) Option {
	return func(o *options) {
		o.newAttester = func(*tokenRuntime) (AttestationClient, error) { return client, nil }
	}
}

// New creates the xreserve adapter for every configured token whose
// mechanism is "xreserve".
func New(
	tokens map[string]relayer.TokenConfig,
	holdings HoldingLister,
	logger *zap.Logger,
	opts ...Option,
) (*Bridge, error) {
	o := &options{httpClient: &http.Client{Timeout: defaultHTTPTimeout}}
	for _, opt := range opts {
		opt(o)
	}
	if o.newAttester == nil {
		o.newAttester = func(t *tokenRuntime) (AttestationClient, error) {
			return NewAttestationClient(t.cfg.AttestationURL, o.httpClient)
		}
	}

	b := &Bridge{
		tokens:   make(map[string]*tokenRuntime, len(tokens)),
		holdings: holdings,
		logger:   logger,
	}

	for symbol, tc := range tokens {
		if tc.Mechanism != Mechanism {
			return nil, fmt.Errorf("token %s: mechanism %q is not %q", symbol, tc.Mechanism, Mechanism)
		}
		if tc.XReserve == nil {
			return nil, fmt.Errorf("token %s: xreserve config block is required", symbol)
		}
		rt := &tokenRuntime{symbol: symbol, cfg: *tc.XReserve}
		circle, err := o.newAttester(rt)
		if err != nil {
			return nil, fmt.Errorf("token %s: %w", symbol, err)
		}
		rt.circle = circle
		b.tokens[symbol] = rt
	}
	if len(b.tokens) == 0 {
		return nil, errors.New("xreserve: no tokens configured")
	}
	return b, nil
}

// Key implements relayer.TokenBridge.
func (*Bridge) Key() string { return Mechanism }

// Sources implements relayer.TokenBridge. Observer mechanism: Circle executes
// the bridge whether or not we watch, so there is nothing to stream —
// transfers enter the store via the relayer registration API.
func (*Bridge) Sources(context.Context) ([]relayer.Source, error) { return nil, nil }

// Step implements relayer.TokenBridge.
func (b *Bridge) Step(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
	rt, ok := b.tokens[t.TokenSymbol]
	if !ok {
		return relayer.StepResult{}, fmt.Errorf("token %q is not configured for xreserve", t.TokenSymbol)
	}

	switch t.Direction {
	case relayer.DirectionEthereumToCanton:
		return b.stepDeposit(ctx, rt, t)
	case relayer.DirectionCantonToEthereum:
		// Outbound (BridgeUserAgreement_Burn + release tracking) lands with #359.
		return relayer.StepResult{}, fmt.Errorf("xreserve withdrawals are not supported yet")
	default:
		return relayer.StepResult{}, fmt.Errorf("unknown direction %q", t.Direction)
	}
}

func (b *Bridge) stepDeposit(ctx context.Context, rt *tokenRuntime, t *relayer.Transfer) (relayer.StepResult, error) {
	switch t.Stage {
	case "":
		return b.snapshotBaseline(ctx, rt, t)
	case StageAwaitingAttestation:
		return b.pollAttestation(ctx, rt, t)
	case StageAwaitingMint:
		return b.checkMinted(ctx, rt, t)
	default:
		return relayer.StepResult{}, fmt.Errorf("unknown deposit stage %q", t.Stage)
	}
}

// snapshotBaseline records the recipient's pre-mint balance so mint arrival
// is detectable as a balance increase, then moves to attestation polling.
func (b *Bridge) snapshotBaseline(ctx context.Context, rt *tokenRuntime, t *relayer.Transfer) (relayer.StepResult, error) {
	baseline, err := partyBalance(ctx, b.holdings, t.Recipient, rt.cfg.InstrumentAdmin, rt.cfg.InstrumentID)
	if err != nil {
		return relayer.StepResult{}, err
	}

	return relayer.StepResult{
		Status:     relayer.TransferStatusInProgress,
		Stage:      StageAwaitingAttestation,
		Metadata:   map[string]string{metaBaselineBalance: baseline.String()},
		RetryAfter: rt.attestationPollInterval(),
	}, nil
}

// pollAttestation asks Circle whether the deposit is attested yet. Not-ready
// and transient service failures both keep polling; only unexpected responses
// surface as step errors.
func (b *Bridge) pollAttestation(ctx context.Context, rt *tokenRuntime, t *relayer.Transfer) (relayer.StepResult, error) {
	att, err := rt.circle.GetAttestation(ctx, t.SourceTxHash)
	switch {
	case errors.Is(err, ErrAttestationNotReady):
		return relayer.StepResult{
			Status:     relayer.TransferStatusInProgress,
			Stage:      StageAwaitingAttestation,
			RetryAfter: rt.attestationPollInterval(),
		}, nil
	case errors.Is(err, ErrAttestationUnavailable):
		b.logger.Warn("Attestation service unavailable, will keep polling",
			zap.String("id", t.ID), zap.String("token", rt.symbol), zap.Error(err))
		return relayer.StepResult{
			Status:     relayer.TransferStatusInProgress,
			Stage:      StageAwaitingAttestation,
			RetryAfter: rt.attestationPollInterval(),
		}, nil
	case err != nil:
		return relayer.StepResult{}, err
	}

	return relayer.StepResult{
		Status:     relayer.TransferStatusInProgress,
		Stage:      StageAwaitingMint,
		Metadata:   map[string]string{metaAttestationID: att.ID},
		RetryAfter: rt.mintPollInterval(),
	}, nil
}

// checkMinted completes the transfer once the recipient's balance has grown
// by the deposited amount over the recorded baseline. The mint itself is
// performed by the recipient's pre-approved BridgeUserAgreement, not by us.
func (b *Bridge) checkMinted(ctx context.Context, rt *tokenRuntime, t *relayer.Transfer) (relayer.StepResult, error) {
	baseline, err := decimal.NewFromString(t.Metadata[metaBaselineBalance])
	if err != nil {
		return relayer.StepResult{}, fmt.Errorf("invalid baseline balance %q: %w", t.Metadata[metaBaselineBalance], err)
	}
	amount, err := decimal.NewFromString(t.Amount)
	if err != nil {
		return relayer.StepResult{}, fmt.Errorf("invalid transfer amount %q: %w", t.Amount, err)
	}

	current, err := partyBalance(ctx, b.holdings, t.Recipient, rt.cfg.InstrumentAdmin, rt.cfg.InstrumentID)
	if err != nil {
		return relayer.StepResult{}, err
	}

	if current.LessThan(baseline.Add(amount)) {
		return relayer.StepResult{
			Status:     relayer.TransferStatusInProgress,
			Stage:      StageAwaitingMint,
			RetryAfter: rt.mintPollInterval(),
		}, nil
	}

	return relayer.StepResult{
		Status: relayer.TransferStatusCompleted,
		Stage:  StageMinted,
	}, nil
}

func (rt *tokenRuntime) attestationPollInterval() time.Duration {
	if rt.cfg.AttestationPollInterval > 0 {
		return rt.cfg.AttestationPollInterval
	}
	return defaultAttestationPollInterval
}

func (rt *tokenRuntime) mintPollInterval() time.Duration {
	if rt.cfg.MintPollInterval > 0 {
		return rt.cfg.MintPollInterval
	}
	return defaultMintPollInterval
}
