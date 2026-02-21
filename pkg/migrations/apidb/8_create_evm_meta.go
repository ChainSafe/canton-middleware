package apidb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createEvmMeta() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 8,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating evm_meta table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.EvmMetaDao{}); err != nil {
					return err
				}
				// Insert initial latest block number
				initialMeta := &dao.EvmMetaDao{
					Key:   "latest_block_number",
					Value: "0",
				}
				if err := mghelper.InsertEntry(tx, initialMeta); err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping evm_meta table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.EvmMetaDao{})
			},
		},
	}
}
