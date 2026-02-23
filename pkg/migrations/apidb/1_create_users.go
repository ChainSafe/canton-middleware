package apidb

import (
	"log"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createUsers() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 1,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				log.Println("creating users table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.UserDao{}); err != nil {
					return err
				}
				// Create indexes
				return mghelper.CreateModelIndexes(tx, &dao.UserDao{}, "fingerprint", "canton_party_id")
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping users table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.UserDao{})
			},
		},
	}
}
