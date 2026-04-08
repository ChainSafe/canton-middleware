//go:build e2e

package shim

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

const postgresPingTimeout = 5 * time.Second

// PostgresShim implements stack.APIDatabase. It connects directly to the
// api-server's erc20_api database and is used only for test setup operations
// (whitelisting addresses). Assertions are made through the API, not the DB.
type PostgresShim struct {
	dsn string
	db  *sql.DB
}

// NewPostgres opens a connection to the api-server database. Call Close via
// t.Cleanup when the test suite is done.
func NewPostgres(manifest *stack.ServiceManifest) (*PostgresShim, error) {
	db, err := sql.Open("postgres", manifest.APIDatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresPingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &PostgresShim{dsn: manifest.APIDatabaseDSN, db: db}, nil
}

func (p *PostgresShim) DSN() string  { return p.dsn }
func (p *PostgresShim) Close() error { return p.db.Close() }

// WhitelistAddress inserts evmAddress into the whitelist table so the
// api-server will accept a registration request from that address.
// Idempotent — conflicts on evm_address are silently ignored.
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
