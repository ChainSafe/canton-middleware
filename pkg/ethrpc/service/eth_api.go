package service

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
)

// EthAPI implements the eth_* JSON-RPC namespace.
// It is a thin adapter: each method translates the RPC signature to a Service call,
// dropping unused parameters (blockNrOrHash, overrides) that this facade does not need.
type EthAPI struct {
	svc Service
}

func (api *EthAPI) ChainId(ctx context.Context) hexutil.Uint64 {
	return api.svc.ChainID(ctx)
}

func (api *EthAPI) BlockNumber(ctx context.Context) (hexutil.Uint64, error) {
	return api.svc.BlockNumber(ctx)
}

func (api *EthAPI) GasPrice(ctx context.Context) (*hexutil.Big, error) {
	return api.svc.GasPrice(ctx)
}

func (api *EthAPI) MaxPriorityFeePerGas(ctx context.Context) (*hexutil.Big, error) {
	return api.svc.MaxPriorityFeePerGas(ctx)
}

func (api *EthAPI) EstimateGas(ctx context.Context, args *ethrpc.CallArgs, _ *ethrpc.BlockNumberOrHash) (hexutil.Uint64, error) {
	return api.svc.EstimateGas(ctx, args)
}

func (api *EthAPI) GetBalance(ctx context.Context, address common.Address, _ ethrpc.BlockNumberOrHash) (*hexutil.Big, error) {
	return api.svc.GetBalance(ctx, address)
}

func (api *EthAPI) GetTransactionCount(ctx context.Context, address common.Address, _ ethrpc.BlockNumberOrHash) (hexutil.Uint64, error) {
	return api.svc.GetTransactionCount(ctx, address)
}

func (api *EthAPI) GetCode(ctx context.Context, address common.Address, _ ethrpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	return api.svc.GetCode(ctx, address)
}

func (api *EthAPI) Syncing(ctx context.Context) (any, error) {
	return api.svc.Syncing(ctx), nil
}

func (api *EthAPI) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	return api.svc.SendRawTransaction(ctx, data)
}

func (api *EthAPI) GetTransactionReceipt(ctx context.Context, hash common.Hash) (*ethrpc.RPCReceipt, error) {
	return api.svc.GetTransactionReceipt(ctx, hash)
}

func (api *EthAPI) GetTransactionByHash(ctx context.Context, hash common.Hash) (*ethrpc.RPCTransaction, error) {
	return api.svc.GetTransactionByHash(ctx, hash)
}

func (api *EthAPI) Call(
	ctx context.Context,
	args *ethrpc.CallArgs,
	_ ethrpc.BlockNumberOrHash,
	_ *map[common.Address]any,
) (hexutil.Bytes, error) {
	return api.svc.Call(ctx, args)
}

func (api *EthAPI) GetLogs(ctx context.Context, query ethrpc.FilterQuery) ([]*types.Log, error) {
	return api.svc.GetLogs(ctx, query)
}

func (api *EthAPI) GetBlockByNumber(ctx context.Context, blockNr ethrpc.BlockNumberOrHash, fullTx bool) (*ethrpc.RPCBlock, error) {
	return api.svc.GetBlockByNumber(ctx, blockNr, fullTx)
}

func (api *EthAPI) GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error) {
	return api.svc.GetBlockByHash(ctx, hash, fullTx)
}
