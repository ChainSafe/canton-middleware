package store

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/registration"
)

// Decryptor is a function that decrypts an encrypted key string
type Decryptor func(encryptedKey string) ([]byte, error)

// Store defines the interface for user registration data persistence
type Store interface {
	KeyStore
	WhitelistStore
	CreateUser(ctx context.Context, user *registration.User) error
	GetUser(ctx context.Context, opts ...QueryOption) (*registration.User, error)
	UserExists(ctx context.Context, evmAddress string) (bool, error)
	DeleteUser(ctx context.Context, evmAddress string) error
}

// WhitelistStore defines the interface for whitelist data persistence
type WhitelistStore interface {
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
}

// KeyStore defines the interface for user key persistence and retrieval
type KeyStore interface {
	GetUserKey(ctx context.Context, decryptor Decryptor, opts ...QueryOption) ([]byte, error)
}

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
