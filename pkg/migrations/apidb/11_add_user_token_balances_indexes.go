package apidb

import (
	"context"
	"log"

	reconcilerstore "github.com/chainsafe/canton-middleware/pkg/reconciler/store"

	"github.com/uptrace/bun"
)

// utbIndex describes a composite unique index on user_token_balances.
type utbIndex struct {
	name    string
	columns []string
}

var utbIndexes = []utbIndex{
	{"idx_utb_fingerprint_token", []string{"fingerprint", "token_symbol"}},
	{"idx_utb_evm_address_token", []string{"evm_address", "token_symbol"}},
	{"idx_utb_canton_party_token", []string{"canton_party_id", "token_symbol"}},
}

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding unique indexes to user_token_balances...")

		// These unique indexes back the ON CONFLICT upserts in reconciler/store/pg.go.
		// Because the identifier columns are nullable, PostgreSQL allows multiple NULL rows
		// in a unique index (NULL != NULL), so rows without a given identifier do not conflict.
		for _, idx := range utbIndexes {
			if _, err := db.NewCreateIndex().
				Model((*reconcilerstore.UserTokenBalanceDao)(nil)).
				Index(idx.name).
				Column(idx.columns...).
				Unique().
				IfNotExists().
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping unique indexes from user_token_balances...")
		for _, idx := range utbIndexes {
			if _, err := db.NewDropIndex().
				Index(idx.name).
				IfExists().
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
