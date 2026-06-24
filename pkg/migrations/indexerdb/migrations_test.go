// SPDX-License-Identifier: Apache-2.0

package indexerdb

import (
	"context"
	"testing"
	"time"

	"github.com/uptrace/bun/migrate"

	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func strptr(s string) *string { return &s }

// TestIndexerDBMigrations_Apply runs every indexerdb migration against a fresh
// database and confirms the end state: indexer_transfers exists (migration 7) and
// the retired indexer_pending_offers table is gone. This is the empty-database
// path — no rows to backfill.
func TestIndexerDBMigrations_Apply(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}
	if group.IsZero() {
		t.Fatal("expected migrations to run, but none were applied")
	}

	if _, err := db.NewSelect().Model(&indexerstore.TransferDao{}).Count(ctx); err != nil {
		t.Fatalf("indexer_transfers not queryable after migrate: %v", err)
	}
	if _, err := db.NewSelect().Model(&legacyOffer{}).Count(ctx); err == nil {
		t.Fatal("expected indexer_pending_offers to be dropped, but it still exists")
	}
}

// TestMigration7_BackfillsOffersAndEvents verifies the upgrade path of migration
// 7: an existing database (indexer_pending_offers + indexer_events holding data)
// is migrated into the generalized indexer_transfers table. It checks that offer
// statuses are mapped, settled direct CIP-56 transfers are carried over from the
// event log, non-transfer events (MINT/BURN) are excluded, ordering is by
// ledger_offset, and the retired offers table is dropped.
//
// The seed rows are inserted BEFORE running the migrator: every migration's
// CreateSchema is IF NOT EXISTS, so the steps that (re)declare these source
// tables are no-ops and the seeded rows survive until migration 7 backfills them.
func TestMigration7_BackfillsOffersAndEvents(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Stand up the two source tables migration 7 reads, then seed them.
	if err := mghelper.CreateSchema(ctx, db, &indexerstore.EventDao{}, &legacyOffer{}); err != nil {
		t.Fatalf("create source schema: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(time.Hour)

	offers := []legacyOffer{
		{
			ContractID: "offer-pending", Status: legacyStatusPending,
			SenderPartyID: "alice::p", ReceiverPartyID: "bob::p",
			InstrumentAdmin: "usdc::admin", InstrumentID: "USDCx", Amount: "10",
			LedgerOffset: 100, CreatedAt: now, ExpiresAt: &expires,
		},
		{
			ContractID: "offer-accepted", Status: legacyStatusAccepted,
			SenderPartyID: "alice::p", ReceiverPartyID: "carol::p",
			InstrumentAdmin: "usdc::admin", InstrumentID: "USDCx", Amount: "5",
			LedgerOffset: 90, CreatedAt: now,
		},
	}
	if _, err := db.NewInsert().Model(&offers).Exec(ctx); err != nil {
		t.Fatalf("seed offers: %v", err)
	}

	events := []indexerstore.EventDao{
		{
			ContractID: "evt-transfer", InstrumentID: "DEMO", InstrumentAdmin: "demo::admin", Issuer: "demo::admin",
			EventType: "TRANSFER", Amount: "7", FromPartyID: strptr("dave::p"), ToPartyID: strptr("erin::p"),
			TxID: "tx-1", LedgerOffset: 110, Timestamp: now, EffectiveTime: now,
		},
		// MINT/BURN are not party-to-party transfers and must be excluded.
		{
			ContractID: "evt-mint", InstrumentID: "DEMO", InstrumentAdmin: "demo::admin", Issuer: "demo::admin",
			EventType: "MINT", Amount: "100", ToPartyID: strptr("dave::p"),
			TxID: "tx-2", LedgerOffset: 80, Timestamp: now, EffectiveTime: now,
		},
		{
			ContractID: "evt-burn", InstrumentID: "DEMO", InstrumentAdmin: "demo::admin", Issuer: "demo::admin",
			EventType: "BURN", Amount: "3", FromPartyID: strptr("dave::p"),
			TxID: "tx-3", LedgerOffset: 85, Timestamp: now, EffectiveTime: now,
		},
	}
	if _, err := db.NewInsert().Model(&events).Exec(ctx); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	// Run all migrations; migration 7 backfills indexer_transfers and drops the
	// offers table.
	migrator := migrate.NewMigrator(db, Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	var transfers []indexerstore.TransferDao
	if err := db.NewSelect().Model(&transfers).Order("ledger_offset ASC").Scan(ctx); err != nil {
		t.Fatalf("scan transfers: %v", err)
	}
	if len(transfers) != 3 {
		t.Fatalf("expected 3 backfilled transfers (2 offers + 1 transfer event), got %d: %+v", len(transfers), transfers)
	}

	// Ordered by ledger_offset: offer-accepted(90), offer-pending(100), evt-transfer(110).
	if transfers[0].ContractID != "offer-accepted" ||
		transfers[1].ContractID != "offer-pending" ||
		transfers[2].ContractID != "evt-transfer" {
		t.Fatalf("unexpected ledger_offset ordering: %s, %s, %s",
			transfers[0].ContractID, transfers[1].ContractID, transfers[2].ContractID)
	}

	byID := make(map[string]indexerstore.TransferDao, len(transfers))
	for i := range transfers {
		byID[transfers[i].ContractID] = transfers[i]
	}

	// Pending offer → kind=offer, status=pending, expires_at preserved.
	if got := byID["offer-pending"]; got.Kind != transferKindOffer || got.Status != transferStatusPend {
		t.Fatalf("offer-pending: expected offer/pending, got %s/%s", got.Kind, got.Status)
	}
	if byID["offer-pending"].ExpiresAt == nil {
		t.Fatal("offer-pending: expected expires_at to be preserved")
	}
	// Accepted (legacy) offer → status mapped to completed.
	if got := byID["offer-accepted"]; got.Kind != transferKindOffer || got.Status != transferStatusDone {
		t.Fatalf("offer-accepted: expected offer/completed, got %s/%s", got.Kind, got.Status)
	}
	// TRANSFER event → kind=direct, status=completed, parties dereferenced, tx id carried.
	if got := byID["evt-transfer"]; got.Kind != transferKindDirect || got.Status != transferStatusDone ||
		got.FromPartyID != "dave::p" || got.ToPartyID != "erin::p" || got.TxID != "tx-1" {
		t.Fatalf("evt-transfer: unexpected backfill: %+v", got)
	}
	// MINT / BURN excluded.
	if _, ok := byID["evt-mint"]; ok {
		t.Fatal("evt-mint (MINT) should not be backfilled into indexer_transfers")
	}
	if _, ok := byID["evt-burn"]; ok {
		t.Fatal("evt-burn (BURN) should not be backfilled into indexer_transfers")
	}

	// The retired offers table should be gone.
	if _, err := db.NewSelect().Model(&legacyOffer{}).Count(ctx); err == nil {
		t.Fatal("expected indexer_pending_offers to be dropped, but it still exists")
	}
}
