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
}

// TransferResult represents the result of a transfer
type TransferResult struct {
	Success         bool
	FromFingerprint string
	ToFingerprint   string
}

// Transfer executes a token transfer from one user to another
func (s *TokenService) Transfer(ctx context.Context, req *TransferRequest) (*TransferResult, error) {
	fromAddress := auth.NormalizeAddress(req.FromEVMAddress)
	toAddress := auth.NormalizeAddress(req.ToEVMAddress)

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

	err = s.cantonClient.Transfer(ctx, &canton.TransferRequest{
		FromFingerprint: fromUser.Fingerprint,
		ToFingerprint:   toUser.Fingerprint,
		Amount:          req.Amount,
	})
	if err != nil {
		s.logger.Error("Transfer failed",
			zap.String("from", fromAddress),
			zap.String("to", toAddress),
			zap.String("amount", req.Amount),
			zap.Error(err))

		if isInsufficientFunds(err) {
			return nil, ErrInsufficientFunds
		}
		return nil, fmt.Errorf("canton transfer failed: %w", err)
	}

	if err := s.db.TransferBalanceByFingerprint(fromUser.Fingerprint, toUser.Fingerprint, req.Amount); err != nil {
		s.logger.Warn("Failed to update balance cache",
			zap.String("from_fingerprint", fromUser.Fingerprint),
			zap.String("to_fingerprint", toUser.Fingerprint),
			zap.String("amount", req.Amount),
			zap.Error(err))
	}

	s.logger.Info("Transfer completed",
		zap.String("from", fromAddress),
		zap.String("to", toAddress),
		zap.String("amount", req.Amount))

	return &TransferResult{
		Success:         true,
		FromFingerprint: fromUser.Fingerprint,
		ToFingerprint:   toUser.Fingerprint,
	}, nil
}

// GetBalance returns the token balance for an EVM address
func (s *TokenService) GetBalance(ctx context.Context, evmAddress string) (string, error) {
	addr := auth.NormalizeAddress(evmAddress)

	user, err := s.db.GetUserByEVMAddress(addr)
	if err != nil {
		return "0", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil || user.Fingerprint == "" {
		return "0", nil
	}

	balance, err := s.db.GetUserBalanceByFingerprint(user.Fingerprint)
	if err != nil {
		s.logger.Warn("Failed to get balance from cache, returning 0",
			zap.String("fingerprint", user.Fingerprint),
			zap.Error(err))
		return "0", nil
	}

	return balance, nil
}

// GetTotalSupply returns the total token supply
func (s *TokenService) GetTotalSupply(ctx context.Context) (string, error) {
	return s.db.GetTotalSupply()
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
// Used by EthRPC for DEMO token operations
func (s *TokenService) GetCantonClient() *canton.Client {
	return s.cantonClient
}

// TransferDemoRequest represents a DEMO token transfer request
type TransferDemoRequest struct {
	FromEVMAddress string
	ToEVMAddress   string
	Amount         string
}

// TransferDemo executes a DEMO token transfer from one user to another via Canton
// This works identically to PROMPT transfers, using Burn + Mint via CIP56Manager
func (s *TokenService) TransferDemo(ctx context.Context, req *TransferDemoRequest) (*TransferResult, error) {
	fromAddress := auth.NormalizeAddress(req.FromEVMAddress)
	toAddress := auth.NormalizeAddress(req.ToEVMAddress)

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

	// Transfer via Canton (Burn + Mint)
	err = s.cantonClient.TransferDemo(ctx, &canton.TransferDemoRequest{
		FromFingerprint: fromUser.Fingerprint,
		ToFingerprint:   toUser.Fingerprint,
		Amount:          req.Amount,
	})
	if err != nil {
		s.logger.Error("DEMO transfer failed",
			zap.String("from", fromAddress),
			zap.String("to", toAddress),
			zap.String("amount", req.Amount),
			zap.Error(err))

		if isInsufficientFunds(err) {
			return nil, ErrInsufficientFunds
		}
		return nil, fmt.Errorf("canton DEMO transfer failed: %w", err)
	}

	// Update database balance cache
	// Note: We use a simple approach here - decrement sender, increment recipient
	// The reconciliation process will correct any discrepancies
	if err := s.db.DecrementDemoBalanceByFingerprint(fromUser.Fingerprint, req.Amount); err != nil {
		s.logger.Warn("Failed to update sender DEMO balance cache",
			zap.String("fingerprint", fromUser.Fingerprint),
			zap.Error(err))
	}
	if err := s.db.IncrementDemoBalanceByFingerprint(toUser.Fingerprint, req.Amount); err != nil {
		s.logger.Warn("Failed to update recipient DEMO balance cache",
			zap.String("fingerprint", toUser.Fingerprint),
			zap.Error(err))
	}

	s.logger.Info("DEMO transfer completed",
		zap.String("from", fromAddress),
		zap.String("to", toAddress),
		zap.String("amount", req.Amount))

	return &TransferResult{
		Success:         true,
		FromFingerprint: fromUser.Fingerprint,
		ToFingerprint:   toUser.Fingerprint,
	}, nil
}

// GetDemoBalance returns the DEMO token balance for an EVM address
func (s *TokenService) GetDemoBalance(ctx context.Context, evmAddress string) (string, error) {
	addr := auth.NormalizeAddress(evmAddress)

	user, err := s.db.GetUserByEVMAddress(addr)
	if err != nil {
		return "0", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil || user.Fingerprint == "" {
		return "0", nil
	}

	// Get balance from database cache
	balance, err := s.db.GetDemoBalanceByFingerprint(user.Fingerprint)
	if err != nil {
		s.logger.Warn("Failed to get DEMO balance from cache, returning 0",
			zap.String("fingerprint", user.Fingerprint),
			zap.Error(err))
		return "0", nil
	}

	return balance, nil
}
