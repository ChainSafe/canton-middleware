package service_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/config"
	canton "github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service/mocks"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newTestServer spins up a real httptest.Server with RegisterRoutes and returns
// both a typed ethclient.Client and a raw rpc.Client backed by the same connection.
func newTestServer(t *testing.T, svc service.Service) (*ethclient.Client, *rpc.Client, func()) {
	t.Helper()
	r := chi.NewRouter()
	service.RegisterRoutes(r, svc, 30*time.Second, zap.NewNop())
	srv := httptest.NewServer(r)
	rpcClient, err := rpc.Dial(srv.URL + "/eth")
	require.NoError(t, err)
	cleanup := func() {
		rpcClient.Close()
		srv.Close()
	}
	return ethclient.NewClient(rpcClient), rpcClient, cleanup
}

func mustParseERC20ABI(t *testing.T) abi.ABI {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(canton.ERC20ABI))
	require.NoError(t, err)
	return parsed
}

func abiEncodeUint256(t *testing.T, v *big.Int) hexutil.Bytes {
	t.Helper()
	uint256Type, err := abi.NewType("uint256", "", nil)
	require.NoError(t, err)
	encoded, err := abi.Arguments{{Type: uint256Type}}.Pack(v)
	require.NoError(t, err)
	return encoded
}

func abiEncodeUint8(t *testing.T, v uint8) hexutil.Bytes {
	t.Helper()
	uint8Type, err := abi.NewType("uint8", "", nil)
	require.NoError(t, err)
	encoded, err := abi.Arguments{{Type: uint8Type}}.Pack(v)
	require.NoError(t, err)
	return encoded
}

func abiEncodeString(t *testing.T, s string) hexutil.Bytes {
	t.Helper()
	stringType, err := abi.NewType("string", "", nil)
	require.NoError(t, err)
	encoded, err := abi.Arguments{{Type: stringType}}.Pack(s)
	require.NoError(t, err)
	return encoded
}

// buildSignedTransferTx creates a signed ERC20 transfer transaction for use in SendRawTransaction tests.
func buildSignedTransferTx(t *testing.T, chainID *big.Int, tokenAddr, recipient common.Address, amount *big.Int) ([]byte, common.Hash) {
	t.Helper()
	parsedABI := mustParseERC20ABI(t)
	calldata, err := parsedABI.Pack("transfer", recipient, amount)
	require.NoError(t, err)

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	rawTx := types.NewTx(&types.LegacyTx{
		Nonce:    0,
		To:       &tokenAddr,
		Value:    big.NewInt(0),
		Gas:      21000,
		GasPrice: big.NewInt(1_000_000_000),
		Data:     calldata,
	})
	signer := types.LatestSignerForChainID(chainID)
	signed, err := types.SignTx(rawTx, signer, key)
	require.NoError(t, err)

	payload, err := signed.MarshalBinary()
	require.NoError(t, err)
	return payload, signed.Hash()
}

// ─── eth_chainId ──────────────────────────────────────────────────────────────

func TestEthAPI_ChainId(t *testing.T) {
	// ChainID() returns hexutil.Uint64 with no error path.

	t.Run("returns configured chain id", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().ChainID().Return(hexutil.Uint64(31337))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.ChainID(context.Background())
		require.NoError(t, err)
		assert.Equal(t, big.NewInt(31337), got)
	})

	t.Run("returns mainnet chain id", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().ChainID().Return(hexutil.Uint64(1))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.ChainID(context.Background())
		require.NoError(t, err)
		assert.Equal(t, big.NewInt(1), got)
	})
}

// ─── eth_blockNumber ──────────────────────────────────────────────────────────

func TestEthAPI_BlockNumber(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().BlockNumber().Return(hexutil.Uint64(1012), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint64(1012), got)
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().BlockNumber().Return(hexutil.Uint64(0), errors.New("db unavailable"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.BlockNumber(context.Background())
		fmt.Println(err)
		require.Error(t, err)
	})
}

// ─── eth_gasPrice ─────────────────────────────────────────────────────────────

func TestEthAPI_GasPrice(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := big.NewInt(1_000_000_000)
		svc := mocks.NewService(t)
		svc.EXPECT().GasPrice().Return((*hexutil.Big)(want), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.SuggestGasPrice(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GasPrice().Return(nil, errors.New("invalid gas price config"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.SuggestGasPrice(context.Background())
		require.Error(t, err)
	})
}

// ─── eth_maxPriorityFeePerGas ─────────────────────────────────────────────────

func TestEthAPI_MaxPriorityFeePerGas(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := big.NewInt(1_500_000_000)
		svc := mocks.NewService(t)
		svc.EXPECT().MaxPriorityFeePerGas().Return((*hexutil.Big)(want), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.SuggestGasTipCap(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().MaxPriorityFeePerGas().Return(nil, errors.New("fee error"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.SuggestGasTipCap(context.Background())
		require.Error(t, err)
	})
}

// ─── eth_estimateGas ──────────────────────────────────────────────────────────

func TestEthAPI_EstimateGas(t *testing.T) {
	tokenAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")

	t.Run("success", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().EstimateGas(mock.Anything, mock.Anything).Return(hexutil.Uint64(21000), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.EstimateGas(context.Background(), geth.CallMsg{To: &tokenAddr})
		require.NoError(t, err)
		assert.Equal(t, uint64(21000), got)
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().EstimateGas(mock.Anything, mock.Anything).Return(hexutil.Uint64(0), errors.New("execution reverted"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.EstimateGas(context.Background(), geth.CallMsg{To: &tokenAddr})
		require.Error(t, err)
	})
}

// ─── eth_getBalance ───────────────────────────────────────────────────────────

func TestEthAPI_GetBalance(t *testing.T) {
	addr := common.HexToAddress("0x1234000000000000000000000000000000000001")

	t.Run("success", func(t *testing.T) {
		want := big.NewInt(5_000_000_000_000_000_000)
		svc := mocks.NewService(t)
		svc.EXPECT().GetBalance(mock.Anything, addr).Return((*hexutil.Big)(want), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.BalanceAt(context.Background(), addr, nil)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("zero balance", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetBalance(mock.Anything, addr).Return((*hexutil.Big)(big.NewInt(0)), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.BalanceAt(context.Background(), addr, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, got.Sign())
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetBalance(mock.Anything, addr).Return(nil, errors.New("balance lookup failed"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.BalanceAt(context.Background(), addr, nil)
		require.Error(t, err)
	})
}

// ─── eth_getTransactionCount ──────────────────────────────────────────────────

func TestEthAPI_GetTransactionCount(t *testing.T) {
	addr := common.HexToAddress("0x1234000000000000000000000000000000000001")

	t.Run("success", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionCount(mock.Anything, addr).Return(hexutil.Uint64(7), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.NonceAt(context.Background(), addr, nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(7), got)
	})

	t.Run("zero nonce for new account", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionCount(mock.Anything, addr).Return(hexutil.Uint64(0), nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.NonceAt(context.Background(), addr, nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), got)
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionCount(mock.Anything, addr).Return(hexutil.Uint64(0), errors.New("db error"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.NonceAt(context.Background(), addr, nil)
		require.Error(t, err)
	})
}

// ─── eth_getCode ──────────────────────────────────────────────────────────────

func TestEthAPI_GetCode(t *testing.T) {
	tokenAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	unknownAddr := common.HexToAddress("0x9999999999999999999999999999999999999999")

	t.Run("known token address returns bytecode", func(t *testing.T) {
		wantCode := hexutil.Bytes{0x60, 0x80}
		svc := mocks.NewService(t)
		svc.EXPECT().GetCode(mock.Anything, tokenAddr).Return(wantCode, nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.CodeAt(context.Background(), tokenAddr, nil)
		require.NoError(t, err)
		assert.Equal(t, []byte(wantCode), got)
	})

	t.Run("unknown address returns empty", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetCode(mock.Anything, unknownAddr).Return(hexutil.Bytes{}, nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		got, err := ethClient.CodeAt(context.Background(), unknownAddr, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("service error", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetCode(mock.Anything, tokenAddr).Return(nil, errors.New("code lookup failed"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.CodeAt(context.Background(), tokenAddr, nil)
		require.Error(t, err)
	})
}

// ─── eth_syncing ──────────────────────────────────────────────────────────────

func TestEthAPI_Syncing(t *testing.T) {
	// Syncing() returns bool with no error path.

	t.Run("not syncing returns nil progress", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().Syncing().Return(false)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		progress, err := ethClient.SyncProgress(context.Background())
		require.NoError(t, err)
		assert.Nil(t, progress)
	})
}

// ─── eth_sendRawTransaction ───────────────────────────────────────────────────

func TestEthAPI_SendRawTransaction(t *testing.T) {
	chainID := big.NewInt(31337)
	tokenAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	recipient := common.HexToAddress("0x3000000000000000000000000000000000000003")
	amount := big.NewInt(42_000_000_000_000_000)

	t.Run("success returns tx hash", func(t *testing.T) {
		payload, expectedHash := buildSignedTransferTx(t, chainID, tokenAddr, recipient, amount)

		svc := mocks.NewService(t)
		svc.EXPECT().SendRawTransaction(mock.Anything, hexutil.Bytes(payload)).Return(expectedHash, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var gotHash common.Hash
		err := rpcClient.Call(&gotHash, "eth_sendRawTransaction", hexutil.Bytes(payload))
		require.NoError(t, err)
		assert.Equal(t, expectedHash, gotHash)
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().SendRawTransaction(mock.Anything, mock.Anything).
			Return(common.Hash{}, apperr.BadRequestError(errors.New("bad rlp"), "invalid transaction encoding"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var gotHash common.Hash
		err := rpcClient.Call(&gotHash, "eth_sendRawTransaction", hexutil.Bytes{0xde, 0xad})
		require.Error(t, err)
		var rpcErr rpc.Error
		require.ErrorAs(t, err, &rpcErr)
		assert.Equal(t, -32602, rpcErr.ErrorCode())
	})

	// The following subtests exercise validation inside the real ethService implementation
	// (not a mock) so they reach the code paths changed above.
	realSvc := func(t *testing.T) (*rpc.Client, func()) {
		t.Helper()
		cfg := &config.EthRPCConfig{ChainID: chainID.Uint64()}
		// Store and TokenService are nil because these validation paths never reach them.
		_, rpcClient, cleanup := newTestServer(t, service.NewService(cfg, nil, nil))
		return rpcClient, cleanup
	}

	t.Run("invalid signature returns -32602", func(t *testing.T) {
		// An unsigned transaction has zero V/R/S; types.Sender fails to recover the key.
		rawTx := types.NewTx(&types.LegacyTx{
			Nonce: 0, To: &tokenAddr, Value: big.NewInt(0),
			Gas: 21000, GasPrice: big.NewInt(1_000_000_000),
		})
		payload, err := rawTx.MarshalBinary()
		require.NoError(t, err)

		rpcClient, cleanup := realSvc(t)
		defer cleanup()

		var gotHash common.Hash
		err = rpcClient.Call(&gotHash, "eth_sendRawTransaction", hexutil.Bytes(payload))
		require.Error(t, err)
		var rpcErr rpc.Error
		require.ErrorAs(t, err, &rpcErr)
		assert.Equal(t, -32602, rpcErr.ErrorCode())
	})

	t.Run("deploy tx returns -32602", func(t *testing.T) {
		// A deploy transaction has no To address; the service rejects it as bad input.
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		rawTx := types.NewTx(&types.LegacyTx{
			Nonce: 0, To: nil, Value: big.NewInt(0),
			Gas: 21000, GasPrice: big.NewInt(1_000_000_000),
		})
		signed, err := types.SignTx(rawTx, types.LatestSignerForChainID(chainID), key)
		require.NoError(t, err)
		payload, err := signed.MarshalBinary()
		require.NoError(t, err)

		rpcClient, cleanup := realSvc(t)
		defer cleanup()

		var gotHash common.Hash
		err = rpcClient.Call(&gotHash, "eth_sendRawTransaction", hexutil.Bytes(payload))
		require.Error(t, err)
		var rpcErr rpc.Error
		require.ErrorAs(t, err, &rpcErr)
		assert.Equal(t, -32602, rpcErr.ErrorCode())
	})

	t.Run("eth value transfer returns -32601", func(t *testing.T) {
		// A transaction with non-zero Value attempts a native ETH transfer, which is not supported.
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		rawTx := types.NewTx(&types.LegacyTx{
			Nonce: 0, To: &tokenAddr, Value: big.NewInt(1_000_000_000),
			Gas: 21000, GasPrice: big.NewInt(1_000_000_000),
		})
		signed, err := types.SignTx(rawTx, types.LatestSignerForChainID(chainID), key)
		require.NoError(t, err)
		payload, err := signed.MarshalBinary()
		require.NoError(t, err)

		rpcClient, cleanup := realSvc(t)
		defer cleanup()

		var gotHash common.Hash
		err = rpcClient.Call(&gotHash, "eth_sendRawTransaction", hexutil.Bytes(payload))
		require.Error(t, err)
		var rpcErr rpc.Error
		require.ErrorAs(t, err, &rpcErr)
		assert.Equal(t, -32601, rpcErr.ErrorCode())
	})
}

// ─── eth_getTransactionReceipt ────────────────────────────────────────────────

func TestEthAPI_GetTransactionReceipt(t *testing.T) {
	txHash := common.HexToHash("0xaabbccdd00000000000000000000000000000000000000000000000000000001")
	blockHash := common.HexToHash("0x1111000000000000000000000000000000000000000000000000000000000001")
	from := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	to := common.HexToAddress("0xBBBB000000000000000000000000000000000001")

	receipt := &ethrpc.RPCReceipt{
		TransactionHash:   txHash,
		TransactionIndex:  hexutil.Uint(0),
		BlockHash:         blockHash,
		BlockNumber:       hexutil.Uint64(100),
		From:              from,
		To:                &to,
		CumulativeGasUsed: hexutil.Uint64(21000),
		GasUsed:           hexutil.Uint64(21000),
		ContractAddress:   nil,
		Logs:              []*types.Log{},
		Status:            hexutil.Uint64(1),
		EffectiveGasPrice: hexutil.Uint64(1_000_000_000),
		Type:              hexutil.Uint64(2),
	}

	t.Run("found returns full receipt", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionReceipt(mock.Anything, txHash).Return(receipt, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCReceipt
		err := rpcClient.Call(&got, "eth_getTransactionReceipt", txHash)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, txHash, got.TransactionHash)
		assert.Equal(t, blockHash, got.BlockHash)
		assert.Equal(t, hexutil.Uint64(100), got.BlockNumber)
		assert.Equal(t, hexutil.Uint64(1), got.Status)
		assert.Equal(t, hexutil.Uint64(21000), got.GasUsed)
		assert.Equal(t, &to, got.To)
	})

	t.Run("not found returns null", func(t *testing.T) {
		unknownHash := common.HexToHash("0xdeadbeef00000000000000000000000000000000000000000000000000000001")
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionReceipt(mock.Anything, unknownHash).Return(nil, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCReceipt
		err := rpcClient.Call(&got, "eth_getTransactionReceipt", unknownHash)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionReceipt(mock.Anything, txHash).Return(nil, errors.New("db error"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCReceipt
		err := rpcClient.Call(&got, "eth_getTransactionReceipt", txHash)
		require.Error(t, err)
	})
}

// ─── eth_getTransactionByHash ─────────────────────────────────────────────────

func TestEthAPI_GetTransactionByHash(t *testing.T) {
	txHash := common.HexToHash("0xaabbccdd00000000000000000000000000000000000000000000000000000002")
	blockHash := common.HexToHash("0x2222000000000000000000000000000000000000000000000000000000000002")
	from := common.HexToAddress("0xAAAA000000000000000000000000000000000002")
	to := common.HexToAddress("0xBBBB000000000000000000000000000000000002")
	blockNum := hexutil.Uint64(200)
	txIndex := hexutil.Uint(0)

	tx := &ethrpc.RPCTransaction{
		Hash:             txHash,
		Nonce:            hexutil.Uint64(3),
		BlockHash:        &blockHash,
		BlockNumber:      &blockNum,
		TransactionIndex: &txIndex,
		From:             from,
		To:               &to,
		Value:            (*hexutil.Big)(big.NewInt(0)),
		GasPrice:         (*hexutil.Big)(big.NewInt(1_000_000_000)),
		Gas:              hexutil.Uint64(21000),
		Input:            hexutil.Bytes{0x01, 0x02},
		Type:             hexutil.Uint64(2),
	}

	t.Run("found returns full transaction", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionByHash(mock.Anything, txHash).Return(tx, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCTransaction
		err := rpcClient.Call(&got, "eth_getTransactionByHash", txHash)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, txHash, got.Hash)
		assert.Equal(t, hexutil.Uint64(3), got.Nonce)
		assert.Equal(t, &blockHash, got.BlockHash)
		assert.Equal(t, blockNum, *got.BlockNumber)
		assert.Equal(t, from, got.From)
		assert.Equal(t, &to, got.To)
	})

	t.Run("not found returns null", func(t *testing.T) {
		unknownHash := common.HexToHash("0xdeadbeef00000000000000000000000000000000000000000000000000000002")
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionByHash(mock.Anything, unknownHash).Return(nil, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCTransaction
		err := rpcClient.Call(&got, "eth_getTransactionByHash", unknownHash)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetTransactionByHash(mock.Anything, txHash).Return(nil, errors.New("db error"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCTransaction
		err := rpcClient.Call(&got, "eth_getTransactionByHash", txHash)
		require.Error(t, err)
	})
}

// ─── eth_call ─────────────────────────────────────────────────────────────────

func TestEthAPI_Call(t *testing.T) {
	contractAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	accountAddr := common.HexToAddress("0x2000000000000000000000000000000000000002")
	parsedABI := mustParseERC20ABI(t)

	ethCall := func(t *testing.T, rpcClient *rpc.Client, calldata []byte) (hexutil.Bytes, error) {
		t.Helper()
		callArgs := map[string]any{
			"to":   contractAddr.Hex(),
			"data": hexutil.Encode(calldata),
		}
		var result hexutil.Bytes
		err := rpcClient.Call(&result, "eth_call", callArgs, "latest")
		return result, err
	}

	t.Run("balanceOf", func(t *testing.T) {
		expectedBalance := big.NewInt(123_000_000_000_000_000)
		calldata, err := parsedABI.Pack("balanceOf", accountAddr)
		require.NoError(t, err)

		svc := mocks.NewService(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).Return(abiEncodeUint256(t, expectedBalance), nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		result, err := ethCall(t, rpcClient, calldata)
		require.NoError(t, err)
		values, err := parsedABI.Methods["balanceOf"].Outputs.Unpack(result)
		require.NoError(t, err)
		assert.Equal(t, expectedBalance, values[0].(*big.Int))
	})

	t.Run("decimals", func(t *testing.T) {
		calldata, err := parsedABI.Pack("decimals")
		require.NoError(t, err)

		svc := mocks.NewService(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).Return(abiEncodeUint8(t, 18), nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		result, err := ethCall(t, rpcClient, calldata)
		require.NoError(t, err)
		values, err := parsedABI.Methods["decimals"].Outputs.Unpack(result)
		require.NoError(t, err)
		assert.Equal(t, uint8(18), values[0].(uint8))
	})

	t.Run("symbol", func(t *testing.T) {
		calldata, err := parsedABI.Pack("symbol")
		require.NoError(t, err)

		svc := mocks.NewService(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).Return(abiEncodeString(t, "PROMPT"), nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		result, err := ethCall(t, rpcClient, calldata)
		require.NoError(t, err)
		values, err := parsedABI.Methods["symbol"].Outputs.Unpack(result)
		require.NoError(t, err)
		assert.Equal(t, "PROMPT", values[0].(string))
	})

	t.Run("totalSupply", func(t *testing.T) {
		expectedSupply := big.NewInt(1_000_000_000_000_000_000)
		calldata, err := parsedABI.Pack("totalSupply")
		require.NoError(t, err)

		svc := mocks.NewService(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).Return(abiEncodeUint256(t, expectedSupply), nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		result, err := ethCall(t, rpcClient, calldata)
		require.NoError(t, err)
		values, err := parsedABI.Methods["totalSupply"].Outputs.Unpack(result)
		require.NoError(t, err)
		assert.Equal(t, expectedSupply, values[0].(*big.Int))
	})

	t.Run("unsupported contract returns -32602", func(t *testing.T) {
		calldata, err := parsedABI.Pack("symbol")
		require.NoError(t, err)

		svc := mocks.NewService(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).
			Return(nil, apperr.BadRequestError(errors.New("token not supported"), "contract not supported"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err = ethCall(t, rpcClient, calldata)
		require.Error(t, err)
		var rpcErr rpc.Error
		require.ErrorAs(t, err, &rpcErr)
		assert.Equal(t, -32602, rpcErr.ErrorCode())
	})

	t.Run("unsupported ERC20 method returns -32601", func(t *testing.T) {
		calldata, err := parsedABI.Pack("approve", accountAddr, big.NewInt(100))
		require.NoError(t, err)

		svc := mocks.NewService(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).
			Return(nil, apperr.NotSupportedError(nil, "unsupported method: approve"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err = ethCall(t, rpcClient, calldata)
		require.Error(t, err)
		var rpcErr rpc.Error
		require.ErrorAs(t, err, &rpcErr)
		assert.Equal(t, -32601, rpcErr.ErrorCode())
	})
}

// ─── eth_getLogs ──────────────────────────────────────────────────────────────

func TestEthAPI_GetLogs(t *testing.T) {
	contractAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	// Use explicit block range to avoid "latest" string that can't decode into hexutil.Uint64.
	filterQuery := geth.FilterQuery{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1000),
		Addresses: []common.Address{contractAddr},
	}

	t.Run("empty result", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetLogs(mock.Anything, mock.Anything).Return([]*types.Log{}, nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		logs, err := ethClient.FilterLogs(context.Background(), filterQuery)
		require.NoError(t, err)
		assert.Empty(t, logs)
	})

	t.Run("returns matching logs", func(t *testing.T) {
		txHash := common.HexToHash("0xaabbccdd00000000000000000000000000000000000000000000000000000003")
		transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
		wantLog := &types.Log{
			Address:     contractAddr,
			Topics:      []common.Hash{transferTopic},
			Data:        []byte{0x01},
			BlockNumber: 50,
			TxHash:      txHash,
			BlockHash:   common.HexToHash("0x3333000000000000000000000000000000000000000000000000000000000001"),
		}

		svc := mocks.NewService(t)
		svc.EXPECT().GetLogs(mock.Anything, mock.Anything).Return([]*types.Log{wantLog}, nil)
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		logs, err := ethClient.FilterLogs(context.Background(), filterQuery)
		require.NoError(t, err)
		require.Len(t, logs, 1)
		assert.Equal(t, contractAddr, logs[0].Address)
		assert.Equal(t, txHash, logs[0].TxHash)
		assert.Equal(t, transferTopic, logs[0].Topics[0])
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetLogs(mock.Anything, mock.Anything).Return(nil, errors.New("db error"))
		ethClient, _, cleanup := newTestServer(t, svc)
		defer cleanup()

		_, err := ethClient.FilterLogs(context.Background(), filterQuery)
		require.Error(t, err)
	})
}

// ─── eth_getBlockByNumber ─────────────────────────────────────────────────────

func TestEthAPI_GetBlockByNumber(t *testing.T) {
	newBlock := func(num hexutil.Uint64, hash common.Hash) *ethrpc.RPCBlock {
		return &ethrpc.RPCBlock{
			Number:          num,
			Hash:            hash,
			GasLimit:        hexutil.Uint64(30_000_000),
			GasUsed:         hexutil.Uint64(0),
			Transactions:    []any{},
			Uncles:          []common.Hash{},
			Difficulty:      (*hexutil.Big)(big.NewInt(0)),
			TotalDifficulty: (*hexutil.Big)(big.NewInt(0)),
			ExtraData:       hexutil.Bytes{},
		}
	}

	t.Run("latest returns current head", func(t *testing.T) {
		blockHash := common.HexToHash("0x4444000000000000000000000000000000000000000000000000000000000001")
		want := newBlock(hexutil.Uint64(1000), blockHash)

		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByNumber(mock.Anything, mock.Anything, false).Return(want, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByNumber", "latest", false)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, hexutil.Uint64(1000), got.Number)
		assert.Equal(t, blockHash, got.Hash)
	})

	t.Run("specific block number", func(t *testing.T) {
		blockHash := common.HexToHash("0x5555000000000000000000000000000000000000000000000000000000000001")
		want := newBlock(hexutil.Uint64(42), blockHash)

		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByNumber(mock.Anything, mock.Anything, false).Return(want, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByNumber", "0x2a", false)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, hexutil.Uint64(42), got.Number)
	})

	t.Run("not found returns null", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByNumber(mock.Anything, mock.Anything, false).Return(nil, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByNumber", "0x0", false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByNumber(mock.Anything, mock.Anything, false).Return(nil, errors.New("db error"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByNumber", "latest", false)
		require.Error(t, err)
	})
}

// ─── eth_getBlockByHash ───────────────────────────────────────────────────────

func TestEthAPI_GetBlockByHash(t *testing.T) {
	blockHash := common.HexToHash("0x6666000000000000000000000000000000000000000000000000000000000001")

	newBlock := func(num hexutil.Uint64, hash common.Hash) *ethrpc.RPCBlock {
		return &ethrpc.RPCBlock{
			Number:          num,
			Hash:            hash,
			GasLimit:        hexutil.Uint64(30_000_000),
			Transactions:    []any{},
			Uncles:          []common.Hash{},
			Difficulty:      (*hexutil.Big)(big.NewInt(0)),
			TotalDifficulty: (*hexutil.Big)(big.NewInt(0)),
			ExtraData:       hexutil.Bytes{},
		}
	}

	t.Run("found returns block", func(t *testing.T) {
		want := newBlock(hexutil.Uint64(77), blockHash)

		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByHash(mock.Anything, blockHash, false).Return(want, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByHash", blockHash, false)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, blockHash, got.Hash)
		assert.Equal(t, hexutil.Uint64(77), got.Number)
	})

	t.Run("not found returns null", func(t *testing.T) {
		unknownHash := common.HexToHash("0x9999000000000000000000000000000000000000000000000000000000000001")
		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByHash(mock.Anything, unknownHash, false).Return(nil, nil)
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByHash", unknownHash, false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := mocks.NewService(t)
		svc.EXPECT().GetBlockByHash(mock.Anything, blockHash, false).Return(nil, errors.New("db error"))
		_, rpcClient, cleanup := newTestServer(t, svc)
		defer cleanup()

		var got *ethrpc.RPCBlock
		err := rpcClient.Call(&got, "eth_getBlockByHash", blockHash, false)
		require.Error(t, err)
	})
}
