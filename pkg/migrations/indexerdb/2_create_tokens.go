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
		log.Println("creating indexer_tokens table...")
		return mghelper.CreateSchema(ctx, db, &indexerstore.TokenDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_tokens table...")
		return mghelper.DropTables(ctx, db, &indexerstore.TokenDao{})
	})
}
