// SPDX-License-Identifier: Apache-2.0

package apidb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

// Migration 14 fixes the evm_transactions.status corruption caused by the
// `default:1` bun tag on the status column. Because a failed transaction's
// status (0) is the column's zero value and the column carried a DEFAULT 1,
// bun emitted SQL DEFAULT on insert and Postgres stored 1 — flipping every
// failed receipt to status=0x1 (success) while it still carried its revert
// reason. The struct tag is removed in code; this migration:
//
//  1. Drops the lingering DEFAULT 1 on existing databases (fresh DBs created
//     after the tag removal never had it). With no default, bun now inserts
//     the literal 0/1, so failures persist correctly going forward.
//  2. Backfills already-mined rows. A successful transaction never carries an
//     error_message (the miner only sets it on the failure branch), so a row
//     with status=1 AND a non-empty error_message is unambiguously a failure
//     that was clobbered — safe to flip back to 0.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping DEFAULT on evm_transactions.status...")
		if _, err := db.ExecContext(ctx,
			`ALTER TABLE evm_transactions ALTER COLUMN status DROP DEFAULT`,
		); err != nil {
			return err
		}

		log.Println("backfilling clobbered failed-transaction statuses (status=1 with an error_message -> 0)...")
		res, err := db.ExecContext(ctx,
			`UPDATE evm_transactions
			    SET status = 0
			  WHERE status = 1
			    AND error_message IS NOT NULL
			    AND error_message <> ''`,
		)
		if err != nil {
			return err
		}
		if n, aerr := res.RowsAffected(); aerr == nil {
			log.Printf("repaired %d corrupted failed-transaction receipts", n)
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		// The data repair is intentionally irreversible — the original (wrong)
		// values are indistinguishable from correct ones, so a down migration
		// cannot restore them. Only the schema-level default is reinstated to
		// mirror the prior column definition.
		log.Println("restoring DEFAULT 1 on evm_transactions.status...")
		_, err := db.ExecContext(ctx,
			`ALTER TABLE evm_transactions ALTER COLUMN status SET DEFAULT 1`,
		)
		return err
	})
}
