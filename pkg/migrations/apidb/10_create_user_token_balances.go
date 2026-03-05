package apidb

import (
	"context"
	"log"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	reconcilerstore "github.com/chainsafe/canton-middleware/pkg/reconciler/store"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating user_token_balances table...")
		return mghelper.CreateSchema(ctx, db, &reconcilerstore.UserTokenBalanceDao{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping user_token_balances table...")
		return mghelper.DropTables(ctx, db, &reconcilerstore.UserTokenBalanceDao{})
	})
}
