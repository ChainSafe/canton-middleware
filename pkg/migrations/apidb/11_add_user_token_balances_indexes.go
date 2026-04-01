package apidb

import (
	"context"
	"log"

	reconcilerstore "github.com/chainsafe/canton-middleware/pkg/reconciler/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding unique indexes to user_token_balances...")

		// These unique indexes back the ON CONFLICT upserts in reconciler/store/pg.go.
		// Because the identifier columns are nullable, PostgreSQL allows multiple NULL rows
		// in a unique index (NULL != NULL), so rows without a given identifier do not conflict.
		return mghelper.CreateModelUniqueIndexes(ctx, db, &reconcilerstore.UserTokenBalanceDao{},
			"fingerprint,token_symbol",
			"evm_address,token_symbol",
			"canton_party_id,token_symbol",
		)
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping unique indexes from user_token_balances...")
		return mghelper.DropModelIndexes(ctx, db, &reconcilerstore.UserTokenBalanceDao{},
			"fingerprint,token_symbol",
			"evm_address,token_symbol",
			"canton_party_id,token_symbol",
		)
	})
}
