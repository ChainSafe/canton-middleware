//go:build e2e

// Package system wires all shims together into a single composed view of the
// running devstack. Tests interact with the system through this type.
package system

import (
	"context"
	"fmt"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/dsl"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// Accounts holds the pre-funded EVM test accounts used across E2E scenarios.
type Accounts struct {
	User1 stack.Account
	User2 stack.Account
}

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
}

// New constructs a System from a resolved ServiceManifest, initialises all
// shims, and wires the DSL. Returns an error if any shim fails to connect.
func New(ctx context.Context, t *testing.T, manifest *stack.ServiceManifest) (*System, error) {
	t.Helper()

	anvilShim, err := shim.NewAnvil(ctx, manifest)
	if err != nil {
		return nil, fmt.Errorf("anvil shim: %w", err)
	}

	pgShim, err := shim.NewPostgres(manifest)
	if err != nil {
		return nil, fmt.Errorf("postgres shim: %w", err)
	}
	t.Cleanup(func() { _ = pgShim.Close() })

	sys := &System{
		Manifest:  manifest,
		Anvil:     anvilShim,
		Canton:    shim.NewCanton(manifest),
		APIServer: shim.NewAPIServer(manifest),
		Relayer:   shim.NewRelayer(manifest),
		Indexer:   shim.NewIndexer(manifest),
		Postgres:  pgShim,
		Accounts: &Accounts{
			User1: stack.AnvilAccount0,
			User2: stack.AnvilAccount1,
		},
	}
	sys.DSL = dsl.New(sys.APIServer, sys.Relayer, sys.Indexer, sys.Postgres, sys.Anvil)
	return sys, nil
}
