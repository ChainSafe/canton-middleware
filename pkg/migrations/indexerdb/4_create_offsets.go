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
		log.Println("creating indexer_offsets table...")
		return mghelper.CreateSchema(ctx, db, &indexerstore.OffsetDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_offsets table...")
		return mghelper.DropTables(ctx, db, &indexerstore.OffsetDao{})
	})
}
