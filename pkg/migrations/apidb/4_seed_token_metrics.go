package apidb

import (
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func seedTokenMetrics() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 4,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				log.Println("seeding token_metrics table...")
				tx := db.(*pg.Tx)

				// Insert PROMPT and DEMO tokens with ON CONFLICT for idempotency
				_, err := tx.Model(&dao.TokenMetricsDao{
					TokenSymbol: "PROMPT",
					TotalSupply: "0",
				}).
					OnConflict("(token_symbol) DO NOTHING").
					Insert()
				if err != nil {
					return err
				}

				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("removing seed data from token_metrics table...")
				tx := db.(*pg.Tx)
				// Only delete the seeded PROMPT and DEMO rows, not all data
				_, err := tx.Model((*dao.TokenMetricsDao)(nil)).
					Where("token_symbol IN ('PROMPT', 'DEMO')").
					Delete()
				return err
			},
		},
	}
}
