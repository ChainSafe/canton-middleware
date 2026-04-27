//go:build e2e

// Package dsl provides high-level test operations built on top of the stack
// service interfaces. Methods accept *testing.T and call t.Fatal on error so
// tests read as plain imperative steps without error-handling boilerplate.
package dsl

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
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
//
// If the account is already registered (HTTP 409), the existing registration
// is fetched from Postgres and returned — making this method idempotent. This
// allows multiple tests in a suite to share AnvilAccount0 without conflicting.
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
		var he *shim.HTTPError
		if errors.As(err, &he) && he.Code == http.StatusConflict {
			existing, lookupErr := d.postgres.GetUser(ctx, account.Address.Hex())
			if lookupErr != nil {
				t.Fatalf("register %s: already registered but DB lookup failed: %v", account.Address.Hex(), lookupErr)
			}
			return existing
		}
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
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastErr error
	var lastBal *big.Int
	for time.Now().Before(deadline) {
		bal, err := d.apiServer.ERC20Balance(ctx, tok.Address, ownerAddr)
		if err != nil {
			lastErr = err
		} else {
			lastBal = bal
			if bal.Cmp(minWei) >= 0 {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("context canceled waiting for API balance")
		case <-ticker.C:
		}
	}
	if lastErr != nil {
		t.Fatalf("WaitForAPIBalance: timed out waiting for %s %s balance >= %s (owner %s): last error: %v",
			minTokens, tok.Symbol, minWei, ownerAddr.Hex(), lastErr)
	}
	if lastBal != nil {
		t.Fatalf("WaitForAPIBalance: timed out waiting for %s %s balance >= %s (owner %s): last seen balance: %s",
			minTokens, tok.Symbol, minWei, ownerAddr.Hex(), lastBal.String())
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

// Withdraw looks up the FingerprintMapping and a suitable holding for the given
// party and token, then calls InitiateWithdrawal followed by ProcessWithdrawal
// on the Canton bridge (burning tokens and creating a WithdrawalEvent for the
// relayer). It returns the WithdrawalRequest contract ID. Requires a full-stack
// system.
//
// partyID and fingerprint are the Party and Fingerprint fields from the user's
// RegisterResponse. tokenSymbol identifies the token (e.g. "PROMPT"). amount is
// the decimal withdrawal amount (e.g. "1"). evmDest is the checksummed hex EVM
// address that will receive the released tokens.
//
// The holding selected is the first one whose balance is >= amount. The test
// fails if no holding with sufficient balance exists.
func (d *DSL) Withdraw(ctx context.Context, t *testing.T, partyID, fingerprint, tokenSymbol, amount, evmDest string) string {
	t.Helper()
	if d.canton == nil {
		t.Fatal("Withdraw not available: Canton shim not initialized (use NewFullStack)")
		return ""
	}

	mappingCID, err := d.canton.GetFingerprintMapping(ctx, fingerprint)
	if err != nil {
		t.Fatalf("get fingerprint mapping for %s: %v", fingerprint, err)
	}

	holdings, err := d.canton.GetHoldings(ctx, partyID, tokenSymbol)
	if err != nil {
		t.Fatalf("get %s holdings for party %s: %v", tokenSymbol, partyID, err)
	}

	// Select the first holding whose amount covers the requested withdrawal.
	// big.Rat is used for exact decimal arithmetic — big.Float's 53-bit default
	// precision is insufficient for 18-decimal token amounts.
	amountR, ok := new(big.Rat).SetString(amount)
	if !ok {
		t.Fatalf("Withdraw: invalid amount %q", amount)
	}
	holdingCID := ""
	for _, h := range holdings {
		hR, ok2 := new(big.Rat).SetString(h.Amount)
		if !ok2 {
			t.Fatalf("Withdraw: invalid holding amount %q for contract %s", h.Amount, h.ContractID)
		}
		if hR.Cmp(amountR) >= 0 {
			holdingCID = h.ContractID
			break
		}
	}
	if holdingCID == "" {
		t.Fatalf("no %s holding with amount >= %s for party %s", tokenSymbol, amount, partyID)
	}

	withdrawalReqCID, err := d.canton.InitiateWithdrawal(ctx, mappingCID, holdingCID, amount, evmDest)
	if err != nil {
		t.Fatalf("initiate withdrawal for party %s: %v", partyID, err)
	}

	// Exercise ProcessWithdrawal on the WithdrawalRequest — burns tokens on Canton
	// and creates the WithdrawalEvent that the relayer streams to release on EVM.
	if _, err := d.canton.ProcessWithdrawal(ctx, withdrawalReqCID); err != nil {
		t.Fatalf("process withdrawal for party %s: %v", partyID, err)
	}

	return withdrawalReqCID
}

// anvilFundingMu serializes all NewFundedAccount calls so that concurrent
// parallel tests never race on AnvilAccount0's nonce. A package-level mutex is
// used because each test creates its own DSL instance; the mutex must be shared
// across instances. Each funding call internally waits for the transaction to
// mine, so the nonce is always monotonically incremented before the next caller
// acquires the lock.
var anvilFundingMu sync.Mutex

// NewFundedAccount generates a fresh secp256k1 key and funds it from
// AnvilAccount0. eth is the amount of ETH to transfer (whole units, e.g. 1 for
// 1 ETH). tokens is the amount of the ERC-20 at tokenAddr to transfer (whole
// units, 18-decimal assumed). Pass 0 for either to skip that transfer.
//
// The method is safe to call from parallel tests. Internally it holds a
// package-level mutex while touching AnvilAccount0's nonce, so callers never
// race each other. The returned account is fully funded before the method
// returns.
func (d *DSL) NewFundedAccount(ctx context.Context, t *testing.T, eth int, tokenAddr common.Address, tokens int) stack.Account {
	t.Helper()
	if d.anvil == nil {
		t.Fatal("NewFundedAccount not available: Anvil shim not initialized")
		return stack.Account{}
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("NewFundedAccount: generate key: %v", err)
	}
	acc := stack.Account{
		Address:    crypto.PubkeyToAddress(key.PublicKey),
		PrivateKey: hex.EncodeToString(crypto.FromECDSA(key)),
	}

	const (
		base     = 10
		decimals = 18
	)
	exp18 := new(big.Int).Exp(big.NewInt(base), big.NewInt(decimals), nil)

	anvilFundingMu.Lock()
	defer anvilFundingMu.Unlock()

	funder := stack.AnvilAccount0
	if eth > 0 {
		ethWei := new(big.Int).Mul(big.NewInt(int64(eth)), exp18)
		if err := d.anvil.FundWithETH(ctx, &funder, acc.Address, ethWei); err != nil {
			t.Fatalf("NewFundedAccount: fund ETH: %v", err)
		}
	}
	if tokens > 0 {
		if (tokenAddr == common.Address{}) {
			t.Fatalf("NewFundedAccount: tokens > 0 but tokenAddr is zero address")
		}
		tokenWei := new(big.Int).Mul(big.NewInt(int64(tokens)), exp18)
		if err := d.anvil.TransferERC20(ctx, &funder, acc.Address, tokenAddr, tokenWei); err != nil {
			t.Fatalf("NewFundedAccount: fund ERC20: %v", err)
		}
	}
	return acc
}
