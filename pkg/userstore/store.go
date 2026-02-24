package userstore

import (
	"context"
	"errors"

	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// ErrUserNotFound is returned when a user lookup finds no matching record.
var ErrUserNotFound = errors.New("user not found")

// UserBalanceStore defines all user balance cache operations.
// Used by the reconciler and bridge relayer to keep the balance cache in sync.
type UserBalanceStore interface {
	ListUsers(ctx context.Context) ([]*user.User, error)
	UpdateBalanceByCantonPartyID(ctx context.Context, partyID, balance string, token token.Type) error
	IncrementBalanceByFingerprint(ctx context.Context, fingerprint, amount string, token token.Type) error
	DecrementBalanceByEVMAddress(ctx context.Context, evmAddress, amount string, token token.Type) error
	TransferBalanceByFingerprint(ctx context.Context, fromFingerprint, toFingerprint, amount string, token token.Type) error
	ResetBalances(ctx context.Context, token token.Type) error
}

// Store defines the interface for user registration data persistence
type Store interface {
	KeyStore
	WhitelistStore
	UserBalanceStore
	CreateUser(ctx context.Context, user *user.User) error
	GetUser(ctx context.Context, opts ...QueryOption) (*user.User, error)
	UserExists(ctx context.Context, evmAddress string) (bool, error)
	DeleteUser(ctx context.Context, evmAddress string) error
}

// WhitelistStore defines the interface for whitelist data persistence
type WhitelistStore interface {
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
}

// KeyDecryptor decrypts an encrypted private key string into raw bytes.
type KeyDecryptor func(encryptedKey string) ([]byte, error)

// KeyStore defines the interface for user key persistence and retrieval
type KeyStore interface {
	GetUserKey(ctx context.Context, decryptor KeyDecryptor, opts ...QueryOption) ([]byte, error)
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
