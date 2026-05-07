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
		log.Println("creating indexer_pending_offers table...")
		if err := mghelper.CreateSchema(ctx, db, &indexerstore.PendingOfferDao{}); err != nil {
			return err
		}
		return mghelper.CreateModelIndexes(ctx, db, &indexerstore.PendingOfferDao{}, "receiver_party_id,status")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_pending_offers table...")
		return mghelper.DropTables(ctx, db, &indexerstore.PendingOfferDao{})
	})
}
