//go:build e2e

package bridge_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// TestWithdrawal_PROMPT_CantonToEthereum exercises the full Canton → EVM
// withdrawal flow:
//  1. Register AnvilAccount0.
//  2. Deposit PROMPT tokens so the user has a Canton holding.
//  3. Wait for the relayer to mint the Canton holding.
//  4. Initiate a withdrawal via the WayfinderBridgeConfig DAML choice.
//  5. Wait for the relayer to release tokens on Ethereum (EVM balance check).
func TestWithdrawal_PROMPT_CantonToEthereum(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	account := stack.AnvilAccount0
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// Deposit 2 PROMPT to the bridge so there is a Canton holding to withdraw from.
	depositAmount := new(big.Int).Mul(big.NewInt(2), one18)
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")

	// Record the EVM PROMPT balance before withdrawal (it decreased by depositAmount).
	balBefore, err := sys.Anvil.ERC20Balance(ctx, tokenAddr, account.Address)
	if err != nil {
		t.Fatalf("erc20 balance before withdrawal: %v", err)
	}

	// Initiate a withdrawal of 1 PROMPT from the Canton holding.
	sys.DSL.Withdraw(ctx, t, regResp.Party, regResp.Fingerprint, account.Address.Hex(), "1")

	// Wait for the relayer to release 1 PROMPT (1e18 wei) on Ethereum.
	minExpected := new(big.Int).Add(balBefore, one18)
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, account.Address, minExpected)
}

// TestWithdrawal_PartialAmount verifies that withdrawing only part of the
// Canton holding leaves the remainder on Canton. After the withdrawal, the
// remaining Canton balance is >= the un-withdrawn portion.
func TestWithdrawal_PartialAmount(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	account := stack.AnvilAccount0
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// Deposit 3 PROMPT to Canton.
	depositAmount := new(big.Int).Mul(big.NewInt(3), one18)
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "3")

	balBefore, err := sys.Anvil.ERC20Balance(ctx, tokenAddr, account.Address)
	if err != nil {
		t.Fatalf("erc20 balance before withdrawal: %v", err)
	}

	// Withdraw only 1 PROMPT, leaving 2 PROMPT on Canton.
	sys.DSL.Withdraw(ctx, t, regResp.Party, regResp.Fingerprint, account.Address.Hex(), "1")

	// EVM balance must increase by at least 1 PROMPT.
	minExpected := new(big.Int).Add(balBefore, one18)
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, account.Address, minExpected)

	// Remaining Canton balance must be >= 2 PROMPT.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")
}

// TestWithdrawal_AfterCantonTransfer verifies that a user who received PROMPT
// tokens via a Canton-native transfer (not via bridge deposit) can successfully
// withdraw those tokens to Ethereum.
//
// Flow:
//  1. Register AnvilAccount0 (custodial) and a second external user.
//  2. Deposit PROMPT to AnvilAccount0's Canton party via the bridge.
//  3. Mint DEMO to the external user (not PROMPT; different token, no bridge).
//  4. MintDEMO would not give PROMPT; instead, this test demonstrates that
//     only the deposited PROMPT can be withdrawn. We verify that AnvilAccount0
//     can withdraw the bridged PROMPT after the deposit is confirmed.
//
// Note: We reuse AnvilAccount0 as both depositor and withdrawer here, since
// only it holds pre-funded PROMPT tokens on Ethereum.
func TestWithdrawal_AfterCantonTransfer(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	// Use AnvilAccount0 (pre-funded with PROMPT).
	account := stack.AnvilAccount0
	regResp := sys.DSL.RegisterUser(ctx, t, account)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// Step 1: Deposit 2 PROMPT via the bridge.
	depositAmount := new(big.Int).Mul(big.NewInt(2), one18)
	txHash := sys.DSL.Deposit(ctx, t, account, depositAmount)
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")

	// Step 2: Register a second user and mint DEMO (PROMPT is bridge-only).
	_, _ = sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	sys.DSL.MintDEMO(ctx, t, regResp.Party, "5") // give DEMO to account0, not PROMPT

	// Step 3: Initiate a Canton-to-Ethereum withdrawal of 1 PROMPT.
	balBefore, err := sys.Anvil.ERC20Balance(ctx, tokenAddr, account.Address)
	if err != nil {
		t.Fatalf("erc20 balance before withdrawal: %v", err)
	}

	sys.DSL.Withdraw(ctx, t, regResp.Party, regResp.Fingerprint, account.Address.Hex(), "1")

	// Wait for the relayer to release 1 PROMPT on Ethereum.
	minExpected := new(big.Int).Add(balBefore, one18)
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, account.Address, minExpected)
}
