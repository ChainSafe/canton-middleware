//go:build e2e

// Package dsl provides high-level test operations built on top of the stack
// service interfaces. Methods accept *testing.T and call t.Fatal on error so
// tests read as plain imperative steps without error-handling boilerplate.
package dsl

import (
	"context"
	"math/big"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/ethereum/go-ethereum/common"
)

// DSL exposes high-level operations over the service interfaces. Accessed via
// System.DSL.
type DSL struct {
	APIServer stack.APIServer
	Relayer   stack.Relayer
	Indexer   stack.Indexer
	Postgres  stack.APIDatabase
	Anvil     stack.Anvil
}

// New wires a DSL to the given service interfaces.
func New(api stack.APIServer, relayer stack.Relayer, indexer stack.Indexer, postgres stack.APIDatabase, anvil stack.Anvil) *DSL {
	return &DSL{
		APIServer: api,
		Relayer:   relayer,
		Indexer:   indexer,
		Postgres:  postgres,
		Anvil:     anvil,
	}
}

// RegisterUser whitelists the account's EVM address and registers it as a
// custodial web3 user via POST /register. Returns the RegisterResponse.
func (d *DSL) RegisterUser(ctx context.Context, t *testing.T, account stack.Account) *user.RegisterResponse {
	t.Helper()

	if err := d.Postgres.WhitelistAddress(ctx, account.Address.Hex()); err != nil {
		t.Fatalf("whitelist %s: %v", account.Address.Hex(), err)
	}

	msg := "register"
	sig, err := signEIP191(account.PrivateKey, msg)
	if err != nil {
		t.Fatalf("sign register message: %v", err)
	}

	resp, err := d.APIServer.Register(ctx, &user.RegisterRequest{
		Signature: sig,
		Message:   msg,
	})
	if err != nil {
		t.Fatalf("register %s: %v", account.Address.Hex(), err)
	}
	return resp
}

// Deposit approves the bridge and submits a depositToCanton transaction on
// behalf of account. Returns the deposit transaction hash.
func (d *DSL) Deposit(ctx context.Context, t *testing.T, account stack.Account, amount *big.Int) common.Hash {
	t.Helper()
	hash, err := d.Anvil.ApproveAndDeposit(ctx, &account, amount)
	if err != nil {
		t.Fatalf("deposit for %s: %v", account.Address.Hex(), err)
	}
	return hash
}

// ERC20Balance returns the on-chain ERC-20 balance of account for tokenAddr.
func (d *DSL) ERC20Balance(ctx context.Context, t *testing.T, tokenAddr common.Address, account stack.Account) *big.Int {
	t.Helper()
	bal, err := d.Anvil.ERC20Balance(ctx, tokenAddr, account.Address)
	if err != nil {
		t.Fatalf("erc20 balance for %s: %v", account.Address.Hex(), err)
	}
	return bal
}
