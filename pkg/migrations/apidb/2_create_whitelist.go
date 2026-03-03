package apidb

import (
	"context"
	"log"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	"github.com/chainsafe/canton-middleware/pkg/userstore"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating whitelist table...")
		return mghelper.CreateSchema(ctx, db, &userstore.WhitelistDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping whitelist table...")
		return mghelper.DropTables(ctx, db, &userstore.WhitelistDao{})
	})
}
