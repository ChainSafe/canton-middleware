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
		log.Println("creating users table...")
		if err := mghelper.CreateSchema(ctx, db, &dao.UserDao{}); err != nil {
			return err
		}
		// Create indexes
		return mghelper.CreateModelIndexes(ctx, db, &dao.UserDao{}, "fingerprint", "canton_party_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping users table...")
		return mghelper.DropTables(ctx, db, &dao.UserDao{})
	})
}
