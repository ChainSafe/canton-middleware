package apidb

import (
	"context"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating evm_meta table...")
		if err := mghelper.CreateSchema(ctx, db, &dao.EvmMetaDao{}); err != nil {
			return err
		}
		// Insert initial latest block number with ON CONFLICT for idempotency
		_, err := db.NewInsert().
			Model(&dao.EvmMetaDao{
				Key:   "latest_block_number",
				Value: "0",
			}).
			On("CONFLICT (key) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return err
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping evm_meta table...")
		return mghelper.DropTables(ctx, db, &dao.EvmMetaDao{})
	})
}
