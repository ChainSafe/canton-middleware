package transfer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// allowedTokenSymbols defines the set of valid token symbols for transfers.
var allowedTokenSymbols = map[string]bool{
	"DEMO":   true,
	"PROMPT": true,
}

// UserStore is the narrow interface for looking up users.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
}

// TransferService implements the non-custodial prepare/execute transfer flow.
type TransferService struct {
	cantonToken token.Token
	userStore   UserStore
}

// NewTransferService creates a new TransferService.
func NewTransferService(cantonToken token.Token, userStore UserStore) *TransferService {
	return &TransferService{
		cantonToken: cantonToken,
		userStore:   userStore,
	}
}

// Prepare builds a Canton transaction and returns the hash for external signing.
func (s *TransferService) Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error) {
	if req.To == "" || req.Amount == "" || req.Token == "" {
		return nil, apperrors.BadRequestError(nil, "to, amount, and token are required")
	}

	if !auth.ValidateEVMAddress(req.To) {
		return nil, apperrors.BadRequestError(nil, "invalid recipient address: must be a 0x-prefixed 40-hex-char EVM address")
	}

	amt, err := decimal.NewFromString(req.Amount)
	if err != nil || !amt.IsPositive() {
		return nil, apperrors.BadRequestError(nil, "invalid amount: must be a positive decimal number")
	}

	if !allowedTokenSymbols[req.Token] {
		return nil, apperrors.BadRequestError(nil, "unsupported token: must be DEMO or PROMPT")
	}

	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, "prepare/execute API requires key_mode=external")
	}

	recipient, err := s.userStore.GetUserByEVMAddress(ctx, req.To)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.BadRequestError(err, "recipient not found")
		}
		return nil, fmt.Errorf("lookup recipient: %w", err)
	}

	pt, err := s.cantonToken.PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   recipient.CantonPartyID,
		Amount:      req.Amount,
		TokenSymbol: req.Token,
	})
	if err != nil {
		if errors.Is(err, token.ErrInsufficientBalance) {
			return nil, apperrors.BadRequestError(err, "insufficient balance")
		}
		return nil, fmt.Errorf("prepare transfer: %w", err)
	}

	return &PrepareResponse{
		TransferID:      pt.TransferID,
		TransactionHash: "0x" + hex.EncodeToString(pt.TransactionHash),
		PartyID:         pt.PartyID,
		ExpiresAt:       pt.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// Execute completes a previously prepared transfer using the client's DER signature.
func (s *TransferService) Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error) {
	if req.TransferID == "" || req.Signature == "" || req.SignedBy == "" {
		return nil, apperrors.BadRequestError(nil, "transfer_id, signature, and signed_by are required")
	}

	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.CantonPublicKeyFingerprint != req.SignedBy {
		return nil, apperrors.ForbiddenError(nil, "signature fingerprint does not match registered key")
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(req.Signature, "0x"))
	if err != nil {
		return nil, apperrors.BadRequestError(err, "invalid DER signature")
	}

	err = s.cantonToken.ExecuteTransfer(ctx, &token.ExecuteTransferRequest{
		TransferID: req.TransferID,
		Signature:  sigBytes,
		SignedBy:   req.SignedBy,
	})
	if err != nil {
		if errors.Is(err, token.ErrTransferNotFound) {
			return nil, apperrors.ResourceNotFoundError(err, "transfer not found")
		}
		if errors.Is(err, token.ErrTransferExpired) {
			return nil, apperrors.GoneError(err, "transfer expired")
		}
		return nil, fmt.Errorf("execute transfer: %w", err)
	}

	return &ExecuteResponse{Status: "completed"}, nil
}
