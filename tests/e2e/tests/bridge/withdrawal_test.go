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
)

// TestWithdrawal_PROMPT_CantonToEthereum exercises the full Canton → EVM
// withdrawal flow:
//  1. Fund a fresh isolated account with 1 ETH and 2 PROMPT.
//  2. Register the account and deposit 2 PROMPT via the bridge.
//  3. Wait for the relayer to mint the Canton holding.
//  4. Initiate a withdrawal via the WayfinderBridgeConfig DAML choice.
//  5. Wait for the relayer to release tokens on Ethereum (EVM balance check).
func TestWithdrawal_PROMPT_CantonToEthereum(t *testing.T) {
	t.Parallel()

	sys := presets.NewFullStack(t)
	ctx := context.Background()

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	account := sys.DSL.NewFundedAccount(ctx, t, 1, tokenAddr, 2)

	regResp := sys.DSL.RegisterUser(ctx, t, account)

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
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.PROMPT, account.Address, "1")
}

// TestWithdrawal_PartialAmount verifies that withdrawing only part of the
// Canton holding leaves the remainder on Canton. After the withdrawal, the
// remaining Canton balance is >= the un-withdrawn portion.
func TestWithdrawal_PartialAmount(t *testing.T) {
	t.Parallel()

	sys := presets.NewFullStack(t)
	ctx := context.Background()

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID
	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)

	account := sys.DSL.NewFundedAccount(ctx, t, 1, tokenAddr, 3)

	regResp := sys.DSL.RegisterUser(ctx, t, account)

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
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.PROMPT, account.Address, "2")
}

// TestWithdrawal_AfterCantonTransfer verifies that a user who received PROMPT
// tokens via a Canton-native API transfer (not a bridge deposit) can withdraw
// those tokens to Ethereum. This exercises the bridge handling PROMPT holdings
// that were not directly created by the deposit flow.
//
// Flow:
//  1. Create a fresh funded sender account (1 ETH + 2 PROMPT).
//  2. Register sender as external (PrepareTransfer requires external key mode).
//  3. Register receiver as external (receives the Canton transfer).
//  4. Sender deposits 2 PROMPT via the bridge.
//  5. Transfer 1 PROMPT from sender to receiver via the api-server transfer API.
//  6. Receiver initiates a withdrawal of 1 PROMPT to their EVM address.
//  7. Relayer releases 1 PROMPT to receiver's EVM address.
//
// Sender is a freshly generated account funded via NewFundedAccount.
// Receiver is derived from t.Name() — unique per test run, no EVM funding
// needed since it only receives a Canton transfer and withdraws through the relayer.
func TestWithdrawal_AfterCantonTransfer(t *testing.T) {
	t.Parallel()

	sys := presets.NewFullStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	depositAmount := new(big.Int).Mul(big.NewInt(2), one18)

	// Fresh funded sender. NewFundedAccount serializes AnvilAccount0 nonce ops.
	sender := sys.DSL.NewFundedAccount(ctx, t, 1, tokenAddr, 2)

	// Receiver is derived per test — unique, no EVM funding needed.
	receiver := sys.Accounts.User2

	// Register sender as external (PrepareTransfer requires external key mode).
	regResp0, kp0 := sys.DSL.RegisterExternalUser(ctx, t, sender)

	// Register receiver as external.
	regResp1, _ := sys.DSL.RegisterExternalUser(ctx, t, receiver)

	admin := sys.Manifest.PromptInstrumentAdmin
	id := sys.Manifest.PromptInstrumentID

	// Deposit 2 PROMPT via the bridge into sender's Canton holding.
	txHash := sys.DSL.Deposit(ctx, t, sender, depositAmount)
	sys.DSL.WaitForRelayerTransfer(ctx, t, txHash.Hex())
	sys.DSL.WaitForCantonBalance(ctx, t, regResp0.Party, admin, id, "2")

	// Transfer 1 PROMPT from sender to receiver via the api-server.
	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &sender, &transfer.PrepareRequest{
		To:     receiver.Address.Hex(),
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

	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &sender, &transfer.ExecuteRequest{
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

	// Wait for receiver's Canton PROMPT balance to reflect the incoming transfer.
	sys.DSL.WaitForCantonBalance(ctx, t, regResp1.Party, admin, id, "1")

	// Receiver withdraws their 1 PROMPT to their own EVM address.
	sys.DSL.Withdraw(ctx, t, regResp1.Party, regResp1.Fingerprint, "PROMPT", "1", receiver.Address.Hex())

	// Verify the EVM release and that the api-server reports zero remaining balance
	// (receiver withdrew their entire PROMPT holding).
	sys.DSL.WaitForEthBalance(ctx, t, tokenAddr, receiver.Address, one18)
	sys.DSL.WaitForCantonBalance(ctx, t, regResp1.Party, admin, id, "0")
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.PROMPT, receiver.Address, "0")
}
