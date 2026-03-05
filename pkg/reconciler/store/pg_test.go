package store

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

func setupBalanceStore(t *testing.T) (context.Context, *PGStore) {
	t.Helper()
	requireDockerAccess(t)

	ctx := context.Background()
	db, cleanup := pgutil.SetupTestDB(t)
	t.Cleanup(cleanup)

	if err := mghelper.CreateSchema(
		ctx,
		db,
		&UserTokenBalanceDao{},
		&TokenMetricsDao{},
		&BridgeEventDao{},
		&ReconciliationStateDao{},
	); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	if _, err := db.NewCreateIndex().
		Model((*UserTokenBalanceDao)(nil)).
		Index("idx_user_token_balances_fingerprint_token_symbol").
		Unique().
		Column("fingerprint", "token_symbol").
		IfNotExists().
		Exec(ctx); err != nil {
		t.Fatalf("failed to create fingerprint/token unique index: %v", err)
	}
	if _, err := db.NewCreateIndex().
		Model((*UserTokenBalanceDao)(nil)).
		Index("idx_user_token_balances_evm_address_token_symbol").
		Unique().
		Column("evm_address", "token_symbol").
		IfNotExists().
		Exec(ctx); err != nil {
		t.Fatalf("failed to create evm/token unique index: %v", err)
	}
	if _, err := db.NewCreateIndex().
		Model((*UserTokenBalanceDao)(nil)).
		Index("idx_user_token_balances_canton_party_id_token_symbol").
		Unique().
		Column("canton_party_id", "token_symbol").
		IfNotExists().
		Exec(ctx); err != nil {
		t.Fatalf("failed to create party/token unique index: %v", err)
	}

	_, err := db.NewInsert().
		Model(&ReconciliationStateDao{
			ID:                  1,
			LastProcessedOffset: 0,
			EventsProcessed:     0,
		}).
		On("CONFLICT (id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		t.Fatalf("failed to seed reconciliation_state: %v", err)
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

	t.Skip("docker daemon socket is not accessible; skipping testcontainer-backed reconciler store tests")
}

func assertDecimalEqual(t *testing.T, got, want string) {
	t.Helper()

	gotDec, err := decimal.NewFromString(got)
	if err != nil {
		t.Fatalf("failed to parse got decimal %q: %v", got, err)
	}
	wantDec, err := decimal.NewFromString(want)
	if err != nil {
		t.Fatalf("failed to parse want decimal %q: %v", want, err)
	}
	if !gotDec.Equal(wantDec) {
		t.Fatalf("decimal mismatch: got %s want %s", gotDec.String(), wantDec.String())
	}
}

func TestPGStore_BalanceOperations(t *testing.T) {
	ctx, store := setupBalanceStore(t)

	fromPartyID := "party::from"
	fromFingerprint := "0xabc123"
	fromEVM := "0x9999999999999999999999999999999999999999"
	toFingerprint := "0xdef456"
	toEVM := "0x8888888888888888888888888888888888888888"

	if err := store.SetBalanceByCantonPartyID(ctx, fromPartyID, "PROMPT", "100"); err != nil {
		t.Fatalf("SetBalanceByCantonPartyID(PROMPT) failed: %v", err)
	}
	if err := store.SetBalanceByCantonPartyID(ctx, fromPartyID, "DEMO", "7"); err != nil {
		t.Fatalf("SetBalanceByCantonPartyID(DEMO) failed: %v", err)
	}

	var byParty UserTokenBalanceDao
	err := store.db.NewSelect().
		Model(&byParty).
		Column("balance").
		Where("canton_party_id = ?", fromPartyID).
		Where("token_symbol = ?", "PROMPT").
		Limit(1).
		Scan(ctx)
	if err != nil {
		t.Fatalf("query by party PROMPT failed: %v", err)
	}
	assertDecimalEqual(t, byParty.Balance, "100")

	err = store.db.NewSelect().
		Model(&byParty).
		Column("balance").
		Where("canton_party_id = ?", fromPartyID).
		Where("token_symbol = ?", "DEMO").
		Limit(1).
		Scan(ctx)
	if err != nil {
		t.Fatalf("query by party DEMO failed: %v", err)
	}
	assertDecimalEqual(t, byParty.Balance, "7")

	if err = store.IncrementBalanceByFingerprint(ctx, "abc123", "25.5", "PROMPT"); err != nil {
		t.Fatalf("IncrementBalanceByFingerprint() failed: %v", err)
	}
	bal, err := store.GetBalanceByFingerprint(ctx, fromFingerprint, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByFingerprint(after first increment) failed: %v", err)
	}
	assertDecimalEqual(t, bal, "25.5")

	if err = store.IncrementBalanceByFingerprint(ctx, "abc123", "100", "PROMPT"); err != nil {
		t.Fatalf("IncrementBalanceByFingerprint(second increment) failed: %v", err)
	}
	bal, err = store.GetBalanceByFingerprint(ctx, fromFingerprint, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByFingerprint(after second increment) failed: %v", err)
	}
	assertDecimalEqual(t, bal, "125.5")

	evmSeed := &UserTokenBalanceDao{
		EVMAddress:  &fromEVM,
		TokenSymbol: "PROMPT",
		Balance:     "120",
	}
	if _, err = store.db.NewInsert().
		Model(evmSeed).
		On("CONFLICT (evm_address, token_symbol) DO UPDATE").
		Set("balance = EXCLUDED.balance").
		Set("updated_at = NOW()").
		Exec(ctx); err != nil {
		t.Fatalf("seed evm balance failed: %v", err)
	}

	if err = store.DecrementBalanceByEVMAddress(ctx, fromEVM, "5.5", "PROMPT"); err != nil {
		t.Fatalf("DecrementBalanceByEVMAddress() failed: %v", err)
	}
	bal, err = store.GetBalanceByEVMAddress(ctx, fromEVM, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByEVMAddress(after decrement) failed: %v", err)
	}
	assertDecimalEqual(t, bal, "114.5")

	toEVMSeed := &UserTokenBalanceDao{
		EVMAddress:  &toEVM,
		TokenSymbol: "PROMPT",
		Balance:     "20",
	}
	if _, err = store.db.NewInsert().
		Model(toEVMSeed).
		On("CONFLICT (evm_address, token_symbol) DO UPDATE").
		Set("balance = EXCLUDED.balance").
		Set("updated_at = NOW()").
		Exec(ctx); err != nil {
		t.Fatalf("seed recipient evm balance failed: %v", err)
	}

	if err = store.IncrementBalanceByFingerprint(ctx, toFingerprint, "20", "PROMPT"); err != nil {
		t.Fatalf("IncrementBalanceByFingerprint(recipient) failed: %v", err)
	}
	fromBal, err := store.GetBalanceByEVMAddress(ctx, fromEVM, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByEVMAddress(from) failed: %v", err)
	}
	toBal, err := store.GetBalanceByEVMAddress(ctx, toEVM, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByEVMAddress(to) failed: %v", err)
	}
	toFingerprintBal, err := store.GetBalanceByFingerprint(ctx, toFingerprint, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByFingerprint(to) failed: %v", err)
	}
	assertDecimalEqual(t, fromBal, "114.5")
	assertDecimalEqual(t, toBal, "20")
	assertDecimalEqual(t, toFingerprintBal, "20")

	if err = store.ResetBalancesByTokenSymbol(ctx, "PROMPT"); err != nil {
		t.Fatalf("ResetBalancesByTokenSymbol(PROMPT) failed: %v", err)
	}
	fromBal, err = store.GetBalanceByEVMAddress(ctx, fromEVM, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByEVMAddress(from after reset) failed: %v", err)
	}
	toBal, err = store.GetBalanceByEVMAddress(ctx, toEVM, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByEVMAddress(to after reset) failed: %v", err)
	}
	demoByParty := new(UserTokenBalanceDao)
	err = store.db.NewSelect().
		Model(demoByParty).
		Column("balance").
		Where("canton_party_id = ?", fromPartyID).
		Where("token_symbol = ?", "DEMO").
		Limit(1).
		Scan(ctx)
	if err != nil {
		t.Fatalf("query by party DEMO after reset failed: %v", err)
	}
	assertDecimalEqual(t, fromBal, "0")
	assertDecimalEqual(t, toBal, "0")
	assertDecimalEqual(t, demoByParty.Balance, "7")
}

func TestPGStore_ReconcilerDataOperations(t *testing.T) {
	ctx, store := setupBalanceStore(t)
	fingerprint := "0xabc999"

	if err := store.SetTotalSupply(ctx, "PROMPT", "123.45"); err != nil {
		t.Fatalf("SetTotalSupply() failed: %v", err)
	}
	if err := store.UpdateLastReconciled(ctx, "PROMPT"); err != nil {
		t.Fatalf("UpdateLastReconciled() failed: %v", err)
	}

	var metrics TokenMetricsDao
	err := store.db.NewSelect().
		Model(&metrics).
		Where("token_symbol = ?", "PROMPT").
		Limit(1).
		Scan(ctx)
	if err != nil {
		t.Fatalf("failed to query token metrics: %v", err)
	}
	assertDecimalEqual(t, metrics.TotalSupply, "123.45")
	if metrics.LastReconciledAt == nil {
		t.Fatalf("expected last_reconciled_at to be set")
	}

	processed, err := store.IsEventProcessed(ctx, "mint-contract-1")
	if err != nil {
		t.Fatalf("IsEventProcessed(before) failed: %v", err)
	}
	if processed {
		t.Fatalf("expected mint-contract-1 to be unprocessed")
	}

	now := time.Now()
	mint := &canton.TokenTransferEvent{
		ContractID:   "mint-contract-1",
		FromParty:    "",
		ToParty:      "party::receiver",
		Amount:       "10",
		InstrumentID: "PROMPT",
		Timestamp:    now,
		Meta: map[string]string{
			"bridge.fingerprint":  fingerprint,
			"bridge.externalTxId": "0xabc",
		},
	}
	if err = store.StoreTokenTransferEvent(ctx, mint); err != nil {
		t.Fatalf("StoreTokenTransferEvent(mint) failed: %v", err)
	}

	processed, err = store.IsEventProcessed(ctx, mint.ContractID)
	if err != nil {
		t.Fatalf("IsEventProcessed(after mint) failed: %v", err)
	}
	if !processed {
		t.Fatalf("expected mint event to be marked processed")
	}

	bal, err := store.GetBalanceByFingerprint(ctx, fingerprint, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByFingerprint(after mint) failed: %v", err)
	}
	assertDecimalEqual(t, bal, "10")

	state, err := store.GetReconciliationState(ctx)
	if err != nil {
		t.Fatalf("GetReconciliationState(after mint) failed: %v", err)
	}
	if state.EventsProcessed != 1 {
		t.Fatalf("unexpected events_processed after mint: got %d want 1", state.EventsProcessed)
	}

	// Duplicate contract ID should be ignored (idempotent).
	if err = store.StoreTokenTransferEvent(ctx, mint); err != nil {
		t.Fatalf("StoreTokenTransferEvent(duplicate mint) failed: %v", err)
	}
	state, err = store.GetReconciliationState(ctx)
	if err != nil {
		t.Fatalf("GetReconciliationState(after duplicate mint) failed: %v", err)
	}
	if state.EventsProcessed != 1 {
		t.Fatalf("duplicate mint should not increment counter: got %d want 1", state.EventsProcessed)
	}

	burn := &canton.TokenTransferEvent{
		ContractID:   "burn-contract-1",
		FromParty:    "party::sender",
		ToParty:      "",
		Amount:       "3",
		InstrumentID: "PROMPT",
		Timestamp:    now.Add(time.Second),
		Meta: map[string]string{
			"bridge.fingerprint":     fingerprint,
			"bridge.externalAddress": "0xdef",
		},
	}
	if err = store.StoreTokenTransferEvent(ctx, burn); err != nil {
		t.Fatalf("StoreTokenTransferEvent(burn) failed: %v", err)
	}

	bal, err = store.GetBalanceByFingerprint(ctx, fingerprint, "PROMPT")
	if err != nil {
		t.Fatalf("GetBalanceByFingerprint(after burn) failed: %v", err)
	}
	assertDecimalEqual(t, bal, "7")

	state, err = store.GetReconciliationState(ctx)
	if err != nil {
		t.Fatalf("GetReconciliationState(after burn) failed: %v", err)
	}
	if state.EventsProcessed != 2 {
		t.Fatalf("unexpected events_processed after burn: got %d want 2", state.EventsProcessed)
	}

	if err = store.MarkFullReconcileComplete(ctx); err != nil {
		t.Fatalf("MarkFullReconcileComplete() failed: %v", err)
	}
	state, err = store.GetReconciliationState(ctx)
	if err != nil {
		t.Fatalf("GetReconciliationState(after mark full) failed: %v", err)
	}
	if state.LastFullReconcileAt == nil {
		t.Fatalf("expected last_full_reconcile_at to be set")
	}

	if err = store.ClearBridgeEvents(ctx); err != nil {
		t.Fatalf("ClearBridgeEvents() failed: %v", err)
	}
	state, err = store.GetReconciliationState(ctx)
	if err != nil {
		t.Fatalf("GetReconciliationState(after clear) failed: %v", err)
	}
	if state.EventsProcessed != 0 {
		t.Fatalf("expected events_processed reset to 0, got %d", state.EventsProcessed)
	}
}
