package apidb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding key_mode and canton_public_key_fingerprint columns to users table...")
		_, err := db.ExecContext(ctx, `
			ALTER TABLE users ADD COLUMN IF NOT EXISTS key_mode VARCHAR(20) NOT NULL DEFAULT 'custodial';
			ALTER TABLE users ADD COLUMN IF NOT EXISTS canton_public_key_fingerprint TEXT;
		`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping key_mode and canton_public_key_fingerprint columns from users table...")
		_, err := db.ExecContext(ctx, `
			ALTER TABLE users DROP COLUMN IF EXISTS canton_public_key_fingerprint;
			ALTER TABLE users DROP COLUMN IF EXISTS key_mode;
		`)
		return err
	})
}
