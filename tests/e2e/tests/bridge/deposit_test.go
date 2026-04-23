//go:build e2e

package bridge_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// one18 is 1 × 10^18 — one full token unit expressed in wei (18-decimal tokens).
var one18 = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// TestDeposit_PROMPT_EthereumToCanton exercises the full EVM → Canton bridge
// deposit flow: AnvilAccount0 deposits PROMPT tokens, the relayer picks up the
// event, creates a PendingDeposit on Canton, and mints the corresponding PROMPT
// holding. The test asserts that the relayer records a completed transfer and
// that the Canton PROMPT balance reflects the deposit.
func TestDeposit_PROMPT_EthereumToCanton(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	// AnvilAccount0 is pre-funded with PROMPT tokens and ETH for gas.
	account := stack.AnvilAccount0

	// Register account so the api-server has a Canton party.
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	// Record the index-side admin and instrument ID for balance polling.
	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID

	// Deposit 1 PROMPT (1e18 wei) into the bridge contract.
	depositAmount := new(big.Int).Set(one18)
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)

	// Wait for the relayer to process the EVM deposit and complete it on Canton.
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())

	// Verify the Canton PROMPT balance via the indexer directly.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "1")

	// Also verify the balance is reflected through the api-server's /eth JSON-RPC
	// facade. This exercises the full path: indexer → token service → user store
	// EVM-address→party lookup → eth_call balanceOf response.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.PROMPT, account.Address, "1")
}

// TestDeposit_SmallAmount_Succeeds verifies that a small PROMPT deposit (0.1
// tokens = 1e17 wei) is handled correctly end-to-end. This confirms there is no
// minimum-amount gate in the relayer or DAML bridge.
//
// Uses AnvilAccount1 to avoid balance accumulation from TestDeposit_PROMPT_EthereumToCanton,
// which runs before this test and deposits from AnvilAccount0.
func TestDeposit_SmallAmount_Succeeds(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	account := stack.AnvilAccount1
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID

	// 0.1 PROMPT = 1e17 wei
	depositAmount := new(big.Int).Div(one18, big.NewInt(10))
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)

	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "0.1")
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.PROMPT, account.Address, "0.1")
}

// TestDeposit_TwoDeposits_Accumulate verifies that two sequential deposits from
// the same address accumulate in the user's Canton balance. The relayer must
// process both events independently and the indexer must reflect the sum.
//
// NOTE: This test shares AnvilAccount0 with TestDeposit_PROMPT_EthereumToCanton.
// Canton holdings for a party persist across tests, so AnvilAccount0's balance
// will already be >= 1 PROMPT when this test runs. The WaitForCantonBalance /
// WaitForAPIBalance assertions use >=, so they tolerate that pre-existing balance.
// What this test actually verifies is that each of the two deposits it submits
// is individually processed by the relayer and reflected in the balance.
func TestDeposit_TwoDeposits_Accumulate(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	account := stack.AnvilAccount0
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID

	// First deposit: 1 PROMPT. Balance check uses >= so prior accumulated
	// holdings from other tests using AnvilAccount0 do not cause a false failure.
	tx1 := sys.DSL.Deposit(ctx, t, account, new(big.Int).Set(one18))
	sys.DSL.WaitForRelayerTransfer(ctx, t, tx1.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "1")

	// Second deposit: 1 PROMPT more.
	tx2 := sys.DSL.Deposit(ctx, t, account, new(big.Int).Set(one18))
	sys.DSL.WaitForRelayerTransfer(ctx, t, tx2.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.PROMPT, account.Address, "2")
}
