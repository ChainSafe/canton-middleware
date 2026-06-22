// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding expires_at column + sender_party_id index to indexer_pending_offers...")
		// Adds expires_at to databases deployed before it was part of the table.
		// Nullable, no backfill: legacy rows keep NULL and are treated as
		// never-expiring; new rows carry the offer's executeBefore. On a fresh
		// database migration 5 already created the column, so this is a no-op.
		if _, err := db.NewAddColumn().
			Model(&legacyOffer{}).
			ColumnExpr("expires_at TIMESTAMPTZ").
			IfNotExists().
			Exec(ctx); err != nil {
			return err
		}
		// Index sender_party_id to back outgoing-offer queries (the indexes from
		// migration 5 cover receiver_party_id + status only).
		return mghelper.CreateModelIndexes(ctx, db, &legacyOffer{}, "sender_party_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping expires_at column from indexer_pending_offers...")
		_, err := db.NewDropColumn().
			Model(&legacyOffer{}).
			Column("expires_at").
			Exec(ctx)
		return err
	})
}
