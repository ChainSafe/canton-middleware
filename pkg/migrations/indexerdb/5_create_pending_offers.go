// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating indexer_pending_offers table...")
		// Raw SQL with literal table/column names: the Go DAO that once modeled
		// this table (PendingOfferDao) was removed when the table was generalized
		// into indexer_transfers (migration 8). Pinning the DDL here keeps this
		// historical migration stable regardless of later model changes.
		if _, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS indexer_pending_offers (
				contract_id       VARCHAR(255) PRIMARY KEY,
				status            VARCHAR(20)  NOT NULL DEFAULT 'PENDING',
				receiver_party_id VARCHAR(255) NOT NULL,
				sender_party_id   VARCHAR(255) NOT NULL,
				instrument_admin  VARCHAR(255) NOT NULL,
				instrument_id     VARCHAR(255) NOT NULL,
				amount            TEXT         NOT NULL,
				ledger_offset     BIGINT       NOT NULL,
				created_at        TIMESTAMPTZ  NOT NULL
			)`); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS idx_indexer_pending_offers_receiver_party_id ON indexer_pending_offers (receiver_party_id)`); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS idx_indexer_pending_offers_status ON indexer_pending_offers (status)`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_pending_offers table...")
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS indexer_pending_offers`)
		return err
	})
}
