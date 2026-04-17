//go:build e2e

package api_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// TestERC20Balance_UnregisteredAddress_ReturnsZero checks that the ERC-20
// balance of a fresh address is zero, exercising the /eth JSON-RPC facade.
func TestERC20Balance_UnregisteredAddress_ReturnsZero(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	// Use a deterministic but unused address.
	freshAddr := common.HexToAddress("0x000000000000000000000000000000000000dead")
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	bal, err := sys.APIServer.ERC20Balance(ctx, tokenAddr, freshAddr)
	if err != nil {
		t.Fatalf("erc20 balance: %v", err)
	}
	if bal.Sign() != 0 {
		t.Fatalf("expected zero balance for fresh address, got %s", bal)
	}
}

// TestGetBalance_AfterMintDEMO verifies that after minting DEMO tokens the
// api-server's /eth JSON-RPC facade (balanceOf on the DEMO virtual EVM
// address) reflects the new balance.
func TestGetBalance_AfterMintDEMO(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	resp, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	mintAmount := "100"
	sys.DSL.MintDEMO(ctx, t, resp.Party, mintAmount)

	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.DEMO, sys.Accounts.User1.Address, mintAmount)
}

// TestERC20Balance_AfterDeposit_ReflectsChange verifies that after depositing
// PROMPT tokens via the bridge, the bridge contract's PROMPT balance increases
// by at least the deposited amount.
func TestERC20Balance_AfterDeposit_ReflectsChange(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	bridgeAddr := common.HexToAddress(sys.Manifest.BridgeAddr)

	depositAmount := new(big.Int).Mul(big.NewInt(10), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	// SEQUENTIAL PREAMBLE — fund a fresh isolated account before going parallel.
	account := sys.DSL.NewFundedAccount(ctx, t, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), tokenAddr, depositAmount)

	// Check the bridge balance before deposit (still in preamble — no parallel interference yet).
	balBefore, err := sys.Anvil.ERC20Balance(ctx, tokenAddr, bridgeAddr)
	if err != nil {
		t.Fatalf("erc20 balance before: %v", err)
	}

	t.Parallel()

	// Register account so the bridge knows a Canton party for the recipient.
	sys.DSL.RegisterUser(ctx, t, account)

	sys.DSL.Deposit(ctx, t, account, depositAmount)

	// Bridge contract should hold at least depositAmount more PROMPT tokens.
	// Use >= rather than == because concurrent parallel tests may also deposit.
	balAfter, err := sys.Anvil.ERC20Balance(ctx, tokenAddr, bridgeAddr)
	if err != nil {
		t.Fatalf("erc20 balance after: %v", err)
	}

	diff := new(big.Int).Sub(balAfter, balBefore)
	if diff.Cmp(depositAmount) < 0 {
		t.Fatalf("expected bridge balance to increase by at least %s, got diff %s (before=%s after=%s)",
			depositAmount, diff, balBefore, balAfter)
	}
}
