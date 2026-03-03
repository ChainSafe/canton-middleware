package service

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/config"
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
	ChainID() hexutil.Uint64
	BlockNumber() (hexutil.Uint64, error)
	GasPrice() (*hexutil.Big, error)
	MaxPriorityFeePerGas() (*hexutil.Big, error)
	EstimateGas(ctx context.Context, args *ethrpc.CallArgs) (hexutil.Uint64, error)
	GetBalance(ctx context.Context, address common.Address) (*hexutil.Big, error)
	GetTransactionCount(ctx context.Context, address common.Address) (hexutil.Uint64, error)
	GetCode(ctx context.Context, address common.Address) (hexutil.Bytes, error)
	Syncing() bool
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
	cfg          config.EthRPCConfig
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
	cfg *config.EthRPCConfig,
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

func (s *ethService) ChainID() hexutil.Uint64 {
	return hexutil.Uint64(s.cfg.ChainID)
}

func (s *ethService) BlockNumber() (hexutil.Uint64, error) {
	n, err := s.store.GetLatestEvmBlockNumber()
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

func (s *ethService) GasPrice() (*hexutil.Big, error) {
	gasPrice := new(big.Int)
	if _, ok := gasPrice.SetString(s.cfg.GasPriceWei, decimalStringBase); !ok {
		return nil, apperr.GeneralError(fmt.Errorf("invalid gas price wei: %q", s.cfg.GasPriceWei))
	}

	return (*hexutil.Big)(gasPrice), nil
}

func (*ethService) MaxPriorityFeePerGas() (*hexutil.Big, error) {
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

func (s *ethService) GetTransactionCount(_ context.Context, address common.Address) (hexutil.Uint64, error) {
	count, err := s.store.GetEvmTransactionCount(address.Hex())
	if err != nil {
		return 0, apperr.DependencyError(err, fmt.Sprintf("get transaction count for %s", address.Hex()))
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

func (*ethService) Syncing() bool {
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
	if err = erc20.TransferFrom(ctx, from, toAddr, *amount); err != nil {
		// Pass categorized errors from the token service through directly so
		// callers receive the correct JSON-RPC error code (e.g. -32602 for
		// "sender not found", not the generic -32000 dependency failure).
		return common.Hash{}, err
	}

	txHash := tx.Hash()
	if err = s.recordSyntheticTransfer(txHash, input, tx.Nonce(), from, *contractAddress, toAddr, amount); err != nil {
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
	txHash common.Hash,
	input []byte,
	nonce uint64,
	from common.Address,
	contractAddress common.Address,
	toAddr common.Address,
	amount *big.Int,
) error {
	blockNumber, blockHash, txIndex, err := s.store.NextEvmBlock(s.chainID.Uint64())
	if err != nil {
		return apperr.DependencyError(err, "get next EVM block")
	}
	txNonce, err := uint64ToInt64(nonce, "transaction nonce")
	if err != nil {
		return err
	}
	blockNumberInt64, err := uint64ToInt64(blockNumber, "block number")
	if err != nil {
		return err
	}
	gasUsedInt64, err := uint64ToInt64(s.cfg.GasLimit, "gas limit")
	if err != nil {
		return err
	}

	evmTx := &apidb.EvmTransaction{
		TxHash:      txHash.Bytes(),
		FromAddress: from.Hex(),
		ToAddress:   contractAddress.Hex(),
		Nonce:       txNonce,
		Input:       input,
		ValueWei:    "0",
		Status:      1,
		BlockNumber: blockNumberInt64,
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		GasUsed:     gasUsedInt64,
	}
	if err = s.store.SaveEvmTransaction(evmTx); err != nil {
		return apperr.DependencyError(err, "save EVM transaction")
	}

	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	fromTopic := common.BytesToHash(common.LeftPadBytes(from.Bytes(), topicSizeBytes))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), topicSizeBytes))
	amountBytes := common.LeftPadBytes(amount.Bytes(), topicSizeBytes)

	evmLog := &apidb.EvmLog{
		TxHash:      txHash.Bytes(),
		LogIndex:    0,
		Address:     contractAddress.Bytes(),
		Topics:      [][]byte{transferTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:        amountBytes,
		BlockNumber: blockNumberInt64,
		BlockHash:   blockHash,
		TxIndex:     txIndex,
		Removed:     false,
	}
	if err = s.store.SaveEvmLog(evmLog); err != nil {
		return apperr.DependencyError(err, "save EVM log")
	}

	return nil
}

func (s *ethService) GetTransactionReceipt(_ context.Context, hash common.Hash) (*ethrpc.RPCReceipt, error) {
	row, err := s.store.GetEvmTransaction(hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get transaction receipt for %s", hash.Hex()))
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)

	dbLogs, err := s.store.GetEvmLogsByTxHash(hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get logs for transaction receipt %s", hash.Hex()))
	}

	logs := make([]*types.Log, 0)
	for _, dbLog := range dbLogs {
		logBlockNumber, convErr := int64ToUint64(dbLog.BlockNumber, "log block number")
		if convErr != nil {
			return nil, convErr
		}
		logTxIndex, convErr := intToUint(dbLog.TxIndex, "log transaction index")
		if convErr != nil {
			return nil, convErr
		}
		logIndex, convErr := intToUint(dbLog.LogIndex, "log index")
		if convErr != nil {
			return nil, convErr
		}

		log := &types.Log{
			Address:     common.BytesToAddress(dbLog.Address),
			Data:        dbLog.Data,
			BlockNumber: logBlockNumber,
			TxHash:      hash,
			TxIndex:     logTxIndex,
			BlockHash:   common.BytesToHash(dbLog.BlockHash),
			Index:       logIndex,
			Removed:     dbLog.Removed,
		}
		for _, topic := range dbLog.Topics {
			log.Topics = append(log.Topics, common.BytesToHash(topic))
		}
		logs = append(logs, log)
	}
	bloom := types.CreateBloom(&types.Receipt{Logs: logs})

	txIndex, err := intToHexUint(row.TxIndex, "transaction index")
	if err != nil {
		return nil, err
	}
	blockNumber, err := int64ToHexUint64(row.BlockNumber, "block number")
	if err != nil {
		return nil, err
	}
	gasUsed, err := int64ToHexUint64(row.GasUsed, "gas used")
	if err != nil {
		return nil, err
	}
	status, err := int16ToHexUint64(row.Status, "status")
	if err != nil {
		return nil, err
	}

	return &ethrpc.RPCReceipt{
		TransactionHash:   hash,
		TransactionIndex:  txIndex,
		BlockHash:         common.BytesToHash(row.BlockHash),
		BlockNumber:       blockNumber,
		From:              from,
		To:                &to,
		CumulativeGasUsed: gasUsed,
		GasUsed:           gasUsed,
		ContractAddress:   nil,
		Logs:              logs,
		LogsBloom:         bloom,
		Status:            status,
		EffectiveGasPrice: hexutil.Uint64(defaultGasPriceWeiUint64),
		Type:              hexutil.Uint64(2),
	}, nil
}

func (s *ethService) GetTransactionByHash(_ context.Context, hash common.Hash) (*ethrpc.RPCTransaction, error) {
	row, err := s.store.GetEvmTransaction(hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get transaction by hash %s", hash.Hex()))
	}
	if row == nil {
		return nil, nil
	}

	from := common.HexToAddress(row.FromAddress)
	to := common.HexToAddress(row.ToAddress)
	blockHash := common.BytesToHash(row.BlockHash)
	blockNum, err := int64ToHexUint64(row.BlockNumber, "block number")
	if err != nil {
		return nil, err
	}
	txIndex, err := intToHexUint(row.TxIndex, "transaction index")
	if err != nil {
		return nil, err
	}
	nonce, err := int64ToHexUint64(row.Nonce, "nonce")
	if err != nil {
		return nil, err
	}
	gasPrice := big.NewInt(defaultGasPriceWeiInt64)

	return &ethrpc.RPCTransaction{
		Hash:             hash,
		Nonce:            nonce,
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

func (s *ethService) GetLogs(_ context.Context, query ethrpc.FilterQuery) ([]*types.Log, error) {
	var fromBlock, toBlock int64
	if query.FromBlock != nil {
		var err error
		fromBlock, err = uint64ToInt64(uint64(*query.FromBlock), "from block")
		if err != nil {
			return nil, err
		}
	}
	if query.ToBlock != nil {
		var err error
		toBlock, err = uint64ToInt64(uint64(*query.ToBlock), "to block")
		if err != nil {
			return nil, err
		}
	} else {
		latest, err := s.store.GetLatestEvmBlockNumber()
		if err != nil {
			return nil, apperr.DependencyError(err, "get latest EVM block number for logs")
		}
		toBlock, err = uint64ToInt64(latest, "latest block")
		if err != nil {
			return nil, err
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

	dbLogs, err := s.store.GetEvmLogs(addressFilter, topic0Filter, fromBlock, toBlock)
	if err != nil {
		return nil, apperr.DependencyError(err, "get EVM logs")
	}

	var logs []*types.Log
	for _, dbLog := range dbLogs {
		logBlockNumber, convErr := int64ToUint64(dbLog.BlockNumber, "log block number")
		if convErr != nil {
			return nil, convErr
		}
		logTxIndex, convErr := intToUint(dbLog.TxIndex, "log transaction index")
		if convErr != nil {
			return nil, convErr
		}
		logIndex, convErr := intToUint(dbLog.LogIndex, "log index")
		if convErr != nil {
			return nil, convErr
		}

		log := &types.Log{
			Address:     common.BytesToAddress(dbLog.Address),
			Data:        dbLog.Data,
			BlockNumber: logBlockNumber,
			TxHash:      common.BytesToHash(dbLog.TxHash),
			TxIndex:     logTxIndex,
			BlockHash:   common.BytesToHash(dbLog.BlockHash),
			Index:       logIndex,
			Removed:     dbLog.Removed,
		}
		for _, topic := range dbLog.Topics {
			log.Topics = append(log.Topics, common.BytesToHash(topic))
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func (s *ethService) GetBlockByNumber(_ context.Context, block ethrpc.BlockNumberOrHash, _ bool) (*ethrpc.RPCBlock, error) {
	var blockNum uint64
	if block.BlockNumber != nil {
		blockNum = uint64(*block.BlockNumber)
	} else {
		latestBlockNum, err := s.BlockNumber()
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
	blockNum, err := s.store.GetBlockNumberByHash(hash.Bytes())
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

func uint64ToInt64(v uint64, fieldName string) (int64, error) {
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("%s overflows int64: %d", fieldName, v)
	}
	return int64(v), nil
}

func int64ToUint64(v int64, fieldName string) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("%s must be non-negative: %d", fieldName, v)
	}
	return uint64(v), nil
}

func intToUint(v int, fieldName string) (uint, error) {
	if v < 0 {
		return 0, fmt.Errorf("%s must be non-negative: %d", fieldName, v)
	}
	return uint(v), nil
}

func intToHexUint(v int, fieldName string) (hexutil.Uint, error) {
	parsed, err := intToUint(v, fieldName)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint(parsed), nil
}

func int64ToHexUint64(v int64, fieldName string) (hexutil.Uint64, error) {
	parsed, err := int64ToUint64(v, fieldName)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(parsed), nil
}

func int16ToHexUint64(v int16, fieldName string) (hexutil.Uint64, error) {
	parsed, err := int64ToUint64(int64(v), fieldName)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(parsed), nil
}
