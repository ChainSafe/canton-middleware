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
// reason. The struct tag is removed in code; this migration drops the
// lingering DEFAULT 1 on existing databases (fresh DBs created after the tag
// removal never had it). With no default, bun now inserts the literal 0/1, so
// failures persist correctly going forward.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping DEFAULT on evm_transactions.status...")
		_, err := db.ExecContext(ctx,
			`ALTER TABLE evm_transactions ALTER COLUMN status DROP DEFAULT`,
		)
		return err
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
