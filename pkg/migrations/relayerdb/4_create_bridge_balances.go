package relayerdb

import (
	"log"

	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createBridgeBalances() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 4,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				log.Println("creating bridge_balances table...")
				return mghelper.CreateSchema(db.(*pg.Tx), &dao.BridgeBalanceDao{})
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping bridge_balances table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.BridgeBalanceDao{})
			},
		},
	}
}
