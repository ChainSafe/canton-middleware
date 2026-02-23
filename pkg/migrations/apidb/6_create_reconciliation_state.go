package apidb

import (
	"log"

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
				log.Println("creating reconciliation_state table...")
				tx := db.(*pg.Tx)
				if err := mghelper.CreateSchema(tx, &dao.ReconciliationStateDao{}); err != nil {
					return err
				}
				// Add singleton constraint to ensure only one row with id=1
				_, err := tx.Exec("ALTER TABLE reconciliation_state ADD CONSTRAINT singleton_check CHECK (id = 1)")
				if err != nil {
					return err
				}
				// Insert initial state row with ON CONFLICT for idempotency
				_, err = tx.Model(&dao.ReconciliationStateDao{
					ID:                  1,
					LastProcessedOffset: 0,
					EventsProcessed:     0,
				}).
					OnConflict("(id) DO NOTHING").
					Insert()
				if err != nil {
					return err
				}
				return nil
			},
			DownTx: true,
			Down: func(db migrations.DB) error {
				log.Println("dropping reconciliation_state table...")
				return mghelper.DropTables(db.(*pg.Tx), &dao.ReconciliationStateDao{})
			},
		},
	}
}
