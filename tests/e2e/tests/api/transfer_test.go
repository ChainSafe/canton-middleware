//go:build e2e

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/util"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
// valid-format DER signature produced by the wrong Canton key is rejected with
// HTTP 403. User1 is registered with kp1; the transfer is signed with a fresh
// kp2 — the server finds User1's record via the correct fingerprint but fails
// signature verification because kp2 ≠ kp1.
func TestTransfer_InvalidSignature_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	resp1, kp1 := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
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

	// Generate a second keypair and sign the tx hash with it — produces a
	// valid DER signature but from the wrong key. Submit it with kp1's
	// fingerprint so the server finds User1's record but fails verification.
	kp2, err := keys.GenerateCantonKeyPair()
	if err != nil {
		t.Fatalf("generate wrong keypair: %v", err)
	}
	wrongSig, _ := signTransferHash(t, kp2, prepResp.TransactionHash)
	fp1, err := kp1.Fingerprint()
	if err != nil {
		t.Fatalf("compute kp1 fingerprint: %v", err)
	}

	_, err = sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  wrongSig, // valid DER sig but from the wrong key
		SignedBy:   fp1,      // correct fingerprint — server finds User1 but sig fails
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for wrong-key signature, got %v", err)
	}
}

// TestTransfer_InsufficientBalance_Fails verifies that PrepareTransfer for an
// amount exceeding the user's canton balance returns HTTP 400.
func TestTransfer_InsufficientBalance_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
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

// TestTransfer_MissingAuthHeaders_Returns401 verifies that POST
// /api/v2/transfer/prepare without X-Signature and X-Message headers returns
// HTTP 401. This exercises the authentication gate before any business logic.
func TestTransfer_MissingAuthHeaders_Returns401(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	body, err := json.Marshal(&transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "1",
		Token:  "DEMO",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		sys.APIServer.Endpoint()+"/api/v2/transfer/prepare", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Intentionally no X-Signature or X-Message headers.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST prepare: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for missing auth headers, got %d", resp.StatusCode)
	}
}

// TestTransfer_ExpiredTimestamp_Returns401 verifies that a transfer request
// signed with a timestamp more than 5 minutes old is rejected with HTTP 401.
// This exercises the replay-protection window enforced by ValidateTimedMessage.
func TestTransfer_ExpiredTimestamp_Returns401(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	// Build a message with a timestamp 10 minutes in the past (> 5-minute maxAge).
	oldMsg := fmt.Sprintf("transfer:%d", time.Now().Add(-10*time.Minute).Unix())
	sig, err := util.SignEIP191(sys.Accounts.User1.PrivateKey, oldMsg)
	if err != nil {
		t.Fatalf("sign expired message: %v", err)
	}

	body, err := json.Marshal(&transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "1",
		Token:  "DEMO",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		sys.APIServer.Endpoint()+"/api/v2/transfer/prepare", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Message", oldMsg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST prepare: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for expired timestamp, got %d", resp.StatusCode)
	}
}

// TestTransfer_Execute_MissingFields_Returns400 verifies that ExecuteTransfer
// with an empty request body (missing transfer_id, signature, signed_by) returns
// HTTP 400. Auth headers are valid — the validation happens after authentication.
func TestTransfer_Execute_MissingFields_Returns400(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	// No registration needed: auth recovers the EVM address from the signature
	// without a user-store lookup; the field-validation check fires first.
	_, err := sys.APIServer.ExecuteTransfer(ctx, &sys.Accounts.User1, &transfer.ExecuteRequest{
		// TransferID, Signature, and SignedBy intentionally left empty.
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for missing execute fields, got %v", err)
	}
}

// TestTransfer_PROMPT_InsufficientBalance_Fails verifies that PrepareTransfer
// for the PROMPT token with no Canton balance returns HTTP 400, confirming that
// PROMPT is a recognized token symbol and the balance gate fires correctly.
func TestTransfer_PROMPT_InsufficientBalance_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)
	sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User2)

	// User1 has zero PROMPT Canton balance (no deposit + relayer flow run).
	_, err := sys.APIServer.PrepareTransfer(ctx, &sys.Accounts.User1, &transfer.PrepareRequest{
		To:     sys.Accounts.User2.Address.Hex(),
		Amount: "1",
		Token:  "PROMPT",
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for PROMPT insufficient balance, got %v", err)
	}
}
