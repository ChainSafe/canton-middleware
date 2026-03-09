package relayerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding retry_count column to transfers table...")
		_, err := db.ExecContext(ctx,
			`ALTER TABLE transfers ADD COLUMN IF NOT EXISTS retry_count integer NOT NULL DEFAULT 0`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("removing retry_count column from transfers table...")
		_, err := db.ExecContext(ctx, `ALTER TABLE transfers DROP COLUMN IF EXISTS retry_count`)
		return err
	})
}
