package relayerdb

import (
	"context"
	"log"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	relayerstore "github.com/chainsafe/canton-middleware/pkg/relayer/store"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating transfers table...")
		if err := mghelper.CreateSchema(ctx, db, &relayerstore.TransferDao{}); err != nil {
			return err
		}
		return mghelper.CreateModelIndexes(ctx, db, &relayerstore.TransferDao{}, "status", "direction", "source_tx_hash")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping transfers table...")
		return mghelper.DropTables(ctx, db, &relayerstore.TransferDao{})
	})
}
