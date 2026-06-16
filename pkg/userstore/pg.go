// SPDX-License-Identifier: Apache-2.0

package userstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/user"
)

// KeyDecryptor decrypts an encrypted private key string into raw bytes.
type KeyDecryptor func(encryptedKey string) ([]byte, error)

// whereWhitelistAddress matches a whitelist row by EVM address case-insensitively.
const whereWhitelistAddress = "LOWER(evm_address) = LOWER(?)"

type pgStore struct {
	db *bun.DB
}

// NewStore creates a new postgres implementation of the user store
func NewStore(db *bun.DB) *pgStore {
	return &pgStore{db: db}
}

func (s *pgStore) CreateUser(ctx context.Context, usr *user.User) error {
	dao := toUserDao(usr)

	_, err := s.db.NewInsert().
		Model(dao).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *pgStore) getUserBy(ctx context.Context, column string, value string) (*user.User, error) {
	dao := new(UserDao)
	query := s.db.NewSelect().Model(dao).Where("? = ?", bun.Ident(column), value)

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, user.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return toUser(dao), nil
}

func (s *pgStore) GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error) {
	return s.getUserBy(ctx, "evm_address", evmAddress)
}

func (s *pgStore) GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error) {
	return s.getUserBy(ctx, "canton_party_id", partyID)
}

func (s *pgStore) GetUserByFingerprint(ctx context.Context, fingerprint string) (*user.User, error) {
	return s.getUserBy(ctx, "fingerprint", fingerprint)
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

func (s *pgStore) ListUsers(ctx context.Context) ([]*user.User, error) {
	var daos []UserDao
	err := s.db.NewSelect().Model(&daos).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	users := make([]*user.User, len(daos))
	for i := range daos {
		users[i] = toUser(&daos[i])
	}
	return users, nil
}

func (s *pgStore) ListCustodialUsers(ctx context.Context) ([]*user.User, error) {
	var daos []UserDao
	err := s.db.NewSelect().Model(&daos).
		Where("key_mode = ?", user.KeyModeCustodial).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list custodial users: %w", err)
	}
	users := make([]*user.User, len(daos))
	for i := range daos {
		users[i] = toUser(&daos[i])
	}
	return users, nil
}

func (s *pgStore) IsWhitelisted(ctx context.Context, evmAddress string) (bool, error) {
	exists, err := s.db.NewSelect().
		Model(&WhitelistDao{}).
		Where(whereWhitelistAddress, evmAddress).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check whitelist: %w", err)
	}
	return exists, nil
}

func (s *pgStore) AddToWhitelist(ctx context.Context, evmAddress, note string) error {
	evmAddress = strings.TrimSpace(evmAddress)
	if evmAddress == "" {
		return fmt.Errorf("evm address is required")
	}

	entry := &WhitelistDao{EVMAddress: evmAddress}
	if strings.TrimSpace(note) != "" {
		entry.Note = &note
	}

	_, err := s.db.NewInsert().
		Model(entry).
		On("CONFLICT (evm_address) DO UPDATE").
		Set("note = EXCLUDED.note").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to add whitelist entry: %w", err)
	}

	return nil
}

// RemoveFromWhitelist deletes a whitelist entry and reports whether a row was
// actually removed, so callers can distinguish a real deletion from a no-op on
// an address that was never whitelisted.
func (s *pgStore) RemoveFromWhitelist(ctx context.Context, evmAddress string) (bool, error) {
	res, err := s.db.NewDelete().
		Model((*WhitelistDao)(nil)).
		Where(whereWhitelistAddress, evmAddress).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to remove whitelist entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read rows affected: %w", err)
	}
	return n > 0, nil
}

// ListWhitelist returns up to limit entries ordered by evm_address, starting
// strictly after cursor. Ordering by the unique primary key gives a stable,
// total order so the cursor never skips or repeats rows across pages.
func (s *pgStore) ListWhitelist(ctx context.Context, cursor string, limit int) ([]*user.WhitelistEntry, error) {
	var daos []WhitelistDao
	q := s.db.NewSelect().
		Model(&daos).
		Order("evm_address ASC").
		Limit(limit)
	if cursor != "" {
		q = q.Where("evm_address > ?", cursor)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list whitelist entries: %w", err)
	}
	entries := make([]*user.WhitelistEntry, len(daos))
	for i := range daos {
		entries[i] = toWhitelistEntry(&daos[i])
	}
	return entries, nil
}

func (s *pgStore) getUserKeyBy(ctx context.Context, decryptor KeyDecryptor, column string, value string) ([]byte, error) {
	dao := new(UserDao)
	query := s.db.NewSelect().
		Model(dao).
		Column("canton_private_key_encrypted").
		Where(column+" = ?", value)

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, user.ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to get user key: %w", err)
	}

	if dao.CantonPrivateKeyEncrypted == nil || *dao.CantonPrivateKeyEncrypted == "" {
		return nil, nil
	}

	decryptedKey, err := decryptor(*dao.CantonPrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	return decryptedKey, nil
}

func (s *pgStore) GetUserKeyByCantonPartyID(ctx context.Context, decryptor KeyDecryptor, partyID string) ([]byte, error) {
	return s.getUserKeyBy(ctx, decryptor, "canton_party_id", partyID)
}

func (s *pgStore) GetUserKeyByEVMAddress(ctx context.Context, decryptor KeyDecryptor, evmAddress string) ([]byte, error) {
	return s.getUserKeyBy(ctx, decryptor, "evm_address", evmAddress)
}

func (s *pgStore) GetUserKeyByFingerprint(ctx context.Context, decryptor KeyDecryptor, fingerprint string) ([]byte, error) {
	return s.getUserKeyBy(ctx, decryptor, "fingerprint", fingerprint)
}
