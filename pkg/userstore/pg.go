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

func (s *pgStore) IsWhitelisted(ctx context.Context, evmAddress string) (bool, error) {
	exists, err := s.db.NewSelect().
		Model(&WhitelistDao{}).
		Where("evm_address = ?", evmAddress).
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
