package apidb

import (
	"database/sql"
	"fmt"
	"time"
)

// User represents a registered EVM user mapped to a Canton party
type User struct {
	ID          int64     `json:"id"`
	EVMAddress  string    `json:"evm_address"`
	CantonParty string    `json:"canton_party"`
	Fingerprint string    `json:"fingerprint"`
	MappingCID  string    `json:"mapping_cid,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
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
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, created_at
		FROM users
		WHERE evm_address = $1
	`
	err := s.db.QueryRow(query, evmAddress).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&user.MappingCID,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// GetUserByFingerprint retrieves a user by their fingerprint
func (s *Store) GetUserByFingerprint(fingerprint string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, created_at
		FROM users
		WHERE fingerprint = $1
	`
	err := s.db.QueryRow(query, fingerprint).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&user.MappingCID,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
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

