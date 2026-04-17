//go:build e2e

// Package presets provides TestMain helpers and per-test System constructors
// for E2E tests. It wires together the docker, system, and dsl layers so that
// individual test packages only need to call DoMain and one of the New*Stack
// helpers.
package presets

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/docker"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/system"
)

var (
	mu       sync.Mutex
	manifest *stack.ServiceManifest
)

// DoMain resolves the service manifest from the running devstack, then runs
// all tests via m.Run(). It must be called from TestMain. The exit code from
// m.Run() is returned and the caller should pass it to os.Exit.
//
// The devstack must be running before calling DoMain. Start it with:
//
//	make devstack-up
//
//	func TestMain(m *testing.M) { os.Exit(presets.DoMain(m)) }
func DoMain(m *testing.M, opts ...Option) int {
	o := applyOptions(opts)

	disc := docker.NewServiceDiscovery(o.projectName, o.composeFile)
	mfst, err := disc.Manifest(context.Background())
	if err != nil {
		fmt.Printf("service discovery failed (is the devstack running? try: make devstack-up): %v\n", err)
		return 1
	}

	mu.Lock()
	manifest = mfst
	mu.Unlock()

	return m.Run()
}

// resolvedManifest returns the manifest built by DoMain or fails the test.
func resolvedManifest(t *testing.T) *stack.ServiceManifest {
	t.Helper()
	mu.Lock()
	mfst := manifest
	mu.Unlock()
	if mfst == nil {
		t.Fatal("presets.DoMain was not called before New*Stack")
	}
	return mfst
}

// NewFullStack returns a System with all shims initialized. Use this when the
// test exercises the full bridge flow (Anvil → Canton → Relayer → Indexer).
//
// Accounts are derived uniquely from t.Name() to prevent registration conflicts
// across tests that share the same suite run. Tests that need pre-funded Anvil
// accounts (ETH / ERC-20 balance) must use stack.AnvilAccount0/1 directly.
func NewFullStack(t *testing.T) *system.System {
	t.Helper()
	sys, err := system.New(context.Background(), resolvedManifest(t))
	if err != nil {
		t.Fatalf("full stack init: %v", err)
	}
	sys.Accounts = system.NewTestAccounts(t)
	t.Cleanup(func() { _ = sys.Close() })
	return sys
}

// NewIndexerStack returns an IndexerSystem with only the Canton and Indexer
// shims initialized. Use this for tests that only query or assert on indexer
// state without driving Ethereum transactions.
func NewIndexerStack(t *testing.T) *system.IndexerSystem {
	t.Helper()
	sys, err := system.NewIndexerSystem(resolvedManifest(t))
	if err != nil {
		t.Fatalf("indexer stack init: %v", err)
	}
	t.Cleanup(sys.Close)
	return sys
}

// NewAPIStack returns an APISystem with Anvil, Canton, APIServer, and Postgres
// shims initialized. Use this for tests that register users and call api-server
// endpoints but do not need the relayer or indexer.
//
// Accounts are derived uniquely from t.Name() to prevent registration conflicts
// across tests that share the same suite run. Tests that need pre-funded Anvil
// accounts (ETH / ERC-20 balance) must use stack.AnvilAccount0/1 directly.
func NewAPIStack(t *testing.T) *system.APISystem {
	t.Helper()
	sys, err := system.NewAPISystem(context.Background(), resolvedManifest(t))
	if err != nil {
		t.Fatalf("api stack init: %v", err)
	}
	sys.Accounts = system.NewTestAccounts(t)
	t.Cleanup(func() { _ = sys.Close() })
	return sys
}
