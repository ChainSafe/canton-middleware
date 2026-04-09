//go:build e2e

// Package presets provides TestMain helpers and per-test System constructors
// for E2E tests. It wires together the docker, system, and dsl layers so that
// individual test packages only need to call DoMain and one of the New*Stack
// helpers.
package presets

import (
	"context"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/docker"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/system"
)

var (
	mu       sync.Mutex
	manifest *stack.ServiceManifest
)

// DoMain starts the Docker Compose stack, resolves the service manifest, runs
// all tests via m.Run(), and tears down the stack when done. It must be called
// from TestMain. The exit code from m.Run() is returned and the caller should
// pass it to os.Exit.
//
// SIGINT and SIGTERM are trapped so that Ctrl+C during a test run still
// triggers a clean docker compose down.
//
//	func TestMain(m *testing.M) { os.Exit(presets.DoMain(m)) }
func DoMain(m *testing.M, opts ...Option) int {
	o := applyOptions(opts)

	// Signal-aware context: cancels on SIGINT/SIGTERM so in-flight docker
	// operations (Start) abort promptly. Stop always uses a fresh context so
	// it is not affected by signal cancellation.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	orch := docker.NewOrchestrator(o.composeFile, o.projectName)
	if err := orch.Start(ctx); err != nil {
		fmt.Printf("devstack start: %v\n", err)
		return 1
	}
	defer func() { _ = orch.Stop(context.Background()) }()

	disc := docker.NewServiceDiscovery(o.projectName)
	mfst, err := disc.Manifest(ctx)
	if err != nil {
		fmt.Printf("service discovery: %v\n", err)
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
	sys := system.NewIndexerSystem(resolvedManifest(t))
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
