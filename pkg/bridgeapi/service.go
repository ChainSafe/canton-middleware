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
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

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
}

type bridgeService struct {
	cfg       *Config
	quoters   map[string]DepositQuoter // keyed by mechanism
	quotes    *quoteStore
	userStore UserStore
	relayer   RelayerClient
	logger    *zap.Logger
}

// NewService builds the bridge service from config. allowance may be nil, in
// which case quotes always include the approve step.
func NewService(
	cfg *Config,
	userStore UserStore,
	relayerClient RelayerClient,
	allowance AllowanceChecker,
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
		quotes:    newQuoteStore(),
		userStore: userStore,
		relayer:   relayerClient,
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
// never leaves the server.
func (s *bridgeService) DepositQuote(ctx context.Context, evmAddress string, req *QuoteRequest) (*Quote, error) {
	tc, ok := s.cfg.Tokens[req.Token]
	if !ok {
		return nil, apperrors.BadRequestError(nil, fmt.Sprintf("unsupported bridge token %q", req.Token))
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.IsPositive() {
		return nil, apperrors.BadRequestError(nil, "invalid amount: must be a positive decimal number")
	}
	// Decimals is tag-validated to 0..18, so the int32 conversion is safe.
	if tc.Decimals < 0 || tc.Decimals > 18 {
		return nil, fmt.Errorf("token %s: invalid decimals %d", req.Token, tc.Decimals)
	}
	baseAmount := amount.Shift(int32(tc.Decimals))
	if !baseAmount.IsInteger() {
		return nil, apperrors.BadRequestError(nil,
			fmt.Sprintf("amount exceeds the token's %d decimal places", tc.Decimals))
	}

	usr, err := s.userStore.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		return nil, apperrors.ResourceNotFoundError(err, "user is not registered")
	}

	quoter := s.quoters[tc.Mechanism]
	steps, err := quoter.DepositSteps(ctx, tc, baseAmount.BigInt(), common.HexToAddress(evmAddress), usr.CantonParty)
	if err != nil {
		return nil, fmt.Errorf("build deposit steps: %w", err)
	}

	quote := &Quote{
		QuoteID:          "q_" + uuid.NewString(),
		ChainID:          s.cfg.ChainID,
		Steps:            steps,
		Fees:             Fees{BridgeFee: tc.BridgeFee, Currency: req.Token},
		EstimatedSeconds: tc.EstimatedSeconds,
		ExpiresAt:        s.quotes.now().Add(s.cfg.QuoteTTLOrDefault()),
	}

	s.quotes.Put(&storedQuote{
		QuoteID:        quote.QuoteID,
		Owner:          evmAddress,
		TokenSymbol:    req.Token,
		Mechanism:      tc.Mechanism,
		TokenAddress:   tc.EVMAddress,
		Amount:         amount.String(),
		RecipientParty: usr.CantonParty,
		ExpiresAt:      quote.ExpiresAt,
	})

	return quote, nil
}

// RegisterDeposit forwards a quoted deposit's tx hash to the relayer for
// status tracking. All transfer parameters come from the stored quote, never
// from the caller.
func (s *bridgeService) RegisterDeposit(
	ctx context.Context,
	evmAddress string,
	req *RegisterDepositRequest,
) (*relayer.RegisterTransferResponse, error) {
	quote, ok := s.quotes.Get(req.QuoteID)
	if !ok {
		return nil, apperrors.BadRequestError(nil, "unknown or expired quote_id")
	}
	if quote.Owner != evmAddress {
		return nil, apperrors.UnAuthorizedError(nil, "quote belongs to a different address")
	}

	resp, err := s.relayer.RegisterTransfer(ctx, &relayer.RegisterTransferRequest{
		ID:           req.TxHash,
		BridgeKey:    quote.Mechanism,
		TokenSymbol:  quote.TokenSymbol,
		Direction:    relayer.DirectionEthereumToCanton,
		SourceTxHash: req.TxHash,
		TokenAddress: quote.TokenAddress,
		Amount:       quote.Amount,
		Sender:       evmAddress,
		Recipient:    quote.RecipientParty,
		Metadata:     map[string]string{"quote_id": quote.QuoteID},
	})
	if err != nil {
		return nil, fmt.Errorf("register deposit: %w", err)
	}
	return resp, nil
}

// GetTransfer proxies a transfer status read to the relayer.
func (s *bridgeService) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	return s.relayer.GetTransfer(ctx, id)
}
