package apidb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createTokenMetrics() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 3,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating token_metrics table...")
				return mghelper.CreateSchema(db.(*pg.Tx), &dao.TokenMetricsDao{})
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping token_metrics table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.TokenMetricsDao{})
			},
		},
	}
}
