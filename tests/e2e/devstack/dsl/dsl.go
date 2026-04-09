//go:build e2e

// Package dsl provides high-level test operations built on top of the stack
// service interfaces. Methods accept *testing.T and call t.Fatal on error so
// tests read as plain imperative steps without error-handling boilerplate.
package dsl

import (
	"context"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/util"
)

// DSL exposes high-level operations over the service interfaces. Accessed via
// System.DSL.
type DSL struct {
	apiServer stack.APIServer
	canton    stack.Canton
	relayer   stack.Relayer
	indexer   stack.Indexer
	postgres  stack.APIDatabase
	anvil     stack.Anvil
}

const (
	decimalBase              = 10
	waitForAPIBalanceTimeout = 60 * time.Second
)

// New wires a DSL to the given service interfaces. canton, relayer, and indexer
// may be nil when the system under test does not include those services; calling
// DSL methods that require them will produce a descriptive t.Fatal message.
func New(
	api stack.APIServer,
	canton stack.Canton,
	relayer stack.Relayer,
	indexer stack.Indexer,
	postgres stack.APIDatabase,
	anvil stack.Anvil,
) *DSL {
	return &DSL{
		apiServer: api,
		canton:    canton,
		relayer:   relayer,
		indexer:   indexer,
		postgres:  postgres,
		anvil:     anvil,
	}
}

// RegisterUser whitelists the account's EVM address and registers it as a
// custodial web3 user via POST /register. Returns the RegisterResponse.
func (d *DSL) RegisterUser(ctx context.Context, t *testing.T, account stack.Account) *user.RegisterResponse {
	t.Helper()

	if err := d.postgres.WhitelistAddress(ctx, account.Address.Hex()); err != nil {
		t.Fatalf("whitelist %s: %v", account.Address.Hex(), err)
	}

	msg := "register"
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		t.Fatalf("sign register message: %v", err)
	}

	resp, err := d.apiServer.Register(ctx, &user.RegisterRequest{
		Signature: sig,
		Message:   msg,
	})
	if err != nil {
		t.Fatalf("register %s: %v", account.Address.Hex(), err)
	}
	return resp
}

// RegisterExternalUser whitelists account's EVM address and completes the
// two-step external (non-custodial) registration flow:
//  1. Generates a fresh secp256k1 Canton keypair.
//  2. Calls POST /register/prepare-topology to get the topology hash.
//  3. Signs the topology hash with the Canton key (DER, SHA-256).
//  4. Calls POST /register with key_mode=external.
//
// Returns the RegisterResponse and the Canton keypair (needed to sign transfers).
func (d *DSL) RegisterExternalUser(ctx context.Context, t *testing.T, account stack.Account) (*user.RegisterResponse, *keys.CantonKeyPair) {
	t.Helper()

	if err := d.postgres.WhitelistAddress(ctx, account.Address.Hex()); err != nil {
		t.Fatalf("whitelist %s: %v", account.Address.Hex(), err)
	}

	kp, err := keys.GenerateCantonKeyPair()
	if err != nil {
		t.Fatalf("generate canton keypair: %v", err)
	}

	msg := "register"
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		t.Fatalf("sign register message: %v", err)
	}

	topoResp, err := d.apiServer.PrepareTopology(ctx, &user.RegisterRequest{
		Signature:       sig,
		Message:         msg,
		CantonPublicKey: kp.PublicKeyHex(),
	})
	if err != nil {
		t.Fatalf("prepare-topology %s: %v", account.Address.Hex(), err)
	}

	// TopologyHash is "0x" + hex(multiHash). Sign raw bytes (SignDER SHA-256s internally).
	hashBytes, err := hex.DecodeString(strings.TrimPrefix(topoResp.TopologyHash, "0x"))
	if err != nil {
		t.Fatalf("decode topology hash: %v", err)
	}
	derSig, err := kp.SignDER(hashBytes)
	if err != nil {
		t.Fatalf("sign topology hash: %v", err)
	}
	topologySig := "0x" + hex.EncodeToString(derSig)

	resp, err := d.apiServer.RegisterExternal(ctx, &user.RegisterRequest{
		Signature:         sig,
		Message:           msg,
		KeyMode:           user.KeyModeExternal,
		CantonPublicKey:   kp.PublicKeyHex(),
		RegistrationToken: topoResp.RegistrationToken,
		TopologySignature: topologySig,
	})
	if err != nil {
		t.Fatalf("register-external %s: %v", account.Address.Hex(), err)
	}
	return resp, kp
}

// MintDEMO mints amount of DEMO tokens to recipientParty via the Canton
// ledger (IssuerMint DAML choice). Requires a Canton shim with token client.
func (d *DSL) MintDEMO(ctx context.Context, t *testing.T, recipientParty, amount string) {
	t.Helper()
	if d.canton == nil {
		t.Fatal("MintDEMO not available: Canton shim not initialized")
		return
	}
	if err := d.canton.MintToken(ctx, recipientParty, "DEMO", amount); err != nil {
		t.Fatalf("mint DEMO to %s: %v", recipientParty, err)
	}
}

// Deposit approves the bridge and submits a depositToCanton transaction on
// behalf of account. Returns the deposit transaction hash.
func (d *DSL) Deposit(ctx context.Context, t *testing.T, account stack.Account, amount *big.Int) common.Hash {
	t.Helper()
	hash, err := d.anvil.ApproveAndDeposit(ctx, &account, amount)
	if err != nil {
		t.Fatalf("deposit for %s: %v", account.Address.Hex(), err)
	}
	return hash
}

// WaitForAPIBalance polls the api-server's /eth JSON-RPC facade until the
// ERC-20 balance of ownerAddr for tok is >= minTokens (human-readable token
// amount, e.g. "50"). This is the preferred balance check for api-server tests
// — no indexer needed. Pass sys.Tokens.DEMO or sys.Tokens.PROMPT as tok.
func (d *DSL) WaitForAPIBalance(ctx context.Context, t *testing.T, tok stack.Token, ownerAddr common.Address, minTokens string) {
	t.Helper()
	// Scale minTokens by 10^tok.Decimals.
	exp := new(big.Int).Exp(big.NewInt(decimalBase), big.NewInt(int64(tok.Decimals)), nil)
	minF, ok := new(big.Float).SetString(minTokens)
	if !ok {
		t.Fatalf("WaitForAPIBalance: invalid amount %q", minTokens)
	}
	minF.Mul(minF, new(big.Float).SetInt(exp))
	minWei, _ := minF.Int(nil)

	deadline := time.Now().Add(waitForAPIBalanceTimeout)
	for time.Now().Before(deadline) {
		bal, err := d.apiServer.ERC20Balance(ctx, tok.Address, ownerAddr)
		if err != nil {
			t.Fatalf("WaitForAPIBalance: erc20 balance: %v", err)
		}
		if bal.Cmp(minWei) >= 0 {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("WaitForAPIBalance: timed out waiting for %s %s balance >= %s (owner %s)",
		minTokens, tok.Symbol, minWei, ownerAddr.Hex())
}

// ERC20Balance returns the on-chain ERC-20 balance of account for tokenAddr.
func (d *DSL) ERC20Balance(ctx context.Context, t *testing.T, tokenAddr common.Address, account stack.Account) *big.Int {
	t.Helper()
	bal, err := d.anvil.ERC20Balance(ctx, tokenAddr, account.Address)
	if err != nil {
		t.Fatalf("erc20 balance for %s: %v", account.Address.Hex(), err)
	}
	return bal
}
