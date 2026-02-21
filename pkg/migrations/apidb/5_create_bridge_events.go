package apidb

import (
	"fmt"

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
				fmt.Println("creating bridge_events table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.BridgeEventDao{}); err != nil {
					return err
				}
				// Create indexes
				if err := mghelper.CreateIndex(tx, "bridge_events", "idx_bridge_events_fingerprint", "fingerprint"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "bridge_events", "idx_bridge_events_type", "event_type"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "bridge_events", "idx_bridge_events_evm_tx", "evm_tx_hash"); err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping bridge_events table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.BridgeEventDao{})
			},
		},
	}
}
