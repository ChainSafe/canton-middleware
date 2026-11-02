// SPDX-License-Identifier: Apache-2.0

package apidb

import (
	"context"
	"log"

	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding canton_update_id column to mempool...")
		_, err := db.NewAddColumn().
			Model(&ethrpcstore.MempoolEntryDao{}).
			ColumnExpr("canton_update_id TEXT").
			IfNotExists().
			Exec(ctx)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping canton_update_id column from mempool...")
		_, err := db.NewDropColumn().
			Model(&ethrpcstore.MempoolEntryDao{}).
			Column("canton_update_id").
			Exec(ctx)
		return err
	})
}
