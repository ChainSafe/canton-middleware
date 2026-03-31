package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/token"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Store is the narrow data-access interface consumed by the EthRPC service.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	GetLatestEvmBlockNumber(ctx context.Context) (uint64, error)
	GetEvmTransactionCount(ctx context.Context, fromAddress string) (uint64, error)
	NextEvmBlock(ctx context.Context, chainID uint64) (uint64, []byte, uint, error)
	SaveEvmTransaction(ctx context.Context, tx *ethrpc.EvmTransaction) error
	SaveEvmLog(ctx context.Context, log *ethrpc.EvmLog) error
	GetEvmTransaction(ctx context.Context, txHash []byte) (*ethrpc.EvmTransaction, error)
	GetEvmLogsByTxHash(ctx context.Context, txHash []byte) ([]*ethrpc.EvmLog, error)
	GetEvmLogs(ctx context.Context, address []byte, topic0 []byte, fromBlock, toBlock uint64) ([]*ethrpc.EvmLog, error)
	GetBlockNumberByHash(ctx context.Context, blockHash []byte) (uint64, error)
}

// TokenService is the narrow token-service interface consumed by the EthRPC service.
//
//go:generate mockery --name TokenService --output mocks --outpkg mocks --filename mock_token_service.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/token --name ERC20 --output mocks --outpkg mocks --filename mock_erc20.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/token --name Native --output mocks --outpkg mocks --filename mock_native.go --with-expecter
type TokenService interface {
	ERC20(address common.Address) (token.ERC20, error)
	Native() token.Native
}

// Service defines the Ethereum JSON-RPC business logic interface.
//
//go:generate mockery --name Service --output mocks --outpkg mocks --filename mock_service.go --with-expecter
type Service interface {
	ChainID(ctx context.Context) hexutil.Uint64
	BlockNumber(ctx context.Context) (hexutil.Uint64, error)
	GasPrice(ctx context.Context) (*hexutil.Big, error)
	MaxPriorityFeePerGas(ctx context.Context) (*hexutil.Big, error)
	EstimateGas(ctx context.Context, args *ethrpc.CallArgs) (hexutil.Uint64, error)
	GetBalance(ctx context.Context, address common.Address) (*hexutil.Big, error)
	GetTransactionCount(ctx context.Context, address common.Address) (hexutil.Uint64, error)
	GetCode(ctx context.Context, address common.Address) (hexutil.Bytes, error)
	Syncing(ctx context.Context) bool
	SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error)
	GetTransactionReceipt(ctx context.Context, hash common.Hash) (*ethrpc.RPCReceipt, error)
	GetTransactionByHash(ctx context.Context, hash common.Hash) (*ethrpc.RPCTransaction, error)
	Call(ctx context.Context, args *ethrpc.CallArgs) (hexutil.Bytes, error)
	GetLogs(ctx context.Context, query ethrpc.FilterQuery) ([]*types.Log, error)
	GetBlockByNumber(ctx context.Context, blockNr ethrpc.BlockNumberOrHash, fullTx bool) (*ethrpc.RPCBlock, error)
	GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error)
}

// ethService is the concrete implementation of Service.
type ethService struct {
	cfg          ethrpc.Config
	store        Store
	tokenService TokenService
	chainID      *big.Int
	erc20ABI     abi.ABI
	startTime    time.Time
}

const (
	decimalStringBase         = 10
	defaultGasPriceWeiInt64   = int64(1_000_000_000)
	defaultGasPriceWeiUint64  = uint64(1_000_000_000)
	functionSelectorSize      = 4
	topicSizeBytes            = 32
	confirmationBufferBlocks  = uint64(12)
	syntheticBlockTimeSeconds = uint64(12)
)

// NewService creates a new ethService.
func NewService(
	cfg *ethrpc.Config,
	evmStore Store,
	tokenSvc TokenService,
) Service {
	if cfg == nil {
		panic("ethrpc: config is nil")
	}

	parsedABI, err := abi.JSON(strings.NewReader(ethereum.ERC20ABI))
	if err != nil {
		// ERC20ABI is a hard-coded constant — this can never happen unless a programming error.
		panic("ethrpc: failed to parse ERC20 ABI: " + err.Error())
	}

	return &ethService{
		cfg:          *cfg,
		store:        evmStore,
		tokenService: tokenSvc,
		erc20ABI:     parsedABI,
		startTime:    time.Now(),
		chainID:      new(big.Int).SetUint64(cfg.ChainID),
	}
}

func (s *ethService) ChainID(_ context.Context) hexutil.Uint64 {
	return hexutil.Uint64(s.cfg.ChainID)
}

func (s *ethService) BlockNumber(ctx context.Context) (hexutil.Uint64, error) {
	n, err := s.store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		return 0, apperr.DependencyError(err, "get latest EVM block number")
	}
	// Add time-based block progression to simulate block production
	// This ensures MetaMask sees confirmations accumulating over time
	// Simulates ~1 block per second since we don't have real block production
	timeSinceStart := time.Since(s.startTime).Seconds()
	timeBasedBlocks := uint64(timeSinceStart)

	// Return max of: (latest tx block + 12) or (time-based blocks)
	// This ensures both old transactions and new ones appear confirmed
	baseBlock := n + confirmationBufferBlocks

	return hexutil.Uint64(max(baseBlock, timeBasedBlocks)), nil
}

func (s *ethService) GasPrice(_ context.Context) (*hexutil.Big, error) {
	gasPrice := new(big.Int)
	if _, ok := gasPrice.SetString(s.cfg.GasPriceWei, decimalStringBase); !ok {
		return nil, apperr.GeneralError(fmt.Errorf("invalid gas price wei: %q", s.cfg.GasPriceWei))
	}

	return (*hexutil.Big)(gasPrice), nil
}

func (*ethService) MaxPriorityFeePerGas(_ context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(defaultGasPriceWeiInt64)), nil
}

func (s *ethService) EstimateGas(_ context.Context, _ *ethrpc.CallArgs) (hexutil.Uint64, error) {
	return hexutil.Uint64(s.cfg.GasLimit), nil
}

func (s *ethService) GetBalance(ctx context.Context, address common.Address) (*hexutil.Big, error) {
	bal, err := s.tokenService.Native().GetBalance(ctx, address)
	if err != nil {
		return nil, apperr.DependencyError(err, "get balance")
	}
	return (*hexutil.Big)(&bal), nil
}

func (s *ethService) GetTransactionCount(ctx context.Context, address common.Address) (hexutil.Uint64, error) {
	count, err := s.store.GetEvmTransactionCount(ctx, address.Hex())
	if err != nil {
		return 0, apperr.DependencyError(err, fmt.Sprintf("get transaction count for %s", address.Hex()))
	}
	return hexutil.Uint64(count), nil
}

func (s *ethService) GetCode(_ context.Context, address common.Address) (hexutil.Bytes, error) {
	if s.tokenService == nil {
		return hexutil.Bytes{}, nil
	}
	if _, err := s.tokenService.ERC20(address); err == nil {
		// Return minimal non-empty bytecode for supported ERC20 contracts.
		return hexutil.Bytes{0x60, 0x80}, nil
	}
	return hexutil.Bytes{}, nil
}

func (*ethService) Syncing(_ context.Context) bool {
	return false
}

func (s *ethService) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, apperr.BadRequestError(err, "invalid transaction encoding")
	}
	tx.To()

	signer := types.LatestSignerForChainID(s.chainID)
	from, err := types.Sender(signer, &tx)
	if err != nil {
		return common.Hash{}, apperr.BadRequestError(err, "invalid transaction signature")
	}

	contractAddress := tx.To()
	if contractAddress == nil {
		// Contract deploy transactions have no To address and are not supported.
		return common.Hash{}, apperr.BadRequestError(nil, "contract deploy transactions not supported")
	}

	if tx.Value().Sign() != 0 {
		// Only zero-value ERC20 transfer calls are supported; native ETH sends are not.
		return common.Hash{}, apperr.NotSupportedError(nil, "native ETH transfers not supported")
	}

	input := tx.Data()
	toAddr, amount, err := s.decodeTransferCall(input)
	if err != nil {
		return common.Hash{}, apperr.BadRequestError(err, "invalid transaction data")
	}

	erc20, err := s.tokenService.ERC20(*contractAddress)
	if err != nil {
		return common.Hash{}, apperr.BadRequestError(err, fmt.Sprintf("contract not supported: %s", contractAddress.Hex()))
	}

	txHash := tx.Hash()
	if err = erc20.TransferFrom(ctx, txHash.Hex(), from, toAddr, *amount); err != nil {
		// Pass categorized errors from the token service through directly so
		// callers receive the correct JSON-RPC error code (e.g. -32602 for
		// "sender not found", not the generic -32000 dependency failure).
		return common.Hash{}, err
	}
	if err = s.recordSyntheticTransfer(ctx, txHash, input, tx.Nonce(), from, *contractAddress, toAddr, amount); err != nil {
		return common.Hash{}, err
	}

	return txHash, nil
}

func (s *ethService) decodeTransferCall(input []byte) (common.Address, *big.Int, error) {
	if len(input) < functionSelectorSize {
		return common.Address{}, nil, fmt.Errorf("missing function selector")
	}

	method, err := s.erc20ABI.MethodById(input[:functionSelectorSize])
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("decode method selector: %w", err)
	}
	if method.Name != "transfer" {
		return common.Address{}, nil, fmt.Errorf("unsupported method: %s", method.Name)
	}

	args := make(map[string]any)
	if err = method.Inputs.UnpackIntoMap(args, input[functionSelectorSize:]); err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to decode transfer args: %w", err)
	}

	toAddr, ok := args["to"].(common.Address)
	if !ok {
		return common.Address{}, nil, fmt.Errorf("invalid 'to' address in transfer")
	}
	amount, ok := args["value"].(*big.Int)
	if !ok {
		return common.Address{}, nil, fmt.Errorf("invalid 'value' in transfer")
	}
	return toAddr, amount, nil
}

func (s *ethService) recordSyntheticTransfer(
	ctx context.Context,
	txHash common.Hash,
	input []byte,
	nonce uint64,
	from common.Address,
	contractAddress common.Address,
	toAddr common.Address,
	amount *big.Int,
) error {
	blockNumber, blockHash, txIndex, err := s.store.NextEvmBlock(ctx, s.chainID.Uint64())
	if err != nil {
		return apperr.DependencyError(err, "get next EVM block")
	}

	evmTx := &ethrpc.EvmTransaction{
		TxHash:      txHash.Bytes(),
		FromAddress: from.Hex(),
		ToAddress:   contractAddress.Hex(),
		Nonce:       nonce,
		Input:       input,
		ValueWei:    "0",
		Status:      1,
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		GasUsed:     s.cfg.GasLimit,
	}
	if err = s.store.SaveEvmTransaction(ctx, evmTx); err != nil {
		return apperr.DependencyError(err, "save EVM transaction")
	}

	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	fromTopic := common.BytesToHash(common.LeftPadBytes(from.Bytes(), topicSizeBytes))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), topicSizeBytes))
	amountBytes := common.LeftPadBytes(amount.Bytes(), topicSizeBytes)

	evmLog := &ethrpc.EvmLog{
		TxHash:      txHash.Bytes(),
		LogIndex:    0,
		Address:     contractAddress.Bytes(),
		Topics:      [][]byte{transferTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:        amountBytes,
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		Removed:     false,
	}
	if err = s.store.SaveEvmLog(ctx, evmLog); err != nil {
		return apperr.DependencyError(err, "save EVM log")
	}

	return nil
}

func (s *ethService) GetTransactionReceipt(ctx context.Context, hash common.Hash) (*ethrpc.RPCReceipt, error) {
	row, err := s.store.GetEvmTransaction(ctx, hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get transaction receipt for %s", hash.Hex()))
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)

	dbLogs, err := s.store.GetEvmLogsByTxHash(ctx, hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get logs for transaction receipt %s", hash.Hex()))
	}

	logs := make([]*types.Log, 0)
	for _, dbLog := range dbLogs {
		log := &types.Log{
			Address:     common.BytesToAddress(dbLog.Address),
			Data:        dbLog.Data,
			BlockNumber: dbLog.BlockNumber,
			TxHash:      hash,
			TxIndex:     dbLog.TxIndex,
			BlockHash:   common.BytesToHash(dbLog.BlockHash),
			Index:       dbLog.LogIndex,
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
		EffectiveGasPrice: hexutil.Uint64(defaultGasPriceWeiUint64),
		Type:              hexutil.Uint64(2),
	}, nil
}

func (s *ethService) GetTransactionByHash(ctx context.Context, hash common.Hash) (*ethrpc.RPCTransaction, error) {
	row, err := s.store.GetEvmTransaction(ctx, hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get transaction by hash %s", hash.Hex()))
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)
	blockHash := common.BytesToHash(row.BlockHash)
	blockNum := hexutil.Uint64(row.BlockNumber)
	txIndex := hexutil.Uint(row.TxIndex)
	gasPrice := big.NewInt(defaultGasPriceWeiInt64)

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

func (s *ethService) Call(ctx context.Context, args *ethrpc.CallArgs) (hexutil.Bytes, error) {
	if args == nil || args.To == nil {
		return nil, apperr.BadRequestError(nil, "unsupported 'contract' address")
	}
	erc20, err := s.tokenService.ERC20(*args.To)
	if err != nil {
		return nil, apperr.BadRequestError(err, fmt.Sprintf("contract not supported: %s", args.To.Hex()))
	}

	input := args.GetData()
	if len(input) < functionSelectorSize {
		return nil, apperr.BadRequestError(nil, "missing function selector")
	}

	method, err := s.erc20ABI.MethodById(input[:functionSelectorSize])
	if err != nil {
		return nil, apperr.BadRequestError(nil, "unknown method")
	}

	switch method.Name {
	case "balanceOf":
		return s.callBalanceOf(ctx, input[functionSelectorSize:], erc20)
	case "decimals":
		return callDecimals(ctx, erc20)
	case "symbol":
		return callSymbol(ctx, erc20)
	case "name":
		return callName(ctx, erc20)
	case "totalSupply":
		return callTotalSupply(ctx, erc20)
	case "allowance":
		return s.callAllowance(ctx, input[functionSelectorSize:], erc20)
	default:
		return nil, apperr.NotSupportedError(nil, fmt.Sprintf("unsupported method: %s", method.Name))
	}
}

func (s *ethService) callBalanceOf(ctx context.Context, data []byte, erc20 token.ERC20) (hexutil.Bytes, error) {
	method := s.erc20ABI.Methods["balanceOf"]
	args := make(map[string]any)
	if err := method.Inputs.UnpackIntoMap(args, data); err != nil {
		return nil, fmt.Errorf("decode balanceOf args: %w", err)
	}

	addr, ok := args["account"].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid account address")
	}

	bal := erc20.BalanceOf(ctx, addr)
	return encodeUint256(&bal)
}

func callDecimals(ctx context.Context, erc20 token.ERC20) (hexutil.Bytes, error) {
	return encodeUint8(erc20.Decimals(ctx))
}

func callSymbol(ctx context.Context, erc20 token.ERC20) (hexutil.Bytes, error) {
	return encodeString(erc20.Symbol(ctx))
}

func callName(ctx context.Context, erc20 token.ERC20) (hexutil.Bytes, error) {
	return encodeString(erc20.Name(ctx))
}

func callTotalSupply(ctx context.Context, erc20 token.ERC20) (hexutil.Bytes, error) {
	supply := erc20.TotalSupply(ctx)
	return encodeUint256(&supply)
}

func (s *ethService) callAllowance(ctx context.Context, data []byte, erc20 token.ERC20) (hexutil.Bytes, error) {
	method := s.erc20ABI.Methods["allowance"]
	args := make(map[string]any)
	if err := method.Inputs.UnpackIntoMap(args, data); err != nil {
		return nil, fmt.Errorf("decode allowance args: %w", err)
	}

	owner, ok := args["owner"].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid owner address")
	}
	spender, ok := args["spender"].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid spender address")
	}

	allowance := erc20.Allowance(ctx, owner, spender)
	return encodeUint256(&allowance)
}

func (s *ethService) GetLogs(ctx context.Context, query ethrpc.FilterQuery) ([]*types.Log, error) {
	var fromBlock, toBlock uint64
	if query.FromBlock != nil {
		fromBlock = uint64(*query.FromBlock)
	}
	if query.ToBlock != nil {
		toBlock = uint64(*query.ToBlock)
	} else {
		var err error
		toBlock, err = s.store.GetLatestEvmBlockNumber(ctx)
		if err != nil {
			return nil, apperr.DependencyError(err, "get latest EVM block number for logs")
		}
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

	dbLogs, err := s.store.GetEvmLogs(ctx, addressFilter, topic0Filter, fromBlock, toBlock)
	if err != nil {
		return nil, apperr.DependencyError(err, "get EVM logs")
	}

	var logs []*types.Log
	for _, dbLog := range dbLogs {
		log := &types.Log{
			Address:     common.BytesToAddress(dbLog.Address),
			Data:        dbLog.Data,
			BlockNumber: dbLog.BlockNumber,
			TxHash:      common.BytesToHash(dbLog.TxHash),
			TxIndex:     dbLog.TxIndex,
			BlockHash:   common.BytesToHash(dbLog.BlockHash),
			Index:       dbLog.LogIndex,
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
	if block.BlockNumber != nil {
		blockNum = uint64(*block.BlockNumber)
	} else {
		latestBlockNum, err := s.BlockNumber(ctx)
		if err != nil {
			return nil, err
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
		Timestamp:        hexutil.Uint64(blockNum * syntheticBlockTimeSeconds),
		Transactions:     []any{},
		Uncles:           []common.Hash{},
		BaseFeePerGas:    (*hexutil.Big)(big.NewInt(defaultGasPriceWeiInt64)),
	}, nil
}

func (s *ethService) GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error) {
	blockNum, err := s.store.GetBlockNumberByHash(ctx, hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get block number by hash %s", hash.Hex()))
	}

	if blockNum > 0 {
		blockNumHex := hexutil.Uint64(blockNum)
		return s.GetBlockByNumber(ctx, ethrpc.BlockNumberOrHash{BlockNumber: &blockNumHex}, fullTx)
	}

	return s.GetBlockByNumber(ctx, ethrpc.BlockNumberOrHash{}, fullTx)
}

func encodeUint256(v *big.Int) (hexutil.Bytes, error) {
	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		return nil, fmt.Errorf("create uint256 ABI type: %w", err)
	}
	args := abi.Arguments{{Type: uint256Type}}
	return args.Pack(v)
}

func encodeUint8(v uint8) (hexutil.Bytes, error) {
	uint8Type, err := abi.NewType("uint8", "", nil)
	if err != nil {
		return nil, fmt.Errorf("create uint8 ABI type: %w", err)
	}
	args := abi.Arguments{{Type: uint8Type}}
	return args.Pack(v)
}

func encodeString(s string) (hexutil.Bytes, error) {
	stringType, err := abi.NewType("string", "", nil)
	if err != nil {
		return nil, fmt.Errorf("create string ABI type: %w", err)
	}
	args := abi.Arguments{{Type: stringType}}
	return args.Pack(s)
}
