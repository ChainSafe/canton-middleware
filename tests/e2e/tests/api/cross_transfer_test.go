//go:build e2e

package api_test

import (
	"context"
	"testing"

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

// TODO: add test for same-participant USDCx transfer (P1 holder → P1 holder)
// once a P1-side USDCx holder flow is available.

// TODO: add test for P1 external party → P2 issuer transfer, which requires
// the Interactive Submission API to exercise choices as a secp256k1 party.
