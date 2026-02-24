package store

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/registration"
)

// QueryOptions defines options for querying users
type QueryOptions struct {
	EVMAddress    *string
	CantonPartyID *string
	Fingerprint   *string
}

// QueryOption is a functional option for querying users
type QueryOption func(*QueryOptions)

// WithEVMAddress sets the EVM address filter
func WithEVMAddress(evmAddress string) QueryOption {
	return func(opts *QueryOptions) {
		opts.EVMAddress = &evmAddress
	}
}

// WithCantonPartyID sets the Canton party ID filter
func WithCantonPartyID(cantonPartyID string) QueryOption {
	return func(opts *QueryOptions) {
		opts.CantonPartyID = &cantonPartyID
	}
}

// WithFingerprint sets the fingerprint filter
func WithFingerprint(fingerprint string) QueryOption {
	return func(opts *QueryOptions) {
		opts.Fingerprint = &fingerprint
	}
}

// Store defines the interface for user registration data persistence
type Store interface {
	CreateUser(ctx context.Context, user *registration.User) error
	GetUser(ctx context.Context, opts ...QueryOption) (*registration.User, error)
	UserExists(ctx context.Context, evmAddress string) (bool, error)
	DeleteUser(ctx context.Context, evmAddress string) error
}

// WhitelistStore defines the interface for whitelist data persistence
type WhitelistStore interface {
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
}
