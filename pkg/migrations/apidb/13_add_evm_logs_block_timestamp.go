package apidb

import (
	"context"
	"log"

	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding block_timestamp column to evm_logs...")
		_, err := db.NewAddColumn().
			Model(&ethrpcstore.EvmLogDao{}).
			ColumnExpr("block_timestamp BIGINT NOT NULL DEFAULT 0").
			IfNotExists().
			Exec(ctx)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping block_timestamp column from evm_logs...")
		_, err := db.NewDropColumn().
			Model(&ethrpcstore.EvmLogDao{}).
			Column("block_timestamp").
			Exec(ctx)
		return err
	})
}
