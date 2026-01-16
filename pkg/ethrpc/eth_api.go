package ethrpc

import (
	"context"
	"fmt"
	"math/big"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/zap"
)

// EthAPI implements the eth_* JSON-RPC namespace
type EthAPI struct {
	server *Server
}

// NewEthAPI creates a new EthAPI instance
func NewEthAPI(server *Server) *EthAPI {
	return &EthAPI{server: server}
}

// ChainId returns the chain ID (EIP-155)
func (api *EthAPI) ChainId() hexutil.Uint64 {
	return hexutil.Uint64(api.server.cfg.EthRPC.ChainID)
}

// BlockNumber returns the latest block number
func (api *EthAPI) BlockNumber() (hexutil.Uint64, error) {
	n, err := api.server.db.GetLatestEvmBlockNumber()
	if err != nil {
		api.server.logger.Error("Failed to get block number", zap.Error(err))
		return 0, err
	}
	return hexutil.Uint64(n), nil
}

// GasPrice returns the current gas price
func (api *EthAPI) GasPrice() (*hexutil.Big, error) {
	gasPrice := new(big.Int)
	gasPrice.SetString(api.server.cfg.EthRPC.GasPriceWei, 10)
	return (*hexutil.Big)(gasPrice), nil
}

// MaxPriorityFeePerGas returns the suggested priority fee (EIP-1559)
func (api *EthAPI) MaxPriorityFeePerGas() (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1000000000)), nil
}

// EstimateGas estimates gas for a transaction
func (api *EthAPI) EstimateGas(ctx context.Context, args CallArgs, blockNrOrHash *BlockNumberOrHash) (hexutil.Uint64, error) {
	return hexutil.Uint64(api.server.cfg.EthRPC.GasLimit), nil
}

// GetBalance returns the ETH balance (synthetic for registered users)
func (api *EthAPI) GetBalance(ctx context.Context, address common.Address, blockNrOrHash BlockNumberOrHash) (*hexutil.Big, error) {
	registered, err := api.server.tokenService.IsUserRegistered(address.Hex())
	if err != nil {
		api.server.logger.Error("Failed to check user registration", zap.Error(err))
		return (*hexutil.Big)(big.NewInt(0)), nil
	}

	bal := new(big.Int)
	if registered {
		bal.SetString(api.server.cfg.EthRPC.NativeBalanceWei, 10)
	}
	return (*hexutil.Big)(bal), nil
}

// GetTransactionCount returns the nonce for an address
func (api *EthAPI) GetTransactionCount(ctx context.Context, address common.Address, blockNrOrHash BlockNumberOrHash) (hexutil.Uint64, error) {
	count, err := api.server.db.GetEvmTransactionCount(auth.NormalizeAddress(address.Hex()))
	if err != nil {
		api.server.logger.Warn("Failed to get transaction count", zap.Error(err))
		return 0, nil
	}
	return hexutil.Uint64(count), nil
}

// GetCode returns the code at an address
func (api *EthAPI) GetCode(ctx context.Context, address common.Address, blockNrOrHash BlockNumberOrHash) (hexutil.Bytes, error) {
	if address == api.server.tokenAddress {
		return hexutil.Bytes{0x60, 0x80}, nil
	}
	return hexutil.Bytes{}, nil
}

// Syncing returns false (always synced)
func (api *EthAPI) Syncing() (interface{}, error) {
	return false, nil
}

// SendRawTransaction submits a signed transaction
func (api *EthAPI) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		api.server.logger.Warn("Failed to decode transaction", zap.Error(err))
		return common.Hash{}, fmt.Errorf("invalid transaction: %w", err)
	}

	signer := types.LatestSignerForChainID(api.server.chainID)
	from, err := types.Sender(signer, &tx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid sender: %w", err)
	}

	if tx.To() == nil || *tx.To() != api.server.tokenAddress {
		return common.Hash{}, fmt.Errorf("unsupported contract: only token transfers allowed")
	}

	if tx.Value().Sign() != 0 {
		return common.Hash{}, fmt.Errorf("native ETH transfers not supported")
	}

	input := tx.Data()
	if len(input) < 4 {
		return common.Hash{}, fmt.Errorf("missing function selector")
	}

	method, err := api.server.erc20ABI.MethodById(input[:4])
	if err != nil || method.Name != "transfer" {
		return common.Hash{}, fmt.Errorf("only ERC20 transfer is supported")
	}

	args := make(map[string]interface{})
	if err := method.Inputs.UnpackIntoMap(args, input[4:]); err != nil {
		return common.Hash{}, fmt.Errorf("failed to decode transfer args: %w", err)
	}

	toAddr, ok := args["to"].(common.Address)
	if !ok {
		return common.Hash{}, fmt.Errorf("invalid 'to' address in transfer")
	}
	amount, ok := args["value"].(*big.Int)
	if !ok {
		return common.Hash{}, fmt.Errorf("invalid 'value' in transfer")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, api.server.cfg.EthRPC.RequestTimeout)
	defer cancel()

	_, err = api.server.tokenService.Transfer(timeoutCtx, &service.TransferRequest{
		FromEVMAddress: from.Hex(),
		ToEVMAddress:   toAddr.Hex(),
		Amount:         amount.String(),
	})
	if err != nil {
		api.server.logger.Error("Transfer failed",
			zap.String("from", from.Hex()),
			zap.String("to", toAddr.Hex()),
			zap.String("amount", amount.String()),
			zap.Error(err))
		return common.Hash{}, fmt.Errorf("transfer failed: %w", err)
	}

	txHash := tx.Hash()

	blockNumber, blockHash, txIndex, err := api.server.db.NextEvmBlock(api.server.cfg.EthRPC.ChainID)
	if err != nil {
		api.server.logger.Warn("Failed to allocate block", zap.Error(err))
	}

	evmTx := &apidb.EvmTransaction{
		TxHash:      txHash.Bytes(),
		FromAddress: auth.NormalizeAddress(from.Hex()),
		ToAddress:   auth.NormalizeAddress(toAddr.Hex()),
		Nonce:       int64(tx.Nonce()),
		Input:       input,
		ValueWei:    "0",
		Status:      1,
		BlockNumber: int64(blockNumber),
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		GasUsed:     int64(api.server.cfg.EthRPC.GasLimit),
	}
	if err := api.server.db.SaveEvmTransaction(evmTx); err != nil {
		api.server.logger.Warn("Failed to save evm transaction", zap.Error(err))
	}

	api.server.logger.Info("Transaction submitted",
		zap.String("hash", txHash.Hex()),
		zap.String("from", from.Hex()),
		zap.String("to", toAddr.Hex()),
		zap.String("amount", amount.String()))

	return txHash, nil
}

// GetTransactionReceipt returns the receipt for a transaction
func (api *EthAPI) GetTransactionReceipt(ctx context.Context, hash common.Hash) (*RPCReceipt, error) {
	row, err := api.server.db.GetEvmTransaction(hash.Bytes())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)

	return &RPCReceipt{
		TransactionHash:   hash,
		TransactionIndex:  hexutil.Uint(row.TxIndex),
		BlockHash:         common.BytesToHash(row.BlockHash),
		BlockNumber:       hexutil.Uint64(row.BlockNumber),
		From:              from,
		To:                &to,
		CumulativeGasUsed: hexutil.Uint64(row.GasUsed),
		GasUsed:           hexutil.Uint64(row.GasUsed),
		ContractAddress:   nil,
		Logs:              []*types.Log{},
		LogsBloom:         types.Bloom{},
		Status:            hexutil.Uint64(row.Status),
		EffectiveGasPrice: hexutil.Uint64(1000000000),
		Type:              hexutil.Uint64(2),
	}, nil
}

// GetTransactionByHash returns a transaction by hash
func (api *EthAPI) GetTransactionByHash(ctx context.Context, hash common.Hash) (*RPCTransaction, error) {
	row, err := api.server.db.GetEvmTransaction(hash.Bytes())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)
	blockHash := common.BytesToHash(row.BlockHash)
	blockNum := hexutil.Uint64(row.BlockNumber)
	txIndex := hexutil.Uint(row.TxIndex)
	gasPrice := big.NewInt(1000000000)

	return &RPCTransaction{
		Hash:             hash,
		Nonce:            hexutil.Uint64(row.Nonce),
		BlockHash:        &blockHash,
		BlockNumber:      &blockNum,
		TransactionIndex: &txIndex,
		From:             from,
		To:               &to,
		Value:            (*hexutil.Big)(big.NewInt(0)),
		GasPrice:         (*hexutil.Big)(gasPrice),
		Gas:              hexutil.Uint64(api.server.cfg.EthRPC.GasLimit),
		Input:            row.Input,
		Type:             hexutil.Uint64(2),
		ChainID:          (*hexutil.Big)(api.server.chainID),
	}, nil
}

// Call executes a call without creating a transaction
func (api *EthAPI) Call(ctx context.Context, args CallArgs, blockNrOrHash BlockNumberOrHash, overrides *map[common.Address]interface{}) (hexutil.Bytes, error) {
	if args.To == nil || *args.To != api.server.tokenAddress {
		return nil, fmt.Errorf("unsupported contract")
	}

	input := args.GetData()
	if len(input) < 4 {
		return nil, fmt.Errorf("missing function selector")
	}

	method, err := api.server.erc20ABI.MethodById(input[:4])
	if err != nil {
		return nil, fmt.Errorf("unknown method")
	}

	switch method.Name {
	case "balanceOf":
		return api.callBalanceOf(ctx, input[4:])
	case "decimals":
		return api.callDecimals()
	case "symbol":
		return api.callSymbol()
	case "name":
		return api.callName()
	case "totalSupply":
		return api.callTotalSupply(ctx)
	case "allowance":
		return api.callAllowance()
	default:
		return nil, fmt.Errorf("unsupported method: %s", method.Name)
	}
}

func (api *EthAPI) callBalanceOf(ctx context.Context, data []byte) (hexutil.Bytes, error) {
	method := api.server.erc20ABI.Methods["balanceOf"]
	args := make(map[string]interface{})
	if err := method.Inputs.UnpackIntoMap(args, data); err != nil {
		return nil, err
	}

	addr, ok := args["account"].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid account address")
	}

	balStr, err := api.server.tokenService.GetBalance(ctx, addr.Hex())
	if err != nil {
		return nil, err
	}

	bal := new(big.Int)
	bal.SetString(balStr, 10)
	return api.encodeUint256(bal)
}

func (api *EthAPI) callDecimals() (hexutil.Bytes, error) {
	return api.encodeUint8(uint8(api.server.tokenService.GetTokenDecimals()))
}

func (api *EthAPI) callSymbol() (hexutil.Bytes, error) {
	return api.encodeString(api.server.tokenService.GetTokenSymbol())
}

func (api *EthAPI) callName() (hexutil.Bytes, error) {
	return api.encodeString(api.server.tokenService.GetTokenName())
}

func (api *EthAPI) callTotalSupply(ctx context.Context) (hexutil.Bytes, error) {
	supplyStr, err := api.server.tokenService.GetTotalSupply(ctx)
	if err != nil {
		return nil, err
	}
	supply := new(big.Int)
	supply.SetString(supplyStr, 10)
	return api.encodeUint256(supply)
}

func (api *EthAPI) callAllowance() (hexutil.Bytes, error) {
	return api.encodeUint256(big.NewInt(0))
}

func (api *EthAPI) encodeUint256(v *big.Int) (hexutil.Bytes, error) {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	args := abi.Arguments{{Type: uint256Type}}
	return args.Pack(v)
}

func (api *EthAPI) encodeUint8(v uint8) (hexutil.Bytes, error) {
	uint8Type, _ := abi.NewType("uint8", "", nil)
	args := abi.Arguments{{Type: uint8Type}}
	return args.Pack(v)
}

func (api *EthAPI) encodeString(s string) (hexutil.Bytes, error) {
	stringType, _ := abi.NewType("string", "", nil)
	args := abi.Arguments{{Type: stringType}}
	return args.Pack(s)
}
