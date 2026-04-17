//go:build e2e

package bridge_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// one18 is 1 × 10^18 — one full token unit expressed in wei (18-decimal tokens).
var one18 = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// TestDeposit_PROMPT_EthereumToCanton exercises the full EVM → Canton bridge
// deposit flow: a freshly funded account deposits PROMPT tokens, the relayer
// picks up the event, creates a PendingDeposit on Canton, and mints the
// corresponding PROMPT holding. The test asserts that the relayer records a
// completed transfer and that the Canton PROMPT balance reflects the deposit.
func TestDeposit_PROMPT_EthereumToCanton(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	depositAmount := new(big.Int).Set(one18)

	// SEQUENTIAL PREAMBLE — touches AnvilAccount0 nonce; must finish before t.Parallel().
	account := sys.DSL.NewFundedAccount(ctx, t, one18, tokenAddr, depositAmount)

	t.Parallel()

	// Register account so the api-server has a Canton party.
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	// Deposit 1 PROMPT (1e18 wei) into the bridge contract.
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)

	// Wait for the relayer to process the EVM deposit and complete it on Canton.
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())

	// Verify the Canton PROMPT balance via the indexer directly.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "1")

	// Also verify the balance is reflected through the api-server's /eth JSON-RPC
	// facade. This exercises the full path: indexer → token service → user store
	// EVM-address→party lookup → eth_call balanceOf response.
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.PROMPT, account.Address, "1")
}

// TestDeposit_SmallAmount_Succeeds verifies that a small PROMPT deposit (0.1
// tokens = 1e17 wei) is handled correctly end-to-end. This confirms there is no
// minimum-amount gate in the relayer or DAML bridge.
func TestDeposit_SmallAmount_Succeeds(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// 0.1 PROMPT = 1e17 wei
	depositAmount := new(big.Int).Div(one18, big.NewInt(10))

	// SEQUENTIAL PREAMBLE — fund a fresh isolated account before going parallel.
	account := sys.DSL.NewFundedAccount(ctx, t, one18, tokenAddr, depositAmount)

	t.Parallel()

	regResp := sys.DSL.RegisterUser(ctx, t, account)

	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)

	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "0.1")
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.PROMPT, account.Address, "0.1")
}

// TestDeposit_TwoDeposits_Accumulate verifies that two sequential deposits from
// the same address accumulate in the user's Canton balance. The relayer must
// process both events independently and the indexer must reflect the sum.
func TestDeposit_TwoDeposits_Accumulate(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// Fund with 2 PROMPT to cover both 1-PROMPT deposits.
	totalFund := new(big.Int).Mul(big.NewInt(2), one18)

	// SEQUENTIAL PREAMBLE — fund a fresh isolated account before going parallel.
	account := sys.DSL.NewFundedAccount(ctx, t, one18, tokenAddr, totalFund)

	t.Parallel()

	regResp := sys.DSL.RegisterUser(ctx, t, account)

	// First deposit: 1 PROMPT.
	tx1 := sys.DSL.Deposit(ctx, t, account, new(big.Int).Set(one18))
	sys.DSL.WaitForRelayerTransfer(ctx, t, tx1.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "1")

	// Second deposit: 1 PROMPT more.
	tx2 := sys.DSL.Deposit(ctx, t, account, new(big.Int).Set(one18))
	sys.DSL.WaitForRelayerTransfer(ctx, t, tx2.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.PROMPT, account.Address, "2")
}
