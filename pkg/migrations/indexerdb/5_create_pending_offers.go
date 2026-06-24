// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"
	"time"

	"github.com/uptrace/bun"

	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

// legacyOffer is the bun model for the legacy indexer_pending_offers table.
//
// The store's PendingOfferDao was removed when the table was generalized into
// indexer_transfers (migration 7), so this struct is kept here for backward
// compatibility: it lets the historical migrations (5 = create, 7 = read +
// drop) keep managing the old table with the bun query builder instead of raw
// SQL. Migration-only; intentionally not exported from the store package.
//
// ExpiresAt is part of the model from the start so migration 7 can read it; on a
// fresh database the column is created here.
type legacyOffer struct {
	bun.BaseModel   `bun:"table:indexer_pending_offers"`
	ContractID      string     `bun:"contract_id,pk,type:varchar(255)"`
	Status          string     `bun:"status,notnull,type:varchar(20),default:'PENDING'"`
	ReceiverPartyID string     `bun:"receiver_party_id,notnull,type:varchar(255)"`
	SenderPartyID   string     `bun:"sender_party_id,notnull,type:varchar(255)"`
	InstrumentAdmin string     `bun:"instrument_admin,notnull,type:varchar(255)"`
	InstrumentID    string     `bun:"instrument_id,notnull,type:varchar(255)"`
	Amount          string     `bun:"amount,notnull,type:text"`
	LedgerOffset    int64      `bun:"ledger_offset,notnull"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	ExpiresAt       *time.Time `bun:"expires_at,nullzero"`
}

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating indexer_pending_offers table...")
		if err := mghelper.CreateSchema(ctx, db, &legacyOffer{}); err != nil {
			return err
		}
		return mghelper.CreateModelIndexes(ctx, db, &legacyOffer{}, "receiver_party_id", "status")
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("dropping indexer_pending_offers table...")
		return mghelper.DropTables(ctx, db, &legacyOffer{})
	})
}
