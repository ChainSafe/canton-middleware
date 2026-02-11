package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
)

var (
	ErrUserNotRegistered = errors.New("user not registered")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAddress    = errors.New("invalid address")
	ErrRecipientNotFound = errors.New("recipient not registered")
)

// TokenService provides shared token operations for both RPC and EthRPC endpoints
type TokenService struct {
	config       *config.APIServerConfig
	db           *apidb.Store
	cantonClient *canton.Client
	logger       *zap.Logger
}

// NewTokenService creates a new token service
func NewTokenService(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	cantonClient *canton.Client,
	logger *zap.Logger,
) *TokenService {
	return &TokenService{
		config:       cfg,
		db:           db,
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
		tokenSymbol = "PROMPT"
	}

	if !auth.ValidateEVMAddress(toAddress) {
		return nil, ErrInvalidAddress
	}

	fromUser, err := s.db.GetUserByEVMAddress(fromAddress)
	if err != nil {
		s.logger.Error("Failed to get sender", zap.Error(err))
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}
	if fromUser == nil || fromUser.Fingerprint == "" {
		return nil, ErrUserNotRegistered
	}

	toUser, err := s.db.GetUserByEVMAddress(toAddress)
	if err != nil {
		s.logger.Error("Failed to get recipient", zap.Error(err))
		return nil, fmt.Errorf("failed to get recipient: %w", err)
	}
	if toUser == nil || toUser.Fingerprint == "" {
		return nil, ErrRecipientNotFound
	}

	err = s.cantonClient.TransferAsUserByFingerprint(ctx,
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

	// Determine DB token type from symbol
	dbTokenType := apidb.TokenPrompt
	if tokenSymbol == "DEMO" {
		dbTokenType = apidb.TokenDemo
	}

	if err := s.db.TransferBalanceByFingerprint(fromUser.Fingerprint, toUser.Fingerprint, req.Amount, dbTokenType); err != nil {
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
		tokenSymbol = "PROMPT"
	}

	user, err := s.db.GetUserByEVMAddress(addr)
	if err != nil {
		return "0", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil || user.Fingerprint == "" {
		return "0", nil
	}

	dbTokenType := apidb.TokenPrompt
	if tokenSymbol == "DEMO" {
		dbTokenType = apidb.TokenDemo
	}

	balance, err := s.db.GetBalanceByFingerprint(user.Fingerprint, dbTokenType)
	if err != nil {
		s.logger.Warn("Failed to get balance from cache, returning 0",
			zap.String("token", tokenSymbol),
			zap.String("fingerprint", user.Fingerprint),
			zap.Error(err))
		return "0", nil
	}

	return balance, nil
}

// GetTotalSupply returns the total supply for a specific token
func (s *TokenService) GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error) {
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
func (s *TokenService) IsUserRegistered(evmAddress string) (bool, error) {
	addr := auth.NormalizeAddress(evmAddress)
	user, err := s.db.GetUserByEVMAddress(addr)
	if err != nil {
		return false, err
	}
	return user != nil && user.Fingerprint != "", nil
}

func isInsufficientFunds(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, canton.ErrInsufficientBalance) || errors.Is(err, canton.ErrBalanceFragmented) {
		return true
	}
	return false
}

// GetCantonClient returns the underlying Canton client for direct access
func (s *TokenService) GetCantonClient() *canton.Client {
	return s.cantonClient
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

	// Get sender user by party ID
	fromUser, err := s.db.GetUserByCantonPartyID(req.FromPartyID)
	if err != nil {
		s.logger.Error("Failed to get sender by party ID", zap.Error(err))
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}
	if fromUser == nil {
		return nil, ErrUserNotRegistered
	}

	// Get recipient user by party ID
	toUser, err := s.db.GetUserByCantonPartyID(req.ToPartyID)
	if err != nil {
		s.logger.Error("Failed to get recipient by party ID", zap.Error(err))
		return nil, fmt.Errorf("failed to get recipient: %w", err)
	}
	if toUser == nil {
		return nil, ErrRecipientNotFound
	}

	tokenSymbol := req.Token
	if tokenSymbol == "" {
		tokenSymbol = "DEMO"
	}

	dbTokenType := apidb.TokenDemo
	if tokenSymbol == "PROMPT" {
		dbTokenType = apidb.TokenPrompt
	}

	err = s.cantonClient.TransferAsUserByFingerprint(ctx,
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

	if err := s.db.TransferBalanceByFingerprint(fromUser.Fingerprint, toUser.Fingerprint, req.Amount, dbTokenType); err != nil {
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
func (s *TokenService) GetUserByPartyID(partyID string) (*apidb.User, error) {
	return s.db.GetUserByCantonPartyID(partyID)
}
