package apidb

import (
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createEvmTransactions() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 7,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				log.Println("creating evm_transactions table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.EvmTransactionDao{}); err != nil {
					return err
				}
				// Create indexes
				return mghelper.CreateModelIndexes(tx, &dao.EvmTransactionDao{}, "from_address", "to_address", "block_number")
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping evm_transactions table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.EvmTransactionDao{})
			},
		},
	}
}
