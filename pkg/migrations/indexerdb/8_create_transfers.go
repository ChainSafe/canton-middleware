// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

// execAll runs the given SQL statements in order, stopping at the first error.
func execAll(ctx context.Context, db *bun.DB, stmts ...string) error {
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// Migration 8 generalizes indexer_pending_offers into indexer_transfers: a single
// table holding both direct (atomic CIP-56) and offer-based (2-step) transfers with
// a mutable status lifecycle. Existing offer rows are renamed in place, their status
// casing is normalized to lowercase, and historical direct transfers are backfilled
// from the append-only indexer_events log.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("renaming indexer_pending_offers -> indexer_transfers and backfilling direct transfers...")
		return execAll(ctx, db,
			// 1. Rename the table (carries expires_at + indexes from migration 7).
			`ALTER TABLE indexer_pending_offers RENAME TO indexer_transfers`,
			// 1b. Rename the party columns to the generalized vocabulary. An offer's
			//     sender is the transfer's from-party; its receiver is the to-party.
			`ALTER TABLE indexer_transfers RENAME COLUMN sender_party_id TO from_party_id`,
			`ALTER TABLE indexer_transfers RENAME COLUMN receiver_party_id TO to_party_id`,
			// 2. Add the new columns. All existing rows are offer-based.
			`ALTER TABLE indexer_transfers ADD COLUMN IF NOT EXISTS kind VARCHAR(20) NOT NULL DEFAULT 'offer'`,
			`ALTER TABLE indexer_transfers ADD COLUMN IF NOT EXISTS tx_id VARCHAR(255)`,
			// 3. Normalize legacy status casing to the new lowercase vocabulary.
			`UPDATE indexer_transfers SET status = 'completed' WHERE status = 'ACCEPTED'`,
			`UPDATE indexer_transfers SET status = 'pending' WHERE status = 'PENDING'`,
			// 4. Backfill direct transfers from the event log. Only TRANSFER events
			//    with both parties become direct transfer rows (MINT/BURN excluded).
			`INSERT INTO indexer_transfers (
				contract_id, kind, status, from_party_id, to_party_id,
				instrument_admin, instrument_id, amount, expires_at, tx_id,
				ledger_offset, created_at
			)
			SELECT contract_id, 'direct', 'completed', from_party_id, to_party_id,
				instrument_admin, instrument_id, amount, NULL, tx_id,
				ledger_offset, effective_time
			FROM indexer_events
			WHERE event_type = 'TRANSFER'
				AND from_party_id IS NOT NULL
				AND to_party_id IS NOT NULL
			ON CONFLICT (contract_id) DO NOTHING`,
			// 5. Index status for the lifecycle filters (pending/completed). The
			//    from_party_id (was sender_party_id) and to_party_id (was
			//    receiver_party_id) indexes from migrations 7/5 carry over via the
			//    column renames.
			`CREATE INDEX IF NOT EXISTS idx_indexer_transfers_status ON indexer_transfers (status)`,
		)
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("reverting indexer_transfers -> indexer_pending_offers...")
		// Best-effort reverse: drop backfilled direct rows, drop the new columns
		// and index, rename back, and restore the original status casing.
		return execAll(ctx, db,
			`DROP INDEX IF EXISTS idx_indexer_transfers_status`,
			`DELETE FROM indexer_transfers WHERE kind = 'direct'`,
			`ALTER TABLE indexer_transfers DROP COLUMN IF EXISTS kind`,
			`ALTER TABLE indexer_transfers DROP COLUMN IF EXISTS tx_id`,
			`ALTER TABLE indexer_transfers RENAME COLUMN from_party_id TO sender_party_id`,
			`ALTER TABLE indexer_transfers RENAME COLUMN to_party_id TO receiver_party_id`,
			`ALTER TABLE indexer_transfers RENAME TO indexer_pending_offers`,
			`UPDATE indexer_pending_offers SET status = 'ACCEPTED' WHERE status = 'completed'`,
			`UPDATE indexer_pending_offers SET status = 'PENDING' WHERE status = 'pending'`,
		)
	})
}
