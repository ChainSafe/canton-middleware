// SPDX-License-Identifier: Apache-2.0

package apidb

import (
	"context"
	"log"

	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"

	"github.com/uptrace/bun"
)

// Sealed blocks get a real timestamp, and the synthetic head is anchored in the
// database instead of the API server's uptime. The anchor starts at the higher of
// latest + 12 and the seconds since the first block, close to the uptime-based
// head served before the upgrade.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating evm_blocks and anchoring the synthetic chain head...")
		if err := mghelper.CreateSchema(ctx, db, &ethrpcstore.EvmBlockDao{}); err != nil {
			return err
		}
		for _, col := range []string{"anchor_block BIGINT NOT NULL DEFAULT 0", "anchor_time BIGINT NOT NULL DEFAULT 0"} {
			if _, err := db.NewAddColumn().Model(&ethrpcstore.EvmStateDao{}).ColumnExpr(col).IfNotExists().Exec(ctx); err != nil {
				return err
			}
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO evm_blocks (number, timestamp)
			SELECT t.block_number,
			       COALESCE(NULLIF(MAX(l.block_timestamp), 0), EXTRACT(EPOCH FROM MIN(t.created_at))::bigint)
			FROM evm_transactions t
			LEFT JOIN evm_logs l ON l.tx_hash = t.tx_hash
			GROUP BY t.block_number
			ON CONFLICT (number) DO NOTHING`); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx, `
			UPDATE evm_state SET
			  anchor_time = EXTRACT(EPOCH FROM now())::bigint,
			  anchor_block = GREATEST(
			    latest_block + 12,
			    COALESCE(EXTRACT(EPOCH FROM now())::bigint - (SELECT MIN(timestamp) FROM evm_blocks), 0))
			WHERE id = 1 AND anchor_time = 0`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping evm_blocks and the chain head anchor...")
		if err := mghelper.DropTables(ctx, db, &ethrpcstore.EvmBlockDao{}); err != nil {
			return err
		}
		for _, col := range []string{"anchor_block", "anchor_time"} {
			if _, err := db.NewDropColumn().Model(&ethrpcstore.EvmStateDao{}).Column(col).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
