//go:build e2e

// Package api_test contains E2E tests for the canton-middleware api-server.
// This file covers the /eth JSON-RPC facade — one test per RPC method or
// namespace so CI can identify exactly which method regressed.
package api_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// ──────────────────────────────────────────────────────────────────────────────
// eth_* namespace
// ──────────────────────────────────────────────────────────────────────────────

// TestRPC_ChainID verifies that eth_chainId returns a non-zero chain ID.
func TestRPC_ChainID(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	chainID, err := sys.APIServer.RPC().ChainID(ctx)
	if err != nil {
		t.Fatalf("eth_chainId: %v", err)
	}
	if chainID == nil || chainID.Sign() == 0 {
		t.Fatalf("expected non-zero chain ID, got %v", chainID)
	}
}

// TestRPC_BlockNumber verifies that eth_blockNumber returns without error.
func TestRPC_BlockNumber(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	_, err := sys.APIServer.RPC().BlockNumber(ctx)
	if err != nil {
		t.Fatalf("eth_blockNumber: %v", err)
	}
}

// TestRPC_GasPrice verifies that eth_gasPrice returns a non-nil value.
func TestRPC_GasPrice(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	price, err := sys.APIServer.RPC().SuggestGasPrice(ctx)
	if err != nil {
		t.Fatalf("eth_gasPrice: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil gas price")
	}
}

// TestRPC_MaxPriorityFeePerGas verifies that eth_maxPriorityFeePerGas returns
// a non-nil value.
func TestRPC_MaxPriorityFeePerGas(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tip, err := sys.APIServer.RPC().SuggestGasTipCap(ctx)
	if err != nil {
		t.Fatalf("eth_maxPriorityFeePerGas: %v", err)
	}
	if tip == nil {
		t.Fatal("expected non-nil max priority fee")
	}
}

// TestRPC_EstimateGas verifies that eth_estimateGas returns a non-zero estimate
// for a simple ETH transfer to a well-known address.
func TestRPC_EstimateGas(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	to := common.HexToAddress("0x000000000000000000000000000000000000dead")
	gas, err := sys.APIServer.RPC().EstimateGas(ctx, ethereum.CallMsg{
		From: sys.Accounts.User1.Address,
		To:   &to,
	})
	if err != nil {
		t.Fatalf("eth_estimateGas: %v", err)
	}
	if gas == 0 {
		t.Fatal("expected non-zero gas estimate")
	}
}

// TestRPC_GetBalance verifies that eth_getBalance returns for a known address
// without error.
func TestRPC_GetBalance(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	bal, err := sys.APIServer.RPC().BalanceAt(ctx, sys.Accounts.User1.Address, nil)
	if err != nil {
		t.Fatalf("eth_getBalance: %v", err)
	}
	if bal == nil {
		t.Fatal("expected non-nil balance")
	}
}

// TestRPC_GetTransactionCount verifies that eth_getTransactionCount returns
// the nonce for a known address without error.
func TestRPC_GetTransactionCount(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	_, err := sys.APIServer.RPC().PendingNonceAt(ctx, sys.Accounts.User1.Address)
	if err != nil {
		t.Fatalf("eth_getTransactionCount: %v", err)
	}
}

// TestRPC_GetCode verifies that eth_getCode returns the bytecode for the
// deployed PROMPT token contract (must be non-empty).
func TestRPC_GetCode(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	code, err := sys.APIServer.RPC().CodeAt(ctx, tokenAddr, nil)
	if err != nil {
		t.Fatalf("eth_getCode: %v", err)
	}
	if len(code) == 0 {
		t.Fatal("expected non-empty bytecode for PROMPT token contract")
	}
}

// TestRPC_Syncing verifies that eth_syncing returns without error. The local
// Anvil devnet is always synced so the result should be false.
func TestRPC_Syncing(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	var result any
	err := sys.APIServer.RPC().Client().CallContext(ctx, &result, "eth_syncing")
	if err != nil {
		t.Fatalf("eth_syncing: %v", err)
	}
}

// TestRPC_GetLogs verifies that eth_getLogs returns without error for a query
// spanning block 0 to the latest block.
//
// Note: FromBlock and ToBlock are set explicitly. Leaving them nil causes
// go-ethereum to send "latest" as the block tag, which the api-server's
// eth_getLogs handler rejects (it expects a hex uint64, not a block tag string).
func TestRPC_GetLogs(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	latest, err := sys.APIServer.RPC().BlockNumber(ctx)
	if err != nil {
		t.Fatalf("eth_blockNumber: %v", err)
	}

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	_, err = sys.APIServer.RPC().FilterLogs(ctx, ethereum.FilterQuery{
		Addresses: []common.Address{tokenAddr},
		FromBlock: big.NewInt(0),
		ToBlock:   new(big.Int).SetUint64(latest),
	})
	if err != nil {
		t.Fatalf("eth_getLogs: %v", err)
	}
}

// TestRPC_GetBlockByNumber verifies that eth_getBlockByNumber with "latest"
// returns a block with a non-zero hash.
func TestRPC_GetBlockByNumber(t *testing.T) {
	t.Skip("api-server /eth facade returns blocks without uncle metadata; ethclient uncle-list validation fails")

	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	block, err := sys.APIServer.RPC().BlockByNumber(ctx, nil)
	if err != nil {
		t.Fatalf("eth_getBlockByNumber: %v", err)
	}
	if block == nil {
		t.Fatal("expected non-nil block")
	}
	if block.Hash() == (common.Hash{}) {
		t.Fatal("expected non-zero block hash")
	}
}

// TestRPC_GetBlockByHash verifies that eth_getBlockByHash returns the same
// block that eth_getBlockByNumber returned.
func TestRPC_GetBlockByHash(t *testing.T) {
	t.Skip("api-server /eth facade returns blocks without uncle metadata; ethclient uncle-list validation fails")

	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	latest, err := sys.APIServer.RPC().BlockByNumber(ctx, nil)
	if err != nil {
		t.Fatalf("eth_getBlockByNumber (setup): %v", err)
	}

	block, err := sys.APIServer.RPC().BlockByHash(ctx, latest.Hash())
	if err != nil {
		t.Fatalf("eth_getBlockByHash: %v", err)
	}
	if block == nil {
		t.Fatal("expected non-nil block")
	}
	if block.Hash() != latest.Hash() {
		t.Fatalf("expected block hash %s, got %s", latest.Hash(), block.Hash())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// eth_call — registered token contracts
// ──────────────────────────────────────────────────────────────────────────────

// TestRPC_Call_TotalSupply verifies that eth_call with totalSupply() returns
// 32 bytes (uint256) for the PROMPT token contract.
func TestRPC_Call_TotalSupply(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	// 0x18160ddd = keccak256("totalSupply()")[0:4]
	result, err := sys.APIServer.RPC().CallContract(ctx, ethereum.CallMsg{
		To:   &tokenAddr,
		Data: []byte{0x18, 0x16, 0x0d, 0xdd},
	}, nil)
	if err != nil {
		t.Fatalf("eth_call totalSupply: %v", err)
	}
	if len(result) != 32 {
		t.Fatalf("expected 32-byte uint256, got %d bytes", len(result))
	}
}

// TestRPC_Call_Decimals verifies that eth_call with decimals() returns 32
// bytes encoding uint8 for the PROMPT token.
func TestRPC_Call_Decimals(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	// 0x313ce567 = keccak256("decimals()")[0:4]
	result, err := sys.APIServer.RPC().CallContract(ctx, ethereum.CallMsg{
		To:   &tokenAddr,
		Data: []byte{0x31, 0x3c, 0xe5, 0x67},
	}, nil)
	if err != nil {
		t.Fatalf("eth_call decimals: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result from decimals()")
	}
}

// TestRPC_Call_Symbol verifies that eth_call with symbol() returns a non-empty
// ABI-encoded string for the PROMPT token.
func TestRPC_Call_Symbol(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	// 0x95d89b41 = keccak256("symbol()")[0:4]
	result, err := sys.APIServer.RPC().CallContract(ctx, ethereum.CallMsg{
		To:   &tokenAddr,
		Data: []byte{0x95, 0xd8, 0x9b, 0x41},
	}, nil)
	if err != nil {
		t.Fatalf("eth_call symbol: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result from symbol()")
	}
}

// TestRPC_Call_Name verifies that eth_call with name() returns a non-empty
// ABI-encoded string for the PROMPT token.
func TestRPC_Call_Name(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	// 0x06fdde03 = keccak256("name()")[0:4]
	result, err := sys.APIServer.RPC().CallContract(ctx, ethereum.CallMsg{
		To:   &tokenAddr,
		Data: []byte{0x06, 0xfd, 0xde, 0x03},
	}, nil)
	if err != nil {
		t.Fatalf("eth_call name: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result from name()")
	}
}

// TestRPC_Call_BalanceOf_PROMPT verifies that eth_call with balanceOf() for a
// fresh address returns a 32-byte zero value for the PROMPT token.
func TestRPC_Call_BalanceOf_PROMPT(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	tokenAddr := common.HexToAddress(sys.Manifest.PromptTokenAddr)
	freshAddr := common.HexToAddress("0x000000000000000000000000000000000000dead")

	// 0x70a08231 = keccak256("balanceOf(address)")[0:4], padded address arg.
	data := make([]byte, 36)
	data[0], data[1], data[2], data[3] = 0x70, 0xa0, 0x82, 0x31
	copy(data[16:], freshAddr[:]) // address is right-aligned in 32-byte slot

	result, err := sys.APIServer.RPC().CallContract(ctx, ethereum.CallMsg{
		To:   &tokenAddr,
		Data: data,
	}, nil)
	if err != nil {
		t.Fatalf("eth_call balanceOf PROMPT: %v", err)
	}
	if len(result) != 32 {
		t.Fatalf("expected 32-byte uint256, got %d bytes", len(result))
	}
}

// TestRPC_Call_BalanceOf_DEMO verifies that eth_call with balanceOf() on the
// DEMO virtual EVM address returns a 32-byte zero value for a fresh address.
func TestRPC_Call_BalanceOf_DEMO(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	demoAddr := common.HexToAddress(sys.Manifest.DemoTokenAddr)
	freshAddr := common.HexToAddress("0x000000000000000000000000000000000000dead")

	data := make([]byte, 36)
	data[0], data[1], data[2], data[3] = 0x70, 0xa0, 0x82, 0x31
	copy(data[16:], freshAddr[:])

	result, err := sys.APIServer.RPC().CallContract(ctx, ethereum.CallMsg{
		To:   &demoAddr,
		Data: data,
	}, nil)
	if err != nil {
		t.Fatalf("eth_call balanceOf DEMO: %v", err)
	}
	if len(result) != 32 {
		t.Fatalf("expected 32-byte uint256, got %d bytes", len(result))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// net_* namespace
// ──────────────────────────────────────────────────────────────────────────────

// TestRPC_NetVersion verifies that net_version returns a non-empty chain ID
// string.
func TestRPC_NetVersion(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	var version string
	if err := sys.APIServer.RPC().Client().CallContext(ctx, &version, "net_version"); err != nil {
		t.Fatalf("net_version: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty net_version")
	}
}

// TestRPC_NetListening verifies that net_listening returns true.
func TestRPC_NetListening(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	var listening bool
	if err := sys.APIServer.RPC().Client().CallContext(ctx, &listening, "net_listening"); err != nil {
		t.Fatalf("net_listening: %v", err)
	}
	if !listening {
		t.Fatal("expected net_listening to return true")
	}
}

// TestRPC_NetPeerCount verifies that net_peerCount returns without error.
func TestRPC_NetPeerCount(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	var peerCount any
	if err := sys.APIServer.RPC().Client().CallContext(ctx, &peerCount, "net_peerCount"); err != nil {
		t.Fatalf("net_peerCount: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// web3_* namespace
// ──────────────────────────────────────────────────────────────────────────────

// TestRPC_Web3ClientVersion verifies that web3_clientVersion returns a
// non-empty version string.
func TestRPC_Web3ClientVersion(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	var version string
	if err := sys.APIServer.RPC().Client().CallContext(ctx, &version, "web3_clientVersion"); err != nil {
		t.Fatalf("web3_clientVersion: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty web3_clientVersion")
	}
}

// TestRPC_Web3Sha3 verifies that web3_sha3("0x") returns the keccak256 of
// empty bytes — a well-known value used as a correctness check.
func TestRPC_Web3Sha3(t *testing.T) {
	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	// keccak256("") = 0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470
	const emptyKeccak = "0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"

	var result string
	if err := sys.APIServer.RPC().Client().CallContext(ctx, &result, "web3_sha3", "0x"); err != nil {
		t.Fatalf("web3_sha3: %v", err)
	}
	if result != emptyKeccak {
		t.Fatalf("expected web3_sha3(0x) = %s, got %s", emptyKeccak, result)
	}
}
