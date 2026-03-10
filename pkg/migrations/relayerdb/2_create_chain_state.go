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
		log.Println("creating chain_state table...")
		return mghelper.CreateSchema(ctx, db, &relayerstore.ChainStateDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping chain_state table...")
		return mghelper.DropTables(ctx, db, &relayerstore.ChainStateDao{})
	})
}
