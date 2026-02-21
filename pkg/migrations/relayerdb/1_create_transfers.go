package relayerdb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createTransfers() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 1,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating transfers table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.TransferDao{}); err != nil {
					return err
				}
				// Create indexes
				if err := mghelper.CreateIndex(tx, "transfers", "idx_transfers_status", "status"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "transfers", "idx_transfers_direction", "direction"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "transfers", "idx_transfers_source_tx_hash", "source_tx_hash"); err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping transfers table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.TransferDao{})
			},
		},
	}
}
