// Package service provides the relayer HTTP service layer.
package service

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// Store is the narrow data-access interface for the relayer service.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
}

// Service defines the interface for relayer query operations.
//
//go:generate mockery --name Service --output mocks --outpkg mocks --filename mock_service.go --with-expecter
type Service interface {
	ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
}

type relayerService struct {
	store Store
}

// NewService creates a new relayer service.
func NewService(store Store) Service {
	return &relayerService{store: store}
}

func (s *relayerService) ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error) {
	return s.store.ListTransfers(ctx, limit)
}

func (s *relayerService) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	return s.store.GetTransfer(ctx, id)
}
