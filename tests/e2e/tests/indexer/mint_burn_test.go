//go:build e2e

package indexer_test

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

var one18 = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// TestIndexer_MintEvent mints DEMO directly on Canton and verifies that the
// indexer picks up the TokenTransferEvent and records it with correct fields:
// event_type=MINT, amount>=5, contract_id non-empty, ledger_offset>0,
// to_party_id matches the minted party.
//
// This test uses NewIndexerStack (Canton + Indexer only) because the indexer
// doesn't care whether a MINT originates from a bridge deposit or a direct
// IssuerMint DAML choice — it indexes any TokenTransferEvent it observes.
// Using a freshly allocated party guarantees no prior events to filter.
func TestIndexer_MintEvent(t *testing.T) {
	sys := presets.NewIndexerStack(t)
	ctx := context.Background()

	// Allocate a fresh party. No api-server registration needed — the indexer
	// tracks canton_party_id and Canton allocates parties independently.
	party, err := sys.Canton.AllocateParty(ctx, "indexer-test-mint")
	if err != nil {
		t.Fatalf("allocate party: %v", err)
	}

	admin := sys.Manifest.DemoInstrumentAdmin
	id := sys.Manifest.DemoInstrumentID

	// Mint 5 DEMO via IssuerMint on Canton. This creates a TokenTransferEvent
	// on the ledger which the indexer streams and stores as a MINT event.
	if err := sys.Canton.MintToken(ctx, party, "DEMO", "5"); err != nil {
		t.Fatalf("mint token: %v", err)
	}

	// Wait for the indexer to reflect the balance, then fetch the MINT event.
	// Since the party was freshly allocated, the first event is ours.
	sys.DSL.WaitForCantonBalance(ctx, t, party, admin, id, "5")
	ev := sys.DSL.WaitForPartyEvent(ctx, t, party, indexer.EventMint)

	if ev.ContractID == "" {
		t.Error("contract_id: want non-empty")
	}
	if ev.EventType != indexer.EventMint {
		t.Errorf("event_type: want MINT, got %s", ev.EventType)
	}
	if !amtGTE(ev.Amount, "5") {
		t.Errorf("amount: want >= 5, got %s", ev.Amount)
	}
	if ev.LedgerOffset <= 0 {
		t.Errorf("ledger_offset: want > 0, got %d", ev.LedgerOffset)
	}
	if ev.ToPartyID == nil || *ev.ToPartyID != party {
		t.Errorf("to_party_id: want %s, got %v", party, ev.ToPartyID)
	}
}

// TestIndexer_BurnEvent_AfterWithdrawal deposits 2 PROMPT via the bridge,
// initiates a 1 PROMPT withdrawal, and verifies that the indexer records a
// BURN event with correct fields: event_type=BURN, contract_id non-empty,
// ledger_offset>0, from_party_id matches the party, external_address matches
// the EVM destination. Also verifies total supply decreased after the burn.
//
// This test requires NewFullStack because the BURN TokenTransferEvent is
// created by the relayer when it processes a WithdrawalRequest — there is no
// Canton-native path to create a BURN without the bridge.
func TestIndexer_BurnEvent_AfterWithdrawal(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	// Use AnvilAccount0: it holds all PROMPT tokens (minted to the deployer at
	// contract deployment). Account1 starts with zero PROMPT so its deposit
	// would revert. MaxPartyEventOffset records a sinceOffset before the
	// withdrawal so stale burn events from other tests are skipped.
	account := stack.AnvilAccount0
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// Deposit 2 PROMPT so there is a holding large enough to withdraw 1 from.
	depositAmount := new(big.Int).Mul(big.NewInt(2), one18)
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")

	// Record total supply after the deposit MINT is indexed.
	tok, err := sys.Indexer.GetToken(ctx, admin, id)
	if err != nil {
		t.Fatalf("get token after deposit: %v", err)
	}
	supplyBeforeBurn := tok.TotalSupply

	// Record the EVM balance before withdrawal.
	balBefore, err := sys.Anvil.ERC20Balance(ctx, tokenAddr, account.Address)
	if err != nil {
		t.Fatalf("erc20 balance before withdrawal: %v", err)
	}

	// Record the highest BURN ledger offset already indexed for this party
	// before initiating the withdrawal. AnvilAccount0 is a long-lived account
	// reused across test runs; matching by fingerprint is not sufficient because
	// the fingerprint is stable per registration and would match stale burns.
	burnSinceOffset := sys.DSL.MaxPartyEventOffset(ctx, t, regResp.Party, indexer.EventBurn)

	// Initiate a 1 PROMPT withdrawal to the same EVM address.
	evmDest := account.Address.Hex()
	sys.DSL.Withdraw(ctx, t, regResp.Party, regResp.Fingerprint, "PROMPT", "1", evmDest)

	// Wait for the relayer to release tokens on Ethereum before polling the
	// indexer. Without this gate the 60 s indexerEventTimeout races against
	// relayer processing (which can itself take up to 120 s), causing spurious
	// timeouts. WaitForEthBalance is a pre-condition here, not an assertion.
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, account.Address, new(big.Int).Add(balBefore, one18))

	// Wait for the indexer to record a BURN event produced by this withdrawal.
	// Match by LedgerOffset > burnSinceOffset so that stale burns from prior
	// test runs against AnvilAccount1 are not mistakenly accepted.
	ev := sys.DSL.WaitForPartyEventMatching(ctx, t, regResp.Party, indexer.EventBurn,
		func(e *indexer.ParsedEvent) bool {
			return e.LedgerOffset > burnSinceOffset
		},
	)

	if ev.ContractID == "" {
		t.Error("contract_id: want non-empty")
	}
	if ev.EventType != indexer.EventBurn {
		t.Errorf("event_type: want BURN, got %s", ev.EventType)
	}
	if ev.LedgerOffset <= 0 {
		t.Errorf("ledger_offset: want > 0, got %d", ev.LedgerOffset)
	}
	if ev.FromPartyID == nil || *ev.FromPartyID != regResp.Party {
		t.Errorf("from_party_id: want %s, got %v", regResp.Party, ev.FromPartyID)
	}
	if ev.ExternalAddress == nil || !strings.EqualFold(*ev.ExternalAddress, evmDest) {
		t.Errorf("external_address: want %s, got %v", evmDest, ev.ExternalAddress)
	}

	// Verify total supply decreased after the burn.
	tokAfter, err := sys.Indexer.GetToken(ctx, admin, id)
	if err != nil {
		t.Fatalf("get token after burn: %v", err)
	}
	if !amtLT(tokAfter.TotalSupply, supplyBeforeBurn) {
		t.Errorf("total_supply did not decrease: before=%s after=%s",
			supplyBeforeBurn, tokAfter.TotalSupply)
	}
}
