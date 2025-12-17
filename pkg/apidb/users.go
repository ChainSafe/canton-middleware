package apidb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// normalizeFingerprint returns both 0x-prefixed and non-prefixed versions of a fingerprint.
// This allows queries to match fingerprints stored in either format.
func normalizeFingerprint(fingerprint string) (withPrefix, withoutPrefix string) {
	if strings.HasPrefix(fingerprint, "0x") {
		return fingerprint, fingerprint[2:]
	}
	return "0x" + fingerprint, fingerprint
}

// User represents a registered EVM user mapped to a Canton party
type User struct {
	ID               int64      `json:"id"`
	EVMAddress       string     `json:"evm_address"`
	CantonParty      string     `json:"canton_party"`
	Fingerprint      string     `json:"fingerprint"`
	MappingCID       string     `json:"mapping_cid,omitempty"`
	Balance          string     `json:"balance"`
	BalanceUpdatedAt *time.Time `json:"balance_updated_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// CreateUser creates a new user record
func (s *Store) CreateUser(user *User) error {
	query := `
		INSERT INTO users (evm_address, canton_party, fingerprint, mapping_cid)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	return s.db.QueryRow(
		query,
		user.EVMAddress,
		user.CantonParty,
		user.Fingerprint,
		user.MappingCID,
	).Scan(&user.ID, &user.CreatedAt)
}

// GetUserByEVMAddress retrieves a user by their EVM address
func (s *Store) GetUserByEVMAddress(evmAddress string) (*User, error) {
	user := &User{}
	var balance sql.NullString
	var balanceUpdatedAt sql.NullTime
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, balance, balance_updated_at, created_at
		FROM users
		WHERE evm_address = $1
	`
	err := s.db.QueryRow(query, evmAddress).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&user.MappingCID,
		&balance,
		&balanceUpdatedAt,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if balance.Valid {
		user.Balance = balance.String
	} else {
		user.Balance = "0"
	}
	if balanceUpdatedAt.Valid {
		user.BalanceUpdatedAt = &balanceUpdatedAt.Time
	}
	return user, nil
}

// GetUserByFingerprint retrieves a user by their fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) GetUserByFingerprint(fingerprint string) (*User, error) {
	user := &User{}
	var balance sql.NullString
	var balanceUpdatedAt sql.NullTime

	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, balance, balance_updated_at, created_at
		FROM users
		WHERE fingerprint = $1 OR fingerprint = $2
	`
	err := s.db.QueryRow(query, withPrefix, withoutPrefix).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&user.MappingCID,
		&balance,
		&balanceUpdatedAt,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if balance.Valid {
		user.Balance = balance.String
	} else {
		user.Balance = "0"
	}
	if balanceUpdatedAt.Valid {
		user.BalanceUpdatedAt = &balanceUpdatedAt.Time
	}
	return user, nil
}

// UpdateUserMappingCID updates the Canton FingerprintMapping contract ID for a user
func (s *Store) UpdateUserMappingCID(evmAddress, mappingCID string) error {
	query := `
		UPDATE users
		SET mapping_cid = $1
		WHERE evm_address = $2
	`
	result, err := s.db.Exec(query, mappingCID, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to update mapping CID: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// UserExists checks if a user with the given EVM address exists
func (s *Store) UserExists(evmAddress string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE evm_address = $1)`
	err := s.db.QueryRow(query, evmAddress).Scan(&exists)
	return exists, err
}

// =============================================================================
// Balance Cache Methods
// =============================================================================

// GetUserBalance returns the cached balance for a user by EVM address
func (s *Store) GetUserBalance(evmAddress string) (string, error) {
	var balance sql.NullString
	query := `SELECT balance FROM users WHERE evm_address = $1`
	err := s.db.QueryRow(query, evmAddress).Scan(&balance)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "0", fmt.Errorf("failed to get balance: %w", err)
	}
	if !balance.Valid {
		return "0", nil
	}
	return balance.String, nil
}

// GetUserBalanceByFingerprint returns the cached balance for a user by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) GetUserBalanceByFingerprint(fingerprint string) (string, error) {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	var balance sql.NullString
	query := `SELECT balance FROM users WHERE fingerprint = $1 OR fingerprint = $2`
	err := s.db.QueryRow(query, withPrefix, withoutPrefix).Scan(&balance)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "0", fmt.Errorf("failed to get balance: %w", err)
	}
	if !balance.Valid {
		return "0", nil
	}
	return balance.String, nil
}

// UpdateUserBalance sets the balance for a user (full replacement)
func (s *Store) UpdateUserBalance(evmAddress, newBalance string) error {
	query := `
		UPDATE users
		SET balance = $1, balance_updated_at = NOW()
		WHERE evm_address = $2
	`
	result, err := s.db.Exec(query, newBalance, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// UpdateUserBalanceByFingerprint sets the balance for a user by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) UpdateUserBalanceByFingerprint(fingerprint, newBalance string) error {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	query := `
		UPDATE users
		SET balance = $1, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`
	result, err := s.db.Exec(query, newBalance, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found for fingerprint: %s", fingerprint)
	}
	return nil
}

// IncrementBalance adds amount to user's balance atomically
func (s *Store) IncrementBalance(evmAddress, amount string) error {
	query := `
		UPDATE users
		SET balance = COALESCE(balance, 0) + $1::DECIMAL, balance_updated_at = NOW()
		WHERE evm_address = $2
	`
	result, err := s.db.Exec(query, amount, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to increment balance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// IncrementBalanceByFingerprint adds amount to user's balance atomically by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) IncrementBalanceByFingerprint(fingerprint, amount string) error {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	query := `
		UPDATE users
		SET balance = COALESCE(balance, 0) + $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`
	result, err := s.db.Exec(query, amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to increment balance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found for fingerprint: %s", fingerprint)
	}
	return nil
}

// DecrementBalance subtracts amount from user's balance atomically
func (s *Store) DecrementBalance(evmAddress, amount string) error {
	query := `
		UPDATE users
		SET balance = COALESCE(balance, 0) - $1::DECIMAL, balance_updated_at = NOW()
		WHERE evm_address = $2
	`
	result, err := s.db.Exec(query, amount, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to decrement balance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// DecrementBalanceByFingerprint subtracts amount from user's balance atomically by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) DecrementBalanceByFingerprint(fingerprint, amount string) error {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	query := `
		UPDATE users
		SET balance = COALESCE(balance, 0) - $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`
	result, err := s.db.Exec(query, amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to decrement balance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found for fingerprint: %s", fingerprint)
	}
	return nil
}

// GetAllUsers returns all registered users with their balances
func (s *Store) GetAllUsers() ([]*User, error) {
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, balance, balance_updated_at, created_at
		FROM users
		ORDER BY id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var balance sql.NullString
		var balanceUpdatedAt sql.NullTime
		err := rows.Scan(
			&user.ID,
			&user.EVMAddress,
			&user.CantonParty,
			&user.Fingerprint,
			&user.MappingCID,
			&balance,
			&balanceUpdatedAt,
			&user.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		if balance.Valid {
			user.Balance = balance.String
		} else {
			user.Balance = "0"
		}
		if balanceUpdatedAt.Valid {
			user.BalanceUpdatedAt = &balanceUpdatedAt.Time
		}
		users = append(users, user)
	}
	return users, nil
}
