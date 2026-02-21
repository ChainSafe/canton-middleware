package apidb

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/go-pg/migrations/v8"
	"github.com/go-pg/pg/v10"
)

func createReconciliationState() []*migrations.Migration {
	return []*migrations.Migration{
		{
			Version: 6,
			UpTx:    true,
			Up: func(db migrations.DB) error {
				fmt.Println("creating reconciliation_state table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.ReconciliationStateDao{}); err != nil {
					return err
				}
				// Insert initial state row
				initialState := &dao.ReconciliationStateDao{
					ID:                  1,
					LastProcessedOffset: 0,
					EventsProcessed:     0,
				}
				if err := mghelper.InsertEntry(tx, initialState); err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				fmt.Println("dropping reconciliation_state table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.ReconciliationStateDao{})
			},
		},
	}
}
