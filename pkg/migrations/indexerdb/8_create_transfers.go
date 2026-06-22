// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/uptrace/bun"

	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

// legacyOffer mirrors the pre-migration indexer_pending_offers columns so we can
// read existing rows after PendingOfferDao was removed from the store package.
type legacyOffer struct {
	bun.BaseModel   `bun:"table:indexer_pending_offers"`
	ContractID      string     `bun:"contract_id"`
	Status          string     `bun:"status"`
	SenderPartyID   string     `bun:"sender_party_id"`
	ReceiverPartyID string     `bun:"receiver_party_id"`
	InstrumentAdmin string     `bun:"instrument_admin"`
	InstrumentID    string     `bun:"instrument_id"`
	Amount          string     `bun:"amount"`
	LedgerOffset    int64      `bun:"ledger_offset"`
	CreatedAt       time.Time  `bun:"created_at"`
	ExpiresAt       *time.Time `bun:"expires_at"`
}

// Migration 8 introduces indexer_transfers — a single generalized table for both
// direct (atomic CIP-56) and offer-based (2-step, e.g. USDCx) transfers — and
// retires indexer_pending_offers.
//
// To preserve history it reads every existing transfer from both source tables
// (offers from indexer_pending_offers, settled direct transfers from the
// append-only indexer_events log), forms one chronologically-sorted list, inserts
// it into the new table, then drops the old offers table. The two sources are
// disjoint (our tokens emit events and never create offers; external tokens use
// offers and never emit our events), so nothing is double-counted.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		log.Println("creating indexer_transfers and migrating history from offers + events...")

		// 1. Create the new table and its indexes.
		if err := mghelper.CreateSchema(ctx, db, &indexerstore.TransferDao{}); err != nil {
			return err
		}
		if err := mghelper.CreateModelIndexes(
			ctx, db, &indexerstore.TransferDao{}, "from_party_id", "to_party_id", "status",
		); err != nil {
			return err
		}

		// 2. Fetch all existing offers.
		var offers []legacyOffer
		if err := db.NewSelect().Model(&offers).Scan(ctx); err != nil {
			return err
		}

		// 3. Fetch all settled direct transfers from the event log (TRANSFER only;
		//    MINT/BURN are not transfers between two parties).
		var events []indexerstore.EventDao
		if err := db.NewSelect().Model(&events).
			Where("event_type = ?", "TRANSFER").
			Where("from_party_id IS NOT NULL AND to_party_id IS NOT NULL").
			Scan(ctx); err != nil {
			return err
		}

		// 4. Form the unified transfer history.
		transfers := make([]indexerstore.TransferDao, 0, len(offers)+len(events))
		for i := range offers {
			o := &offers[i]
			status := "pending"
			if o.Status == "ACCEPTED" || o.Status == "completed" {
				status = "completed"
			}
			transfers = append(transfers, indexerstore.TransferDao{
				ContractID:      o.ContractID,
				Kind:            "offer",
				Status:          status,
				FromPartyID:     o.SenderPartyID,
				ToPartyID:       o.ReceiverPartyID,
				InstrumentAdmin: o.InstrumentAdmin,
				InstrumentID:    o.InstrumentID,
				Amount:          o.Amount,
				ExpiresAt:       o.ExpiresAt,
				LedgerOffset:    o.LedgerOffset,
				CreatedAt:       o.CreatedAt,
			})
		}
		for i := range events {
			e := &events[i]
			transfers = append(transfers, indexerstore.TransferDao{
				ContractID:      e.ContractID,
				Kind:            "direct",
				Status:          "completed",
				FromPartyID:     deref(e.FromPartyID),
				ToPartyID:       deref(e.ToPartyID),
				InstrumentAdmin: e.InstrumentAdmin,
				InstrumentID:    e.InstrumentID,
				Amount:          e.Amount,
				TxID:            e.TxID,
				LedgerOffset:    e.LedgerOffset,
				CreatedAt:       e.EffectiveTime,
			})
		}

		// 5. Sort chronologically (oldest first).
		sort.Slice(transfers, func(i, j int) bool {
			return transfers[i].CreatedAt.Before(transfers[j].CreatedAt)
		})

		// 6. Insert the history into the new table.
		if len(transfers) > 0 {
			if _, err := db.NewInsert().Model(&transfers).
				On("CONFLICT (contract_id) DO NOTHING").
				Exec(ctx); err != nil {
				return err
			}
		}

		// 7. Drop the retired offers table.
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS indexer_pending_offers`)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("reverting indexer_transfers -> indexer_pending_offers...")
		// Best-effort reverse: recreate the offers table, copy offer-kind rows back
		// (restoring the original status casing/column names), drop the new table.
		return execAll(ctx, db,
			`CREATE TABLE IF NOT EXISTS indexer_pending_offers (
				contract_id varchar(255) PRIMARY KEY,
				status varchar(20) NOT NULL,
				receiver_party_id varchar(255) NOT NULL,
				sender_party_id varchar(255) NOT NULL,
				instrument_admin varchar(255) NOT NULL,
				instrument_id varchar(255) NOT NULL,
				amount text NOT NULL,
				ledger_offset bigint NOT NULL,
				created_at timestamptz NOT NULL,
				expires_at timestamptz
			)`,
			`INSERT INTO indexer_pending_offers
				(contract_id, status, receiver_party_id, sender_party_id, instrument_admin,
				 instrument_id, amount, ledger_offset, created_at, expires_at)
			SELECT contract_id,
				CASE WHEN status = 'completed' THEN 'ACCEPTED' ELSE 'PENDING' END,
				to_party_id, from_party_id, instrument_admin, instrument_id, amount,
				ledger_offset, created_at, expires_at
			FROM indexer_transfers WHERE kind = 'offer'
			ON CONFLICT (contract_id) DO NOTHING`,
			`DROP TABLE IF EXISTS indexer_transfers`,
		)
	})
}

// execAll runs the given SQL statements in order, stopping at the first error.
func execAll(ctx context.Context, db *bun.DB, stmts ...string) error {
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func deref(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
