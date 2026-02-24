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
		log.Println("creating reconciliation_state table...")
		if err := mghelper.CreateSchema(ctx, db, &dao.ReconciliationStateDao{}); err != nil {
			return err
		}
		// Add singleton constraint to ensure only one row with id=1
		_, err := db.NewAddColumn().
			Model((*dao.ReconciliationStateDao)(nil)).
			ColumnExpr("CONSTRAINT singleton_check CHECK (id = 1)").
			Exec(ctx)
		if err != nil {
			return err
		}
		// Insert initial state row with ON CONFLICT for idempotency
		_, err = db.NewInsert().
			Model(&dao.ReconciliationStateDao{
				ID:                  1,
				LastProcessedOffset: 0,
				EventsProcessed:     0,
			}).
			On("CONFLICT (id) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return err
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping reconciliation_state table...")
		return mghelper.DropTables(ctx, db, &dao.ReconciliationStateDao{})
	})
}
