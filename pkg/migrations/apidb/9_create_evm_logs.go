package apidb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createEvmLogs() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 9,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating evm_logs table...")
				return mghelper.CreateSchema(db.(*pg.Tx), &dao.EvmLogDao{})
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping evm_logs table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.EvmLogDao{})
			},
		},
	}
}
