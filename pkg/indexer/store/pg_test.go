// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func setupIndexerStore(t *testing.T) (context.Context, *PGStore) {
	t.Helper()
	requireDockerAccess(t)

	ctx := context.Background()
	db, cleanup := pgutil.SetupTestDB(t)
	t.Cleanup(cleanup)

	if err := mghelper.CreateSchema(ctx, db, &EventDao{}, &TokenDao{}, &BalanceDao{}, &OffsetDao{}, &TransferDao{}); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	// Mirror migration 7: a status index plus composite (party, created_at)
	// indexes that back the list-by-party-newest-first queries.
	if err := mghelper.CreateModelIndexes(ctx, db, &TransferDao{}, "status"); err != nil {
		t.Fatalf("failed to create transfer status index: %v", err)
	}
	for _, col := range []string{"from_party_id", "to_party_id"} {
		if _, err := db.NewCreateIndex().
			Model(&TransferDao{}).
			Index("idx_indexer_transfers_"+col+"_created_at").
			Column(col, "created_at").
			IfNotExists().
			Exec(ctx); err != nil {
			t.Fatalf("failed to create transfer composite index: %v", err)
		}
	}
	// Mirror the indexes created by the migration files so the test schema matches
	// production. This catches index-related issues (e.g. constraint violations) and
	// ensures query plans exercised in tests reflect real query plans.
	if err := mghelper.CreateModelIndexes(ctx, db, &EventDao{}, "ledger_offset", "from_party_id", "to_party_id"); err != nil {
		t.Fatalf("failed to create event indexes: %v", err)
	}
	if err := mghelper.CreateModelIndexes(ctx, db, &BalanceDao{}, "party_id"); err != nil {
		t.Fatalf("failed to create balance indexes: %v", err)
	}

	return ctx, NewStore(db)
}

func requireDockerAccess(t *testing.T) {
	t.Helper()

	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("HOME"), ".docker/run/docker.sock"),
	}

	for _, sock := range candidates {
		if sock == "" {
			continue
		}
		if _, err := os.Stat(sock); err != nil {
			continue
		}
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", sock)
		if err == nil {
			_ = conn.Close()
			return
		}
	}

	t.Skip("docker daemon socket is not accessible; skipping testcontainer-backed indexer store tests")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func ptr[T any](v T) *T { return &v }

func makeEvent(contractID string, offset int64, eventType indexer.EventType, from, to *string) *indexer.ParsedEvent {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &indexer.ParsedEvent{
		ContractID:      contractID,
		InstrumentID:    "DEMO",
		InstrumentAdmin: "admin-1",
		Issuer:          "issuer-1",
		EventType:       eventType,
		Amount:          "100",
		FromPartyID:     from,
		ToPartyID:       to,
		TxID:            "tx-" + contractID,
		LedgerOffset:    offset,
		Timestamp:       now,
		EffectiveTime:   now,
	}
}

func makeToken(admin, id string, offset int64) *indexer.Token {
	return &indexer.Token{
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Issuer:          "issuer-1",
		FirstSeenOffset: offset,
		FirstSeenAt:     time.Now().UTC().Truncate(time.Millisecond),
	}
}

// ── LatestOffset ─────────────────────────────────────────────────────────────

func TestPGStore_LatestOffset(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// fresh DB — no row yet
	off, err := s.LatestOffset(ctx)
	if err != nil {
		t.Fatalf("LatestOffset(fresh) failed: %v", err)
	}
	if off != 0 {
		t.Fatalf("LatestOffset(fresh) expected 0, got %d", off)
	}

	// after SaveOffset
	if err = s.SaveOffset(ctx, 42); err != nil {
		t.Fatalf("SaveOffset(42) failed: %v", err)
	}
	off, err = s.LatestOffset(ctx)
	if err != nil {
		t.Fatalf("LatestOffset(after save) failed: %v", err)
	}
	if off != 42 {
		t.Fatalf("LatestOffset(after save) expected 42, got %d", off)
	}

	// upsert — should update
	if err = s.SaveOffset(ctx, 99); err != nil {
		t.Fatalf("SaveOffset(99) failed: %v", err)
	}
	off, err = s.LatestOffset(ctx)
	if err != nil {
		t.Fatalf("LatestOffset(after update) failed: %v", err)
	}
	if off != 99 {
		t.Fatalf("LatestOffset(after update) expected 99, got %d", off)
	}
}

// ── RunInTx ──────────────────────────────────────────────────────────────────

func TestPGStore_RunInTx(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// success path: changes committed
	err := s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.SaveOffset(ctx, 77)
	})
	if err != nil {
		t.Fatalf("RunInTx(success) failed: %v", err)
	}
	off, err := s.LatestOffset(ctx)
	if err != nil {
		t.Fatalf("LatestOffset after committed tx failed: %v", err)
	}
	if off != 77 {
		t.Fatalf("expected offset 77 after commit, got %d", off)
	}
}

func TestPGStore_RunInTx_Rollback(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// seed a known offset
	if err := s.SaveOffset(ctx, 5); err != nil {
		t.Fatalf("SaveOffset seed failed: %v", err)
	}

	// error path: changes rolled back
	errSentinel := context.DeadlineExceeded
	_ = s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		if err := tx.SaveOffset(ctx, 999); err != nil {
			return err
		}
		return errSentinel
	})

	off, err := s.LatestOffset(ctx)
	if err != nil {
		t.Fatalf("LatestOffset after rolled-back tx failed: %v", err)
	}
	if off != 5 {
		t.Fatalf("expected offset 5 after rollback, got %d", off)
	}
}

// ── InsertEvent ───────────────────────────────────────────────────────────────

func TestPGStore_InsertEvent(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	ev := makeEvent("contract-1", 10, indexer.EventTransfer, ptr("alice"), ptr("bob"))

	// first insert
	inserted, err := s.InsertEvent(ctx, ev)
	if err != nil {
		t.Fatalf("InsertEvent(first) failed: %v", err)
	}
	if !inserted {
		t.Fatalf("InsertEvent(first) expected inserted=true")
	}

	// duplicate — idempotent
	inserted, err = s.InsertEvent(ctx, ev)
	if err != nil {
		t.Fatalf("InsertEvent(duplicate) failed: %v", err)
	}
	if inserted {
		t.Fatalf("InsertEvent(duplicate) expected inserted=false")
	}
}

// ── UpsertToken ───────────────────────────────────────────────────────────────

func TestPGStore_UpsertToken(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	tok := makeToken("admin-1", "DEMO", 1)

	// first call creates the row
	if err := s.UpsertToken(ctx, tok); err != nil {
		t.Fatalf("UpsertToken(first) failed: %v", err)
	}

	got, err := s.GetToken(ctx, "admin-1", "DEMO")
	if err != nil {
		t.Fatalf("GetToken after first upsert failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetToken after first upsert returned nil")
	}
	if got.TotalSupply != "0" {
		t.Fatalf("expected TotalSupply=0, got %s", got.TotalSupply)
	}

	// second call with different issuer — should be a no-op
	tok2 := makeToken("admin-1", "DEMO", 99)
	tok2.Issuer = "other-issuer"
	err = s.UpsertToken(ctx, tok2)
	if err != nil {
		t.Fatalf("UpsertToken(second) failed: %v", err)
	}

	got2, err := s.GetToken(ctx, "admin-1", "DEMO")
	if err != nil {
		t.Fatalf("GetToken after second upsert failed: %v", err)
	}
	// first_seen_offset must not have changed
	if got2.FirstSeenOffset != 1 {
		t.Fatalf("expected FirstSeenOffset=1 after no-op upsert, got %d", got2.FirstSeenOffset)
	}
}

// ── ApplySupplyDelta ──────────────────────────────────────────────────────────

func TestPGStore_ApplySupplyDelta(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// seed token
	if err := s.UpsertToken(ctx, makeToken("admin-1", "DEMO", 1)); err != nil {
		t.Fatalf("UpsertToken failed: %v", err)
	}

	// positive delta
	if err := s.ApplySupplyDelta(ctx, "admin-1", "DEMO", "500"); err != nil {
		t.Fatalf("ApplySupplyDelta(+500) failed: %v", err)
	}
	tok, _ := s.GetToken(ctx, "admin-1", "DEMO")
	if tok.TotalSupply != "500" {
		t.Fatalf("expected TotalSupply=500, got %s", tok.TotalSupply)
	}

	// second positive delta
	if err := s.ApplySupplyDelta(ctx, "admin-1", "DEMO", "250"); err != nil {
		t.Fatalf("ApplySupplyDelta(+250) failed: %v", err)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.TotalSupply != "750" {
		t.Fatalf("expected TotalSupply=750, got %s", tok.TotalSupply)
	}

	// negative delta (burn)
	if err := s.ApplySupplyDelta(ctx, "admin-1", "DEMO", "-100"); err != nil {
		t.Fatalf("ApplySupplyDelta(-100) failed: %v", err)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.TotalSupply != "650" {
		t.Fatalf("expected TotalSupply=650, got %s", tok.TotalSupply)
	}
}

// ── ApplyBalanceDelta ─────────────────────────────────────────────────────────

func TestPGStore_ApplyBalanceDelta(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// seed token (HolderCount tracked here)
	if err := s.UpsertToken(ctx, makeToken("admin-1", "DEMO", 1)); err != nil {
		t.Fatalf("UpsertToken failed: %v", err)
	}

	// new balance from zero → positive: HolderCount must increment
	err := s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.ApplyBalanceDelta(ctx, "alice", "admin-1", "DEMO", "300")
	})
	if err != nil {
		t.Fatalf("ApplyBalanceDelta(new +300) failed: %v", err)
	}

	bal, _ := s.GetBalance(ctx, "alice", "admin-1", "DEMO")
	if bal == nil || bal.Amount != "300" {
		t.Fatalf("expected balance=300, got %v", bal)
	}
	tok, _ := s.GetToken(ctx, "admin-1", "DEMO")
	if tok.HolderCount != 1 {
		t.Fatalf("expected HolderCount=1 after first positive balance, got %d", tok.HolderCount)
	}

	// add to existing balance: HolderCount stays
	err = s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.ApplyBalanceDelta(ctx, "alice", "admin-1", "DEMO", "200")
	})
	if err != nil {
		t.Fatalf("ApplyBalanceDelta(+200) failed: %v", err)
	}
	bal, _ = s.GetBalance(ctx, "alice", "admin-1", "DEMO")
	if bal.Amount != "500" {
		t.Fatalf("expected balance=500, got %s", bal.Amount)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.HolderCount != 1 {
		t.Fatalf("expected HolderCount=1 (no change), got %d", tok.HolderCount)
	}

	// underflow is rejected and leaves balance / holder count unchanged
	err = s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.ApplyBalanceDelta(ctx, "alice", "admin-1", "DEMO", "-600")
	})
	if err == nil {
		t.Fatal("ApplyBalanceDelta underflow expected error, got nil")
	}
	bal, _ = s.GetBalance(ctx, "alice", "admin-1", "DEMO")
	if bal.Amount != "500" {
		t.Fatalf("expected balance=500 after rejected underflow, got %s", bal.Amount)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.HolderCount != 1 {
		t.Fatalf("expected HolderCount=1 after rejected underflow, got %d", tok.HolderCount)
	}

	// second holder joins
	err = s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.ApplyBalanceDelta(ctx, "bob", "admin-1", "DEMO", "100")
	})
	if err != nil {
		t.Fatalf("ApplyBalanceDelta(bob +100) failed: %v", err)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.HolderCount != 2 {
		t.Fatalf("expected HolderCount=2 after second holder, got %d", tok.HolderCount)
	}

	// alice burns all: balance → zero, HolderCount decrements
	err = s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.ApplyBalanceDelta(ctx, "alice", "admin-1", "DEMO", "-500")
	})
	if err != nil {
		t.Fatalf("ApplyBalanceDelta(alice -500) failed: %v", err)
	}
	bal, _ = s.GetBalance(ctx, "alice", "admin-1", "DEMO")
	if bal.Amount != "0" {
		t.Fatalf("expected balance=0 after full burn, got %s", bal.Amount)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.HolderCount != 1 {
		t.Fatalf("expected HolderCount=1 after alice burns all, got %d", tok.HolderCount)
	}

	// alice re-enters: balance 0 → positive, HolderCount increments again
	err = s.RunInTx(ctx, func(ctx context.Context, tx engine.Store) error {
		return tx.ApplyBalanceDelta(ctx, "alice", "admin-1", "DEMO", "50")
	})
	if err != nil {
		t.Fatalf("ApplyBalanceDelta(alice re-enter +50) failed: %v", err)
	}
	tok, _ = s.GetToken(ctx, "admin-1", "DEMO")
	if tok.HolderCount != 2 {
		t.Fatalf("expected HolderCount=2 after alice re-enters, got %d", tok.HolderCount)
	}
}

// ── GetToken / ListTokens ─────────────────────────────────────────────────────

func TestPGStore_GetToken(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// not found
	tok, err := s.GetToken(ctx, "admin-1", "DEMO")
	if err != nil {
		t.Fatalf("GetToken(missing) failed: %v", err)
	}
	if tok != nil {
		t.Fatalf("GetToken(missing) expected nil, got %+v", tok)
	}

	// seed and retrieve
	err = s.UpsertToken(ctx, makeToken("admin-1", "DEMO", 5))
	if err != nil {
		t.Fatalf("UpsertToken failed: %v", err)
	}
	tok, err = s.GetToken(ctx, "admin-1", "DEMO")
	if err != nil {
		t.Fatalf("GetToken(found) failed: %v", err)
	}
	if tok == nil {
		t.Fatal("GetToken(found) returned nil")
	}
	if tok.InstrumentAdmin != "admin-1" || tok.InstrumentID != "DEMO" {
		t.Fatalf("GetToken returned wrong token: %+v", tok)
	}
}

func TestPGStore_ListTokens(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// seed 3 tokens with distinct offsets
	tokens := []struct {
		admin, id string
		offset    int64
	}{
		{"admin-1", "AAA", 10},
		{"admin-1", "BBB", 20},
		{"admin-2", "AAA", 30},
	}
	for _, tk := range tokens {
		if err := s.UpsertToken(ctx, makeToken(tk.admin, tk.id, tk.offset)); err != nil {
			t.Fatalf("UpsertToken(%s/%s) failed: %v", tk.admin, tk.id, err)
		}
	}

	// page 1 of 2
	page1, total, err := s.ListTokens(ctx, indexer.Pagination{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListTokens(page1) failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page1, got %d", len(page1))
	}
	// ordered by first_seen_offset ASC
	if page1[0].InstrumentID != "AAA" || page1[1].InstrumentID != "BBB" {
		t.Fatalf("unexpected page1 order: %s, %s", page1[0].InstrumentID, page1[1].InstrumentID)
	}

	// page 2
	page2, _, err := s.ListTokens(ctx, indexer.Pagination{Page: 2, Limit: 2})
	if err != nil {
		t.Fatalf("ListTokens(page2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page2, got %d", len(page2))
	}
	if page2[0].InstrumentAdmin != "admin-2" {
		t.Fatalf("unexpected page2 token: %+v", page2[0])
	}
}

// ── GetBalance / ListBalancesForParty / ListBalancesForToken ──────────────────

func TestPGStore_GetBalance(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// not found
	bal, err := s.GetBalance(ctx, "alice", "admin-1", "DEMO")
	if err != nil {
		t.Fatalf("GetBalance(missing) failed: %v", err)
	}
	if bal != nil {
		t.Fatalf("GetBalance(missing) expected nil, got %+v", bal)
	}

	// seed balance directly via DAO
	_, err = s.db.NewInsert().Model(&BalanceDao{
		PartyID:         "alice",
		InstrumentAdmin: "admin-1",
		InstrumentID:    "DEMO",
		Amount:          "123",
	}).Exec(ctx)
	if err != nil {
		t.Fatalf("seed balance failed: %v", err)
	}

	bal, err = s.GetBalance(ctx, "alice", "admin-1", "DEMO")
	if err != nil {
		t.Fatalf("GetBalance(found) failed: %v", err)
	}
	if bal == nil {
		t.Fatal("GetBalance(found) returned nil")
	}
	if bal.Amount != "123" {
		t.Fatalf("expected Amount=123, got %s", bal.Amount)
	}
}

func TestPGStore_ListBalancesForParty(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	seed := []BalanceDao{
		{PartyID: "alice", InstrumentAdmin: "admin-1", InstrumentID: "AAA", Amount: "10"},
		{PartyID: "alice", InstrumentAdmin: "admin-1", InstrumentID: "BBB", Amount: "20"},
		{PartyID: "alice", InstrumentAdmin: "admin-1", InstrumentID: "CCC", Amount: "30"},
		{PartyID: "bob", InstrumentAdmin: "admin-1", InstrumentID: "AAA", Amount: "5"},
	}
	for i := range seed {
		if _, err := s.db.NewInsert().Model(&seed[i]).Exec(ctx); err != nil {
			t.Fatalf("seed balance failed: %v", err)
		}
	}

	// all alice
	page1, total, err := s.ListBalancesForParty(ctx, "alice", indexer.Pagination{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListBalancesForParty(alice, page1) failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1))
	}

	page2, _, err := s.ListBalancesForParty(ctx, "alice", indexer.Pagination{Page: 2, Limit: 2})
	if err != nil {
		t.Fatalf("ListBalancesForParty(alice, page2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page2, got %d", len(page2))
	}

	// bob only has 1
	bobs, total, err := s.ListBalancesForParty(ctx, "bob", indexer.Pagination{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("ListBalancesForParty(bob) failed: %v", err)
	}
	if total != 1 || len(bobs) != 1 {
		t.Fatalf("expected 1 balance for bob, got total=%d len=%d", total, len(bobs))
	}
}

func TestPGStore_ListBalancesForToken(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	seed := []BalanceDao{
		{PartyID: "alice", InstrumentAdmin: "admin-1", InstrumentID: "DEMO", Amount: "100"},
		{PartyID: "bob", InstrumentAdmin: "admin-1", InstrumentID: "DEMO", Amount: "200"},
		{PartyID: "carol", InstrumentAdmin: "admin-1", InstrumentID: "DEMO", Amount: "300"},
		{PartyID: "alice", InstrumentAdmin: "admin-1", InstrumentID: "OTHER", Amount: "50"},
	}
	for i := range seed {
		if _, err := s.db.NewInsert().Model(&seed[i]).Exec(ctx); err != nil {
			t.Fatalf("seed balance failed: %v", err)
		}
	}

	// page 1
	page1, total, err := s.ListBalancesForToken(ctx, "admin-1", "DEMO", indexer.Pagination{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListBalancesForToken(page1) failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page1, got %d", len(page1))
	}

	// page 2
	page2, _, err := s.ListBalancesForToken(ctx, "admin-1", "DEMO", indexer.Pagination{Page: 2, Limit: 2})
	if err != nil {
		t.Fatalf("ListBalancesForToken(page2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page2, got %d", len(page2))
	}

	// "OTHER" token has only alice
	others, total, err := s.ListBalancesForToken(ctx, "admin-1", "OTHER", indexer.Pagination{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("ListBalancesForToken(OTHER) failed: %v", err)
	}
	if total != 1 || len(others) != 1 {
		t.Fatalf("expected 1 balance for OTHER, got total=%d len=%d", total, len(others))
	}
}

// ── GetEvent / ListEvents ─────────────────────────────────────────────────────

func TestPGStore_GetEvent(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// not found
	ev, err := s.GetEvent(ctx, "missing-contract")
	if err != nil {
		t.Fatalf("GetEvent(missing) failed: %v", err)
	}
	if ev != nil {
		t.Fatalf("GetEvent(missing) expected nil, got %+v", ev)
	}

	// insert then retrieve
	event := makeEvent("contract-xyz", 7, indexer.EventMint, nil, ptr("alice"))
	_, err = s.InsertEvent(ctx, event)
	if err != nil {
		t.Fatalf("InsertEvent failed: %v", err)
	}

	ev, err = s.GetEvent(ctx, "contract-xyz")
	if err != nil {
		t.Fatalf("GetEvent(found) failed: %v", err)
	}
	if ev == nil {
		t.Fatal("GetEvent(found) returned nil")
	}
	if ev.ContractID != "contract-xyz" || ev.EventType != indexer.EventMint {
		t.Fatalf("GetEvent returned unexpected event: %+v", ev)
	}
}

func TestPGStore_ListEvents(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	// seed events
	seedEvents := []struct {
		contractID string
		offset     int64
		eventType  indexer.EventType
		from, to   *string
		admin, id  string
	}{
		{"c1", 1, indexer.EventMint, nil, ptr("alice"), "admin-1", "DEMO"},
		{"c2", 2, indexer.EventTransfer, ptr("alice"), ptr("bob"), "admin-1", "DEMO"},
		{"c3", 3, indexer.EventBurn, ptr("bob"), nil, "admin-1", "DEMO"},
		{"c4", 4, indexer.EventMint, nil, ptr("carol"), "admin-2", "OTHER"},
		{"c5", 5, indexer.EventTransfer, ptr("carol"), ptr("alice"), "admin-2", "OTHER"},
	}
	for _, se := range seedEvents {
		ev := makeEvent(se.contractID, se.offset, se.eventType, se.from, se.to)
		ev.InstrumentAdmin = se.admin
		ev.InstrumentID = se.id
		if _, err := s.InsertEvent(ctx, ev); err != nil {
			t.Fatalf("InsertEvent(%s) failed: %v", se.contractID, err)
		}
	}

	p := indexer.Pagination{Page: 1, Limit: 10}

	// no filter — all 5
	evs, total, err := s.ListEvents(ctx, indexer.EventFilter{}, p)
	if err != nil {
		t.Fatalf("ListEvents(no filter) failed: %v", err)
	}
	if total != 5 || len(evs) != 5 {
		t.Fatalf("expected 5 events, got total=%d len=%d", total, len(evs))
	}
	// ordered by ledger_offset ASC
	if evs[0].LedgerOffset != 1 || evs[4].LedgerOffset != 5 {
		t.Fatalf("unexpected offset order: first=%d last=%d", evs[0].LedgerOffset, evs[4].LedgerOffset)
	}

	// filter by InstrumentAdmin
	evs, total, err = s.ListEvents(ctx, indexer.EventFilter{InstrumentAdmin: "admin-1"}, p)
	if err != nil {
		t.Fatalf("ListEvents(admin filter) failed: %v", err)
	}
	if total != 3 || len(evs) != 3 {
		t.Fatalf("expected 3 events for admin-1, got total=%d len=%d", total, len(evs))
	}

	// filter by InstrumentID
	evs, total, err = s.ListEvents(ctx, indexer.EventFilter{InstrumentID: "OTHER"}, p)
	if err != nil {
		t.Fatalf("ListEvents(id filter) failed: %v", err)
	}
	if total != 2 || len(evs) != 2 {
		t.Fatalf("expected 2 events for OTHER, got total=%d len=%d", total, len(evs))
	}

	// filter by EventType MINT
	evs, total, err = s.ListEvents(ctx, indexer.EventFilter{EventType: indexer.EventMint}, p)
	if err != nil {
		t.Fatalf("ListEvents(mint filter) failed: %v", err)
	}
	if total != 2 || len(evs) != 2 {
		t.Fatalf("expected 2 mint events, got total=%d len=%d", total, len(evs))
	}

	// filter by PartyID — matches from_party_id OR to_party_id
	// alice appears in: c1 (to), c2 (from), c5 (to) → 3 events
	evs, total, err = s.ListEvents(ctx, indexer.EventFilter{PartyID: "alice"}, p)
	if err != nil {
		t.Fatalf("ListEvents(party alice) failed: %v", err)
	}
	if total != 3 || len(evs) != 3 {
		t.Fatalf("expected 3 events for alice, got total=%d len=%d", total, len(evs))
	}

	// combined filter: admin-1 + MINT
	evs, total, err = s.ListEvents(ctx, indexer.EventFilter{InstrumentAdmin: "admin-1", EventType: indexer.EventMint}, p)
	if err != nil {
		t.Fatalf("ListEvents(admin+mint) failed: %v", err)
	}
	if total != 1 || len(evs) != 1 {
		t.Fatalf("expected 1 admin-1 MINT event, got total=%d len=%d", total, len(evs))
	}
	if evs[0].ContractID != "c1" {
		t.Fatalf("expected contract c1, got %s", evs[0].ContractID)
	}

	// pagination: 2 per page, 3 total (admin-1 events)
	page1, total, err := s.ListEvents(ctx, indexer.EventFilter{InstrumentAdmin: "admin-1"}, indexer.Pagination{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListEvents(paged, page1) failed: %v", err)
	}
	if total != 3 || len(page1) != 2 {
		t.Fatalf("expected total=3 len=2 on page1, got total=%d len=%d", total, len(page1))
	}
	page2, _, err := s.ListEvents(ctx, indexer.EventFilter{InstrumentAdmin: "admin-1"}, indexer.Pagination{Page: 2, Limit: 2})
	if err != nil {
		t.Fatalf("ListEvents(paged, page2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 event on page2, got %d", len(page2))
	}
}

func makeOffer(cid, sender, receiver string, offset int64, expiresAt *time.Time) *indexer.Transfer {
	return &indexer.Transfer{
		ContractID:      cid,
		Kind:            indexer.TransferKindOffer,
		Status:          indexer.TransferStatusPending,
		FromPartyID:     sender,
		ToPartyID:       receiver,
		InstrumentAdmin: "admin-1",
		InstrumentID:    "DEMO",
		Amount:          "10",
		LedgerOffset:    offset,
		CreatedAt:       time.Now().UTC().Truncate(time.Millisecond),
		ExpiresAt:       expiresAt,
	}
}

func makeDirect(cid, from, to string, offset int64) *indexer.Transfer {
	return &indexer.Transfer{
		ContractID:      cid,
		Kind:            indexer.TransferKindDirect,
		Status:          indexer.TransferStatusCompleted,
		FromPartyID:     from,
		ToPartyID:       to,
		InstrumentAdmin: "admin-1",
		InstrumentID:    "DEMO",
		Amount:          "10",
		TxID:            "tx-" + cid,
		LedgerOffset:    offset,
		CreatedAt:       time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestListTransfers(t *testing.T) {
	ctx, s := setupIndexerStore(t)

	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)

	// alice sends three offers (live / expired / never-expires) and receives one completed.
	offers := []*indexer.Transfer{
		makeOffer("o-live", "alice", "bob", 1, &future),
		makeOffer("o-expired", "alice", "carol", 2, &past),
		makeOffer("o-noexp", "alice", "eve", 3, nil),
		makeOffer("o-done", "dave", "alice", 4, &past),
	}
	for _, o := range offers {
		if err := s.InsertTransfer(ctx, o); err != nil {
			t.Fatalf("InsertTransfer(%s): %v", o.ContractID, err)
		}
	}
	// Complete the inbound offer so it reads as a completed transfer for alice.
	if err := s.CompleteTransfer(ctx, "o-done"); err != nil {
		t.Fatalf("CompleteTransfer(o-done): %v", err)
	}

	ids := func(list []indexer.Transfer) map[string]string {
		m := map[string]string{}
		for _, o := range list {
			m[o.ContractID] = o.Status
		}
		return m
	}
	page := indexer.Pagination{Page: 1, Limit: 50}

	// role=sender, status=pending → only live + never-expires (expired excluded).
	got, total, err := s.ListTransfers(ctx, "alice",
		indexer.TransferQuery{Role: indexer.TransferRoleSender, Status: indexer.TransferStatusPending}, page)
	if err != nil {
		t.Fatalf("sender/pending: %v", err)
	}
	m := ids(got)
	if total != 2 || len(m) != 2 || m["o-live"] != indexer.TransferStatusPending || m["o-noexp"] != indexer.TransferStatusPending {
		t.Fatalf("sender/pending unexpected: total=%d m=%v", total, m)
	}

	// role=sender, status=expired → only the past-dated pending offer, surfaced as expired.
	got, total, err = s.ListTransfers(ctx, "alice",
		indexer.TransferQuery{Role: indexer.TransferRoleSender, Status: indexer.TransferStatusExpired}, page)
	if err != nil {
		t.Fatalf("sender/expired: %v", err)
	}
	m = ids(got)
	if total != 1 || m["o-expired"] != indexer.TransferStatusExpired {
		t.Fatalf("sender/expired unexpected: total=%d m=%v", total, m)
	}

	// role=any, status=completed → the completed offer where alice is receiver.
	got, total, err = s.ListTransfers(ctx, "alice",
		indexer.TransferQuery{Role: indexer.TransferRoleAny, Status: indexer.TransferStatusCompleted}, page)
	if err != nil {
		t.Fatalf("any/completed: %v", err)
	}
	m = ids(got)
	if total != 1 || m["o-done"] != indexer.TransferStatusCompleted {
		t.Fatalf("any/completed unexpected: total=%d m=%v", total, m)
	}

	// role=receiver default, status=all → only the offer alice receives.
	got, total, err = s.ListTransfers(ctx, "alice", indexer.TransferQuery{}, page)
	if err != nil {
		t.Fatalf("receiver/all: %v", err)
	}
	if total != 1 || got[0].ContractID != "o-done" {
		t.Fatalf("receiver/all unexpected: total=%d got=%v", total, ids(got))
	}
}

func TestListTransfers_DirectAndOffer(t *testing.T) {
	ctx, s := setupIndexerStore(t)
	page := indexer.Pagination{Page: 1, Limit: 50}

	// A direct CIP-56 transfer (alice sender) and an offer-based transfer settled
	// to alice (alice receiver). Both are completed and live in indexer_transfers.
	if err := s.InsertTransfer(ctx, makeDirect("ev-direct", "alice", "bob", 1)); err != nil {
		t.Fatalf("upsert direct transfer: %v", err)
	}
	// Idempotent: a replay is a no-op.
	if err := s.InsertTransfer(ctx, makeDirect("ev-direct", "alice", "bob", 1)); err != nil {
		t.Fatalf("upsert direct transfer (replay): %v", err)
	}
	if err := s.InsertTransfer(ctx, makeOffer("of-usdcx", "carol", "alice", 2, nil)); err != nil {
		t.Fatalf("insert offer: %v", err)
	}
	if err := s.CompleteTransfer(ctx, "of-usdcx"); err != nil {
		t.Fatalf("complete offer: %v", err)
	}

	// completed (role=any) → both transfers.
	got, total, err := s.ListTransfers(ctx, "alice",
		indexer.TransferQuery{Role: indexer.TransferRoleAny, Status: indexer.TransferStatusCompleted}, page)
	if err != nil {
		t.Fatalf("ListTransfers(completed): %v", err)
	}
	if total != 2 || len(got) != 2 {
		t.Fatalf("expected 2 completed transfers, got total=%d len=%d", total, len(got))
	}
	byKind := map[string]indexer.Transfer{}
	for _, c := range got {
		byKind[c.Kind] = c
	}
	if byKind[indexer.TransferKindDirect].ContractID != "ev-direct" {
		t.Fatalf("expected direct transfer ev-direct, got %+v", byKind[indexer.TransferKindDirect])
	}
	if byKind[indexer.TransferKindOffer].ContractID != "of-usdcx" || byKind[indexer.TransferKindOffer].FromPartyID != "carol" {
		t.Fatalf("expected offer transfer of-usdcx from carol, got %+v", byKind[indexer.TransferKindOffer])
	}
}

func TestListPendingTransfers(t *testing.T) {
	ctx, s := setupIndexerStore(t)
	page := indexer.Pagination{Page: 1, Limit: 50}

	past := time.Now().UTC().Add(-time.Hour)
	// A live pending offer, an expired one, a completed one, and a direct transfer.
	if err := s.InsertTransfer(ctx, makeOffer("p-live", "dave", "alice", 1, nil)); err != nil {
		t.Fatalf("insert live offer: %v", err)
	}
	if err := s.InsertTransfer(ctx, makeOffer("p-expired", "erin", "bob", 2, &past)); err != nil {
		t.Fatalf("insert expired offer: %v", err)
	}
	if err := s.InsertTransfer(ctx, makeOffer("p-done", "fred", "carol", 3, nil)); err != nil {
		t.Fatalf("insert done offer: %v", err)
	}
	if err := s.CompleteTransfer(ctx, "p-done"); err != nil {
		t.Fatalf("complete offer: %v", err)
	}
	if err := s.InsertTransfer(ctx, makeDirect("p-direct", "gita", "hank", 4)); err != nil {
		t.Fatalf("upsert direct transfer: %v", err)
	}

	// Only the live, not-yet-expired offer qualifies.
	got, total, err := s.ListPendingTransfers(ctx, page)
	if err != nil {
		t.Fatalf("ListPendingTransfers: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ContractID != "p-live" {
		t.Fatalf("expected only p-live pending, got total=%d %+v", total, got)
	}
}
