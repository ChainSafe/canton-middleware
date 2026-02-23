package relayerdb

import (
	"log"

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
				log.Println("creating transfers table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.TransferDao{}); err != nil {
					return err
				}
				// Create indexes
				return mghelper.CreateModelIndexes(tx, &dao.TransferDao{}, "status", "direction", "source_tx_hash")
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping transfers table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.TransferDao{})
			},
		},
	}
}
