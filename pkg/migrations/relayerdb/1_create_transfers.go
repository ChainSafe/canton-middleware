package relayerdb

import (
	"context"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating transfers table...")
		if err := mghelper.CreateSchema(ctx, db, &dao.TransferDao{}); err != nil {
			return err
		}
		// Create indexes
		return mghelper.CreateModelIndexes(ctx, db, &dao.TransferDao{}, "status", "direction", "source_tx_hash")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping transfers table...")
		return mghelper.DropTables(ctx, db, &dao.TransferDao{})
	})
}
