// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding expires_at column + sender_party_id index to indexer_pending_offers...")
		// Raw SQL with literal names: PendingOfferDao was removed when the table
		// was generalized into indexer_transfers (migration 8), and this migration
		// runs against indexer_pending_offers before that rename.
		//
		// Nullable, no backfill: legacy rows keep NULL and are treated as
		// never-expiring. New rows carry the offer's executeBefore.
		if _, err := db.ExecContext(ctx,
			`ALTER TABLE indexer_pending_offers ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`); err != nil {
			return err
		}
		// Index sender_party_id to back outgoing-offer queries (the existing
		// index covers receiver_party_id + status only).
		_, err := db.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS idx_indexer_pending_offers_sender_party_id ON indexer_pending_offers (sender_party_id)`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping expires_at column from indexer_pending_offers...")
		_, err := db.ExecContext(ctx,
			`ALTER TABLE indexer_pending_offers DROP COLUMN IF EXISTS expires_at`)
		return err
	})
}
