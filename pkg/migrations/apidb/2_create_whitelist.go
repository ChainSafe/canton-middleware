package apidb

import (
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createWhitelist() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 2,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				log.Println("creating whitelist table...")
				return mghelper.CreateSchema(db.(*pg.Tx), &dao.WhitelistDao{})
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping whitelist table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.WhitelistDao{})
			},
		},
	}
}
