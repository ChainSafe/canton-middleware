package token

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// UserStore defines user persistence required by Service.
//
//go:generate mockery --name UserStore --output mocks --outpkg mocks --filename mock_user_store.go --with-expecter
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
	GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error)
}

// Provider defines token data provider.
//
//go:generate mockery --name Provider --output mocks --outpkg mocks --filename mock_provider.go --with-expecter
type Provider interface {
	GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error)
	GetBalance(ctx context.Context, tokenSymbol, fingerprint string) (string, error)
}

//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/cantonsdk/token --name Token --output mocks --outpkg mocks --filename mock_canton_token.go --with-expecter

// Service provides token operations shared by API and EthRPC endpoints.
type Service struct {
	cfg          *Config
	provider     Provider
	userStore    UserStore
	cantonClient canton.Token
}

// NewTokenService creates a Service.
func NewTokenService(
	cfg *Config,
	provider Provider,
	userStore UserStore,
	cantonClient canton.Token,
) *Service {
	return &Service{
		cfg:          cfg,
		provider:     provider,
		userStore:    userStore,
		cantonClient: cantonClient,
	}
}

// ERC20 returns an ERC-20 view for the given contract address, or an error if
// the address is not a supported token contract.
func (s *Service) ERC20(address common.Address) (ERC20, error) {
	if _, err := s.cfg.getToken(address); err != nil {
		return nil, err
	}
	return NewERC20(address, s), nil
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
		return apperr.BadRequestError(err, fmt.Sprintf("token not supported: %s", contract.Hex()))
	}
	fromUser, err := s.userStore.GetUserByEVMAddress(ctx, from.Hex())
	if err != nil {
		return apperr.BadRequestError(fmt.Errorf("failed to get sender: %w", err), "failed to get sender")
	}
	toUser, err := s.userStore.GetUserByEVMAddress(ctx, to.Hex())
	if err != nil {
		return apperr.BadRequestError(fmt.Errorf("failed to get recipient: %w", err), "failed to get recipient")
	}

	err = s.cantonClient.TransferByFingerprint(ctx,
		fromUser.Fingerprint,
		toUser.Fingerprint,
		amount,
		tkn.Symbol,
	)
	if err != nil {
		if errors.Is(err, canton.ErrInsufficientBalance) {
			return apperr.BadRequestError(err, "insufficient balance")
		}
		return apperr.DependencyError(fmt.Errorf("canton transfer failed: %w", err), "canton transfer failed")
	}

	return nil
}

// getBalance returns the token balance for an EVM address.
func (s *Service) getBalance(ctx context.Context, contract, address common.Address) (string, error) {
	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return "0", err
	}

	usr, err := s.userStore.GetUserByEVMAddress(ctx, address.Hex())
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return "0", nil
		}
		return "0", fmt.Errorf("failed to get user: %w", err)
	}

	return s.provider.GetBalance(ctx, tkn.Symbol, usr.Fingerprint)
}

// getTotalSupply returns the total supply for a specific token
func (s *Service) getTotalSupply(ctx context.Context, contract common.Address) (string, error) {
	if s.provider == nil {
		return "0", fmt.Errorf("token provider not configured")
	}

	tkn, err := s.cfg.getToken(contract)
	if err != nil {
		return "0", err
	}

	return s.provider.GetTotalSupply(ctx, tkn.Symbol)
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
