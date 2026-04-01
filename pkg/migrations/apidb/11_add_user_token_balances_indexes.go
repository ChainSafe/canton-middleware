package apidb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding unique indexes to user_token_balances...")

		// These unique indexes back the ON CONFLICT upserts in reconciler/store/pg.go.
		// Because the identifier columns are nullable, PostgreSQL allows multiple NULL rows
		// in a unique index (NULL != NULL), so rows without a given identifier do not conflict.
		for _, ddl := range []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_utb_fingerprint_token
				ON user_token_balances (fingerprint, token_symbol)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_utb_evm_address_token
				ON user_token_balances (evm_address, token_symbol)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_utb_canton_party_token
				ON user_token_balances (canton_party_id, token_symbol)`,
		} {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping unique indexes from user_token_balances...")
		for _, ddl := range []string{
			`DROP INDEX IF EXISTS idx_utb_fingerprint_token`,
			`DROP INDEX IF EXISTS idx_utb_evm_address_token`,
			`DROP INDEX IF EXISTS idx_utb_canton_party_token`,
		} {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				return err
			}
		}
		return nil
	})
}
