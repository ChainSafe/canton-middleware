package apidb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding block_timestamp column to evm_logs...")
		_, err := db.ExecContext(ctx, `ALTER TABLE evm_logs ADD COLUMN IF NOT EXISTS block_timestamp BIGINT NOT NULL DEFAULT 0`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping block_timestamp column from evm_logs...")
		_, err := db.ExecContext(ctx, `ALTER TABLE evm_logs DROP COLUMN IF EXISTS block_timestamp`)
		return err
	})
}
