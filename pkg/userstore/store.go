package userstore

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/user"
)

// Store is the full interface for user persistence.
// Consumers should define narrower interfaces for the methods they need.
type Store interface {
	CreateUser(ctx context.Context, usr *user.User) error
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
	GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error)
	GetUserByFingerprint(ctx context.Context, fingerprint string) (*user.User, error)
	UserExists(ctx context.Context, evmAddress string) (bool, error)
	DeleteUser(ctx context.Context, evmAddress string) error
	ListUsers(ctx context.Context) ([]*user.User, error)
	ListCustodialUsers(ctx context.Context) ([]*user.User, error)
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
	AddToWhitelist(ctx context.Context, evmAddress, note string) error
	RemoveFromWhitelist(ctx context.Context, evmAddress string) (removed bool, err error)
	// ListWhitelist returns up to limit entries ordered by evm_address ascending,
	// starting strictly after cursor (the last evm_address of the previous page;
	// empty starts from the beginning). evm_address is the unique primary key, so
	// it is a stable cursor.
	ListWhitelist(ctx context.Context, cursor string, limit int) (entries []*user.WhitelistEntry, err error)
	GetUserKeyByCantonPartyID(ctx context.Context, decryptor KeyDecryptor, partyID string) ([]byte, error)
	GetUserKeyByEVMAddress(ctx context.Context, decryptor KeyDecryptor, evmAddress string) ([]byte, error)
	GetUserKeyByFingerprint(ctx context.Context, decryptor KeyDecryptor, fingerprint string) ([]byte, error)
}

// Compile-time check that pgStore implements Store.
var _ Store = (*pgStore)(nil)
