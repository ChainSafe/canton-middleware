//go:build e2e

package api_test

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
)

// signTransferHash decodes the 0x-prefixed transaction hash from PrepareResponse
// and returns a 0x-prefixed hex-encoded DER signature and the Canton fingerprint
// to set as SignedBy in ExecuteRequest.
func signTransferHash(t *testing.T, kp interface {
	SignDER([]byte) ([]byte, error)
	Fingerprint() (string, error)
}, txHashHex string) (sig, fingerprint string) {
	t.Helper()
	hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHashHex, "0x"))
	if err != nil {
		t.Fatalf("decode tx hash: %v", err)
	}
	derSig, err := kp.SignDER(hashBytes)
	if err != nil {
		t.Fatalf("sign tx hash: %v", err)
	}
	fp, err := kp.Fingerprint()
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	return "0x" + hex.EncodeToString(derSig), fp
}

// TestTransfer_DEMO_BetweenExternalUsers exercises the full DEMO transfer flow:
// register two external users, mint DEMO to User1, prepare+sign+execute a
// transfer to User2, and assert User2's balance via the api-server RPC.
func TestTransfer_DEMO_BetweenExternalUsers(t *testing.T) {
	sys := presets.NewFullStack(t)
	ctx := context.Background()

	resp1, kp1 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	_, _ = sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	mintAmount := "50"
	sys.DSL.MintDEMO(ctx, t, resp1.Party, mintAmount)
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.DEMO, sys.Accounts.User1.Address, mintAmount)

	transferAmount := "10"
	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: transferAmount,
		Token:  "DEMO",
	})
	if err != nil {
		t.Fatalf("prepare transfer: %v", err)
	}

	sig, fingerprint := signTransferHash(t, kp1, prepResp.TransactionHash)

	execResp, err := sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  sig,
		SignedBy:   fingerprint,
	})
	if err != nil {
		t.Fatalf("execute transfer: %v", err)
	}
	if execResp.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", execResp.Status)
	}

	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.DEMO, sys.Accounts.User2.Address, transferAmount)
}

// TestTransfer_CustodialUser_PrepareRejects verifies that calling PrepareTransfer
// for a custodial (web3) user returns HTTP 400 since the API requires external key mode.
func TestTransfer_CustodialUser_PrepareRejects(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	sys.DSL.RegisterUser(ctx, t, sys.Accounts.User1)

	_, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "1",
		Token:  "DEMO",
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for custodial user PrepareTransfer, got %v", err)
	}
}

// TestTransfer_InvalidAmount_Zero verifies that PrepareTransfer with amount "0"
// returns HTTP 400.
func TestTransfer_InvalidAmount_Zero(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	_, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "0",
		Token:  "DEMO",
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for zero amount, got %v", err)
	}
}

// TestTransfer_InvalidAmount_Negative verifies that PrepareTransfer with a
// negative amount returns HTTP 400.
func TestTransfer_InvalidAmount_Negative(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	_, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "-1",
		Token:  "DEMO",
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for negative amount, got %v", err)
	}
}

// TestTransfer_UnknownRecipient_Fails verifies that PrepareTransfer where the
// recipient EVM address is not registered returns HTTP 400.
func TestTransfer_UnknownRecipient_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	_, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     "0x000000000000000000000000000000000000dead",
		Amount: "1",
		Token:  "DEMO",
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for unregistered recipient, got %v", err)
	}
}

// TestTransfer_InvalidSignature_Fails verifies that ExecuteTransfer with a
// fingerprint that does not match the registered key returns HTTP 403.
func TestTransfer_InvalidSignature_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	resp1, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	_, _ = sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	sys.DSL.MintDEMO(ctx, t, resp1.Party, "10")
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.DEMO, sys.Accounts.User1.Address, "10")

	prepResp, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "1",
		Token:  "DEMO",
	})
	if err != nil {
		t.Fatalf("prepare transfer: %v", err)
	}

	_, err = sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  "0xdeadbeef",   // garbage signature
		SignedBy:   "0x1234567890", // garbage fingerprint — does not match registered key
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for fingerprint mismatch, got %v", err)
	}
}

// TestTransfer_InsufficientBalance_Fails verifies that PrepareTransfer for an
// amount exceeding the user's canton balance returns HTTP 400.
func TestTransfer_InsufficientBalance_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	resp1, _ := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	_, _ = sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	// Mint a small amount.
	sys.DSL.MintDEMO(ctx, t, resp1.Party, "1")
	sys.DSL.WaitForAPIBalance(ctx, t, sys.Tokens.DEMO, sys.Accounts.User1.Address, "1")

	// Try to transfer more than the minted balance.
	_, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "999999999",
		Token:  "DEMO",
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for insufficient balance, got %v", err)
	}
}
