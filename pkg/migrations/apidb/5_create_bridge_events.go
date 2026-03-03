package apidb

import (
	"context"
	"log"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	reconcilerstore "github.com/chainsafe/canton-middleware/pkg/reconciler/store"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating bridge_events table...")
		if err := mghelper.CreateSchema(ctx, db, &reconcilerstore.BridgeEventDao{}); err != nil {
			return err
		}
		// Create indexes
		return mghelper.CreateModelIndexes(ctx, db, &reconcilerstore.BridgeEventDao{}, "fingerprint", "event_type", "evm_tx_hash")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping bridge_events table...")
		return mghelper.DropTables(ctx, db, &reconcilerstore.BridgeEventDao{})
	})
}
