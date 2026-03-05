package service_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service/mocks"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// defaultCfg returns a minimal EthRPCConfig suitable for unit tests.
func defaultCfg() *config.EthRPCConfig {
	return &config.EthRPCConfig{
		ChainID:          31337,
		GasPriceWei:      "1000000000",
		GasLimit:         21000,
		TokenAddress:     common.HexToAddress("0x1000000000000000000000000000000000000001"),
		DemoTokenAddress: common.HexToAddress("0x2000000000000000000000000000000000000002"),
	}
}

// newSvc creates a real ethService backed by the supplied (possibly nil) dependencies.
func newSvc(t *testing.T, cfg *config.EthRPCConfig, store service.Store, tokenSvc service.TokenService) service.Service {
	t.Helper()
	return service.NewService(cfg, store, tokenSvc)
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

func TestService_GasPrice(t *testing.T) {
	t.Run("valid config returns configured price", func(t *testing.T) {
		svc := newSvc(t, defaultCfg(), nil, nil)

		got, err := svc.GasPrice(context.Background())
		require.NoError(t, err)
		assert.Equal(t, big.NewInt(1_000_000_000), got.ToInt())
	})

	t.Run("non-numeric wei string returns error", func(t *testing.T) {
		cfg := defaultCfg()
		cfg.GasPriceWei = "not-a-number"
		svc := newSvc(t, cfg, nil, nil)

		_, err := svc.GasPrice(context.Background())
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryGeneralError))
	})
}

// ─── MaxPriorityFeePerGas ─────────────────────────────────────────────────────

func TestService_MaxPriorityFeePerGas(t *testing.T) {
	svc := newSvc(t, defaultCfg(), nil, nil)

	got, err := svc.MaxPriorityFeePerGas(context.Background())
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(1_000_000_000), got.ToInt())
}

// ─── EstimateGas ──────────────────────────────────────────────────────────────

func TestService_EstimateGas(t *testing.T) {
	cfg := defaultCfg()
	cfg.GasLimit = 50_000
	svc := newSvc(t, cfg, nil, nil)

	got, err := svc.EstimateGas(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, hexutil.Uint64(50_000), got)
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
	cfg := defaultCfg()
	svc := newSvc(t, cfg, nil, nil)

	t.Run("primary token address returns stub bytecode", func(t *testing.T) {
		got, err := svc.GetCode(context.Background(), cfg.TokenAddress)
		require.NoError(t, err)
		assert.Equal(t, hexutil.Bytes{0x60, 0x80}, got)
	})

	t.Run("demo token address returns stub bytecode", func(t *testing.T) {
		got, err := svc.GetCode(context.Background(), cfg.DemoTokenAddress)
		require.NoError(t, err)
		assert.Equal(t, hexutil.Bytes{0x60, 0x80}, got)
	})

	t.Run("unknown address returns empty", func(t *testing.T) {
		unknown := common.HexToAddress("0x9999999999999999999999999999999999999999")
		got, err := svc.GetCode(context.Background(), unknown)
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
	blockHashBytes := make([]byte, 32)

	t.Run("success stores tx and log then returns hash", func(t *testing.T) {
		payload, expectedHash := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().
			TransferFrom(mock.Anything, mock.Anything, recipient, mock.Anything).
			Return(nil)

		store := mocks.NewStore(t)
		store.EXPECT().NextEvmBlock(mock.Anything, uint64(31337)).Return(uint64(1), blockHashBytes, 0, nil)
		store.EXPECT().SaveEvmTransaction(mock.Anything, mock.Anything).Return(nil)
		store.EXPECT().SaveEvmLog(mock.Anything, mock.Anything).Return(nil)

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mockERC20, nil)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		got, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.NoError(t, err)
		assert.Equal(t, expectedHash, got)
	})

	t.Run("unsupported contract returns BadRequestError", func(t *testing.T) {
		unsupportedAddr := common.HexToAddress("0x9999999999999999999999999999999999999999")
		payload, _ := buildSignedTransferTx(t, chainID, unsupportedAddr, recipient, amount)

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(unsupportedAddr).Return(nil, errors.New("token not supported"))

		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("TransferFrom categorized error is passed through", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().
			TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(apperr.BadRequestError(errors.New("user not found"), "failed to get sender"))

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mockERC20, nil)

		svc := newSvc(t, defaultCfg(), nil, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("NextEvmBlock error propagates", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().
			TransferFrom(mock.Anything, mock.Anything, recipient, mock.Anything).
			Return(nil)

		store := mocks.NewStore(t)
		store.EXPECT().NextEvmBlock(mock.Anything, uint64(31337)).Return(uint64(0), nil, 0, errors.New("next block failed"))

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mockERC20, nil)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})

	t.Run("SaveEvmTransaction error propagates", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().
			TransferFrom(mock.Anything, mock.Anything, recipient, mock.Anything).
			Return(nil)

		store := mocks.NewStore(t)
		store.EXPECT().NextEvmBlock(mock.Anything, uint64(31337)).Return(uint64(1), blockHashBytes, 0, nil)
		store.EXPECT().SaveEvmTransaction(mock.Anything, mock.Anything).Return(errors.New("save tx failed"))

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mockERC20, nil)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})

	t.Run("SaveEvmLog error propagates", func(t *testing.T) {
		payload, _ := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		mockERC20 := mocks.NewERC20(t)
		mockERC20.EXPECT().
			TransferFrom(mock.Anything, mock.Anything, recipient, mock.Anything).
			Return(nil)

		store := mocks.NewStore(t)
		store.EXPECT().NextEvmBlock(mock.Anything, uint64(31337)).Return(uint64(1), blockHashBytes, 0, nil)
		store.EXPECT().SaveEvmTransaction(mock.Anything, mock.Anything).Return(nil)
		store.EXPECT().SaveEvmLog(mock.Anything, mock.Anything).Return(errors.New("save log failed"))

		mockTokenSvc := mocks.NewTokenService(t)
		mockTokenSvc.EXPECT().ERC20(tokenAddr).Return(mockERC20, nil)

		svc := newSvc(t, defaultCfg(), store, mockTokenSvc)
		_, err := svc.SendRawTransaction(context.Background(), hexutil.Bytes(payload))
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
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
