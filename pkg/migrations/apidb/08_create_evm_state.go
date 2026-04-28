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
		log.Println("creating evm_state table...")
		if err := mghelper.CreateSchema(ctx, db, &ethrpcstore.EvmStateDao{}); err != nil {
			return err
		}
		_, err := db.NewInsert().
			Model(&ethrpcstore.EvmStateDao{ID: 1, LatestBlock: 0}).
			On("CONFLICT (id) DO NOTHING").
			Exec(ctx)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping evm_state table...")
		return mghelper.DropTables(ctx, db, &ethrpcstore.EvmStateDao{})
	})
}
