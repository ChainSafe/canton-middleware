package apidb

import (
	"context"
	"log"

	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating mempool table...")
		return mghelper.CreateSchema(ctx, db, &ethrpcstore.MempoolEntryDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping mempool table...")
		return mghelper.DropTables(ctx, db, &ethrpcstore.MempoolEntryDao{})
	})
}
