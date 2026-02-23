package apidb

import (
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createBridgeEvents() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 5,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				log.Println("creating bridge_events table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.BridgeEventDao{}); err != nil {
					return err
				}
				// Create indexes
				return mghelper.CreateModelIndexes(tx, &dao.BridgeEventDao{}, "fingerprint", "event_type", "evm_tx_hash")
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping bridge_events table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.BridgeEventDao{})
			},
		},
	}
}
