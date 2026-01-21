package ethrpc

import (
	"context"
	"fmt"
	"math/big"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
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
	// Return latest block + 12 to ensure transactions have enough confirmations
	// This helps MetaMask recognize transactions as confirmed rather than pending
	return hexutil.Uint64(n + 12), nil
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

	// Check whitelist
	normalizedAddr := auth.NormalizeAddress(from.Hex())
	whitelisted, err := api.server.db.IsWhitelisted(normalizedAddr)
	if err != nil {
		api.server.logger.Error("Failed to check whitelist",
			zap.String("address", normalizedAddr),
			zap.Error(err))
		return common.Hash{}, fmt.Errorf("whitelist check failed")
	}
	if !whitelisted {
		api.server.logger.Warn("Transaction rejected: address not whitelisted",
			zap.String("address", normalizedAddr),
			zap.String("tx_hash", tx.Hash().Hex()))
		return common.Hash{}, fmt.Errorf("sender address %s is not whitelisted for transactions", normalizedAddr)
	}

	if tx.To() == nil || *tx.To() != api.server.tokenAddress {
		api.server.logger.Warn("Transaction rejected: unsupported contract",
			zap.String("tx_to", func() string { if tx.To() == nil { return "<nil>" }; return tx.To().Hex() }()),
			zap.String("expected_token", api.server.tokenAddress.Hex()),
			zap.Bool("addresses_match", tx.To() != nil && *tx.To() == api.server.tokenAddress))
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

	// Convert Wei amount to human-readable decimal format for Canton
	// Canton expects amounts like "25.0" not "25000000000000000000"
	decimals := api.server.tokenService.GetTokenDecimals()
	humanReadableAmount := canton.BigIntToDecimal(amount, decimals)

	timeoutCtx, cancel := context.WithTimeout(ctx, api.server.cfg.EthRPC.RequestTimeout)
	defer cancel()

	_, err = api.server.tokenService.Transfer(timeoutCtx, &service.TransferRequest{
		FromEVMAddress: from.Hex(),
		ToEVMAddress:   toAddr.Hex(),
		Amount:         humanReadableAmount,
	})
	if err != nil {
		api.server.logger.Error("Transfer failed",
			zap.String("from", from.Hex()),
			zap.String("to", toAddr.Hex()),
			zap.String("amount_wei", amount.String()),
			zap.String("amount_human", humanReadableAmount),
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
		ToAddress:   auth.NormalizeAddress(api.server.tokenAddress.Hex()), // Token contract address, not recipient
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

	// Create ERC20 Transfer event log
	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	fromTopic := common.BytesToHash(common.LeftPadBytes(from.Bytes(), 32))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), 32))

	// ABI-encode the amount as uint256
	amountBytes := common.LeftPadBytes(amount.Bytes(), 32)

	evmLog := &apidb.EvmLog{
		TxHash:      txHash.Bytes(),
		LogIndex:    0,
		Address:     api.server.tokenAddress.Bytes(),
		Topics:      [][]byte{transferTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:        amountBytes,
		BlockNumber: int64(blockNumber),
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		Removed:     false,
	}
	if err := api.server.db.SaveEvmLog(evmLog); err != nil {
		api.server.logger.Warn("Failed to save evm log", zap.Error(err))
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

	// Fetch logs for this transaction
	dbLogs, err := api.server.db.GetEvmLogsByTxHash(hash.Bytes())
	if err != nil {
		api.server.logger.Warn("Failed to get logs for receipt", zap.Error(err))
	}

	// Initialize as empty slice to ensure JSON marshals to [] not null
	logs := make([]*types.Log, 0)
	for _, dbLog := range dbLogs {
		log := &types.Log{
			Address:     common.BytesToAddress(dbLog.Address),
			Data:        dbLog.Data,
			BlockNumber: uint64(dbLog.BlockNumber),
			TxHash:      hash,
			TxIndex:     uint(dbLog.TxIndex),
			BlockHash:   common.BytesToHash(dbLog.BlockHash),
			Index:       uint(dbLog.LogIndex),
			Removed:     dbLog.Removed,
		}
		for _, topic := range dbLog.Topics {
			log.Topics = append(log.Topics, common.BytesToHash(topic))
		}
		logs = append(logs, log)
	}
	bloom := types.CreateBloom(&types.Receipt{Logs: logs})

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
		Logs:              logs,
		LogsBloom:         bloom,
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

	// Convert human-readable balance to Wei for ERC20 compatibility
	decimals := api.server.tokenService.GetTokenDecimals()
	bal, err := canton.DecimalToBigInt(balStr, decimals)
	if err != nil {
		return nil, fmt.Errorf("failed to convert balance: %w", err)
	}
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
	// Convert human-readable supply to Wei for ERC20 compatibility
	decimals := api.server.tokenService.GetTokenDecimals()
	supply, err := canton.DecimalToBigInt(supplyStr, decimals)
	if err != nil {
		return nil, fmt.Errorf("failed to convert total supply: %w", err)
	}
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

// FilterQuery represents the filter for eth_getLogs
type FilterQuery struct {
	BlockHash *common.Hash    `json:"blockHash,omitempty"`
	FromBlock *hexutil.Uint64 `json:"fromBlock,omitempty"`
	ToBlock   *hexutil.Uint64 `json:"toBlock,omitempty"`
	Address   interface{}     `json:"address,omitempty"` // single address or array
	Topics    []interface{}   `json:"topics,omitempty"`
}

// GetLogs returns logs matching the filter criteria
func (api *EthAPI) GetLogs(ctx context.Context, query FilterQuery) ([]*types.Log, error) {
	// Parse from/to blocks
	var fromBlock, toBlock int64
	if query.FromBlock != nil {
		fromBlock = int64(*query.FromBlock)
	}
	if query.ToBlock != nil {
		toBlock = int64(*query.ToBlock)
	} else {
		latest, err := api.server.db.GetLatestEvmBlockNumber()
		if err != nil {
			return nil, err
		}
		toBlock = int64(latest)
	}

	// Parse address filter (only support single address for now)
	var addressFilter []byte
	if query.Address != nil {
		switch addr := query.Address.(type) {
		case string:
			addressFilter = common.HexToAddress(addr).Bytes()
		case common.Address:
			addressFilter = addr.Bytes()
		}
	}

	// Parse topic0 filter
	var topic0Filter []byte
	if len(query.Topics) > 0 && query.Topics[0] != nil {
		switch t := query.Topics[0].(type) {
		case string:
			topic0Filter = common.HexToHash(t).Bytes()
		case common.Hash:
			topic0Filter = t.Bytes()
		}
	}

	dbLogs, err := api.server.db.GetEvmLogs(addressFilter, topic0Filter, fromBlock, toBlock)
	if err != nil {
		return nil, err
	}

	var logs []*types.Log
	for _, dbLog := range dbLogs {
		log := &types.Log{
			Address:     common.BytesToAddress(dbLog.Address),
			Data:        dbLog.Data,
			BlockNumber: uint64(dbLog.BlockNumber),
			TxHash:      common.BytesToHash(dbLog.TxHash),
			TxIndex:     uint(dbLog.TxIndex),
			BlockHash:   common.BytesToHash(dbLog.BlockHash),
			Index:       uint(dbLog.LogIndex),
			Removed:     dbLog.Removed,
		}
		for _, topic := range dbLog.Topics {
			log.Topics = append(log.Topics, common.BytesToHash(topic))
		}
		logs = append(logs, log)
	}
	return logs, nil
}

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
	Transactions     []interface{}    `json:"transactions"`
	Uncles           []common.Hash    `json:"uncles"`
	BaseFeePerGas    *hexutil.Big     `json:"baseFeePerGas,omitempty"`
}

// GetBlockByNumber returns a synthetic block by number
func (api *EthAPI) GetBlockByNumber(ctx context.Context, blockNr BlockNumberOrHash, fullTx bool) (*RPCBlock, error) {
	var blockNum uint64
	if blockNr.BlockNumber != nil {
		blockNum = uint64(*blockNr.BlockNumber)
	} else {
		// "latest" or unspecified
		latest, err := api.server.db.GetLatestEvmBlockNumber()
		if err != nil {
			return nil, err
		}
		blockNum = latest
	}

	if blockNum == 0 {
		return nil, nil
	}

	blockHash := common.BytesToHash(ethereum.ComputeBlockHash(api.server.cfg.EthRPC.ChainID, blockNum))
	parentHash := common.Hash{}
	if blockNum > 1 {
		parentHash = common.BytesToHash(ethereum.ComputeBlockHash(api.server.cfg.EthRPC.ChainID, blockNum-1))
	}

	return &RPCBlock{
		Number:           hexutil.Uint64(blockNum),
		Hash:             blockHash,
		ParentHash:       parentHash,
		Nonce:            types.BlockNonce{},
		Sha3Uncles:       common.Hash{},
		LogsBloom:        types.Bloom{},
		TransactionsRoot: common.Hash{},
		StateRoot:        common.Hash{},
		ReceiptsRoot:     common.Hash{},
		Miner:            common.Address{},
		Difficulty:       (*hexutil.Big)(big.NewInt(0)),
		TotalDifficulty:  (*hexutil.Big)(big.NewInt(0)),
		ExtraData:        []byte{},
		Size:             hexutil.Uint64(0),
		GasLimit:         hexutil.Uint64(api.server.cfg.EthRPC.GasLimit),
		GasUsed:          hexutil.Uint64(0),
		Timestamp:        hexutil.Uint64(blockNum * 12), // synthetic timestamp
		Transactions:     []interface{}{},
		Uncles:           []common.Hash{},
		BaseFeePerGas:    (*hexutil.Big)(big.NewInt(1000000000)),
	}, nil
}
