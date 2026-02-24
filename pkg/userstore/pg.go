package userstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

var ErrKeyNotFound = errors.New("key not found")
var ErrUserNotFound = errors.New("user not found")

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
	query := s.db.NewSelect().Model(dao).Where(column+" = ?", value)

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
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

func (s *pgStore) UpdateBalanceByCantonPartyID(ctx context.Context, partyID, balance string, tokenType token.Type) error {
	col := balanceCol(tokenType)
	q := s.db.NewUpdate().
		Model((*UserDao)(nil)).
		Set(col+" = ?", balance).
		Set("balance_updated_at = NOW()").
		Where("canton_party_id = ?", partyID)

	_, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update %s balance: %w", tokenType, err)
	}
	return nil
}

func (s *pgStore) IncrementBalanceByFingerprint(ctx context.Context, fingerprint, amount string, tokenType token.Type) error {
	withPrefix, withoutPrefix := normalizeFP(fingerprint)
	col := balanceCol(tokenType)

	_, err := s.db.NewUpdate().
		TableExpr("users").
		Set(col+" = COALESCE("+col+", 0) + ?::DECIMAL", amount).
		Set("balance_updated_at = NOW()").
		Where("fingerprint = ? OR fingerprint = ?", withPrefix, withoutPrefix).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to increment %s balance: %w", tokenType, err)
	}
	return nil
}

func (s *pgStore) DecrementBalanceByEVMAddress(ctx context.Context, evmAddress, amount string, tokenType token.Type) error {
	col := balanceCol(tokenType)

	_, err := s.db.NewUpdate().
		TableExpr("users").
		Set(col+" = COALESCE("+col+", 0) - ?::DECIMAL", amount).
		Set("balance_updated_at = NOW()").
		Where("evm_address = ?", evmAddress).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to decrement %s balance: %w", tokenType, err)
	}
	return nil
}

func (s *pgStore) TransferBalanceByFingerprint(
	ctx context.Context,
	fromFingerprint string,
	toFingerprint string,
	amount string,
	tokenType token.Type,
) error {
	fromWithPrefix, fromWithoutPrefix := normalizeFP(fromFingerprint)
	toWithPrefix, toWithoutPrefix := normalizeFP(toFingerprint)
	col := balanceCol(tokenType)

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewUpdate().
			TableExpr("users").
			Set(col+" = COALESCE("+col+", 0) - ?::DECIMAL", amount).
			Set("balance_updated_at = NOW()").
			Where("fingerprint = ? OR fingerprint = ?", fromWithPrefix, fromWithoutPrefix).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to decrement sender %s balance: %w", tokenType, err)
		}

		_, err = tx.NewUpdate().
			TableExpr("users").
			Set(col+" = COALESCE("+col+", 0) + ?::DECIMAL", amount).
			Set("balance_updated_at = NOW()").
			Where("fingerprint = ? OR fingerprint = ?", toWithPrefix, toWithoutPrefix).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to increment recipient %s balance: %w", tokenType, err)
		}
		return nil
	})
}

func (s *pgStore) ResetBalances(ctx context.Context, tokenType token.Type) error {
	col := balanceCol(tokenType)

	_, err := s.db.NewUpdate().
		TableExpr("users").
		Set(col + " = '0'").
		Set("balance_updated_at = NOW()").
		Where("TRUE").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset %s balances: %w", tokenType, err)
	}
	return nil
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

func (s *pgStore) getUserKeyBy(ctx context.Context, decryptor KeyDecryptor, column string, value string) ([]byte, error) {
	dao := new(UserDao)
	query := s.db.NewSelect().
		Model(dao).
		Column("canton_private_key_encrypted").
		Where(column+" = ?", value)

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrKeyNotFound
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

// balanceCol returns the column name for a given token type.
func balanceCol(tokenType token.Type) string {
	if tokenType == token.Demo {
		return "demo_balance"
	}
	return "prompt_balance"
}

// normalizeFP returns (withPrefix, withoutPrefix) for fingerprint lookups.
func normalizeFP(fingerprint string) (string, string) {
	if strings.HasPrefix(fingerprint, "0x") {
		return fingerprint, fingerprint[2:]
	}
	return "0x" + fingerprint, fingerprint
}
