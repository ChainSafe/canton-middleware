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

// DoMain starts the Docker Compose stack, resolves the service manifest, runs
// all tests via m.Run(), and tears down the stack when done. It must be called
// from TestMain. The exit code from m.Run() is returned and the caller should
// pass it to os.Exit.
//
//	func TestMain(m *testing.M) { os.Exit(presets.DoMain(m)) }
func DoMain(m *testing.M, opts ...Option) int {
	o := applyOptions(opts)
	ctx := context.Background()

	orch := docker.NewOrchestrator(o.composeFile, o.projectName)
	if err := orch.Start(ctx); err != nil {
		fmt.Printf("devstack start: %v\n", err)
		return 1
	}
	defer func() { _ = orch.Stop(ctx) }()

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

// NewFullStack returns a System with all shims initialised. Use this when the
// test exercises the full bridge flow (Anvil → Canton → Relayer → Indexer).
func NewFullStack(t *testing.T) *system.System {
	t.Helper()
	mfst := resolvedManifest(t)
	sys, err := system.New(context.Background(), t, mfst)
	if err != nil {
		t.Fatalf("full stack init: %v", err)
	}
	return sys
}

// NewIndexerStack returns an IndexerSystem with only the Canton and Indexer
// shims initialised. Use this for tests that only query or assert on indexer
// state without driving Ethereum transactions.
func NewIndexerStack(t *testing.T) *system.IndexerSystem {
	t.Helper()
	return system.NewIndexerSystem(resolvedManifest(t))
}

// NewAPIStack returns an APISystem with Anvil, Canton, APIServer, and Postgres
// shims initialised. Use this for tests that register users and call api-server
// endpoints but do not need the relayer or indexer.
func NewAPIStack(t *testing.T) *system.APISystem {
	t.Helper()
	mfst := resolvedManifest(t)
	sys, err := system.NewAPISystem(context.Background(), t, mfst)
	if err != nil {
		t.Fatalf("api stack init: %v", err)
	}
	return sys
}
