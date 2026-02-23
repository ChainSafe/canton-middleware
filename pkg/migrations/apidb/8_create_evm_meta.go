package apidb

import (
	"log"

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
				log.Println("creating evm_meta table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.EvmMetaDao{}); err != nil {
					return err
				}
				// Insert initial latest block number with ON CONFLICT for idempotency
				_, err := tx.Model(&dao.EvmMetaDao{
					Key:   "latest_block_number",
					Value: "0",
				}).
					OnConflict("(key) DO NOTHING").
					Insert()
				if err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping evm_meta table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.EvmMetaDao{})
			},
		},
	}
}
