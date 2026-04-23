//go:build e2e

package bridge_test

import (
	"context"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/pkg/transfer"
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
	sys.DSL.Withdraw(ctx, t, regResp.Party, regResp.Fingerprint, "PROMPT", "1", account.Address.Hex())

	// Wait for the relayer to release 1 PROMPT (1e18 wei) on Ethereum.
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, account.Address, new(big.Int).Add(balBefore, one18))

	// The remaining 1 PROMPT should still be visible on Canton and via the api-server.
	// WaitForAPIBalance uses the same indexer data source as WaitForCantonBalance but
	// exercises the additional api-server path: user-store EVM→party lookup + eth_call facade.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "1")
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.PROMPT, account.Address, "1")
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
	sys.DSL.Withdraw(ctx, t, regResp.Party, regResp.Fingerprint, "PROMPT", "1", account.Address.Hex())

	// EVM balance must increase by at least 1 PROMPT.
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, account.Address, new(big.Int).Add(balBefore, one18))

	// The remaining 2 PROMPT must be visible both on Canton and via the api-server.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp.Party, admin, id, "2")
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.PROMPT, account.Address, "2")
}

// TestWithdrawal_AfterCantonTransfer verifies that a user who received PROMPT
// tokens via a Canton-native API transfer (not a bridge deposit) can withdraw
// those tokens to Ethereum. This exercises the bridge handling PROMPT holdings
// that were not directly created by the deposit flow.
//
// Flow:
//  1. Register AnvilAccount0 as an external user and deposit 2 PROMPT.
//  2. Register User1 as an external user (receives the Canton transfer).
//  3. Transfer 1 PROMPT from Account0 to User1 via the api-server transfer API.
//  4. User1 initiates a withdrawal of 1 PROMPT to their EVM address.
//  5. Relayer releases 1 PROMPT to User1's EVM address.
func TestWithdrawal_AfterCantonTransfer(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	// Register AnvilAccount0 as external (PrepareTransfer requires external key mode).
	regResp0, kp0 := sys.DSL.RegisterExternalUser(ctx, t, stack.AnvilAccount0)

	// Register a second external user who will receive the Canton transfer.
	regResp1, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	// Deposit 2 PROMPT via the bridge into Account0's Canton holding.
	depositAmount := new(big.Int).Mul(big.NewInt(2), one18)
	txHash := sys.DSL.Deposit(ctx, t, stack.AnvilAccount0, depositAmount)
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp0.Party, admin, id, "2")

	// Transfer 1 PROMPT from Account0 to User1 via the api-server.
	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &stack.AnvilAccount0, &transfer.PrepareRequest{
		To:     sys.Accounts.User1.Address.Hex(),
		Amount: "1",
		Token:  "PROMPT",
	})
	if err != nil {
		t.Fatalf("prepare transfer: %v", err)
	}

	hashBytes, err := hex.DecodeString(strings.TrimPrefix(prepResp.TransactionHash, "0x"))
	if err != nil {
		t.Fatalf("decode tx hash: %v", err)
	}
	derSig, err := kp0.SignDER(hashBytes)
	if err != nil {
		t.Fatalf("sign tx hash: %v", err)
	}
	fp, err := kp0.Fingerprint()
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &stack.AnvilAccount0, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  "0x" + hex.EncodeToString(derSig),
		SignedBy:   fp,
	})
	if err != nil {
		t.Fatalf("execute transfer: %v", err)
	}
	if execResp.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", execResp.Status)
	}

	// Wait for User1's Canton PROMPT balance to reflect the incoming transfer.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp1.Party, admin, id, "1")

	// User1 withdraws their 1 PROMPT to their own EVM address.
	// Their EVM address starts at 0 PROMPT balance (derived account, not pre-funded).
	sys.DSL.Withdraw(ctx, t, regResp1.Party, regResp1.Fingerprint, "PROMPT", "1", sys.Accounts.User1.Address.Hex())

	// Verify the EVM release and that the api-server reports zero remaining balance
	// (User1 withdrew their entire PROMPT holding).
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, sys.Accounts.User1.Address, one18)
	sys.DSL.WaitForCantonBalance(ctx, t, regResp1.Party, admin, id, "0")
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.PROMPT, sys.Accounts.User1.Address, "0")
}
