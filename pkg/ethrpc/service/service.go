package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/token"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"
)

// Store is the narrow data-access interface consumed by the EthRPC service.
type Store interface {
	GetLatestEvmBlockNumber() (uint64, error)
	GetEvmTransactionCount(fromAddress string) (uint64, error)
	NextEvmBlock(chainID uint64) (uint64, []byte, int, error)
	SaveEvmTransaction(tx *apidb.EvmTransaction) error
	SaveEvmLog(log *apidb.EvmLog) error
	GetEvmTransaction(txHash []byte) (*apidb.EvmTransaction, error)
	GetEvmLogsByTxHash(txHash []byte) ([]*apidb.EvmLog, error)
	GetEvmLogs(address []byte, topic0 []byte, fromBlock, toBlock int64) ([]*apidb.EvmLog, error)
	GetBlockNumberByHash(blockHash []byte) (uint64, error)
}

// TokenService is the narrow token-service interface consumed by the EthRPC service.
type TokenService interface {
	ERC20(address common.Address) token.ERC20
	Native() token.Native
}

// Service defines the Ethereum JSON-RPC business logic interface.
type Service interface {
	ChainID() hexutil.Uint64
	BlockNumber() (hexutil.Uint64, error)
	GasPrice() (*hexutil.Big, error)
	MaxPriorityFeePerGas() (*hexutil.Big, error)
	EstimateGas(ctx context.Context, args ethrpc.CallArgs) (hexutil.Uint64, error)
	GetBalance(ctx context.Context, address common.Address) (*hexutil.Big, error)
	GetTransactionCount(ctx context.Context, address common.Address) (hexutil.Uint64, error)
	GetCode(ctx context.Context, address common.Address) (hexutil.Bytes, error)
	Syncing() bool
	SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error)
	GetTransactionReceipt(ctx context.Context, hash common.Hash) (*ethrpc.RPCReceipt, error)
	GetTransactionByHash(ctx context.Context, hash common.Hash) (*ethrpc.RPCTransaction, error)
	Call(ctx context.Context, args ethrpc.CallArgs) (hexutil.Bytes, error)
	GetLogs(ctx context.Context, query ethrpc.FilterQuery) ([]*types.Log, error)
	GetBlockByNumber(ctx context.Context, blockNr ethrpc.BlockNumberOrHash, fullTx bool) (*ethrpc.RPCBlock, error)
	GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error)
}

// ethService is the concrete implementation of Service.
type ethService struct {
	cfg          config.EthRPCConfig
	store        Store
	tokenService TokenService
	chainID      *big.Int
	erc20ABI     abi.ABI
	startTime    time.Time
}

// NewService creates a new ethService.
func NewService(
	cfg config.EthRPCConfig,
	evmStore Store,
	tokenSvc TokenService,
) Service {
	parsedABI, err := abi.JSON(strings.NewReader(ethereum.ERC20ABI))
	if err != nil {
		// ERC20ABI is a hard-coded constant — this can never happen unless a programming error.
		panic("ethrpc: failed to parse ERC20 ABI: " + err.Error())
	}

	return &ethService{
		cfg:          cfg,
		store:        evmStore,
		tokenService: tokenSvc,
		erc20ABI:     parsedABI,
		startTime:    time.Now(),
		chainID:      new(big.Int).SetUint64(cfg.ChainID),
	}
}

func (s *ethService) ChainID() hexutil.Uint64 {
	return hexutil.Uint64(s.cfg.ChainID)
}

func (s *ethService) BlockNumber() (hexutil.Uint64, error) {
	n, err := s.store.GetLatestEvmBlockNumber()
	if err != nil {
		return 0, fmt.Errorf("get latest EVM block number: %w", err)
	}
	// Add time-based block progression to simulate block production
	// This ensures MetaMask sees confirmations accumulating over time
	// Simulates ~1 block per second since we don't have real block production
	timeSinceStart := time.Since(s.startTime).Seconds()
	timeBasedBlocks := uint64(timeSinceStart)

	// Return max of: (latest tx block + 12) or (time-based blocks)
	// This ensures both old transactions and new ones appear confirmed
	baseBlock := n + 12

	return hexutil.Uint64(max(baseBlock, timeBasedBlocks)), nil
}

func (s *ethService) GasPrice() (*hexutil.Big, error) {
	gasPrice := new(big.Int)
	gasPrice.SetString(s.cfg.GasPriceWei, 10)

	return (*hexutil.Big)(gasPrice), nil
}

func (s *ethService) MaxPriorityFeePerGas() (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1000000000)), nil
}

func (s *ethService) EstimateGas(_ context.Context, _ ethrpc.CallArgs) (hexutil.Uint64, error) {
	return hexutil.Uint64(s.cfg.GasLimit), nil
}

func (s *ethService) GetBalance(ctx context.Context, address common.Address) (*hexutil.Big, error) {
	return s.tokenService.Native().GetBalance(ctx, address)
}

func (s *ethService) GetTransactionCount(_ context.Context, address common.Address) (hexutil.Uint64, error) {
	count, err := s.store.GetEvmTransactionCount(address.Hex())
	if err != nil {
		return 0, fmt.Errorf("get transaction count for %s: %w", address.Hex(), err)
	}
	return hexutil.Uint64(count), nil
}

func (s *ethService) GetCode(_ context.Context, address common.Address) (hexutil.Bytes, error) {
	// TODO: get from the token service
	switch address {
	case s.cfg.TokenAddress, s.cfg.DemoTokenAddress:
		return hexutil.Bytes{0x60, 0x80}, nil
	default:
		return hexutil.Bytes{}, nil
	}
}

func (s *ethService) Syncing() bool {
	return false
}

func (s *ethService) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, fmt.Errorf("invalid transaction: %w", err)
	}
	tx.To()

	signer := types.LatestSignerForChainID(s.chainID)
	from, err := types.Sender(signer, &tx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid sender: %w", err)
	}

	contractAddress := tx.To()
	if contractAddress == nil {
		return common.Hash{}, fmt.Errorf("unsupported contract: empty 'contract' address")
	}

	if tx.Value().Sign() != 0 {
		return common.Hash{}, fmt.Errorf("native ETH transfers not supported")
	}

	input := tx.Data()
	if len(input) < 4 {
		return common.Hash{}, fmt.Errorf("missing function selector")
	}

	method, err := s.erc20ABI.MethodById(input[:4])
	if err != nil {
		return common.Hash{}, fmt.Errorf("decode method selector: %w", err)
	}
	if method.Name != "transfer" {
		return common.Hash{}, fmt.Errorf("unsupported method: %s", method.Name)
	}

	args := make(map[string]interface{})
	if err = method.Inputs.UnpackIntoMap(args, input[4:]); err != nil {
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

	err = s.tokenService.ERC20(*contractAddress).TransferFrom(ctx, from, toAddr, *amount)
	if err != nil {
		return common.Hash{}, fmt.Errorf("transfer failed: %w", err)
	}

	txHash := tx.Hash()

	blockNumber, blockHash, txIndex, _ := s.store.NextEvmBlock(s.chainID.Uint64())

	evmTx := &apidb.EvmTransaction{
		TxHash:      txHash.Bytes(),
		FromAddress: from.Hex(),
		ToAddress:   contractAddress.Hex(),
		Nonce:       int64(tx.Nonce()),
		Input:       input,
		ValueWei:    "0",
		Status:      1,
		BlockNumber: int64(blockNumber),
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		GasUsed:     int64(s.cfg.GasLimit),
	}
	_ = s.store.SaveEvmTransaction(evmTx)

	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	fromTopic := common.BytesToHash(common.LeftPadBytes(from.Bytes(), 32))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), 32))
	amountBytes := common.LeftPadBytes(amount.Bytes(), 32)

	evmLog := &apidb.EvmLog{
		TxHash:      txHash.Bytes(),
		LogIndex:    0,
		Address:     contractAddress.Bytes(),
		Topics:      [][]byte{transferTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:        amountBytes,
		BlockNumber: int64(blockNumber),
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		Removed:     false,
	}
	_ = s.store.SaveEvmLog(evmLog)

	return txHash, nil
}

func (s *ethService) GetTransactionReceipt(_ context.Context, hash common.Hash) (*ethrpc.RPCReceipt, error) {
	row, err := s.store.GetEvmTransaction(hash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("get transaction receipt for %s: %w", hash.Hex(), err)
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)

	dbLogs, err := s.store.GetEvmLogsByTxHash(hash.Bytes())
	if err != nil {
		dbLogs = nil
	}

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

	return &ethrpc.RPCReceipt{
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

func (s *ethService) GetTransactionByHash(_ context.Context, hash common.Hash) (*ethrpc.RPCTransaction, error) {
	row, err := s.store.GetEvmTransaction(hash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("get transaction by hash %s: %w", hash.Hex(), err)
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

	return &ethrpc.RPCTransaction{
		Hash:             hash,
		Nonce:            hexutil.Uint64(row.Nonce),
		BlockHash:        &blockHash,
		BlockNumber:      &blockNum,
		TransactionIndex: &txIndex,
		From:             from,
		To:               &to,
		Value:            (*hexutil.Big)(big.NewInt(0)),
		GasPrice:         (*hexutil.Big)(gasPrice),
		Gas:              hexutil.Uint64(s.cfg.GasLimit),
		Input:            row.Input,
		Type:             hexutil.Uint64(2),
		ChainID:          (*hexutil.Big)(new(big.Int).Set(s.chainID)),
	}, nil
}

func (s *ethService) Call(ctx context.Context, args ethrpc.CallArgs) (hexutil.Bytes, error) {
	if args.To == nil {
		return nil, fmt.Errorf("unsupported 'contract' address")
	}
	tkn, err := s.getTokenByContractAddress(*args.To)
	if err != nil {
		return nil, fmt.Errorf("resolve 'contract' address %s: %w", args.To.Hex(), err)
	}

	input := args.GetData()
	if len(input) < 4 {
		return nil, fmt.Errorf("missing function selector")
	}

	method, err := s.erc20ABI.MethodById(input[:4])
	if err != nil {
		return nil, fmt.Errorf("unknown method")
	}

	switch method.Name {
	case "balanceOf":
		return s.callBalanceOf(ctx, input[4:], tkn.Symbol, tkn.Decimals)
	case "decimals":
		return s.callDecimals(tkn.Decimals)
	case "symbol":
		return s.callSymbol(tkn.Symbol)
	case "name":
		return s.callName(tkn.Name)
	case "totalSupply":
		return s.callTotalSupply(ctx, tkn.Symbol, tkn.Decimals)
	case "allowance":
		return s.callAllowance()
	default:
		return nil, fmt.Errorf("unsupported method: %s", method.Name)
	}
}

func (s *ethService) callBalanceOf(ctx context.Context, data []byte, tokenSymbol string, decimals int) (hexutil.Bytes, error) {
	method := s.erc20ABI.Methods["balanceOf"]
	args := make(map[string]interface{})
	if err := method.Inputs.UnpackIntoMap(args, data); err != nil {
		return nil, fmt.Errorf("decode balanceOf args: %w", err)
	}

	addr, ok := args["account"].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid account address")
	}

	balStr, err := s.tokenService.GetBalance(ctx, addr.Hex(), tokenSymbol)
	if err != nil || balStr == "" || balStr == "0" {
		return encodeUint256(big.NewInt(0))
	}
	bal, err := decimalToBigInt(balStr, decimals)
	if err != nil {
		return encodeUint256(big.NewInt(0))
	}
	return encodeUint256(bal)
}

func (s *ethService) callDecimals(decimals int) (hexutil.Bytes, error) {
	return encodeUint8(uint8(decimals))
}

func (s *ethService) callSymbol(tokenSymbol string) (hexutil.Bytes, error) {
	return encodeString(tokenSymbol)
}

func (s *ethService) callName(name string) (hexutil.Bytes, error) {
	return encodeString(name)
}

func (s *ethService) callTotalSupply(ctx context.Context, tokenSymbol string, decimals int) (hexutil.Bytes, error) {
	supplyStr, err := s.tokenService.GetTotalSupply(ctx, tokenSymbol)
	if err != nil {
		return nil, fmt.Errorf("get total supply for %s: %w", tokenSymbol, err)
	}
	supply, err := decimalToBigInt(supplyStr, decimals)
	if err != nil {
		return nil, fmt.Errorf("failed to convert total supply: %w", err)
	}
	return encodeUint256(supply)
}

func (s *ethService) callAllowance() (hexutil.Bytes, error) {
	return encodeUint256(big.NewInt(0))
}

func (s *ethService) GetLogs(_ context.Context, query ethrpc.FilterQuery) ([]*types.Log, error) {
	var fromBlock, toBlock int64
	if query.FromBlock != nil {
		fromBlock = int64(*query.FromBlock)
	}
	if query.ToBlock != nil {
		toBlock = int64(*query.ToBlock)
	} else {
		latest, err := s.store.GetLatestEvmBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("get latest EVM block number for logs: %w", err)
		}
		toBlock = int64(latest)
	}

	var addressFilter []byte
	if query.Address != nil {
		switch addr := query.Address.(type) {
		case string:
			addressFilter = common.HexToAddress(addr).Bytes()
		case common.Address:
			addressFilter = addr.Bytes()
		}
	}

	var topic0Filter []byte
	if len(query.Topics) > 0 && query.Topics[0] != nil {
		switch t := query.Topics[0].(type) {
		case string:
			topic0Filter = common.HexToHash(t).Bytes()
		case common.Hash:
			topic0Filter = t.Bytes()
		}
	}

	dbLogs, err := s.store.GetEvmLogs(addressFilter, topic0Filter, fromBlock, toBlock)
	if err != nil {
		return nil, fmt.Errorf("get EVM logs: %w", err)
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

func (s *ethService) GetBlockByNumber(ctx context.Context, block ethrpc.BlockNumberOrHash, _ bool) (*ethrpc.RPCBlock, error) {
	var blockNum uint64
	if block.BlockNumber != nil && *block.BlockNumber >= 0 {
		blockNum = uint64(*block.BlockNumber)
	} else {
		latestBlockNum, err := s.BlockNumber()
		if err != nil {
			return nil, fmt.Errorf("resolve latest block number: %w", err)
		}
		blockNum = uint64(latestBlockNum)
	}

	if blockNum == 0 {
		return nil, nil
	}

	blockHash := common.BytesToHash(ethereum.ComputeBlockHash(s.chainID.Uint64(), blockNum))
	parentHash := common.Hash{}
	if blockNum > 1 {
		parentHash = common.BytesToHash(ethereum.ComputeBlockHash(s.chainID.Uint64(), blockNum-1))
	}

	return &ethrpc.RPCBlock{
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
		GasLimit:         hexutil.Uint64(s.cfg.GasLimit),
		GasUsed:          hexutil.Uint64(0),
		Timestamp:        hexutil.Uint64(blockNum * 12),
		Transactions:     []interface{}{},
		Uncles:           []common.Hash{},
		BaseFeePerGas:    (*hexutil.Big)(big.NewInt(1000000000)),
	}, nil
}

func (s *ethService) GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error) {
	blockNum, err := s.store.GetBlockNumberByHash(hash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("get block number by hash %s: %w", hash.Hex(), err)
	}

	if blockNum > 0 {
		return s.GetBlockByNumber(ctx, ethrpc.BlockNumberOrHash{BlockNumber: (*hexutil.Uint64)(&blockNum)}, fullTx)
	}

	return s.GetBlockByNumber(ctx, ethrpc.BlockNumberOrHash{}, fullTx)
}

// getTokenConfig returns the token metadata config for the given token symbol.
func (s *ethService) getTokenConfig(tokenSymbol string) config.TokenConfig {
	if strings.EqualFold(tokenSymbol, s.demoToken.Symbol) {
		return s.demoToken
	}
	return s.promptToken
}

func (s *ethService) getTokenByContractAddress(address common.Address) (config.TokenConfig, error) {
	switch address {
	case s.cfg.DemoTokenAddress:
		return s.demoToken, nil
	case s.cfg.TokenAddress:
		return s.promptToken, nil
	default:
		return config.TokenConfig{}, fmt.Errorf("unsupported contract address: %s", address.Hex())
	}
}

func encodeUint256(v *big.Int) (hexutil.Bytes, error) {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	args := abi.Arguments{{Type: uint256Type}}
	return args.Pack(v)
}

func encodeUint8(v uint8) (hexutil.Bytes, error) {
	uint8Type, _ := abi.NewType("uint8", "", nil)
	args := abi.Arguments{{Type: uint8Type}}
	return args.Pack(v)
}

func encodeString(s string) (hexutil.Bytes, error) {
	stringType, _ := abi.NewType("string", "", nil)
	args := abi.Arguments{{Type: stringType}}
	return args.Pack(s)
}

func decimalToBigInt(s string, decimals int) (*big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid decimal format: %w", err)
	}
	d = d.Mul(decimal.New(1, int32(decimals)))
	return d.BigInt(), nil
}

func bigIntToDecimal(amount *big.Int, decimals int) string {
	d := decimal.NewFromBigInt(amount, int32(-decimals))
	return d.String()
}
