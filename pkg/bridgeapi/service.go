// SPDX-License-Identifier: Apache-2.0

// Package bridgeapi exposes the token-agnostic bridge endpoints on the
// api-server. The dapp never encodes a bridge transaction: it asks for a
// quote (fully ABI-encoded unsigned transactions), signs blindly, then
// registers the sent transaction for status tracking — the EVM mirror of the
// Canton prepare/execute pattern.
package bridgeapi

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// txHashPattern matches a 0x-prefixed 32-byte EVM transaction hash.
var txHashPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

// maxUint256 is 2^256 - 1, the largest value that fits an ABI uint256.
const uint256Bits = 256

var maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint256Bits), big.NewInt(1))

// UserStore resolves authenticated EVM addresses to Canton parties.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
}

// Service defines the bridge API operations.
type Service interface {
	Tokens(ctx context.Context) []TokenInfo
	DepositQuote(ctx context.Context, evmAddress string, req *QuoteRequest) (*Quote, error)
	RegisterDeposit(ctx context.Context, evmAddress string, req *RegisterDepositRequest) (*relayer.RegisterTransferResponse, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
	WithdrawPrepare(ctx context.Context, evmAddress string, req *WithdrawRequest) (*WithdrawPrepareResponse, error)
	WithdrawExecute(ctx context.Context, evmAddress string, req *WithdrawExecuteRequest) (*relayer.RegisterTransferResponse, error)
	WithdrawCustodial(ctx context.Context, evmAddress string, req *WithdrawRequest) (*relayer.RegisterTransferResponse, error)
}

type bridgeService struct {
	cfg       *Config
	quoters   map[string]DepositQuoter // keyed by mechanism
	burns     *burnStore
	userStore UserStore
	relayer   RelayerClient
	canton    CantonBurner // nil disables withdrawals
	logger    *zap.Logger
}

// NewService builds the bridge service from config. allowance may be nil, in
// which case quotes always include the approve step; canton may be nil, which
// disables withdrawals.
func NewService(
	cfg *Config,
	userStore UserStore,
	relayerClient RelayerClient,
	allowance AllowanceChecker,
	canton CantonBurner,
	logger *zap.Logger,
) (Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	xr, err := newXReserveQuoter(allowance, logger)
	if err != nil {
		return nil, err
	}
	quoters := map[string]DepositQuoter{MechanismXReserve: xr}

	for symbol, tc := range cfg.Tokens {
		if _, ok := quoters[tc.Mechanism]; !ok {
			return nil, fmt.Errorf("bridge token %s: unknown mechanism %q", symbol, tc.Mechanism)
		}
	}

	return &bridgeService{
		cfg:       cfg,
		quoters:   quoters,
		burns:     newBurnStore(),
		userStore: userStore,
		relayer:   relayerClient,
		canton:    canton,
		logger:    logger,
	}, nil
}

// Tokens lists the bridgeable tokens in deterministic order.
func (s *bridgeService) Tokens(context.Context) []TokenInfo {
	tokens := make([]TokenInfo, 0, len(s.cfg.Tokens))
	for symbol, tc := range s.cfg.Tokens {
		tokens = append(tokens, TokenInfo{
			Symbol:           symbol,
			Mechanism:        tc.Mechanism,
			EVMAddress:       tc.EVMAddress,
			Decimals:         tc.Decimals,
			BridgeFee:        tc.BridgeFee,
			EstimatedSeconds: tc.EstimatedSeconds,
		})
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Symbol < tokens[j].Symbol })
	return tokens
}

// DepositQuote builds the unsigned transactions that bridge req.Amount of
// req.Token from the caller to their Canton party. The recipient encoding
// never leaves the server. Stateless: nothing is stored, so quotes can be
// re-requested freely.
func (s *bridgeService) DepositQuote(ctx context.Context, evmAddress string, req *QuoteRequest) (*Quote, error) {
	tc, amount, err := s.parseTokenAmount(req.Token, req.Amount)
	if err != nil {
		return nil, err
	}

	usr, err := s.userStore.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		return nil, apperrors.ResourceNotFoundError(err, "user is not registered")
	}

	quoter := s.quoters[tc.Mechanism]
	baseAmount := amount.Shift(int32(tc.Decimals)) //nolint:gosec // decimals bounded by parseTokenAmount
	steps, err := quoter.DepositSteps(ctx, tc, baseAmount.BigInt(), common.HexToAddress(evmAddress), usr.CantonParty)
	if err != nil {
		return nil, fmt.Errorf("build deposit steps: %w", err)
	}

	return &Quote{
		ChainID:          s.cfg.ChainID,
		Steps:            steps,
		Fees:             Fees{BridgeFee: tc.BridgeFee, Currency: req.Token},
		EstimatedSeconds: tc.EstimatedSeconds,
	}, nil
}

// RegisterDeposit forwards a submitted deposit to the relayer for tracking.
// Token/amount are re-validated and the recipient party comes from the
// session, so a caller can't register on anyone else's behalf.
func (s *bridgeService) RegisterDeposit(
	ctx context.Context,
	evmAddress string,
	req *RegisterDepositRequest,
) (*relayer.RegisterTransferResponse, error) {
	tc, amount, err := s.parseTokenAmount(req.Token, req.Amount)
	if err != nil {
		return nil, err
	}

	if !txHashPattern.MatchString(req.TxHash) {
		return nil, apperrors.BadRequestError(nil, "tx_hash must be a 0x-prefixed 32-byte hex string")
	}

	usr, err := s.userStore.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		return nil, apperrors.ResourceNotFoundError(err, "user is not registered")
	}

	resp, err := s.relayer.RegisterTransfer(ctx, &relayer.RegisterTransferRequest{
		// Namespace by mechanism so a crafted tx_hash can't collide with the
		// observer's "<txhash>-<logindex>" ids and squat a real deposit's row.
		ID:           registrationID(tc.Mechanism, req.TxHash),
		BridgeKey:    tc.Mechanism,
		TokenSymbol:  req.Token,
		Direction:    relayer.DirectionEthereumToCanton,
		SourceTxHash: req.TxHash,
		TokenAddress: tc.EVMAddress,
		Amount:       amount.String(),
		Sender:       evmAddress,
		Recipient:    usr.CantonParty,
	})
	if err != nil {
		return nil, fmt.Errorf("register deposit: %w", err)
	}
	return resp, nil
}

// registrationID namespaces the transfer id by mechanism. The dapp polls the
// id from the response, so the namespacing is transparent.
func registrationID(mechanism, txHash string) string {
	return mechanism + ":" + txHash
}

// parseTokenAmount resolves a configured token and validates the token-unit
// amount against its precision. Shared by quoting and registration.
func (s *bridgeService) parseTokenAmount(token, rawAmount string) (TokenConfig, decimal.Decimal, error) {
	tc, ok := s.cfg.Tokens[token]
	if !ok {
		return TokenConfig{}, decimal.Zero, apperrors.BadRequestError(nil,
			fmt.Sprintf("unsupported bridge token %q", token))
	}

	amount, err := decimal.NewFromString(rawAmount)
	if err != nil || !amount.IsPositive() {
		return TokenConfig{}, decimal.Zero, apperrors.BadRequestError(nil,
			"invalid amount: must be a positive decimal number")
	}
	// Decimals is tag-validated to 0..18, so int32 conversions are safe.
	if tc.Decimals < 0 || tc.Decimals > 18 {
		return TokenConfig{}, decimal.Zero, fmt.Errorf("token %s: invalid decimals %d", token, tc.Decimals)
	}
	baseAmount := amount.Shift(int32(tc.Decimals))
	if !baseAmount.IsInteger() {
		return TokenConfig{}, decimal.Zero, apperrors.BadRequestError(nil,
			fmt.Sprintf("amount exceeds the token's %d decimal places", tc.Decimals))
	}
	// Anything over uint256 would silently wrap in the encoded calldata.
	if baseAmount.BigInt().Cmp(maxUint256) > 0 {
		return TokenConfig{}, decimal.Zero, apperrors.BadRequestError(nil, "amount exceeds the maximum supported value")
	}
	return tc, amount, nil
}

// GetTransfer proxies a transfer status read to the relayer.
func (s *bridgeService) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	return s.relayer.GetTransfer(ctx, id)
}
