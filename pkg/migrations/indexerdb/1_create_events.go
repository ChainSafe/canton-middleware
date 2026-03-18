package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating indexer_events table...")
		if err := mghelper.CreateSchema(ctx, db, &indexerstore.EventDao{}); err != nil {
			return err
		}
		// Index for ledger_offset (resume point queries and ordered listing).
		if err := mghelper.CreateModelIndexes(ctx, db, &indexerstore.EventDao{}, "ledger_offset"); err != nil {
			return err
		}
		// Indexes for party event queries (from_party_id OR to_party_id).
		if err := mghelper.CreateModelIndexes(ctx, db, &indexerstore.EventDao{}, "from_party_id"); err != nil {
			return err
		}
		return mghelper.CreateModelIndexes(ctx, db, &indexerstore.EventDao{}, "to_party_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_events table...")
		return mghelper.DropTables(ctx, db, &indexerstore.EventDao{})
	})
}
