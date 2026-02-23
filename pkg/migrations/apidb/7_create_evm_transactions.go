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
		log.Println("creating evm_transactions table...")
		if err := mghelper.CreateSchema(ctx, db, &dao.EvmTransactionDao{}); err != nil {
			return err
		}
		// Create indexes
		return mghelper.CreateModelIndexes(ctx, db, &dao.EvmTransactionDao{}, "from_address", "to_address", "block_number")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping evm_transactions table...")
		return mghelper.DropTables(ctx, db, &dao.EvmTransactionDao{})
	})
}
