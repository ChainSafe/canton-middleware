package relayerdb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createNonceState() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 3,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating nonce_state table...")
				return mghelper.CreateSchema(db.(*pg.Tx), &dao.NonceStateDao{})
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping nonce_state table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.NonceStateDao{})
			},
		},
	}
}
