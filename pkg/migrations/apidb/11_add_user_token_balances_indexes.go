package apidb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding unique indexes to user_token_balances...")

		// These three unique indexes back the ON CONFLICT upserts in reconciler/store/pg.go.
		// They are partial (WHERE col IS NOT NULL) so that rows with a NULL identifier do not
		// conflict with each other — PostgreSQL treats NULL as distinct in unique indexes, but
		// a partial index keeps the predicate explicit and allows bun's ON CONFLICT inference.
		for _, ddl := range []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_utb_fingerprint_token
				ON user_token_balances (fingerprint, token_symbol)
				WHERE fingerprint IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_utb_evm_address_token
				ON user_token_balances (evm_address, token_symbol)
				WHERE evm_address IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_utb_canton_party_token
				ON user_token_balances (canton_party_id, token_symbol)
				WHERE canton_party_id IS NOT NULL`,
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
