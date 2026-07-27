// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// CantonBurner is the slice of the Canton token client the withdraw flow
// needs: burn preparation/execution for external keys and direct burns for
// custodial parties.
type CantonBurner interface {
	PrepareBurn(ctx context.Context, req *cantontkn.PrepareBurnRequest) (*cantontkn.PreparedTransfer, error)
	BurnByPartyID(ctx context.Context, req *cantontkn.PrepareBurnRequest) error
	ExecuteTransfer(ctx context.Context, req *cantontkn.ExecuteTransferRequest) error
}

// WithdrawRequest asks to bridge Amount of Token from the authenticated
// user's Canton holdings back to an EVM address.
type WithdrawRequest struct {
	Token string `json:"token"`
	// Amount is a decimal string in token units.
	Amount string `json:"amount"`
	// Recipient is the destination EVM address.
	Recipient string `json:"recipient"`
	// IdempotencyKey ties a retry to the same burn so it never burns twice. Required.
	IdempotencyKey string `json:"idempotency_key"`
}

// WithdrawPrepareResponse carries the hash an external-key user signs.
type WithdrawPrepareResponse struct {
	TransferID      string    `json:"transfer_id"`
	TransactionHash string    `json:"transaction_hash"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// WithdrawExecuteRequest completes a prepared withdrawal with the user's
// signature over the transaction hash.
type WithdrawExecuteRequest struct {
	TransferID string `json:"transfer_id"`
	Signature  string `json:"signature"`
	SignedBy   string `json:"signed_by"`
}

// preparedBurn holds a prepared-but-unsigned burn between prepare and execute.
type preparedBurn struct {
	Prepared     *cantontkn.PreparedTransfer
	Owner        string // authenticated EVM address
	TokenSymbol  string
	TokenAddress string
	Amount       string
	Recipient    string // destination EVM address
	Sender       string // Canton party performing the burn
	RequestID    string
}

// burnStore is the in-memory holding pen for prepared burns, mirroring the
// quote store's process-local semantics.
type burnStore struct {
	mu    sync.Mutex
	burns map[string]*preparedBurn
	now   func() time.Time
}

func newBurnStore() *burnStore {
	return &burnStore{burns: make(map[string]*preparedBurn), now: time.Now}
}

func (s *burnStore) Put(b *preparedBurn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, stored := range s.burns {
		if now.After(stored.Prepared.ExpiresAt) {
			delete(s.burns, id)
		}
	}
	s.burns[b.Prepared.TransferID] = b
}

// Peek returns the prepared burn without consuming it, so callers can validate
// before the single-use Take.
func (s *burnStore) Peek(transferID string) (*preparedBurn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.burns[transferID]
	if !ok || s.now().After(b.Prepared.ExpiresAt) {
		return nil, false
	}
	return b, true
}

// Take removes and returns the prepared burn if present and unexpired.
func (s *burnStore) Take(transferID string) (*preparedBurn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.burns[transferID]
	if !ok {
		return nil, false
	}
	delete(s.burns, transferID)
	if s.now().After(b.Prepared.ExpiresAt) {
		return nil, false
	}
	return b, true
}

// WithdrawPrepare builds a BridgeUserAgreement_Burn for an external-key user
// and returns the hash to sign.
func (s *bridgeService) WithdrawPrepare(
	ctx context.Context, evmAddress string, req *WithdrawRequest,
) (*WithdrawPrepareResponse, error) {
	tc, usr, err := s.validateWithdraw(ctx, evmAddress, req)
	if err != nil {
		return nil, err
	}
	if usr.KeyMode != user.KeyModeCustodial && usr.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, fmt.Sprintf("unknown key mode %q", usr.KeyMode))
	}
	if usr.KeyMode == user.KeyModeCustodial {
		return nil, apperrors.BadRequestError(nil, "custodial users must use the custodial withdraw endpoint")
	}

	// Use the idempotency key as the stable request id so a re-prepare dedupes.
	requestID := req.IdempotencyKey
	prepared, err := s.canton.PrepareBurn(ctx, burnRequest(tc, usr.CantonParty, req, requestID))
	if err != nil {
		return nil, fmt.Errorf("prepare burn: %w", err)
	}

	s.burns.Put(&preparedBurn{
		Prepared:     prepared,
		Owner:        evmAddress,
		TokenSymbol:  req.Token,
		TokenAddress: tc.EVMAddress,
		Amount:       req.Amount,
		Recipient:    req.Recipient,
		Sender:       usr.CantonParty,
		RequestID:    requestID,
	})

	return &WithdrawPrepareResponse{
		TransferID:      prepared.TransferID,
		TransactionHash: "0x" + hex.EncodeToString(prepared.TransactionHash),
		ExpiresAt:       prepared.ExpiresAt,
	}, nil
}

// WithdrawExecute submits the user-signed burn and tracks it. It validates
// (via Peek) before consuming the burn, so a bad request can't destroy it, and
// registers before submitting so a crash never leaves an untracked burn.
func (s *bridgeService) WithdrawExecute(
	ctx context.Context, evmAddress string, req *WithdrawExecuteRequest,
) (*relayer.RegisterTransferResponse, error) {
	burn, ok := s.burns.Peek(req.TransferID)
	if !ok {
		return nil, apperrors.ResourceNotFoundError(nil, "unknown or expired transfer_id")
	}
	if burn.Owner != evmAddress {
		return nil, apperrors.UnAuthorizedError(nil, "transfer belongs to a different address")
	}

	usr, err := s.userStore.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		return nil, apperrors.UnAuthorizedError(err, "user not found")
	}
	if usr.CantonPublicKeyFingerprint != req.SignedBy {
		return nil, apperrors.ForbiddenError(nil, "signature fingerprint does not match registered key")
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(req.Signature, "0x"))
	if err != nil {
		return nil, apperrors.BadRequestError(err, "invalid DER signature")
	}

	// Consume the single-use burn; a concurrent duplicate loses this race.
	if _, ok = s.burns.Take(req.TransferID); !ok {
		return nil, apperrors.ResourceNotFoundError(nil, "transfer already executed")
	}

	resp, created, err := s.registerWithdrawal(ctx, burn.TokenSymbol, burn.TokenAddress,
		burn.Amount, burn.Sender, burn.Recipient, burn.RequestID)
	if err != nil {
		return nil, err
	}
	if !created {
		return resp, nil // already submitted under this id; don't burn again
	}

	if err = s.canton.ExecuteTransfer(ctx, &cantontkn.ExecuteTransferRequest{
		PreparedTransfer: burn.Prepared,
		Signature:        sigBytes,
		SignedBy:         req.SignedBy,
	}); err != nil {
		return nil, fmt.Errorf("execute burn: %w", err)
	}
	return resp, nil
}

// WithdrawCustodial burns server-side for a custodial user. It registers
// (keyed by the idempotency key) before burning, so a retry of a lost-response
// call returns the existing transfer instead of burning twice.
func (s *bridgeService) WithdrawCustodial(
	ctx context.Context, evmAddress string, req *WithdrawRequest,
) (*relayer.RegisterTransferResponse, error) {
	tc, usr, err := s.validateWithdraw(ctx, evmAddress, req)
	if err != nil {
		return nil, err
	}
	if usr.KeyMode != user.KeyModeCustodial {
		return nil, apperrors.BadRequestError(nil, "only custodial users can use this endpoint")
	}

	requestID := req.IdempotencyKey
	resp, created, err := s.registerWithdrawal(ctx, req.Token, tc.EVMAddress,
		req.Amount, usr.CantonParty, req.Recipient, requestID)
	if err != nil {
		return nil, err
	}
	if !created {
		return resp, nil // retry: already submitted under this id, don't burn again
	}

	if err = s.canton.BurnByPartyID(ctx, burnRequest(tc, usr.CantonParty, req, requestID)); err != nil {
		return nil, fmt.Errorf("burn: %w", err)
	}
	return resp, nil
}

// validateWithdraw shares the request/token/user checks between the
// external-key and custodial withdraw paths.
func (s *bridgeService) validateWithdraw(
	ctx context.Context, evmAddress string, req *WithdrawRequest,
) (TokenConfig, *user.User, error) {
	tc, ok := s.cfg.Tokens[req.Token]
	if !ok || tc.Mechanism != MechanismXReserve || tc.XReserve == nil {
		return TokenConfig{}, nil, apperrors.BadRequestError(nil,
			fmt.Sprintf("token %q does not support bridged withdrawals", req.Token))
	}
	if s.canton == nil {
		return TokenConfig{}, nil, errors.New("withdrawals are not configured (no canton client)")
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.IsPositive() {
		return TokenConfig{}, nil, apperrors.BadRequestError(nil, "invalid amount: must be a positive decimal number")
	}
	if !auth.ValidateEVMAddress(req.Recipient) {
		return TokenConfig{}, nil, apperrors.BadRequestError(nil,
			"invalid recipient: must be a 0x-prefixed 40-hex-char EVM address")
	}
	if req.IdempotencyKey == "" {
		return TokenConfig{}, nil, apperrors.BadRequestError(nil, "idempotency_key is required")
	}

	usr, err := s.userStore.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		return TokenConfig{}, nil, apperrors.ResourceNotFoundError(err, "user is not registered")
	}
	return tc, usr, nil
}

func burnRequest(tc TokenConfig, partyID string, req *WithdrawRequest, requestID string) *cantontkn.PrepareBurnRequest {
	return &cantontkn.PrepareBurnRequest{
		PartyID: partyID,
		// The Canton instrument id, not the API symbol: holdings selection
		// queries the Splice HoldingV1 interface by instrument.
		TokenSymbol:       tc.XReserve.InstrumentID,
		Amount:            req.Amount,
		InstrumentAdmin:   tc.XReserve.InstrumentAdmin,
		DestinationDomain: tc.XReserve.WithdrawDestinationDomain,
		EvmRecipient:      req.Recipient,
		RequestID:         requestID,
	}
}

// registerWithdrawal records the pending withdrawal (before the burn) so the
// adapter can track the release. Its created result drives idempotency: false
// means this id was already registered (a retry), so don't burn again.
func (s *bridgeService) registerWithdrawal(
	ctx context.Context,
	tokenSymbol, tokenAddress, amount, senderParty, recipient, requestID string,
) (resp *relayer.RegisterTransferResponse, created bool, err error) {
	id := registrationID(MechanismXReserve, requestID)
	resp, err = s.relayer.RegisterTransfer(ctx, &relayer.RegisterTransferRequest{
		ID:           id,
		BridgeKey:    MechanismXReserve,
		TokenSymbol:  tokenSymbol,
		Direction:    relayer.DirectionCantonToEthereum,
		SourceTxHash: requestID,
		TokenAddress: tokenAddress,
		Amount:       amount,
		Sender:       senderParty,
		Recipient:    recipient,
		Metadata:     map[string]string{relayer.MetaBurnRequestID: requestID},
	})
	if err != nil {
		return nil, false, fmt.Errorf("register withdrawal: %w", err)
	}
	return resp, resp.Created, nil
}
