// SPDX-License-Identifier: Apache-2.0

// Package service provides the relayer HTTP service layer.
package service

import (
	"context"
	"fmt"
	"slices"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// Store is the narrow data-access interface for the relayer service.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
	CreateTransfer(ctx context.Context, transfer *relayer.Transfer) (bool, error)
}

// Service defines the interface for relayer query and registration operations.
//
//go:generate mockery --name Service --output mocks --outpkg mocks --filename mock_service.go --with-expecter
type Service interface {
	ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
	RegisterTransfer(ctx context.Context, req *relayer.RegisterTransferRequest) (*relayer.RegisterTransferResponse, error)
}

type relayerService struct {
	store Store
	// bridgeKeys are the registered TokenBridge adapters; registration is
	// rejected for keys no adapter owns, since nothing would ever step them.
	bridgeKeys []string
}

// NewService creates a new relayer service. bridgeKeys is the set of
// registered TokenBridge adapter keys accepted for transfer registration.
func NewService(store Store, bridgeKeys []string) Service {
	return &relayerService{store: store, bridgeKeys: bridgeKeys}
}

func (s *relayerService) ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error) {
	return s.store.ListTransfers(ctx, limit)
}

func (s *relayerService) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	return s.store.GetTransfer(ctx, id)
}

func (s *relayerService) RegisterTransfer(
	ctx context.Context,
	req *relayer.RegisterTransferRequest,
) (*relayer.RegisterTransferResponse, error) {
	if err := s.validateRegistration(req); err != nil {
		return nil, err
	}

	transfer := relayer.TransferFromEvent(req.BridgeKey, &relayer.Event{
		ID:           req.ID,
		TokenSymbol:  req.TokenSymbol,
		Direction:    req.Direction,
		SourceChain:  sourceChain(req.Direction),
		SourceTxHash: req.SourceTxHash,
		TokenAddress: req.TokenAddress,
		Amount:       req.Amount,
		Sender:       req.Sender,
		Recipient:    req.Recipient,
	})
	transfer.DestinationChain = destinationChain(req.Direction)
	transfer.Metadata = req.Metadata

	created, err := s.store.CreateTransfer(ctx, transfer)
	if err != nil {
		return nil, fmt.Errorf("register transfer: %w", err)
	}

	if !created {
		existing, getErr := s.store.GetTransfer(ctx, transfer.ID)
		if getErr != nil {
			return nil, fmt.Errorf("load existing transfer: %w", getErr)
		}
		return &relayer.RegisterTransferResponse{Transfer: existing, Created: false}, nil
	}
	return &relayer.RegisterTransferResponse{Transfer: transfer, Created: true}, nil
}

func (s *relayerService) validateRegistration(req *relayer.RegisterTransferRequest) error {
	if req.SourceTxHash == "" {
		return apperrors.BadRequestError(nil, "source_tx_hash is required")
	}
	if req.ID == "" {
		req.ID = req.SourceTxHash
	}
	if req.TokenSymbol == "" || req.Amount == "" || req.Recipient == "" {
		return apperrors.BadRequestError(nil, "token_symbol, amount, and recipient are required")
	}
	if req.Direction != relayer.DirectionEthereumToCanton && req.Direction != relayer.DirectionCantonToEthereum {
		return apperrors.BadRequestError(nil, "direction must be ethereum_to_canton or canton_to_ethereum")
	}
	if !slices.Contains(s.bridgeKeys, req.BridgeKey) {
		return apperrors.BadRequestError(nil, fmt.Sprintf("unknown bridge key %q", req.BridgeKey))
	}
	return nil
}

func sourceChain(direction relayer.TransferDirection) string {
	if direction == relayer.DirectionEthereumToCanton {
		return relayer.ChainEthereum
	}
	return relayer.ChainCanton
}

func destinationChain(direction relayer.TransferDirection) string {
	if direction == relayer.DirectionEthereumToCanton {
		return relayer.ChainCanton
	}
	return relayer.ChainEthereum
}
