package pg

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/registration"
	"github.com/chainsafe/canton-middleware/pkg/registration/store"
)

type pgStore struct {
	db *sql.DB
}

// NewStore creates a new postgres implementation of the registration store
func NewStore(db *sql.DB) store.Store {
	return &pgStore{db: db}
}

func (s *pgStore) CreateUser(ctx context.Context, user *registration.User) error {
	query := `
		INSERT INTO users (evm_address, canton_party, fingerprint, mapping_cid, canton_party_id, canton_key_created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.ExecContext(ctx, query,
		user.EVMAddress,
		user.CantonParty,
		user.Fingerprint,
		user.MappingCID,
		sql.NullString{String: user.CantonPartyID, Valid: user.CantonPartyID != ""},
		user.CantonKeyCreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (s *pgStore) GetUserByEVMAddress(ctx context.Context, evmAddress string) (*registration.User, error) {
	var user registration.User
	var mappingCID sql.NullString
	var cantonPartyID sql.NullString
	var cantonKeyCreatedAt sql.NullTime

	query := `
		SELECT evm_address, canton_party, fingerprint, mapping_cid, canton_party_id, canton_key_created_at
		FROM users
		WHERE evm_address = $1
	`
	err := s.db.QueryRowContext(ctx, query, evmAddress).Scan(
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&mappingCID,
		&cantonPartyID,
		&cantonKeyCreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by evm address: %w", err)
	}

	if mappingCID.Valid {
		user.MappingCID = mappingCID.String
	}
	if cantonPartyID.Valid {
		user.CantonPartyID = cantonPartyID.String
	}
	if cantonKeyCreatedAt.Valid {
		user.CantonKeyCreatedAt = &cantonKeyCreatedAt.Time
	}

	return &user, nil
}

func (s *pgStore) GetUserByCantonPartyID(ctx context.Context, cantonPartyID string) (*registration.User, error) {
	var user registration.User
	var mappingCID sql.NullString
	var cPartyID sql.NullString
	var cantonKeyCreatedAt sql.NullTime

	query := `
		SELECT evm_address, canton_party, fingerprint, mapping_cid, canton_party_id, canton_key_created_at
		FROM users
		WHERE canton_party_id = $1
	`
	err := s.db.QueryRowContext(ctx, query, cantonPartyID).Scan(
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&mappingCID,
		&cPartyID,
		&cantonKeyCreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by canton party id: %w", err)
	}

	if mappingCID.Valid {
		user.MappingCID = mappingCID.String
	}
	if cPartyID.Valid {
		user.CantonPartyID = cPartyID.String
	}
	if cantonKeyCreatedAt.Valid {
		user.CantonKeyCreatedAt = &cantonKeyCreatedAt.Time
	}

	return &user, nil
}

func (s *pgStore) UserExists(ctx context.Context, evmAddress string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE evm_address = $1)`
	err := s.db.QueryRowContext(ctx, query, evmAddress).Scan(&exists)
	return exists, err
}

func (s *pgStore) IsWhitelisted(ctx context.Context, evmAddress string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM whitelist WHERE evm_address = $1)`
	err := s.db.QueryRowContext(ctx, query, evmAddress).Scan(&exists)
	return exists, err
}

func (s *pgStore) DeleteUser(ctx context.Context, evmAddress string) error {
	query := `DELETE FROM users WHERE evm_address = $1`
	_, err := s.db.ExecContext(ctx, query, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}
