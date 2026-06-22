// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"log"
	"sort"

	"github.com/uptrace/bun"

	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

// Transfer kind/status literals, kept local so this historical migration stays
// stable regardless of later changes to the indexer package's constants.
const (
	transferKindOffer    = "offer"
	transferKindDirect   = "direct"
	transferStatusPend   = "pending"
	transferStatusDone   = "completed"
	legacyStatusAccepted = "ACCEPTED"
	legacyStatusPending  = "PENDING"
)

// Migration 7 introduces indexer_transfers — a single generalized table for both
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
			status := transferStatusPend
			if o.Status == legacyStatusAccepted || o.Status == transferStatusDone {
				status = transferStatusDone
			}
			transfers = append(transfers, indexerstore.TransferDao{
				ContractID:      o.ContractID,
				Kind:            transferKindOffer,
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
				Kind:            transferKindDirect,
				Status:          transferStatusDone,
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
		return mghelper.DropTables(ctx, db, &legacyOffer{})
	}, func(ctx context.Context, db *bun.DB) error {
		log.Println("reverting indexer_transfers -> indexer_pending_offers...")
		// Best-effort reverse: recreate the offers table, copy offer-kind rows back
		// (restoring the original status casing/column names), drop the new table.
		if err := mghelper.CreateSchema(ctx, db, &legacyOffer{}); err != nil {
			return err
		}

		var transfers []indexerstore.TransferDao
		if err := db.NewSelect().Model(&transfers).
			Where("kind = ?", transferKindOffer).
			Scan(ctx); err != nil {
			return err
		}

		offers := make([]legacyOffer, 0, len(transfers))
		for i := range transfers {
			t := &transfers[i]
			status := legacyStatusPending
			if t.Status == transferStatusDone {
				status = legacyStatusAccepted
			}
			offers = append(offers, legacyOffer{
				ContractID:      t.ContractID,
				Status:          status,
				ReceiverPartyID: t.ToPartyID,
				SenderPartyID:   t.FromPartyID,
				InstrumentAdmin: t.InstrumentAdmin,
				InstrumentID:    t.InstrumentID,
				Amount:          t.Amount,
				LedgerOffset:    t.LedgerOffset,
				CreatedAt:       t.CreatedAt,
				ExpiresAt:       t.ExpiresAt,
			})
		}
		if len(offers) > 0 {
			if _, err := db.NewInsert().Model(&offers).
				On("CONFLICT (contract_id) DO NOTHING").
				Exec(ctx); err != nil {
				return err
			}
		}

		return mghelper.DropTables(ctx, db, &indexerstore.TransferDao{})
	})
}

func deref(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
