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
		log.Println("creating indexer_balances table...")
		if err := mghelper.CreateSchema(ctx, db, &indexerstore.BalanceDao{}); err != nil {
			return err
		}
		// Index for ListBalancesForParty queries.
		return mghelper.CreateModelIndexes(ctx, db, &indexerstore.BalanceDao{}, "party_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_balances table...")
		return mghelper.DropTables(ctx, db, &indexerstore.BalanceDao{})
	})
}
