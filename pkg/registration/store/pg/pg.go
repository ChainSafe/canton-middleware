package pg

import (
	"context"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/registration"
	"github.com/chainsafe/canton-middleware/pkg/registration/store"
	"github.com/uptrace/bun"
)

type pgStore struct {
	db *bun.DB
}

// NewStore creates a new postgres implementation of the registration store
func NewStore(db *bun.DB) store.Store {
	return &pgStore{db: db}
}

type pgWhitelistStore struct {
	db *bun.DB
}

// NewWhitelistStore creates a new postgres implementation of the whitelist store
func NewWhitelistStore(db *bun.DB) store.WhitelistStore {
	return &pgWhitelistStore{db: db}
}

func (s *pgStore) CreateUser(ctx context.Context, user *registration.User) error {
	dao := toUserDao(user)

	_, err := s.db.NewInsert().
		Model(dao).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *pgStore) GetUser(ctx context.Context, opts ...store.QueryOption) (*registration.User, error) {
	options := &store.QueryOptions{}
	for _, opt := range opts {
		opt(options)
	}

	dao := new(UserDao)
	query := s.db.NewSelect().Model(dao)

	if options.EVMAddress != nil {
		query = query.Where("evm_address = ?", *options.EVMAddress)
	}
	if options.CantonPartyID != nil {
		query = query.Where("canton_party_id = ?", *options.CantonPartyID)
	}
	if options.Fingerprint != nil {
		query = query.Where("fingerprint = ?", *options.Fingerprint)
	}

	err := query.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return toUser(dao), nil
}

func (s *pgStore) UserExists(ctx context.Context, evmAddress string) (bool, error) {
	exists, err := s.db.NewSelect().
		Model((*UserDao)(nil)).
		Where("evm_address = ?", evmAddress).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check user exists: %w", err)
	}
	return exists, nil
}

func (s *pgStore) DeleteUser(ctx context.Context, evmAddress string) error {
	_, err := s.db.NewDelete().
		Model((*UserDao)(nil)).
		Where("evm_address = ?", evmAddress).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (s *pgWhitelistStore) IsWhitelisted(ctx context.Context, evmAddress string) (bool, error) {
	exists, err := s.db.NewSelect().
		TableExpr("whitelist").
		Where("evm_address = ?", evmAddress).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check whitelist: %w", err)
	}
	return exists, nil
}
