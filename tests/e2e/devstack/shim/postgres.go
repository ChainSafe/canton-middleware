//go:build e2e

package shim

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	_ "github.com/lib/pq"
)

// PostgresShim implements stack.APIDatabase using database/sql with the lib/pq driver.
// It connects directly to the api-server's erc20_api database.
type PostgresShim struct {
	dsn string
	db  *sql.DB
}

// NewPostgres opens a connection to the api-server database using the DSN
// from the manifest. The caller is responsible for calling Close when done.
func NewPostgres(manifest *stack.ServiceManifest) (*PostgresShim, error) {
	db, err := sql.Open("postgres", manifest.APIDatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &PostgresShim{dsn: manifest.APIDatabaseDSN, db: db}, nil
}

func (p *PostgresShim) DSN() string { return p.dsn }

// Close releases the database connection. Tests should call this via t.Cleanup.
func (p *PostgresShim) Close() error { return p.db.Close() }

// WhitelistAddress inserts evmAddress into the whitelist table.
// Conflicts on evm_address are ignored so the call is idempotent.
func (p *PostgresShim) WhitelistAddress(ctx context.Context, evmAddress string) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO whitelist (evm_address, note) VALUES ($1, 'e2e-test') ON CONFLICT (evm_address) DO NOTHING`,
		evmAddress,
	)
	if err != nil {
		return fmt.Errorf("whitelist %s: %w", evmAddress, err)
	}
	return nil
}

// GetUserByEVMAddress returns the user row for evmAddress, or nil if not found.
func (p *PostgresShim) GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT evm_address, canton_party_id, fingerprint, key_mode FROM users WHERE evm_address = $1`,
		evmAddress,
	)
	var u user.User
	var cantonPartyID sql.NullString
	err := row.Scan(&u.EVMAddress, &cantonPartyID, &u.Fingerprint, &u.KeyMode)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user %s: %w", evmAddress, err)
	}
	if cantonPartyID.Valid {
		u.CantonPartyID = cantonPartyID.String
		u.CantonParty = cantonPartyID.String
	}
	return &u, nil
}
