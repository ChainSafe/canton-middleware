package ethrpc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// CallArgs represents the arguments to eth_call and eth_estimateGas
type CallArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Nonce                *hexutil.Uint64 `json:"nonce"`
	Data                 *hexutil.Bytes  `json:"data"`
	Input                *hexutil.Bytes  `json:"input"`
}

// GetData returns the input data, preferring 'input' over 'data' per EIP-2929
func (args *CallArgs) GetData() []byte {
	if args.Input != nil {
		return *args.Input
	}
	if args.Data != nil {
		return *args.Data
	}
	return nil
}

// BlockNumberOrHash represents a block number or hash parameter
type BlockNumberOrHash struct {
	BlockNumber *hexutil.Uint64 `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
}

// RPCReceipt represents a transaction receipt in JSON-RPC format
type RPCReceipt struct {
	TransactionHash   common.Hash     `json:"transactionHash"`
	TransactionIndex  hexutil.Uint    `json:"transactionIndex"`
	BlockHash         common.Hash     `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	From              common.Address  `json:"from"`
	To                *common.Address `json:"to"`
	CumulativeGasUsed hexutil.Uint64  `json:"cumulativeGasUsed"`
	GasUsed           hexutil.Uint64  `json:"gasUsed"`
	ContractAddress   *common.Address `json:"contractAddress"`
	Logs              []*types.Log    `json:"logs"`
	LogsBloom         types.Bloom     `json:"logsBloom"`
	Status            hexutil.Uint64  `json:"status"`
	EffectiveGasPrice hexutil.Uint64  `json:"effectiveGasPrice"`
	Type              hexutil.Uint64  `json:"type"`
}

// RPCTransaction represents a transaction in JSON-RPC format
type RPCTransaction struct {
	Hash             common.Hash     `json:"hash"`
	Nonce            hexutil.Uint64  `json:"nonce"`
	BlockHash        *common.Hash    `json:"blockHash"`
	BlockNumber      *hexutil.Uint64 `json:"blockNumber"`
	TransactionIndex *hexutil.Uint   `json:"transactionIndex"`
	From             common.Address  `json:"from"`
	To               *common.Address `json:"to"`
	Value            *hexutil.Big    `json:"value"`
	GasPrice         *hexutil.Big    `json:"gasPrice"`
	Gas              hexutil.Uint64  `json:"gas"`
	Input            hexutil.Bytes   `json:"input"`
	V                *hexutil.Big    `json:"v"`
	R                *hexutil.Big    `json:"r"`
	S                *hexutil.Big    `json:"s"`
	Type             hexutil.Uint64  `json:"type"`
	ChainID          *hexutil.Big    `json:"chainId,omitempty"`
}

// SyncStatus represents the syncing status response
type SyncStatus struct {
	StartingBlock hexutil.Uint64 `json:"startingBlock"`
	CurrentBlock  hexutil.Uint64 `json:"currentBlock"`
	HighestBlock  hexutil.Uint64 `json:"highestBlock"`
}
