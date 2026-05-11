package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"

	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating indexer_holdings table...")
		if err := mghelper.CreateSchema(ctx, db, &indexerstore.HoldingDao{}); err != nil {
			return err
		}
		return mghelper.CreateModelIndexes(ctx, db, &indexerstore.HoldingDao{},
			"owner,instrument_admin,instrument_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_holdings table...")
		return mghelper.DropTables(ctx, db, &indexerstore.HoldingDao{})
	})
}
