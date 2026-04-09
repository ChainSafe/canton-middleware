//go:build e2e

// Package system wires all shims together into a single composed view of the
// running devstack. Tests interact with the system through this type.
package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/dsl"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// Accounts holds the pre-funded EVM test accounts used across E2E scenarios.
type Accounts struct {
	User1 stack.Account
	User2 stack.Account
}

var defaultAccounts = &Accounts{
	User1: stack.AnvilAccount0,
	User2: stack.AnvilAccount1,
}

// Tokens holds the well-known ERC-20 tokens resolved from the service manifest.
// Tests pass these directly to helpers such as DSL.WaitForAPIBalance instead of
// constructing raw addresses and decimal values.
type Tokens struct {
	DEMO   stack.Token
	PROMPT stack.Token
}

// NewTokens builds the well-known token descriptors from a resolved manifest.
func NewTokens(manifest *stack.ServiceManifest) *Tokens {
	return &Tokens{
		DEMO: stack.Token{
			ERC20Token: token.ERC20Token{Symbol: "DEMO", Decimals: 18},
			Address:    common.HexToAddress(manifest.DemoTokenAddr),
		},
		PROMPT: stack.Token{
			ERC20Token: token.ERC20Token{Symbol: "PROMPT", Decimals: 18},
			Address:    common.HexToAddress(manifest.PromptTokenAddr),
		},
	}
}

// NewTestAccounts derives unique EVM test accounts from t.Name() so that tests
// running sequentially within the same suite do not conflict on the api-server's
// user registry. The derived accounts are not pre-funded on Anvil; tests that
// require on-chain ETH or ERC-20 balances must use stack.AnvilAccount0/1 directly.
func NewTestAccounts(t *testing.T) *Accounts {
	return &Accounts{
		User1: deriveAccount(t.Name() + ":user1"),
		User2: deriveAccount(t.Name() + ":user2"),
	}
}

// deriveAccount deterministically derives a stack.Account from a string seed
// by hashing it with SHA-256 and interpreting the result as a secp256k1 private key.
func deriveAccount(seed string) stack.Account {
	h := sha256.Sum256([]byte(seed))
	key, err := crypto.ToECDSA(h[:])
	if err != nil {
		panic("deriveAccount: " + err.Error())
	}
	return stack.Account{
		Address:    crypto.PubkeyToAddress(key.PublicKey),
		PrivateKey: hex.EncodeToString(h[:]),
	}
}

// coreShims holds the shims that require explicit teardown (Anvil dials an
// ethclient, APIServer dials an RPC client, Postgres opens a SQL pool, Canton
// opens a gRPC connection).
type coreShims struct {
	anvil     *shim.AnvilShim
	postgres  *shim.PostgresShim
	apiServer *shim.APIServerShim
	canton    *shim.CantonShim
}

// initCoreShims dials Anvil, Postgres, APIServer, and Canton. On partial
// failure each already-opened resource is closed before the error is returned.
func initCoreShims(ctx context.Context, manifest *stack.ServiceManifest) (*coreShims, error) {
	anvilShim, err := shim.NewAnvil(ctx, manifest)
	if err != nil {
		return nil, fmt.Errorf("anvil shim: %w", err)
	}

	pgShim, err := shim.NewPostgres(manifest)
	if err != nil {
		anvilShim.Close()
		return nil, fmt.Errorf("postgres shim: %w", err)
	}

	apiShim, err := shim.NewAPIServer(ctx, manifest)
	if err != nil {
		_ = pgShim.Close()
		anvilShim.Close()
		return nil, fmt.Errorf("api-server shim: %w", err)
	}

	cantonShim, err := shim.NewCanton(manifest)
	if err != nil {
		apiShim.Close()
		_ = pgShim.Close()
		anvilShim.Close()
		return nil, fmt.Errorf("canton shim: %w", err)
	}

	return &coreShims{
		anvil:     anvilShim,
		postgres:  pgShim,
		apiServer: apiShim,
		canton:    cantonShim,
	}, nil
}

func (c *coreShims) close() error {
	c.canton.Close()
	c.apiServer.Close()
	c.anvil.Close()
	return c.postgres.Close()
}

// ---------------------------------------------------------------------------
// System — full stack
// ---------------------------------------------------------------------------

// System is the composed view of the running devstack. It is built once per
// test package by DoMain and shared across tests in read-only fashion.
type System struct {
	Manifest  *stack.ServiceManifest
	Anvil     stack.Anvil
	Canton    stack.Canton
	APIServer stack.APIServer
	Relayer   stack.Relayer
	Indexer   stack.Indexer
	Postgres  stack.APIDatabase
	DSL       *dsl.DSL
	Accounts  *Accounts
	Tokens    *Tokens

	closeFunc func() error
}

// Close releases resources held by the System (Postgres connection, ethclient,
// Canton gRPC connection). Callers should register this via t.Cleanup:
//
//	t.Cleanup(func() { _ = sys.Close() })
func (s *System) Close() error {
	if s.closeFunc != nil {
		return s.closeFunc()
	}
	return nil
}

// New constructs a System from a resolved ServiceManifest and initializes all
// shims. Returns an error if any shim fails to connect. Call Close() to release
// resources when done.
func New(ctx context.Context, manifest *stack.ServiceManifest) (*System, error) {
	core, err := initCoreShims(ctx, manifest)
	if err != nil {
		return nil, err
	}

	sys := &System{
		Manifest:  manifest,
		Anvil:     core.anvil,
		Canton:    core.canton,
		APIServer: core.apiServer,
		Relayer:   shim.NewRelayer(manifest),
		Indexer:   shim.NewIndexer(manifest),
		Postgres:  core.postgres,
		Accounts:  defaultAccounts,
		Tokens:    NewTokens(manifest),
		closeFunc: core.close,
	}
	sys.DSL = dsl.New(sys.APIServer, sys.Canton, sys.Relayer, sys.Indexer, sys.Postgres, sys.Anvil)
	return sys, nil
}

// ---------------------------------------------------------------------------
// Subset views
// ---------------------------------------------------------------------------

// IndexerSystem is a minimal view for indexer-focused tests. It only
// initializes the Canton and Indexer shims — no Postgres connection, no Anvil.
type IndexerSystem struct {
	Manifest *stack.ServiceManifest
	Canton   stack.Canton
	Indexer  stack.Indexer

	closeFunc func()
}

// Close releases the Canton gRPC connection held by the IndexerSystem.
func (s *IndexerSystem) Close() {
	if s.closeFunc != nil {
		s.closeFunc()
	}
}

// NewIndexerSystem constructs an IndexerSystem from a resolved manifest.
// Returns an error if the Canton shim fails to initialize.
func NewIndexerSystem(manifest *stack.ServiceManifest) (*IndexerSystem, error) {
	cantonShim, err := shim.NewCanton(manifest)
	if err != nil {
		return nil, fmt.Errorf("canton shim: %w", err)
	}
	return &IndexerSystem{
		Manifest:  manifest,
		Canton:    cantonShim,
		Indexer:   shim.NewIndexer(manifest),
		closeFunc: cantonShim.Close,
	}, nil
}

// APISystem is a minimal view for api-server focused tests. It initializes
// Anvil, Canton, APIServer, and Postgres shims together with the DSL and
// pre-funded accounts.
type APISystem struct {
	Manifest  *stack.ServiceManifest
	Anvil     stack.Anvil
	Canton    stack.Canton
	APIServer stack.APIServer
	Postgres  stack.APIDatabase
	DSL       *dsl.DSL
	Accounts  *Accounts
	Tokens    *Tokens

	closeFunc func() error
}

// Close releases resources held by the APISystem (Postgres connection, ethclient,
// Canton gRPC connection).
func (s *APISystem) Close() error {
	if s.closeFunc != nil {
		return s.closeFunc()
	}
	return nil
}

// NewAPISystem constructs an APISystem from a resolved manifest. Returns an
// error if any shim fails to connect. Call Close() to release resources when done.
func NewAPISystem(ctx context.Context, manifest *stack.ServiceManifest) (*APISystem, error) {
	core, err := initCoreShims(ctx, manifest)
	if err != nil {
		return nil, err
	}

	sys := &APISystem{
		Manifest:  manifest,
		Anvil:     core.anvil,
		Canton:    core.canton,
		APIServer: core.apiServer,
		Postgres:  core.postgres,
		Accounts:  defaultAccounts,
		Tokens:    NewTokens(manifest),
		closeFunc: core.close,
	}
	// Relayer and Indexer are not part of the API stack; nil is passed
	// deliberately. DSL methods that require them call t.Fatal with a clear
	// message rather than panicking.
	sys.DSL = dsl.New(sys.APIServer, sys.Canton, nil, nil, sys.Postgres, sys.Anvil)
	return sys, nil
}
