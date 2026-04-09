//go:build e2e

package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/util"
)

const registerMessage = "register"

// TestRegister_Web3_Success verifies that a whitelisted EVM address can
// register in custodial (web3) mode via POST /register.
func TestRegister_Web3_Success(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	resp := sys.DSL.RegisterUser(ctx, t, sys.Accounts.User1)

	if resp.Party == "" {
		t.Fatal("expected non-empty Party in register response")
	}
	if resp.Fingerprint == "" {
		t.Fatal("expected non-empty Fingerprint in register response")
	}
}

// TestRegister_Duplicate_Idempotent verifies that registering the same EVM
// address a second time returns HTTP 409 Conflict. The api-server rejects
// duplicate registrations rather than silently returning the existing record.
func TestRegister_Duplicate_Idempotent(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	resp1 := sys.DSL.RegisterUser(ctx, t, sys.Accounts.User1)
	if resp1.Party == "" {
		t.Fatal("expected non-empty Party on first register")
	}

	// Second registration of the same address must return 409.
	sig, err := util.SignEIP191(sys.Accounts.User1.PrivateKey, registerMessage)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err = sys.APIServer.Register(ctx, &user.RegisterRequest{
		Signature: sig,
		Message:   registerMessage,
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusConflict {
		t.Fatalf("expected HTTP 409 on duplicate register, got %v", err)
	}
}

// TestRegister_NotWhitelisted_Fails verifies that a non-whitelisted EVM
// address is rejected with HTTP 403 Forbidden.
func TestRegister_NotWhitelisted_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	// Use AnvilAccount1 — not whitelisted.
	account := stack.Account{
		Address:    stack.AnvilAccount1.Address,
		PrivateKey: stack.AnvilAccount1.PrivateKey,
	}

	msg := registerMessage
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		t.Fatalf("sign register message: %v", err)
	}

	_, err = sys.APIServer.Register(ctx, &user.RegisterRequest{
		Signature: sig,
		Message:   msg,
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403, got %v", err)
	}
}

// TestRegister_InvalidSignature_Fails verifies that a signature signed with a
// different private key is rejected. The server recovers the wrong EVM address
// from the mismatched signature and rejects it as not whitelisted (HTTP 403).
func TestRegister_InvalidSignature_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	// Whitelist User1 but sign with User2's key — the server will recover User2's
	// address which is not whitelisted.
	if err := sys.Postgres.WhitelistAddress(ctx, sys.Accounts.User1.Address.Hex()); err != nil {
		t.Fatalf("whitelist: %v", err)
	}

	msg := registerMessage
	sig, err := util.SignEIP191(sys.Accounts.User2.PrivateKey, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = sys.APIServer.Register(ctx, &user.RegisterRequest{
		Signature: sig,
		Message:   msg,
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403, got %v", err)
	}
}

// TestRegister_MissingFields_Fails verifies that an empty POST /register body
// is rejected with HTTP 401 (missing signature and message).
func TestRegister_MissingFields_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	_, err := sys.APIServer.Register(ctx, &user.RegisterRequest{})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401, got %v", err)
	}
}

// TestRegister_ExternalUser_TwoStep_Success verifies the non-custodial
// two-step registration flow: prepare-topology → sign → register with external key.
func TestRegister_ExternalUser_TwoStep_Success(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	resp, kp := sys.DSL.RegisterExternalUser(ctx, t, sys.Accounts.User1)

	if resp.Party == "" {
		t.Fatal("expected non-empty Party in external register response")
	}
	if kp == nil {
		t.Fatal("expected non-nil CantonKeyPair from RegisterExternalUser")
	}
	// resp.Fingerprint is the EVM-address fingerprint (keccak256), not the
	// Canton key fingerprint returned by kp.Fingerprint(). Just verify it is
	// present; the key mode confirms this was an external registration.
	if resp.Fingerprint == "" {
		t.Fatal("expected non-empty Fingerprint in external register response")
	}
	if resp.KeyMode != user.KeyModeExternal {
		t.Fatalf("expected key_mode=%q, got %q", user.KeyModeExternal, resp.KeyMode)
	}
}

// TestRegister_ExternalUser_MissingTopologySignature_Fails verifies that
// step 2 of external registration is rejected with HTTP 400 when the topology
// signature is absent.
func TestRegister_ExternalUser_MissingTopologySignature_Fails(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	if err := sys.Postgres.WhitelistAddress(ctx, sys.Accounts.User1.Address.Hex()); err != nil {
		t.Fatalf("whitelist: %v", err)
	}

	kp, err := keys.GenerateCantonKeyPair()
	if err != nil {
		t.Fatalf("generate canton keypair: %v", err)
	}

	msg := registerMessage
	sig, err := util.SignEIP191(sys.Accounts.User1.PrivateKey, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	topoResp, err := sys.APIServer.PrepareTopology(ctx, &user.RegisterRequest{
		Signature:       sig,
		Message:         msg,
		CantonPublicKey: kp.PublicKeyHex(),
	})
	if err != nil {
		t.Fatalf("prepare-topology: %v", err)
	}

	// Attempt to register without providing TopologySignature.
	_, err = sys.APIServer.RegisterExternal(ctx, &user.RegisterRequest{
		Signature:         sig,
		Message:           msg,
		KeyMode:           user.KeyModeExternal,
		CantonPublicKey:   kp.PublicKeyHex(),
		RegistrationToken: topoResp.RegistrationToken,
		// TopologySignature intentionally omitted.
	})
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %v", err)
	}
}
