package apidb

import (
	"context"
	"log"

	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating evm_transactions table...")
		if err := mghelper.CreateSchema(ctx, db, &ethrpcstore.EvmTransactionDao{}); err != nil {
			return err
		}
		// Create indexes
		return mghelper.CreateModelIndexes(ctx, db, &ethrpcstore.EvmTransactionDao{}, "from_address", "to_address", "block_number")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping evm_transactions table...")
		return mghelper.DropTables(ctx, db, &ethrpcstore.EvmTransactionDao{})
	})
}
