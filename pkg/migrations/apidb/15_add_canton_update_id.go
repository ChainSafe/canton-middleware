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
		log.Println("adding canton_update_id column to mempool and evm_transactions...")
		for _, model := range []any{&ethrpcstore.MempoolEntryDao{}, &ethrpcstore.EvmTransactionDao{}} {
			if _, err := db.NewAddColumn().
				Model(model).
				ColumnExpr("canton_update_id TEXT").
				IfNotExists().
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping canton_update_id column from mempool and evm_transactions...")
		for _, model := range []any{&ethrpcstore.MempoolEntryDao{}, &ethrpcstore.EvmTransactionDao{}} {
			if _, err := db.NewDropColumn().
				Model(model).
				Column("canton_update_id").
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
