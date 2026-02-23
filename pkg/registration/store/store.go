package store

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/registration"
)

// Store defines the interface for user registration data persistence
type Store interface {
	CreateUser(ctx context.Context, user *registration.User) error
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*registration.User, error)
	GetUserByCantonPartyID(ctx context.Context, cantonPartyID string) (*registration.User, error)
	UserExists(ctx context.Context, evmAddress string) (bool, error)
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
	DeleteUser(ctx context.Context, evmAddress string) error
}
