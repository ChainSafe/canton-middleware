// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service/mocks"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist"
	wlmocks "github.com/chainsafe/canton-middleware/pkg/user/whitelist/mocks"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// defaultCfg returns a minimal EthRPCConfig suitable for unit tests.
func defaultCfg() *ethrpc.Config {
	return &ethrpc.Config{
		ChainID:        31337,
		RequestTimeout: 30 * time.Second,
	}
}

// newSvc creates a real ethService backed by the supplied (possibly nil)
// dependencies, with a whitelist that allows any sender. The expectation is
// optional (.Maybe) so tests that never reach SendRawTransaction don't need to
// set one; see newSvcWithWhitelist when the whitelist behaviour is under test.
func newSvc(t *testing.T, cfg *ethrpc.Config, store service.Store, tokenSvc service.TokenService) service.Service {
	t.Helper()
	wl := wlmocks.NewChecker(t)
	wl.EXPECT().IsWhitelisted(mock.Anything, mock.Anything).Return(true, nil).Maybe()
	return service.NewService(cfg, store, tokenSvc, wl)
}

// newSvcWithWhitelist creates a service backed by the supplied whitelist checker.
func newSvcWithWhitelist(
	t *testing.T, cfg *ethrpc.Config, store service.Store, tokenSvc service.TokenService, wl whitelist.Checker,
) service.Service {
	t.Helper()
	return service.NewService(cfg, store, tokenSvc, wl)
}

// ─── ChainID ──────────────────────────────────────────────────────────────────

func TestService_ChainID(t *testing.T) {
	cfg := defaultCfg()
	cfg.ChainID = 12345
	svc := newSvc(t, cfg, nil, nil)
	assert.Equal(t, hexutil.Uint64(12345), svc.ChainID(context.Background()))
}

// ─── BlockNumber ──────────────────────────────────────────────────────────────

func TestService_BlockNumber(t *testing.T) {
	t.Run("adds 12-block confirmation buffer to latest", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetLatestEvmBlockNumber(mock.Anything).Return(uint64(100), nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, uint64(got), uint64(112)) // 100 + confirmationBufferBlocks
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetLatestEvmBlockNumber(mock.Anything).Return(uint64(0), errors.New("db down"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.BlockNumber(context.Background())
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── GasPrice ─────────────────────────────────────────────────────────────────

// Gas is fixed at 0 in code (not configurable). See TestService_ZeroGas for the
// MetaMask-compatibility rationale.
func TestService_GasPrice(t *testing.T) {
	got, err := newSvc(t, defaultCfg(), nil, nil).GasPrice(context.Background())
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(0), got.ToInt())
}

// ─── MaxPriorityFeePerGas ─────────────────────────────────────────────────────

func TestService_MaxPriorityFeePerGas(t *testing.T) {
	got, err := newSvc(t, defaultCfg(), nil, nil).MaxPriorityFeePerGas(context.Background())
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(0), got.ToInt())
}

// ─── Zero gas (MetaMask compatibility) ────────────────────────────────────────

// TestService_ZeroGas locks in the MetaMask-compatibility contract: gas is fixed
// at 0 across every fee surface a wallet reads, so a zero native balance
// satisfies MetaMask's `balance >= value + gas*price` pre-flight check for the
// zero-value ERC-20 transfers this facade accepts. A non-zero gas price would
// make MetaMask reject transfers with "insufficient funds for gas".
func TestService_ZeroGas(t *testing.T) {
	blockNum := hexutil.Uint64(42)
	got, err := newSvc(t, defaultCfg(), nil, nil).
		GetBlockByNumber(context.Background(), ethrpc.BlockNumberOrHash{BlockNumber: &blockNum}, false)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, big.NewInt(0), got.BaseFeePerGas.ToInt())
}

// ─── EstimateGas ──────────────────────────────────────────────────────────────

func TestService_EstimateGas(t *testing.T) {
	got, err := newSvc(t, defaultCfg(), nil, nil).EstimateGas(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, hexutil.Uint64(service.DefaultGasLimit), got)
}

// ─── GetBalance ───────────────────────────────────────────────────────────────

func TestService_GetBalance(t *testing.T) {
	addr := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	want := big.NewInt(5_000_000_000_000_000_000)

	t.Run("success", func(t *testing.T) {
		mockNative := mocks.NewNative(t)
		mockNative.EXPECT().GetBalance(mock.Anything, addr).Return(*want, nil)
		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().Native().Return(mockNative)
		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)

		got, err := svc.GetBalance(context.Background(), addr)
		require.NoError(t, err)
		assert.Equal(t, want, got.ToInt())
	})

	t.Run("token service error propagates", func(t *testing.T) {
		mockNative := mocks.NewNative(t)
		mockNative.EXPECT().GetBalance(mock.Anything, addr).Return(big.Int{}, errors.New("unavailable"))
		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().Native().Return(mockNative)
		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)

		_, err := svc.GetBalance(context.Background(), addr)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── GetTransactionCount ──────────────────────────────────────────────────────

func TestService_GetTransactionCount(t *testing.T) {
	addr := common.HexToAddress("0xAAAA000000000000000000000000000000000001")

	t.Run("returns nonce from store", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransactionCount(mock.Anything, addr.Hex()).Return(uint64(5), nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionCount(context.Background(), addr)
		require.NoError(t, err)
		assert.Equal(t, hexutil.Uint64(5), got)
	})

	t.Run("zero for new account", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransactionCount(mock.Anything, addr.Hex()).Return(uint64(0), nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionCount(context.Background(), addr)
		require.NoError(t, err)
		assert.Equal(t, hexutil.Uint64(0), got)
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransactionCount(mock.Anything, addr.Hex()).Return(uint64(0), errors.New("db down"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetTransactionCount(context.Background(), addr)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── GetCode ──────────────────────────────────────────────────────────────────

func TestService_GetCode(t *testing.T) {
	supportedAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	unknownAddr := common.HexToAddress("0x9999999999999999999999999999999999999999")

	t.Run("supported token address returns stub bytecode", func(t *testing.T) {
		mockERC20 := mocks.NewERC20(t)
		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(supportedAddr).Return(mockERC20, nil)
		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)

		got, err := svc.GetCode(context.Background(), supportedAddr)
		require.NoError(t, err)
		assert.Equal(t, hexutil.Bytes{0x60, 0x80}, got)
	})

	t.Run("unsupported token address returns empty", func(t *testing.T) {
		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(unknownAddr).Return(nil, errors.New("unsupported token"))
		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)

		got, err := svc.GetCode(context.Background(), unknownAddr)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("nil token service returns empty", func(t *testing.T) {
		svc := newSvc(t, defaultCfg(), nil, nil)

		got, err := svc.GetCode(context.Background(), unknownAddr)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// ─── Syncing ──────────────────────────────────────────────────────────────────

func TestService_Syncing(t *testing.T) {
	svc := newSvc(t, defaultCfg(), nil, nil)
	assert.False(t, svc.Syncing(context.Background()))
}

// ─── SendRawTransaction ───────────────────────────────────────────────────────

func TestService_SendRawTransaction(t *testing.T) {
	chainID := big.NewInt(31337)
	tokenAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	recipient := common.HexToAddress("0x3000000000000000000000000000000000000003")
	amount := big.NewInt(42_000_000_000_000_000)

	t.Run("returns hash immediately after inserting pending mempool entry", func(t *testing.T) {
		payload, expectedHash := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		store := mocks.NewStore(t)
		store.EXPECT().InsertMempoolEntry(mock.Anything, mock.MatchedBy(func(entry *ethrpc.MempoolEntry) bool {
			return entry != nil && entry.RecipientAddress == recipient.Hex() && entry.ContractAddress == tokenAddr.Hex()
		})).Return(nil)

		// ERC20() is called only as a whitelist check; the actual TransferFrom
		// happens asynchronously in the submitter, not here.
		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mocks.NewERC20(t), nil)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		got, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.NoError(t, err)
		assert.Equal(t, expectedHash, got)
	})

	t.Run("unsupported contract returns BadRequestError without touching mempool", func(t *testing.T) {
		unsupportedAddr := common.HexToAddress("0x9999999999999999999999999999999999999999")
		payload, _ := buildSignedTransferTx(t, chainID, unsupportedAddr, recipient, amount)

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(unsupportedAddr).Return(nil, errors.New("token not supported"))

		// Store mock with no expectations — must not be called when whitelist rejects.
		store := mocks.NewStore(t)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
		store.AssertNotCalled(t, "InsertMempoolEntry", mock.Anything, mock.Anything)
	})

	t.Run("InsertMempoolEntry error propagates as DependencyFailure", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mocks.NewERC20(t), nil)

		store := mocks.NewStore(t)
		store.EXPECT().InsertMempoolEntry(mock.Anything, mock.Anything).Return(errors.New("db error"))

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})

	t.Run("whitelisted sender is accepted", func(t *testing.T) {
		payload, expectedHash := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mocks.NewERC20(t), nil)

		store := mocks.NewStore(t)
		store.EXPECT().InsertMempoolEntry(mock.Anything, mock.Anything).Return(nil)

		wl := wlmocks.NewChecker(t)
		wl.EXPECT().IsWhitelisted(mock.Anything, mock.Anything).Return(true, nil).Once()

		svc := newSvcWithWhitelist(t, defaultCfg(), store, mockTokenSvc, wl)
		got, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.NoError(t, err)
		assert.Equal(t, expectedHash, got)
	})

	t.Run("non-whitelisted sender is rejected without touching mempool or token service", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		// Neither the token service nor the store may be consulted once the
		// sender fails the whitelist — leave both bare with no expectations.
		mockTokenSvc := mocks.NewTokenService(t)
		store := mocks.NewStore(t)

		wl := wlmocks.NewChecker(t)
		wl.EXPECT().IsWhitelisted(mock.Anything, mock.Anything).Return(false, nil).Once()

		svc := newSvcWithWhitelist(t, defaultCfg(), store, mockTokenSvc, wl)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})

	t.Run("whitelist lookup error propagates as DependencyFailure", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockTokenSvc := mocks.NewTokenService(t)
		store := mocks.NewStore(t)

		wl := wlmocks.NewChecker(t)
		wl.EXPECT().IsWhitelisted(mock.Anything, mock.Anything).Return(false, errors.New("db down")).Once()

		svc := newSvcWithWhitelist(t, defaultCfg(), store, mockTokenSvc, wl)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})

	t.Run("does not call Canton TransferFrom synchronously", func(t *testing.T) {
		// Regression guard: the whole point of the async path is that
		// SendRawTransaction must never wait on Canton. If ERC20.TransferFrom
		// ever gets wired back in here, this test will fail because the mock
		// is left bare with no expectations.
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockERC20 := mocks.NewERC20(t)
		// No TransferFrom expectation — calling it would fail the mock.

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mockERC20, nil)

		store := mocks.NewStore(t)
		store.EXPECT().InsertMempoolEntry(mock.Anything, mock.Anything).Return(nil)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.NoError(t, err)
	})
}

// ─── GetTransactionReceipt ────────────────────────────────────────────────────

func TestService_GetTransactionReceipt(t *testing.T) {
	txHash := common.HexToHash("0xaabb000000000000000000000000000000000000000000000000000000000001")
	from := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	to := common.HexToAddress("0xBBBB000000000000000000000000000000000001")
	blockHashBytes := common.HexToHash("0x1111000000000000000000000000000000000000000000000000000000000001").Bytes()

	row := &ethrpc.EvmTransaction{
		TxHash:      txHash.Bytes(),
		FromAddress: from.Hex(),
		ToAddress:   to.Hex(),
		Nonce:       3,
		Status:      1,
		BlockNumber: 42,
		BlockHash:   blockHashBytes,
		TxIndex:     0,
		GasUsed:     21000,
	}

	t.Run("found returns receipt with correct fields", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(row, nil)
		store.EXPECT().GetEvmLogsByTxHash(mock.Anything, txHash.Bytes()).Return(nil, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionReceipt(context.Background(), txHash)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, txHash, got.TransactionHash)
		assert.Equal(t, hexutil.Uint64(42), got.BlockNumber)
		assert.Equal(t, hexutil.Uint64(1), got.Status)
		assert.Equal(t, hexutil.Uint64(21000), got.GasUsed)
		assert.Equal(t, from, got.From)
		assert.Equal(t, &to, got.To)
		assert.Empty(t, got.Logs)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(nil, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionReceipt(context.Background(), txHash)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("failed mined entry returns status=0 receipt with revert reason", func(t *testing.T) {
		// After the async refactor, failed mempool entries get mined as status=0
		// EVM transactions with ErrorMessage set. The receipt must surface both
		// the failure status and the human-readable cause so wallets can show
		// it to the user instead of polling forever.
		failedRow := &ethrpc.EvmTransaction{
			TxHash:       txHash.Bytes(),
			FromAddress:  from.Hex(),
			ToAddress:    to.Hex(),
			Nonce:        4,
			Status:       0,
			BlockNumber:  43,
			BlockHash:    blockHashBytes,
			TxIndex:      0,
			GasUsed:      21000,
			ErrorMessage: "canton transfer failed: insufficient balance",
		}

		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(failedRow, nil)
		store.EXPECT().GetEvmLogsByTxHash(mock.Anything, txHash.Bytes()).Return(nil, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionReceipt(context.Background(), txHash)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, hexutil.Uint64(0), got.Status)
		assert.Equal(t, failedRow.ErrorMessage, got.RevertReason)
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(nil, errors.New("db error"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetTransactionReceipt(context.Background(), txHash)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})

	t.Run("GetEvmLogsByTxHash error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(row, nil)
		store.EXPECT().GetEvmLogsByTxHash(mock.Anything, txHash.Bytes()).Return(nil, errors.New("db error"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetTransactionReceipt(context.Background(), txHash)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── GetTransactionByHash ─────────────────────────────────────────────────────

func TestService_GetTransactionByHash(t *testing.T) {
	txHash := common.HexToHash("0xaabb000000000000000000000000000000000000000000000000000000000002")
	from := common.HexToAddress("0xAAAA000000000000000000000000000000000002")
	to := common.HexToAddress("0xBBBB000000000000000000000000000000000002")
	blockHashBytes := common.HexToHash("0x2222000000000000000000000000000000000000000000000000000000000002").Bytes()

	row := &ethrpc.EvmTransaction{
		TxHash:      txHash.Bytes(),
		FromAddress: from.Hex(),
		ToAddress:   to.Hex(),
		Nonce:       7,
		Status:      1,
		BlockNumber: 99,
		BlockHash:   blockHashBytes,
		TxIndex:     2,
		GasUsed:     21000,
		Input:       []byte{0x01, 0x02, 0x03},
	}

	t.Run("found returns transaction with correct fields", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(row, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionByHash(context.Background(), txHash)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, txHash, got.Hash)
		assert.Equal(t, hexutil.Uint64(7), got.Nonce)
		assert.Equal(t, hexutil.Uint64(99), *got.BlockNumber)
		assert.Equal(t, from, got.From)
		assert.Equal(t, &to, got.To)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(nil, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetTransactionByHash(context.Background(), txHash)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmTransaction(mock.Anything, txHash.Bytes()).Return(nil, errors.New("db error"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetTransactionByHash(context.Background(), txHash)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── Call ─────────────────────────────────────────────────────────────────────

func TestService_Call(t *testing.T) {
	contractAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	accountAddr := common.HexToAddress("0x2000000000000000000000000000000000000002")
	spenderAddr := common.HexToAddress("0x3000000000000000000000000000000000000003")
	parsedABI := mustParseERC20ABI(t)

	// callArgs wraps raw calldata into a CallArgs targeting contractAddr.
	makeCallArgs := func(calldata []byte) *ethrpc.CallArgs {
		data := hexutil.Bytes(calldata)
		return &ethrpc.CallArgs{To: &contractAddr, Data: &data}
	}

	// setupCallSvc creates a service whose TokenService.ERC20() returns mockERC20.
	setupCallSvc := func(t *testing.T, mockERC20 *mocks.ERC20) service.Service {
		t.Helper()
		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(contractAddr).Return(mockERC20, nil)
		return newSvc(t, defaultCfg(), nil, mockTokenSvc)
	}

	t.Run("balanceOf returns ABI-encoded balance", func(t *testing.T) {
		balance := big.NewInt(123_000_000_000_000_000)
		calldata, err := parsedABI.Pack("balanceOf", accountAddr)
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().BalanceOf(mock.Anything, accountAddr).Return(*balance)
		svc := setupCallSvc(t, mockERC20)

		got, err := svc.Call(context.Background(), makeCallArgs(calldata))
		require.NoError(t, err)
		require.NotEmpty(t, got)
	})

	t.Run("decimals returns ABI-encoded uint8", func(t *testing.T) {
		calldata, err := parsedABI.Pack("decimals")
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().Decimals(mock.Anything).Return(uint8(18))
		svc := setupCallSvc(t, mockERC20)

		got, err := svc.Call(context.Background(), makeCallArgs(calldata))
		require.NoError(t, err)
		require.NotEmpty(t, got)
	})

	t.Run("symbol returns ABI-encoded string", func(t *testing.T) {
		calldata, err := parsedABI.Pack("symbol")
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().Symbol(mock.Anything).Return("PROMPT")
		svc := setupCallSvc(t, mockERC20)

		got, err := svc.Call(context.Background(), makeCallArgs(calldata))
		require.NoError(t, err)
		require.NotEmpty(t, got)
	})

	t.Run("name returns ABI-encoded string", func(t *testing.T) {
		calldata, err := parsedABI.Pack("name")
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().Name(mock.Anything).Return("Prompt Token")
		svc := setupCallSvc(t, mockERC20)

		got, err := svc.Call(context.Background(), makeCallArgs(calldata))
		require.NoError(t, err)
		require.NotEmpty(t, got)
	})

	t.Run("totalSupply returns ABI-encoded uint256", func(t *testing.T) {
		supply := big.NewInt(1_000_000_000_000_000_000)
		calldata, err := parsedABI.Pack("totalSupply")
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().TotalSupply(mock.Anything).Return(*supply)
		svc := setupCallSvc(t, mockERC20)

		got, err := svc.Call(context.Background(), makeCallArgs(calldata))
		require.NoError(t, err)
		require.NotEmpty(t, got)
	})

	t.Run("allowance returns ABI-encoded uint256", func(t *testing.T) {
		allowance := big.NewInt(500_000_000)
		calldata, err := parsedABI.Pack("allowance", accountAddr, spenderAddr)
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().Allowance(mock.Anything, accountAddr, spenderAddr).Return(*allowance)
		svc := setupCallSvc(t, mockERC20)

		got, err := svc.Call(context.Background(), makeCallArgs(calldata))
		require.NoError(t, err)
		require.NotEmpty(t, got)
	})

	t.Run("unsupported contract returns BadRequestError", func(t *testing.T) {
		unsupportedAddr := common.HexToAddress("0x9999999999999999999999999999999999999999")
		data := hexutil.Bytes([]byte{0x01, 0x02, 0x03, 0x04})
		args := &ethrpc.CallArgs{To: &unsupportedAddr, Data: &data}

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(unsupportedAddr).Return(nil, errors.New("token not supported"))
		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)

		_, err := svc.Call(context.Background(), args)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("nil args returns error", func(t *testing.T) {
		// No tokenService needed: nil check fires before ERC20() is called.
		svc := newSvc(t, defaultCfg(), nil, nil)
		_, err := svc.Call(context.Background(), nil)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("calldata shorter than selector returns error", func(t *testing.T) {
		// tokenService.ERC20 is called before length check in service.go.
		mockERC20 := mocks.NewERC20(t)
		svc := setupCallSvc(t, mockERC20)
		_, err := svc.Call(context.Background(), makeCallArgs([]byte{0x01, 0x02}))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("unsupported ERC20 method returns error", func(t *testing.T) {
		// `approve` is in the ABI but not in the service's switch.
		calldata, err := parsedABI.Pack("approve", spenderAddr, big.NewInt(100))
		require.NoError(t, err)

		mockERC20 := mocks.NewERC20(t)
		svc := setupCallSvc(t, mockERC20)
		_, err = svc.Call(context.Background(), makeCallArgs(calldata))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryNotSupported))
	})
}

// ─── GetLogs ──────────────────────────────────────────────────────────────────

func TestService_GetLogs(t *testing.T) {
	contractAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	from := hexutil.Uint64(0)
	to := hexutil.Uint64(100)
	// Explicit FromBlock/ToBlock avoids the store.GetLatestEvmBlockNumber() branch.
	query := ethrpc.FilterQuery{
		FromBlock: &from,
		ToBlock:   &to,
		Address:   contractAddr,
	}

	t.Run("empty result", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmLogs(mock.Anything, mock.Anything, mock.Anything, uint64(0), uint64(100)).Return(nil, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetLogs(context.Background(), query)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("logs are converted and returned", func(t *testing.T) {
		txHash := common.HexToHash("0xaaaa000000000000000000000000000000000000000000000000000000000001")
		blockHash := common.HexToHash("0xbbbb000000000000000000000000000000000000000000000000000000000001")
		dbLog := &ethrpc.EvmLog{
			TxHash:      txHash.Bytes(),
			LogIndex:    0,
			Address:     contractAddr.Bytes(),
			BlockNumber: 50,
			BlockHash:   blockHash.Bytes(),
			TxIndex:     0,
		}

		store := mocks.NewStore(t)
		store.EXPECT().GetEvmLogs(mock.Anything, mock.Anything, mock.Anything, uint64(0), uint64(100)).
			Return([]*ethrpc.EvmLog{dbLog}, nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetLogs(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, contractAddr, got[0].Address)
		assert.Equal(t, txHash, got[0].TxHash)
		assert.Equal(t, uint64(50), got[0].BlockNumber)
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetEvmLogs(mock.Anything, mock.Anything, mock.Anything, uint64(0), uint64(100)).
			Return(nil, errors.New("db error"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetLogs(context.Background(), query)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── GetBlockByNumber ─────────────────────────────────────────────────────────

func TestService_GetBlockByNumber(t *testing.T) {
	t.Run("specific block number returns synthetic block", func(t *testing.T) {
		blockNum := hexutil.Uint64(42)
		svc := newSvc(t, defaultCfg(), nil, nil)

		got, err := svc.GetBlockByNumber(context.Background(), ethrpc.BlockNumberOrHash{BlockNumber: &blockNum}, false)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, hexutil.Uint64(42), got.Number)
		// Hash must be non-zero and deterministic from chain+block
		assert.NotEqual(t, common.Hash{}, got.Hash)
	})

	t.Run("block zero returns nil", func(t *testing.T) {
		zero := hexutil.Uint64(0)
		svc := newSvc(t, defaultCfg(), nil, nil)

		got, err := svc.GetBlockByNumber(context.Background(), ethrpc.BlockNumberOrHash{BlockNumber: &zero}, false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("nil block number resolves to latest with confirmation buffer", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetLatestEvmBlockNumber(mock.Anything).Return(uint64(77), nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetBlockByNumber(context.Background(), ethrpc.BlockNumberOrHash{}, false)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.GreaterOrEqual(t, uint64(got.Number), uint64(89)) // 77 + 12
	})

	t.Run("store error when resolving latest", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetLatestEvmBlockNumber(mock.Anything).Return(uint64(0), errors.New("db down"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetBlockByNumber(context.Background(), ethrpc.BlockNumberOrHash{}, false)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── GetBlockByHash ───────────────────────────────────────────────────────────

func TestService_GetBlockByHash(t *testing.T) {
	blockHash := common.HexToHash("0x6666000000000000000000000000000000000000000000000000000000000001")

	t.Run("found returns block for stored number", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetBlockNumberByHash(mock.Anything, blockHash.Bytes()).Return(uint64(55), nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetBlockByHash(context.Background(), blockHash, false)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, hexutil.Uint64(55), got.Number)
	})

	t.Run("hash not in store falls back to latest block", func(t *testing.T) {
		// blockNum=0 means not found; service falls back to GetBlockByNumber("latest")
		store := mocks.NewStore(t)
		store.EXPECT().GetBlockNumberByHash(mock.Anything, blockHash.Bytes()).Return(uint64(0), nil)
		store.EXPECT().GetLatestEvmBlockNumber(mock.Anything).Return(uint64(50), nil)
		svc := newSvc(t, defaultCfg(), store, nil)

		got, err := svc.GetBlockByHash(context.Background(), blockHash, false)
		require.NoError(t, err)
		require.NotNil(t, got)                                   // returns latest, not nil
		assert.GreaterOrEqual(t, uint64(got.Number), uint64(62)) // 50 + 12
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().GetBlockNumberByHash(mock.Anything, blockHash.Bytes()).Return(uint64(0), errors.New("db error"))
		svc := newSvc(t, defaultCfg(), store, nil)

		_, err := svc.GetBlockByHash(context.Background(), blockHash, false)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}
