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
		log.Println("creating bridge_balances table...")
		return mghelper.CreateSchema(ctx, db, &dao.BridgeBalanceDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping bridge_balances table...")
		return mghelper.DropTables(ctx, db, &dao.BridgeBalanceDao{})
	})
}
