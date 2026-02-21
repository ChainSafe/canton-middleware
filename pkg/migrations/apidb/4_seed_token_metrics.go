package apidb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func seedTokenMetrics() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 4,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("seeding token_metrics table...")
				tx := db.(*pg.Tx)

				promptSupply := "0"
				demoSupply := "0"

				// Insert PROMPT token
				promptMetrics := &dao.TokenMetricsDao{
					TokenSymbol: "PROMPT",
					TotalSupply: promptSupply,
				}

				// Insert DEMO token
				demoMetrics := &dao.TokenMetricsDao{
					TokenSymbol: "DEMO",
					TotalSupply: demoSupply,
				}

				if err := mghelper.InsertEntry(tx, promptMetrics, demoMetrics); err != nil {
					return err
				}

				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("removing seed data from token_metrics table...")
				return mghelper.TruncateTables(db.(*pg.Tx), &dao.TokenMetricsDao{})
			},
		},
	}
}
