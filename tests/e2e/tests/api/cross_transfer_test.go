//go:build e2e

package api_test

import (
	"context"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// TestUSDCx_CrossParticipantTransfer_CustodialReceiver verifies that when
// USDCxIssuer (P2) transfers USDCx to a custodial P1 user, the accept worker
// automatically accepts the TransferOffer and the receiver's balance is credited
// without any explicit API accept call.
//
// Prerequisites: devstack must be running with USDCx bootstrapped on P2
// (docker-bootstrap.sh + bootstrap-usdcx.go run during docker compose up).
func TestUSDCx_CrossParticipantTransfer_CustodialReceiver(t *testing.T) {
	t.Parallel()

	sys := presets.NewMultiParticipantStack(t)
	ctx := context.Background()

	// Register User1 in custodial web3 mode; the accept worker handles acceptance
	// automatically by polling the indexer and calling AcceptTransferInstruction.
	regResp := sys.DSL.RegisterUser(ctx, t, sys.Accounts.User1)

	// Self-mint 20 USDCx to the P2 issuer party.
	if err := sys.Canton2.MintToken(ctx, sys.Manifest.USDCxInstrumentAdmin, "USDCx", "20"); err != nil {
		t.Fatalf("mint USDCx to issuer: %v", err)
	}

	// Cross-participant transfer: P2 issuer → User1's P1 party (10 USDCx).
	// Creates a TransferOffer; the accept worker auto-accepts for custodial parties.
	if err := sys.Canton2.TransferToken(ctx, sys.Manifest.USDCxInstrumentAdmin, regResp.Party, "USDCx", "10"); err != nil {
		t.Fatalf("transfer USDCx P2→P1: %v", err)
	}

	// The accept worker polls the indexer and auto-accepts the offer — just wait.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User1.Address, "10")
}

// TestUSDCx_CrossParticipantTransfer_NonCustodialReceiver verifies that when
// USDCxIssuer (P2) transfers USDCx to a non-custodial P1 user (registered with
// an external signing key), the receiver explicitly accepts the inbound
// TransferOffer via PrepareAccept/ExecuteAccept and the resulting Utility.Registry.Holding
// is reflected in the api-server's balance lookup.
//
// This is the manual-accept counterpart of TestUSDCx_CrossParticipantTransfer_CustodialReceiver
// (which uses the auto-accept worker) and exercises the indexer's Holding tracking
// independently of the on-chain P1→P1 transfer step covered by the InternalTransfer test.
func TestUSDCx_CrossParticipantTransfer_NonCustodialReceiver(t *testing.T) {
	t.Parallel()

	sys := presets.NewMultiParticipantStack(t)
	ctx := context.Background()

	// Register User1 with an external signing key so the test can sign the
	// PrepareAccept transaction hash that completes the inbound transfer.
	regResp, kp := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	// Self-mint 15 USDCx to the P2 issuer so there is enough to fund the receiver.
	if err := sys.Canton2.MintToken(ctx, sys.Manifest.USDCxInstrumentAdmin, "USDCx", "15"); err != nil {
		t.Fatalf("mint USDCx to issuer: %v", err)
	}

	// Cross-participant transfer: P2 issuer → User1's P1 party (10 USDCx).
	// Creates a TransferOffer that User1 must accept manually.
	if err := sys.Canton2.TransferToken(ctx, sys.Manifest.USDCxInstrumentAdmin, regResp.Party, "USDCx", "10"); err != nil {
		t.Fatalf("transfer USDCx P2→P1: %v", err)
	}

	// User1 sees the inbound offer (indexer must surface it cross-participant).
	cid := sys.DSL.WaitForIncomingTransferOffer(ctx, t, sys.Accounts.User1)

	// Manually accept: prepare → sign → execute.
	prep, err := sys.APIServer.PrepareAcceptTransfer(ctx, &sys.Accounts.User1, cid,
		&transfer.PrepareAcceptRequest{InstrumentAdmin: sys.Manifest.USDCxInstrumentAdmin})
	if err != nil {
		t.Fatalf("prepare accept: %v", err)
	}
	sig, fp := signTransferHash(t, kp, prep.TransactionHash)
	if _, err = sys.APIServer.ExecuteAcceptTransfer(ctx, &sys.Accounts.User1, cid,
		&transfer.ExecuteRequest{TransferID: prep.TransferID, Signature: sig, SignedBy: fp}); err != nil {
		t.Fatalf("execute accept: %v", err)
	}

	// Once accept lands, the indexer should observe the new Holding owned by
	// User1 and the api-server's balance lookup should return 10 USDCx.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User1.Address, "10")
}

// TestUSDCx_InternalTransfer_P1HolderToP1Holder verifies that USDCx can be
// transferred between two non-custodial P1 holders using the api-server's
// PrepareTransfer / ExecuteTransfer flow, and that each receiver explicitly
// accepts their inbound TransferOffer via the incoming accept API.
//
// Setup: seed User1 with USDCx via a cross-participant P2 issuer → P1 holder
// transfer. User1 accepts via the incoming accept API, then transfers 10 USDCx
// to User2, who also accepts via the incoming accept API.
func TestUSDCx_InternalTransfer_P1HolderToP1Holder(t *testing.T) {
	t.Parallel()

	sys := presets.NewMultiParticipantStack(t)
	ctx := context.Background()

	// Register both P1 holders in external key mode so they can sign Canton
	// transaction hashes for PrepareTransfer and PrepareAcceptTransfer.
	regResp1, kp1 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	_, kp2 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	// Self-mint 20 USDCx to the P2 issuer so there is enough to fund User1.
	if err := sys.Canton2.MintToken(ctx, sys.Manifest.USDCxInstrumentAdmin, "USDCx", "20"); err != nil {
		t.Fatalf("mint USDCx to issuer: %v", err)
	}

	// Cross-participant transfer: P2 issuer → User1's P1 party (15 USDCx).
	// Creates a TransferOffer that User1 must accept via the incoming accept API.
	if err := sys.Canton2.TransferToken(ctx, sys.Manifest.USDCxInstrumentAdmin, regResp1.Party, "USDCx", "15"); err != nil {
		t.Fatalf("transfer USDCx P2→P1 (User1): %v", err)
	}

	// User1 accepts the inbound offer via the api-server incoming accept flow.
	cid1 := sys.DSL.WaitForIncomingTransferOffer(ctx, t, sys.Accounts.User1)
	prepAccept1, err := sys.APIServer.PrepareAcceptTransfer(ctx, &sys.Accounts.User1, cid1,
		&transfer.PrepareAcceptRequest{InstrumentAdmin: sys.Manifest.USDCxInstrumentAdmin})
	if err != nil {
		t.Fatalf("prepare accept (User1): %v", err)
	}
	sig1, fp1 := signTransferHash(t, kp1, prepAccept1.TransactionHash)
	_, err = sys.APIServer.ExecuteAcceptTransfer(ctx, &sys.Accounts.User1, cid1,
		&transfer.ExecuteRequest{TransferID: prepAccept1.TransferID, Signature: sig1, SignedBy: fp1})
	if err != nil {
		t.Fatalf("execute accept (User1): %v", err)
	}

	// Wait for User1's balance before preparing the same-participant transfer.
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
	sig, fp := signTransferHash(t, kp1, prepResp.TransactionHash)
	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  sig,
		SignedBy:   fp,
	})
	if err != nil {
		t.Fatalf("execute USDCx P1→P1 transfer: %v", err)
	}
	if execResp.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", execResp.Status)
	}

	// User2 accepts the inbound offer via the api-server incoming accept flow.
	cid2 := sys.DSL.WaitForIncomingTransferOffer(ctx, t, sys.Accounts.User2)
	prepAccept2, err := sys.APIServer.PrepareAcceptTransfer(ctx, &sys.Accounts.User2, cid2,
		&transfer.PrepareAcceptRequest{InstrumentAdmin: sys.Manifest.USDCxInstrumentAdmin})
	if err != nil {
		t.Fatalf("prepare accept (User2): %v", err)
	}
	sig2, fp2 := signTransferHash(t, kp2, prepAccept2.TransactionHash)
	_, err = sys.APIServer.ExecuteAcceptTransfer(ctx, &sys.Accounts.User2, cid2,
		&transfer.ExecuteRequest{TransferID: prepAccept2.TransferID, Signature: sig2, SignedBy: fp2})
	if err != nil {
		t.Fatalf("execute accept (User2): %v", err)
	}

	// Verify final balances: User2 received 10, User1 retained 5.
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User2.Address, "10")
	sys.DSL.WaitForAPIBalance(ctx, t, &sys.Tokens.USDCx, sys.Accounts.User1.Address, "5")
}
