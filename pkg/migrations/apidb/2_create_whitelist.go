package apidb

import (
	"context"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating whitelist table...")
		return mghelper.CreateSchema(ctx, db, &dao.WhitelistDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping whitelist table...")
		return mghelper.DropTables(ctx, db, &dao.WhitelistDao{})
	})
}
