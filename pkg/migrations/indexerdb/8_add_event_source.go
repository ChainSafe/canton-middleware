// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"

	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("adding source column to indexer_events...")
		// Discriminates how a transfer event was produced: 'cip56' = our atomic
		// TokenTransferEvent (drives balances), 'offer' = a settled external
		// TransferOffer recorded for history only. Existing rows are CIP-56.
		_, err := db.NewAddColumn().
			Model(&indexerstore.EventDao{}).
			ColumnExpr("source VARCHAR(20) NOT NULL DEFAULT 'cip56'").
			IfNotExists().
			Exec(ctx)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping source column from indexer_events...")
		_, err := db.NewDropColumn().
			Model(&indexerstore.EventDao{}).
			Column("source").
			Exec(ctx)
		return err
	})
}
