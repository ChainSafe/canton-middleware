package token

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// UserStore defines user persistence required by Service.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
	GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error)
	TransferBalanceByFingerprint(ctx context.Context, fromFingerprint, toFingerprint, amount string, tokenType Type) error
}

// Store defines token persistence required by Service.
type Store interface {
	GetTotalSupply(tokenSymbol string) (string, error)
}

// Service provides token operations shared by API and EthRPC endpoints.
type Service struct {
	cfg          *Config
	tokenStore   Store
	userStore    UserStore
	cantonClient canton.Token
}

// NewTokenService creates a Service.
func NewTokenService(
	cfg *Config,
	tokenStore Store,
	userStore UserStore,
	cantonClient canton.Token,
) *Service {
	return &Service{
		cfg:          cfg,
		tokenStore:   tokenStore,
		userStore:    userStore,
		cantonClient: cantonClient,
	}
}

// ERC20 returns an ERC-20 view for the given contract address.
func (s *Service) ERC20(address common.Address) ERC20 {
	return NewERC20(address, s)
}

// Native returns the native token view.
func (s *Service) Native() Native {
	return NewNative(s)
}

// transfer executes a token transfer from one user to another using user-owned holdings.
// Works for any CIP-56 whitelisted token.
func (s *Service) transfer(ctx context.Context, contract, from, to common.Address, amount string) error {
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return err
	}
	fromUser, err := s.userStore.GetUserByEVMAddress(ctx, from.Hex())
	if err != nil {
		return fmt.Errorf("failed to get sender: %w", err)
	}
	toUser, err := s.userStore.GetUserByEVMAddress(ctx, to.Hex())
	if err != nil {
		return fmt.Errorf("failed to get recipient: %w", err)
	}

	err = s.cantonClient.TransferByFingerprint(ctx,
		fromUser.Fingerprint,
		toUser.Fingerprint,
		amount,
		tkn.Symbol,
	)
	if err != nil {
		return fmt.Errorf("canton transfer failed: %w", err)
	}

	// TODO: if transfer doesn't happen through middleware the stored balance might not be correct.
	_ = s.userStore.TransferBalanceByFingerprint(
		ctx,
		fromUser.Fingerprint,
		toUser.Fingerprint,
		amount,
		Type(tkn.Symbol),
	)

	return nil
}

// getBalance returns the token balance for an EVM address.
func (s *Service) getBalance(ctx context.Context, contract, address common.Address) (string, error) {
	// TODO: implement balance provider - can be database or canton network
	// For now we should be calling the cantonsdk until we implement a balance cacher.
	// This call on db is dependent on the reconciler
	// TODO: Also create a separate table for balance
	usr, err := s.userStore.GetUserByEVMAddress(ctx, address.Hex())
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return "0", nil
		}
		return "0", fmt.Errorf("failed to get user: %w", err)
	}
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return "0", err
	}
	if strings.EqualFold(tkn.Symbol, string(Demo)) {
		return usr.DemoBalance, nil
	}
	return usr.PromptBalance, nil
}

// getTotalSupply returns the total supply for a specific token
func (s *Service) getTotalSupply(_ context.Context, contract common.Address) (string, error) {
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return "0", err
	}
	return s.tokenStore.GetTotalSupply(tkn.Symbol)
}

// getTokenName returns the token name from config
func (s *Service) getTokenName(_ context.Context, contract common.Address) (string, error) {
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return "", err
	}
	return tkn.Name, nil
}

// isUserRegistered checks if an EVM address is registered
func (s *Service) isUserRegistered(ctx context.Context, address common.Address) (bool, error) {
	_, err := s.userStore.GetUserByEVMAddress(ctx, address.Hex())
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// getTokenSymbol returns the token symbol from config
func (s *Service) getTokenSymbol(_ context.Context, contract common.Address) (string, error) {
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return "", err
	}
	return tkn.Symbol, nil
}

// getTokenDecimals returns the token decimals from config
func (s *Service) getTokenDecimals(_ context.Context, contract common.Address) (int, error) {
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return 0, err
	}
	return tkn.Decimals, nil
}
