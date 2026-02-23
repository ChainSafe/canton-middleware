package relayerdb

import (
	"context"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating nonce_state table...")
		return mghelper.CreateSchema(ctx, db, &dao.NonceStateDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping nonce_state table...")
		return mghelper.DropTables(ctx, db, &dao.NonceStateDao{})
	})
}
