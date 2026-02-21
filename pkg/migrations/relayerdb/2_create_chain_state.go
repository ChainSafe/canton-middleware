package relayerdb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createChainState() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 2,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating chain_state table...")
				return mghelper.CreateSchema(db.(*pg.Tx), &dao.ChainStateDao{})
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping chain_state table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.ChainStateDao{})
			},
		},
	}
}
