//go:build e2e

// Package presets provides TestMain helpers and per-test System constructors
// for E2E tests. It wires together the docker, system, and dsl layers so that
// individual test packages only need to call DoMain and NewFullStack.
package presets

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/docker"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/system"
)

var (
	mu     sync.Mutex
	active *system.System
)

// DoMain starts the Docker Compose stack, runs all tests via m.Run(), and tears
// down the stack when done. It must be called from TestMain. The exit code from
// m.Run() is returned and the caller should pass it to os.Exit.
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
	manifest, err := disc.Manifest(ctx)
	if err != nil {
		fmt.Printf("service discovery: %v\n", err)
		return 1
	}

	// system.New requires a *testing.T — use a throwaway one for setup.
	// Errors here are fatal: no test should run against a broken stack.
	t := &testing.T{}
	sys, err := system.New(ctx, t, manifest)
	if err != nil {
		fmt.Printf("system init: %v\n", err)
		return 1
	}

	mu.Lock()
	active = sys
	mu.Unlock()

	return m.Run()
}

// NewFullStack returns the shared System built by DoMain. It fails the test
// immediately if DoMain was not called first.
func NewFullStack(t *testing.T) *system.System {
	t.Helper()
	mu.Lock()
	sys := active
	mu.Unlock()
	if sys == nil {
		t.Fatal("presets.DoMain was not called before NewFullStack")
	}
	return sys
}
