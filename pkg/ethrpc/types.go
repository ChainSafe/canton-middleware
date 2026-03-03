package ethrpc

import (
	"encoding/json"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// RPCBlock represents a block in JSON-RPC format
type RPCBlock struct {
	Number           hexutil.Uint64   `json:"number"`
	Hash             common.Hash      `json:"hash"`
	ParentHash       common.Hash      `json:"parentHash"`
	Nonce            types.BlockNonce `json:"nonce"`
	Sha3Uncles       common.Hash      `json:"sha3Uncles"`
	LogsBloom        types.Bloom      `json:"logsBloom"`
	TransactionsRoot common.Hash      `json:"transactionsRoot"`
	StateRoot        common.Hash      `json:"stateRoot"`
	ReceiptsRoot     common.Hash      `json:"receiptsRoot"`
	Miner            common.Address   `json:"miner"`
	Difficulty       *hexutil.Big     `json:"difficulty"`
	TotalDifficulty  *hexutil.Big     `json:"totalDifficulty"`
	ExtraData        hexutil.Bytes    `json:"extraData"`
	Size             hexutil.Uint64   `json:"size"`
	GasLimit         hexutil.Uint64   `json:"gasLimit"`
	GasUsed          hexutil.Uint64   `json:"gasUsed"`
	Timestamp        hexutil.Uint64   `json:"timestamp"`
	Transactions     []any            `json:"transactions"`
	Uncles           []common.Hash    `json:"uncles"`
	BaseFeePerGas    *hexutil.Big     `json:"baseFeePerGas,omitempty"`
}

// FilterQuery represents the filter for eth_getLogs
type FilterQuery struct {
	BlockHash *common.Hash    `json:"blockHash,omitempty"`
	FromBlock *hexutil.Uint64 `json:"fromBlock,omitempty"`
	ToBlock   *hexutil.Uint64 `json:"toBlock,omitempty"`
	Address   any             `json:"address,omitempty"` // single address or array
	Topics    []any           `json:"topics,omitempty"`
}

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

// UnmarshalJSON implements custom unmarshaling for block parameters
// Handles: "latest", "earliest", "pending", hex block numbers, and block hashes
func (b *BlockNumberOrHash) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		// Handle special block tags
		switch str {
		case "latest", "earliest", "pending":
			// For now, treat all these as latest (block number will be nil)
			return nil
		default:
			// Try to parse as hex block number or hash
			if len(str) == 66 && strings.HasPrefix(str, "0x") {
				// Looks like a block hash
				hash := common.HexToHash(str)
				b.BlockHash = &hash
			} else {
				// Try to parse as block number
				var num hexutil.Uint64
				if err := num.UnmarshalText([]byte(str)); err == nil {
					b.BlockNumber = &num
				}
			}
		}
		return nil
	}

	// Try to unmarshal as object with blockNumber or blockHash fields
	type Alias BlockNumberOrHash
	aux := (*Alias)(b)
	return json.Unmarshal(data, aux)
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

// EvmTransaction represents a synthetic EVM transaction persisted for JSON-RPC responses.
type EvmTransaction struct {
	TxHash       []byte
	FromAddress  string
	ToAddress    string
	Nonce        int64
	Input        []byte
	ValueWei     string
	Status       int16
	BlockNumber  int64
	BlockHash    []byte
	TxIndex      int
	GasUsed      int64
	ErrorMessage string
}

// EvmLog represents a synthetic EVM log persisted for JSON-RPC responses.
type EvmLog struct {
	TxHash      []byte
	LogIndex    int
	Address     []byte   // Contract address (20 bytes)
	Topics      [][]byte // Topic hashes (each 32 bytes)
	Data        []byte
	BlockNumber int64
	BlockHash   []byte
	TxIndex     int
	Removed     bool
}

// SyncStatus represents the syncing status response
type SyncStatus struct {
	StartingBlock hexutil.Uint64 `json:"startingBlock"`
	CurrentBlock  hexutil.Uint64 `json:"currentBlock"`
	HighestBlock  hexutil.Uint64 `json:"highestBlock"`
}
