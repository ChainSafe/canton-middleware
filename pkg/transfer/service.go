package transfer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// UserStore is the narrow interface for looking up users.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
}

// TransferCache is the interface for caching prepared transfers.
type TransferCache interface {
	Put(transfer *token.PreparedTransfer) error
	GetAndDelete(transferID string) (*token.PreparedTransfer, error)
}

// Service is the interface for the non-custodial prepare/execute transfer flow.
type Service interface {
	Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error)
	Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error)
}

// TransferService implements the non-custodial prepare/execute transfer flow.
type TransferService struct {
	cantonToken         token.Token
	userStore           UserStore
	cache               TransferCache
	allowedTokenSymbols map[string]bool
}

// NewTransferService creates a new TransferService.
// allowedSymbols defines the set of valid token symbols (e.g. "DEMO", "PROMPT").
func NewTransferService(cantonToken token.Token, userStore UserStore, cache TransferCache, allowedSymbols []string) *TransferService {
	allowed := make(map[string]bool, len(allowedSymbols))
	for _, s := range allowedSymbols {
		allowed[s] = true
	}
	return &TransferService{
		cantonToken:         cantonToken,
		userStore:           userStore,
		cache:               cache,
		allowedTokenSymbols: allowed,
	}
}

// Prepare builds a Canton transaction and returns the hash for external signing.
func (s *TransferService) Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error) {
	if !s.allowedTokenSymbols[req.Token] {
		return nil, apperrors.BadRequestError(nil, "unsupported token")
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

	if err := s.cache.Put(pt); err != nil {
		return nil, apperrors.GeneralError(fmt.Errorf("too many pending transfers: %w", err))
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

	pt, err := s.cache.GetAndDelete(req.TransferID)
	if err != nil {
		if errors.Is(err, ErrTransferNotFound) {
			return nil, apperrors.ResourceNotFoundError(err, "transfer not found")
		}
		if errors.Is(err, ErrTransferExpired) {
			return nil, apperrors.GoneError(err, "transfer expired")
		}
		return nil, fmt.Errorf("retrieve prepared transfer: %w", err)
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(req.Signature, "0x"))
	if err != nil {
		return nil, apperrors.BadRequestError(err, "invalid DER signature")
	}

	err = s.cantonToken.ExecuteTransfer(ctx, &token.ExecuteTransferRequest{
		PreparedTransfer: pt,
		Signature:        sigBytes,
		SignedBy:         req.SignedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("execute transfer: %w", err)
	}

	return &ExecuteResponse{Status: "completed"}, nil
}
