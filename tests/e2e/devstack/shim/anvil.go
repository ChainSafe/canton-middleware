//go:build e2e

// Package shim provides concrete implementations of the stack service
// interfaces. Each shim wraps a real network client (go-ethereum, HTTP, SQL)
// and is initialized from a ServiceManifest produced by ServiceDiscovery.
package shim

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

var _ stack.Anvil = (*AnvilShim)(nil)

// txGasLimit is a fixed gas ceiling for approve and depositToCanton transactions
// on the local Anvil devnet. Anvil's instant mining makes estimation unnecessary.
const (
	txGasLimit    = 300_000
	txWaitTimeout = 30 * time.Second
	bytes32Len    = 32
)

// AnvilShim implements stack.Anvil against a local Anvil node.
type AnvilShim struct {
	endpoint   string
	rpc        *ethclient.Client
	chainID    *big.Int
	tokenAddr  common.Address
	bridgeAddr common.Address
}

// NewAnvil dials the Anvil RPC endpoint from the manifest and returns a ready
// shim. It resolves chainID eagerly so callers do not need a context.
func NewAnvil(ctx context.Context, manifest *stack.ServiceManifest) (*AnvilShim, error) {
	client, err := ethclient.DialContext(ctx, manifest.AnvilRPC)
	if err != nil {
		return nil, fmt.Errorf("dial anvil: %w", err)
	}
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get anvil chain ID: %w", err)
	}
	return &AnvilShim{
		endpoint:   manifest.AnvilRPC,
		rpc:        client,
		chainID:    chainID,
		tokenAddr:  common.HexToAddress(manifest.PromptTokenAddr),
		bridgeAddr: common.HexToAddress(manifest.BridgeAddr),
	}, nil
}

func (a *AnvilShim) Endpoint() string       { return a.endpoint }
func (a *AnvilShim) RPC() *ethclient.Client { return a.rpc }
func (a *AnvilShim) ChainID() *big.Int      { return a.chainID }
func (a *AnvilShim) Close()                 { a.rpc.Close() }

// ERC20Balance returns the on-chain ERC-20 balance of owner for tokenAddr.
func (a *AnvilShim) ERC20Balance(ctx context.Context, tokenAddr, owner common.Address) (*big.Int, error) {
	token, err := contracts.NewPromptToken(tokenAddr, a.rpc)
	if err != nil {
		return nil, fmt.Errorf("bind erc20: %w", err)
	}
	bal, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, owner)
	if err != nil {
		return nil, fmt.Errorf("balanceOf: %w", err)
	}
	return bal, nil
}

// ApproveAndDeposit approves the bridge contract and submits a depositToCanton
// transaction for account. The canton recipient bytes32 is derived from the
// account's EVM address fingerprint via auth.ComputeFingerprint.
func (a *AnvilShim) ApproveAndDeposit(ctx context.Context, account *stack.Account, amount *big.Int) (common.Hash, error) {
	key, err := parseKey(account.PrivateKey)
	if err != nil {
		return common.Hash{}, err
	}

	fingerprint := auth.ComputeFingerprint(account.Address.Hex())
	recipient, err := fingerprintToBytes32(fingerprint)
	if err != nil {
		return common.Hash{}, err
	}

	token, err := contracts.NewPromptToken(a.tokenAddr, a.rpc)
	if err != nil {
		return common.Hash{}, fmt.Errorf("bind prompt token: %w", err)
	}
	bridge, err := contracts.NewCantonBridge(a.bridgeAddr, a.rpc)
	if err != nil {
		return common.Hash{}, fmt.Errorf("bind canton bridge: %w", err)
	}

	// Step 1: approve.
	auth, err := newTransactor(ctx, a.rpc, key, a.chainID)
	if err != nil {
		return common.Hash{}, err
	}
	approveTx, err := token.Approve(auth, a.bridgeAddr, amount)
	if err != nil {
		return common.Hash{}, fmt.Errorf("approve: %w", err)
	}
	if waitErr := waitForTx(ctx, a.rpc, approveTx.Hash(), txWaitTimeout); waitErr != nil {
		return common.Hash{}, fmt.Errorf("wait approve tx: %w", waitErr)
	}

	// Step 2: deposit.
	auth, err = newTransactor(ctx, a.rpc, key, a.chainID)
	if err != nil {
		return common.Hash{}, err
	}
	depositTx, err := bridge.DepositToCanton(auth, a.tokenAddr, amount, recipient)
	if err != nil {
		return common.Hash{}, fmt.Errorf("depositToCanton: %w", err)
	}
	if waitErr := waitForTx(ctx, a.rpc, depositTx.Hash(), txWaitTimeout); waitErr != nil {
		return common.Hash{}, fmt.Errorf("wait deposit tx: %w", waitErr)
	}

	return depositTx.Hash(), nil
}

// newTransactor creates a TransactOpts with current nonce and suggested gas price.
func newTransactor(ctx context.Context, client *ethclient.Client, key *ecdsa.PrivateKey, chainID *big.Int) (*bind.TransactOpts, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, fmt.Errorf("keyed transactor: %w", err)
	}
	nonce, err := client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}
	auth.Nonce = new(big.Int).SetUint64(nonce)
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest gas price: %w", err)
	}
	auth.GasPrice = gasPrice
	auth.GasLimit = txGasLimit
	return auth, nil
}

// waitForTx polls until the transaction is mined or the timeout is reached.
// It returns immediately on any RPC error other than ethereum.NotFound (tx not
// yet visible) to avoid masking genuine node failures.
func waitForTx(ctx context.Context, client *ethclient.Client, hash common.Hash, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err == nil {
			if receipt.Status == 1 {
				return nil
			}
			return fmt.Errorf("transaction %s reverted", hash.Hex())
		}
		if !errors.Is(err, ethereum.NotFound) {
			return fmt.Errorf("receipt query for %s: %w", hash.Hex(), err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for tx %s: %w", hash.Hex(), ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// parseKey decodes a hex-encoded ECDSA private key (without 0x prefix).
func parseKey(hexKey string) (*ecdsa.PrivateKey, error) {
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return key, nil
}

// fingerprintToBytes32 converts a hex fingerprint string to a [32]byte.
func fingerprintToBytes32(fingerprint string) ([32]byte, error) {
	var result [32]byte
	fingerprint = strings.TrimPrefix(fingerprint, "0x")
	data, err := hex.DecodeString(fingerprint)
	if err != nil {
		return result, fmt.Errorf("decode fingerprint: %w", err)
	}
	if len(data) > bytes32Len {
		return result, fmt.Errorf("fingerprint too long: %d bytes", len(data))
	}
	copy(result[:], data)
	return result, nil
}
