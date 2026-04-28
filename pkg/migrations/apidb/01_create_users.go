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
		log.Println("creating users table...")
		if err := mghelper.CreateSchema(ctx, db, &userstore.UserDao{}); err != nil {
			return err
		}
		// Create indexes
		return mghelper.CreateModelIndexes(ctx, db, &userstore.UserDao{}, "fingerprint", "canton_party_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping users table...")
		return mghelper.DropTables(ctx, db, &userstore.UserDao{})
	})
}
