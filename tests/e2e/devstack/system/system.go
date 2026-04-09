//go:build e2e

// Package system wires all shims together into a single composed view of the
// running devstack. Tests interact with the system through this type.
package system

import (
	"context"
	"fmt"

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

// coreShims holds the three shims that require explicit teardown (Anvil dials
// an ethclient, APIServer dials an RPC client, Postgres opens a SQL pool).
type coreShims struct {
	anvil     *shim.AnvilShim
	postgres  *shim.PostgresShim
	apiServer *shim.APIServerShim
}

// initCoreShims dials Anvil, Postgres, and APIServer. On partial failure each
// already-opened resource is closed before the error is returned.
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

	return &coreShims{anvil: anvilShim, postgres: pgShim, apiServer: apiShim}, nil
}

func (c *coreShims) close() error {
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

	closeFunc func() error
}

// Close releases resources held by the System (Postgres connection, ethclient).
// Callers should register this via t.Cleanup: t.Cleanup(func() { _ = sys.Close() })
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
		Canton:    shim.NewCanton(manifest),
		APIServer: core.apiServer,
		Relayer:   shim.NewRelayer(manifest),
		Indexer:   shim.NewIndexer(manifest),
		Postgres:  core.postgres,
		Accounts:  defaultAccounts,
		closeFunc: core.close,
	}
	sys.DSL = dsl.New(sys.APIServer, sys.Relayer, sys.Indexer, sys.Postgres, sys.Anvil)
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
}

// NewIndexerSystem constructs an IndexerSystem from a resolved manifest.
func NewIndexerSystem(manifest *stack.ServiceManifest) *IndexerSystem {
	return &IndexerSystem{
		Manifest: manifest,
		Canton:   shim.NewCanton(manifest),
		Indexer:  shim.NewIndexer(manifest),
	}
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

	closeFunc func() error
}

// Close releases resources held by the APISystem (Postgres connection, ethclient).
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
		Canton:    shim.NewCanton(manifest),
		APIServer: core.apiServer,
		Postgres:  core.postgres,
		Accounts:  defaultAccounts,
		closeFunc: core.close,
	}
	// Relayer and Indexer are not part of the API stack; nil is passed
	// deliberately. DSL methods that require them call t.Fatal with a clear
	// message rather than panicking.
	sys.DSL = dsl.New(sys.APIServer, nil, nil, sys.Postgres, sys.Anvil)
	return sys, nil
}
