//go:build e2e

package api_test

import (
	"context"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// TestUSDCx_CrossParticipantTransfer_P2ToP1 verifies that when USDCxIssuer (P2)
// transfers USDCx directly to a P1-registered party, the indexer and api-server
// both reflect the receiver's balance — without any api-server bridge involvement.
//
// This replicates the exact scenario that triggered the negative-balance infinite
// retry bug (issue #245): the self-mint on P2 is invisible to the P1 indexer
// (no P1 stakeholders), but the subsequent transfer IS visible because the
// receiver is a P1 party. The processor must skip the sender's deduction rather
// than failing.
//
// Prerequisites: devstack must be running with USDCx bootstrapped on P2
// (docker-bootstrap.sh + bootstrap-usdcx.go run during docker compose up).
func TestUSDCx_CrossParticipantTransfer_P2ToP1(t *testing.T) {
	t.Skip("will fail until we fix usdcx transfer flow")
	t.Parallel()

	sys := presets.NewMultiParticipantStack(t)
	ctx := context.Background()

	// Register a P1 holder via api-server (external key mode) so the api-server
	// balance lookup has a known canton_party_id → EVM address mapping.
	regResp, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	// Self-mint 20 USDCx to the issuer party on P2 so there is a holding large
	// enough to cover the 10 USDCx transfer below.
	if err := sys.Canton2.MintToken(ctx, sys.Manifest.USDCxInstrumentAdmin, "USDCx", "20"); err != nil {
		t.Fatalf("mint USDCx to issuer: %v", err)
	}

	// Check the initial balance of the user - should be zero
	b, err := sys.APIServer.ERC20Balance(ctx, sys.Tokens.USDCx.Address, sys.Accounts.User1.Address)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if b.Sign() != 0 {
		t.Fatalf("expected zero balance, got %s", b)
	}

	// Transfer 10 USDCx from P2 issuer to the P1 holder. The indexer will
	// observe the TokenTransferEvent and credit the receiver's balance. The
	// sender's deduction is silently skipped (cross-participant incomplete
	// history — see issue #245 and processor.go ErrNegativeBalance handling).
	if err := sys.Canton2.TransferToken(ctx, sys.Manifest.USDCxInstrumentAdmin, regResp.Party, "USDCx", "10"); err != nil {
		t.Fatalf("transfer USDCx P2→P1: %v", err)
	}

	// Verify the receiver balance in the indexer.
	sys.DSL.WaitForCantonBalance(ctx, t,
		regResp.Party, sys.Manifest.USDCxInstrumentAdmin, sys.Manifest.USDCxInstrumentID, "10")

	// Verify the receiver balance in the api-server's /eth JSON-RPC facade.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User1.Address, "10")
}

// TestUSDCx_InternalTransfer_P1HolderToP1Holder verifies that USDCx can be
// transferred between two P1-registered parties using the api-server's
// PrepareTransfer / ExecuteTransfer flow.
//
// Setup: seed User1 with USDCx via a cross-participant P2 issuer → P1 holder
// transfer (the same mechanism exercised by TestUSDCx_CrossParticipantTransfer_P2ToP1).
// Once User1 holds USDCx on P1, the api-server's transfer API is used to move
// a portion to User2, exercising the same-participant P1 → P1 path through the
// CIP56TransferFactory visible to both parties on the shared synchronizer.
func TestUSDCx_InternalTransfer_P1HolderToP1Holder(t *testing.T) {
	t.Skip("will fail until we fix usdcx transfer flow")
	t.Parallel()

	sys := presets.NewMultiParticipantStack(t)
	ctx := context.Background()

	// Register two P1 holders in external key mode so PrepareTransfer is available
	// to them (custodial users cannot sign Canton transaction hashes).
	regResp1, kp1 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	_, _ = sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	// Self-mint 20 USDCx to the P2 issuer so there is enough to fund User1.
	if err := sys.Canton2.MintToken(ctx, sys.Manifest.USDCxInstrumentAdmin, "USDCx", "20"); err != nil {
		t.Fatalf("mint USDCx to issuer: %v", err)
	}

	// Cross-participant transfer: P2 issuer → User1's P1 party (15 USDCx).
	// This gives User1 a USDCx holding on P1 that the api-server can query.
	if err := sys.Canton2.TransferToken(ctx, sys.Manifest.USDCxInstrumentAdmin, regResp1.Party, "USDCx", "15"); err != nil {
		t.Fatalf("transfer USDCx P2→P1 (User1): %v", err)
	}

	// Wait for User1's balance to appear before preparing the P1→P1 transfer.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User1.Address, "15")

	// Prepare a same-participant transfer of 10 USDCx from User1 to User2.
	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "10",
		Token:  "USDCx",
	})
	if err != nil {
		t.Fatalf("prepare USDCx P1→P1 transfer: %v", err)
	}

	sig, fingerprint := signTransferHash(t, kp1, prepResp.TransactionHash)

	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  sig,
		SignedBy:   fingerprint,
	})
	if err != nil {
		t.Fatalf("execute USDCx P1→P1 transfer: %v", err)
	}
	if execResp.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", execResp.Status)
	}

	// Verify User2 received the tokens.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User2.Address, "10")
	// Verify User1's balance was deducted (15 - 10 = 5).
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User1.Address, "5")
}

// TODO: add test for P1 external party → P2 issuer transfer, which requires
// the Interactive Submission API to exercise choices as a secp256k1 party.
