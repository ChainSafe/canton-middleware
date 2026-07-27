// SPDX-License-Identifier: Apache-2.0

// Package xreserve is the TokenBridge adapter for USDCx (Circle xReserve).
// It's an observer: Circle runs the bridge, so there are no event sources —
// transfers are registered when initiated and Step just tracks them.
//
// Deposit: "" -> awaiting_attestation -> awaiting_mint -> completed ("minted").
// Mint detection snapshots the recipient balance on the first step (before
// finality, so before any mint) and completes once it grows by the amount.
//
// Limitation (#360, needs real mint-event data): balance-delta can't tie a
// mint to a specific deposit, so two concurrent same-recipient deposits (or an
// unrelated credit) can complete the wrong one. Bounded for now by rejecting
// non-positive amounts and failing anything past depositCompletionDeadline.
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

// Withdrawal stages.
const (
	StageAwaitingRelease = "awaiting_release"
	StageReleased        = "released"
)

// Metadata keys accumulated on transfers.
const (
	metaBaselineBalance = "baseline_balance"
	metaAttestationID   = "attestation_id"
	metaBurnRequestID   = "burn_request_id"
)

const (
	defaultAttestationPollInterval = time.Minute
	defaultMintPollInterval        = 15 * time.Second
	defaultHTTPTimeout             = 10 * time.Second

	// depositCompletionDeadline reaps deposits that never mint (bogus hashes,
	// never-attested). Well past finality+attestation (~15 min) so a healthy
	// deposit always completes first.
	depositCompletionDeadline = 2 * time.Hour
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
		return b.stepWithdrawal(ctx, rt, t)
	default:
		return relayer.StepResult{}, fmt.Errorf("unknown direction %q", t.Direction)
	}
}

// stepWithdrawal tracks a burn's destination-chain release. The burn itself
// was already exercised by the api-server (BridgeUserAgreement_Burn is a
// user-party choice); Circle controls the release timing, so this adapter
// only observes.
func (b *Bridge) stepWithdrawal(ctx context.Context, rt *tokenRuntime, t *relayer.Transfer) (relayer.StepResult, error) {
	switch t.Stage {
	case "", StageAwaitingRelease:
		requestID := t.Metadata[metaBurnRequestID]
		if requestID == "" {
			return relayer.StepResult{}, fmt.Errorf("withdrawal is missing %s metadata", metaBurnRequestID)
		}

		st, err := rt.circle.GetBurnStatus(ctx, requestID)
		switch {
		case errors.Is(err, ErrReleasePending):
			return relayer.StepResult{
				Status:     relayer.TransferStatusPending,
				Stage:      StageAwaitingRelease,
				RetryAfter: rt.attestationPollInterval(),
			}, nil
		case errors.Is(err, ErrAttestationUnavailable):
			b.logger.Warn("Burn-status service unavailable, will keep polling",
				zap.String("id", t.ID), zap.String("token", rt.symbol), zap.Error(err))
			return relayer.StepResult{
				Status:     relayer.TransferStatusPending,
				Stage:      StageAwaitingRelease,
				RetryAfter: rt.attestationPollInterval(),
			}, nil
		case err != nil:
			return relayer.StepResult{}, err
		}

		result := relayer.StepResult{
			Status: relayer.TransferStatusCompleted,
			Stage:  StageReleased,
		}
		if st.TxHash != "" {
			txHash := st.TxHash
			result.DestTxHash = &txHash
		}
		return result, nil
	default:
		return relayer.StepResult{}, fmt.Errorf("unknown withdrawal stage %q", t.Stage)
	}
}

func (b *Bridge) stepDeposit(ctx context.Context, rt *tokenRuntime, t *relayer.Transfer) (relayer.StepResult, error) {
	// Past the deadline it's never going to mint; fail it so the driver stops
	// reloading it forever.
	if !t.CreatedAt.IsZero() && time.Since(t.CreatedAt) > depositCompletionDeadline {
		b.logger.Warn("Deposit exceeded completion deadline, failing",
			zap.String("id", t.ID), zap.String("token", rt.symbol), zap.String("stage", t.Stage))
		return relayer.StepResult{
			Status: relayer.TransferStatusFailed,
			Stage:  t.Stage,
			Reason: fmt.Sprintf("deposit not minted within %s", depositCompletionDeadline),
		}, nil
	}

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
		Status:     relayer.TransferStatusPending,
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
			Status:     relayer.TransferStatusPending,
			Stage:      StageAwaitingAttestation,
			RetryAfter: rt.attestationPollInterval(),
		}, nil
	case errors.Is(err, ErrAttestationUnavailable):
		b.logger.Warn("Attestation service unavailable, will keep polling",
			zap.String("id", t.ID), zap.String("token", rt.symbol), zap.Error(err))
		return relayer.StepResult{
			Status:     relayer.TransferStatusPending,
			Stage:      StageAwaitingAttestation,
			RetryAfter: rt.attestationPollInterval(),
		}, nil
	case err != nil:
		return relayer.StepResult{}, err
	}

	return relayer.StepResult{
		Status:     relayer.TransferStatusPending,
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
			Status:     relayer.TransferStatusPending,
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
