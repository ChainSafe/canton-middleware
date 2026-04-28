package apidb

import (
	"context"
	"log"

	reconcilerstore "github.com/chainsafe/canton-middleware/pkg/reconciler/store"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("seeding token_metrics table...")

		// Insert PROMPT token with ON CONFLICT for idempotency
		_, err := db.NewInsert().
			Model(&reconcilerstore.TokenMetricsDao{
				TokenSymbol: "PROMPT",
				TotalSupply: "0",
			}).
			On("CONFLICT (token_symbol) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return err
		}

		// Insert DEMO token with ON CONFLICT for idempotency
		_, err = db.NewInsert().
			Model(&reconcilerstore.TokenMetricsDao{
				TokenSymbol: "DEMO",
				TotalSupply: "0",
			}).
			On("CONFLICT (token_symbol) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return err
		}

		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("removing seed data from token_metrics table...")
		// Only delete the seeded PROMPT and DEMO rows, not all data
		_, err := db.NewDelete().
			Model((*reconcilerstore.TokenMetricsDao)(nil)).
			Where("token_symbol IN (?)", bun.List([]string{"PROMPT", "DEMO"})).
			Exec(ctx)
		return err
	})
}
