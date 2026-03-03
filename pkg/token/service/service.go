package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
)

var (
	ErrUserNotRegistered = errors.New("user not registered")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAddress    = errors.New("invalid address")
	ErrRecipientNotFound = errors.New("recipient not registered")
)

const (
	tokenSymbolPrompt = "PROMPT"
	tokenSymbolDemo   = "DEMO"
)

// UserStore defines the minimal user persistence behavior this service needs.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
	GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error)
	TransferBalanceByFingerprint(ctx context.Context, fromFingerprint, toFingerprint, amount string, tokenType token.Type) error
}

// TokenService provides shared token operations for both RPC and EthRPC endpoints
type TokenService struct {
	config       *config.APIServerConfig
	db           *apidb.Store // financial metrics: total supply
	userStore    UserStore
	cantonClient canton.Token
	logger       *zap.Logger
}

// NewTokenService creates a new token service
func NewTokenService(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	userStore UserStore,
	cantonClient canton.Token,
	logger *zap.Logger,
) *TokenService {
	return &TokenService{
		config:       cfg,
		db:           db,
		userStore:    userStore,
		cantonClient: cantonClient,
		logger:       logger,
	}
}

// TransferRequest represents a token transfer request
type TransferRequest struct {
	FromEVMAddress string
	ToEVMAddress   string
	Amount         string
	TokenSymbol    string // "PROMPT" or "DEMO" (defaults to "PROMPT" if empty)
}

// TransferResult represents the result of a transfer
type TransferResult struct {
	Success         bool
	FromFingerprint string
	ToFingerprint   string
}

// Transfer executes a token transfer from one user to another using user-owned holdings.
// Works for any token (PROMPT, DEMO, etc.) based on TokenSymbol field.
func (s *TokenService) Transfer(ctx context.Context, req *TransferRequest) (*TransferResult, error) {
	fromAddress := auth.NormalizeAddress(req.FromEVMAddress)
	toAddress := auth.NormalizeAddress(req.ToEVMAddress)

	tokenSymbol := req.TokenSymbol
	if tokenSymbol == "" {
		tokenSymbol = tokenSymbolPrompt
	}

	if !auth.ValidateEVMAddress(toAddress) {
		return nil, ErrInvalidAddress
	}

	fromUser, err := s.userStore.GetUserByEVMAddress(ctx, fromAddress)
	if err != nil {
		if errors.Is(err, userstore.ErrUserNotFound) {
			return nil, ErrUserNotRegistered
		}
		s.logger.Error("Failed to get sender", zap.Error(err))
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}
	if fromUser.Fingerprint == "" {
		return nil, ErrUserNotRegistered
	}

	toUser, err := s.userStore.GetUserByEVMAddress(ctx, toAddress)
	if err != nil {
		if errors.Is(err, userstore.ErrUserNotFound) {
			return nil, ErrRecipientNotFound
		}
		s.logger.Error("Failed to get recipient", zap.Error(err))
		return nil, fmt.Errorf("failed to get recipient: %w", err)
	}
	if toUser.Fingerprint == "" {
		return nil, ErrRecipientNotFound
	}

	err = s.cantonClient.TransferByFingerprint(ctx,
		fromUser.Fingerprint,
		toUser.Fingerprint,
		req.Amount,
		tokenSymbol)
	if err != nil {
		s.logger.Error("Transfer failed",
			zap.String("token", tokenSymbol),
			zap.String("from", fromAddress),
			zap.String("to", toAddress),
			zap.String("amount", req.Amount),
			zap.Error(err))

		if isInsufficientFunds(err) {
			return nil, ErrInsufficientFunds
		}
		return nil, fmt.Errorf("canton transfer failed: %w", err)
	}

	tokenType := token.Prompt
	if tokenSymbol == tokenSymbolDemo {
		tokenType = token.Demo
	}

	if err := s.userStore.TransferBalanceByFingerprint(ctx, fromUser.Fingerprint, toUser.Fingerprint, req.Amount, tokenType); err != nil {
		s.logger.Warn("Failed to update balance cache",
			zap.String("token", tokenSymbol),
			zap.String("from_fingerprint", fromUser.Fingerprint),
			zap.String("to_fingerprint", toUser.Fingerprint),
			zap.String("amount", req.Amount),
			zap.Error(err))
	}

	s.logger.Info("Transfer completed",
		zap.String("token", tokenSymbol),
		zap.String("from", fromAddress),
		zap.String("to", toAddress),
		zap.String("amount", req.Amount))

	return &TransferResult{
		Success:         true,
		FromFingerprint: fromUser.Fingerprint,
		ToFingerprint:   toUser.Fingerprint,
	}, nil
}

// GetBalance returns the token balance for an EVM address.
// tokenSymbol defaults to "PROMPT" if empty.
func (s *TokenService) GetBalance(ctx context.Context, evmAddress, tokenSymbol string) (string, error) {
	addr := auth.NormalizeAddress(evmAddress)

	if tokenSymbol == "" {
		tokenSymbol = tokenSymbolPrompt
	}

	user, err := s.userStore.GetUserByEVMAddress(ctx, addr)
	if err != nil {
		if errors.Is(err, userstore.ErrUserNotFound) {
			return "0", nil
		}
		return "0", fmt.Errorf("failed to get user: %w", err)
	}
	if user.Fingerprint == "" {
		return "0", nil
	}

	if tokenSymbol == tokenSymbolDemo {
		if user.DemoBalance == "" {
			return "0", nil
		}
		return user.DemoBalance, nil
	}
	if user.PromptBalance == "" {
		return "0", nil
	}
	return user.PromptBalance, nil
}

// GetTotalSupply returns the total supply for a specific token
func (s *TokenService) GetTotalSupply(_ context.Context, tokenSymbol string) (string, error) {
	return s.db.GetTotalSupply(tokenSymbol)
}

// GetTokenName returns the token name from config
func (s *TokenService) GetTokenName() string {
	return s.config.Token.Name
}

// GetTokenSymbol returns the token symbol from config
func (s *TokenService) GetTokenSymbol() string {
	return s.config.Token.Symbol
}

// GetTokenDecimals returns the token decimals from config
func (s *TokenService) GetTokenDecimals() int {
	return s.config.Token.Decimals
}

// IsUserRegistered checks if an EVM address is registered
func (s *TokenService) IsUserRegistered(ctx context.Context, evmAddress string) (bool, error) {
	addr := auth.NormalizeAddress(evmAddress)
	_, err := s.userStore.GetUserByEVMAddress(ctx, addr)
	if err != nil {
		if errors.Is(err, userstore.ErrUserNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isInsufficientFunds(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, canton.ErrInsufficientBalance) {
		return true
	}
	return false
}

// CantonTransferRequest represents a Canton-native token transfer request using party IDs
type CantonTransferRequest struct {
	FromPartyID string // Sender's Canton party ID
	ToPartyID   string // Recipient's Canton party ID
	Amount      string // Amount in token units (e.g., "50.5")
	Token       string // "DEMO" or "PROMPT"
}

// TransferByPartyID executes a CIP56 token transfer using Canton party IDs directly
// This is the native Canton way to transfer tokens, without going through the EVM facade
func (s *TokenService) TransferByPartyID(ctx context.Context, req *CantonTransferRequest) (*TransferResult, error) {
	// Validate party ID format
	if err := auth.ValidateCantonPartyID(req.FromPartyID); err != nil {
		return nil, fmt.Errorf("invalid sender party ID: %w", err)
	}
	if err := auth.ValidateCantonPartyID(req.ToPartyID); err != nil {
		return nil, fmt.Errorf("invalid recipient party ID: %w", err)
	}

	fromUser, err := s.userStore.GetUserByCantonPartyID(ctx, req.FromPartyID)
	if err != nil {
		if errors.Is(err, userstore.ErrUserNotFound) {
			return nil, ErrUserNotRegistered
		}
		s.logger.Error("Failed to get sender by party ID", zap.Error(err))
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}

	toUser, err := s.userStore.GetUserByCantonPartyID(ctx, req.ToPartyID)
	if err != nil {
		if errors.Is(err, userstore.ErrUserNotFound) {
			return nil, ErrRecipientNotFound
		}
		s.logger.Error("Failed to get recipient by party ID", zap.Error(err))
		return nil, fmt.Errorf("failed to get recipient: %w", err)
	}

	tokenSymbol := req.Token
	if tokenSymbol == "" {
		tokenSymbol = tokenSymbolDemo
	}

	tokenType := token.Demo
	if tokenSymbol == tokenSymbolPrompt {
		tokenType = token.Prompt
	}

	err = s.cantonClient.TransferByFingerprint(ctx,
		fromUser.Fingerprint,
		toUser.Fingerprint,
		req.Amount,
		tokenSymbol)
	if err != nil {
		s.logger.Error("Canton transfer failed",
			zap.String("from_party", req.FromPartyID),
			zap.String("to_party", req.ToPartyID),
			zap.String("amount", req.Amount),
			zap.String("token", tokenSymbol),
			zap.Error(err))

		if isInsufficientFunds(err) {
			return nil, ErrInsufficientFunds
		}
		return nil, fmt.Errorf("canton transfer failed: %w", err)
	}

	if err := s.userStore.TransferBalanceByFingerprint(ctx, fromUser.Fingerprint, toUser.Fingerprint, req.Amount, tokenType); err != nil {
		s.logger.Warn("Failed to update balance cache",
			zap.String("from_fingerprint", fromUser.Fingerprint),
			zap.String("to_fingerprint", toUser.Fingerprint),
			zap.String("token", tokenSymbol),
			zap.Error(err))
	}

	s.logger.Info("Canton native transfer completed",
		zap.String("from_party", req.FromPartyID),
		zap.String("to_party", req.ToPartyID),
		zap.String("amount", req.Amount),
		zap.String("token", tokenSymbol))

	return &TransferResult{
		Success:         true,
		FromFingerprint: fromUser.Fingerprint,
		ToFingerprint:   toUser.Fingerprint,
	}, nil
}

// GetUserByPartyID retrieves user info by Canton party ID
func (s *TokenService) GetUserByPartyID(ctx context.Context, partyID string) (*user.User, error) {
	return s.userStore.GetUserByCantonPartyID(ctx, partyID)
}
