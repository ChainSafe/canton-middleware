package apidb

import (
	"fmt"

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
				fmt.Println("creating evm_transactions table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.EvmTransactionDao{}); err != nil {
					return err
				}
				// Create indexes
				if err := mghelper.CreateIndex(tx, "evm_transactions", "idx_evm_transactions_from", "from_address"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "evm_transactions", "idx_evm_transactions_to", "to_address"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "evm_transactions", "idx_evm_transactions_block", "block_number"); err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping evm_transactions table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.EvmTransactionDao{})
			},
		},
	}
}
