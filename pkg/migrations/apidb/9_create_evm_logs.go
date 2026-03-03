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
		log.Println("creating evm_logs table...")
		return mghelper.CreateSchema(ctx, db, &ethrpcstore.EvmLogDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping evm_logs table...")
		return mghelper.DropTables(ctx, db, &ethrpcstore.EvmLogDao{})
	})
}
