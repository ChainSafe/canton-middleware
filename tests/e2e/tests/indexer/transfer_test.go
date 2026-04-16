//go:build e2e

package indexer_test

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// signCantonTx decodes the 0x-prefixed transaction hash and returns a
// 0x-prefixed DER signature and the Canton fingerprint for the given keypair.
func signCantonTx(t *testing.T, kp interface {
	SignDER([]byte) ([]byte, error)
	Fingerprint() (string, error)
}, txHashHex string) (sig, fingerprint string) {
	t.Helper()
	hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHashHex, "0x"))
	if err != nil {
		t.Fatalf("decode tx hash: %v", err)
	}
	derSig, err := kp.SignDER(hashBytes)
	if err != nil {
		t.Fatalf("sign tx hash: %v", err)
	}
	fp, err := kp.Fingerprint()
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	return "0x" + hex.EncodeToString(derSig), fp
}

// TestIndexer_TransferEvent_AfterAPITransfer registers two external users,
// mints 50 DEMO to User1, transfers 10 DEMO to User2 via the api-server, and
// verifies that the indexer records a TRANSFER event with correct fields:
// event_type=TRANSFER, from_party_id=User1, to_party_id=User2, amount>=10,
// contract_id non-empty, ledger_offset>0. Also verifies both parties'
// indexer balances reflect the transfer.
func TestIndexer_TransferEvent_AfterAPITransfer(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	resp1, kp1 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	resp2, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	admin := sys.Manifest.DemoInstrumentAdmin
	id := sys.Manifest.DemoInstrumentID

	// Mint 50 DEMO to User1 and wait for the indexer to reflect the balance.
	mintAmount := "50"
	sys.DSL.MintDEMO(ctx, t, resp1.Party, mintAmount)
	sys.DSL.WaitForCantonBalance(ctx, t, resp1.Party, admin, id, mintAmount)

	// Transfer 10 DEMO from User1 to User2 via the api-server.
	transferAmount := "10"
	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: transferAmount,
		Token:  "DEMO",
	})
	if err != nil {
		t.Fatalf("prepare transfer: %v", err)
	}

	sig, fingerprint := signCantonTx(t, kp1, prepResp.TransactionHash)
	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  sig,
		SignedBy:   fingerprint,
	})
	if err != nil {
		t.Fatalf("execute transfer: %v", err)
	}
	if execResp.Status != "completed" {
		t.Fatalf("transfer status: want completed, got %q", execResp.Status)
	}

	// Wait for User2's indexer balance to reflect the incoming DEMO.
	// NewTestAccounts derives unique parties per test name, so User2 starts at
	// zero DEMO — the first event for this party is our TRANSFER.
	sys.DSL.WaitForCantonBalance(ctx, t, resp2.Party, admin, id, transferAmount)

	ev := sys.DSL.WaitForPartyEvent(ctx, t, resp2.Party, indexer.EventTransfer)

	if ev.ContractID == "" {
		t.Error("contract_id: want non-empty")
	}
	if ev.EventType != indexer.EventTransfer {
		t.Errorf("event_type: want TRANSFER, got %s", ev.EventType)
	}
	if ev.LedgerOffset <= 0 {
		t.Errorf("ledger_offset: want > 0, got %d", ev.LedgerOffset)
	}
	if ev.FromPartyID == nil || *ev.FromPartyID != resp1.Party {
		t.Errorf("from_party_id: want %s, got %v", resp1.Party, ev.FromPartyID)
	}
	if ev.ToPartyID == nil || *ev.ToPartyID != resp2.Party {
		t.Errorf("to_party_id: want %s, got %v", resp2.Party, ev.ToPartyID)
	}
	if !amtGTE(ev.Amount, transferAmount) {
		t.Errorf("amount: want >= %s, got %s", transferAmount, ev.Amount)
	}

	// Verify both parties' balances in the indexer.
	bal1, err := sys.Indexer.GetBalance(ctx, resp1.Party, admin, id)
	if err != nil {
		t.Fatalf("get balance user1: %v", err)
	}
	if !amtGTE(bal1.Amount, "40") { // 50 minted - 10 transferred = 40
		t.Errorf("user1 balance: want >= 40, got %s", bal1.Amount)
	}

	bal2, err := sys.Indexer.GetBalance(ctx, resp2.Party, admin, id)
	if err != nil {
		t.Fatalf("get balance user2: %v", err)
	}
	if !amtGTE(bal2.Amount, transferAmount) {
		t.Errorf("user2 balance: want >= %s, got %s", transferAmount, bal2.Amount)
	}
}

// TestIndexer_HolderCount_Updates verifies that Token.HolderCount increments
// when a party first receives tokens and decrements when a party's balance
// returns to zero.
//
// Flow:
//  1. Record baseline HolderCount C0 for DEMO.
//  2. Mint 100 DEMO to User1 (new holder) — HolderCount → C0+1.
//  3. Mint 100 DEMO to User2 (new holder) — HolderCount → C0+2.
//  4. Transfer User2's full 100 DEMO to User1 via the api-server.
//     User2 balance → 0 (holder removed) — HolderCount → C0+1.
func TestIndexer_HolderCount_Updates(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	resp1, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	resp2, kp2 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	admin := sys.Manifest.DemoInstrumentAdmin
	id := sys.Manifest.DemoInstrumentID

	// Record baseline HolderCount. The token may not exist yet if no DEMO
	// events have been indexed — treat that as HolderCount=0.
	var c0 int64
	if tok, err := sys.Indexer.GetToken(ctx, admin, id); err == nil && tok != nil {
		c0 = tok.HolderCount
	}

	// Mint 100 DEMO to User1 and wait for the indexer to register a new holder.
	sys.DSL.MintDEMO(ctx, t, resp1.Party, "100")
	sys.DSL.WaitForCantonBalance(ctx, t, resp1.Party, admin, id, "100")
	sys.DSL.WaitForHolderCount(ctx, t, admin, id, c0+1)

	// Mint 100 DEMO to User2 and wait for a second holder to be registered.
	sys.DSL.MintDEMO(ctx, t, resp2.Party, "100")
	sys.DSL.WaitForCantonBalance(ctx, t, resp2.Party, admin, id, "100")
	sys.DSL.WaitForHolderCount(ctx, t, admin, id, c0+2)

	// Transfer User2's entire 100 DEMO to User1 via the api-server. When
	// User2's balance reaches zero, the indexer must decrement HolderCount.
	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User2, &transfer.PrepareRequest{
		To:     sys.Accounts.User1.Address.Hex(),
		Amount: "100",
		Token:  "DEMO",
	})
	if err != nil {
		t.Fatalf("prepare transfer: %v", err)
	}

	sig, fingerprint := signCantonTx(t, kp2, prepResp.TransactionHash)
	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User2, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  sig,
		SignedBy:   fingerprint,
	})
	if err != nil {
		t.Fatalf("execute transfer: %v", err)
	}
	if execResp.Status != "completed" {
		t.Fatalf("transfer status: want completed, got %q", execResp.Status)
	}

	// Wait for User1's indexer balance to reflect the additional 100 DEMO
	// (cumulative: 200). This confirms the TRANSFER event was processed,
	// which also updates User2's balance and HolderCount atomically.
	sys.DSL.WaitForCantonBalance(ctx, t, resp1.Party, admin, id, "200")

	// HolderCount must drop back to C0+1: User2 gave away all its DEMO,
	// so it is no longer counted as a holder.
	sys.DSL.WaitForHolderCount(ctx, t, admin, id, c0+1)
}
