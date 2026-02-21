package apidb

import (
	"fmt"

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
				fmt.Println("creating users table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.UserDao{}); err != nil {
					return err
				}
				// Create indexes
				if err := mghelper.CreateIndex(tx, "users", "idx_users_evm", "evm_address"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "users", "idx_users_fingerprint", "fingerprint"); err != nil {
					return err
				}
				if err := mghelper.CreateIndex(tx, "users", "idx_users_canton_party_id", "canton_party_id"); err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping users table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.UserDao{})
			},
		},
	}
}
