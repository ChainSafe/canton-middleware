// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"

	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding expires_at column + sender_party_id index to indexer_pending_offers...")
		// Nullable, no backfill: legacy rows keep NULL and are treated as
		// never-expiring. New rows carry the offer's executeBefore.
		if _, err := db.NewAddColumn().
			Model(&indexerstore.PendingOfferDao{}).
			ColumnExpr("expires_at TIMESTAMPTZ").
			IfNotExists().
			Exec(ctx); err != nil {
			return err
		}
		// Index sender_party_id to back outgoing-offer queries (the existing
		// index covers receiver_party_id + status only).
		return mghelper.CreateModelIndexes(ctx, db, &indexerstore.PendingOfferDao{}, "sender_party_id")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping expires_at column from indexer_pending_offers...")
		_, err := db.NewDropColumn().
			Model(&indexerstore.PendingOfferDao{}).
			Column("expires_at").
			Exec(ctx)
		return err
	})
}
