// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

// Store is the narrow data-access interface consumed by the EthRPC service.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/ethrpc --name PendingBlock --output mocks --outpkg mocks --filename mock_pending_block.go --with-expecter
type Store interface {
	// Read-only queries used by JSON-RPC handlers.
	GetLatestEvmBlockNumber(ctx context.Context) (uint64, error)
	GetEvmTransactionCount(ctx context.Context, fromAddress string) (uint64, error)
	GetEvmTransaction(ctx context.Context, txHash []byte) (*ethrpc.EvmTransaction, error)
	GetEvmLogsByTxHash(ctx context.Context, txHash []byte) ([]*ethrpc.EvmLog, error)
	GetEvmLogs(ctx context.Context, address []byte, topic0 []byte, fromBlock, toBlock uint64) ([]*ethrpc.EvmLog, error)
	GetBlockNumberByHash(ctx context.Context, blockHash []byte) (uint64, error)
	GetChainHead(ctx context.Context) (*ethrpc.ChainHead, error)
	GetEvmBlock(ctx context.Context, number uint64) (*ethrpc.EvmBlock, error)
	GetSealedBlockBefore(ctx context.Context, number uint64) (*ethrpc.EvmBlock, error)

	// InsertMempoolEntry records a new transfer intent with status=pending.
	// SendRawTransaction returns immediately after this insert; the submitter
	// worker drives pending → completed/failed asynchronously, and the miner
	// then seals the entry into a synthetic EVM block.
	InsertMempoolEntry(ctx context.Context, entry *ethrpc.MempoolEntry) error
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
	whitelist    whitelist.Checker
	chainID      *big.Int
	erc20ABI     abi.ABI
}

const (
	functionSelectorSize      = 4
	confirmationBufferBlocks  = uint64(12)
	syntheticBlockTimeSeconds = uint64(12)
	// recentBlockHashWindow is how many blocks below the head eth_getBlockByHash
	// searches for an unsealed block, as the EVM's BLOCKHASH does.
	recentBlockHashWindow = uint64(256)
	// DefaultGasLimit is the cosmetic gas limit the facade reports (eth_estimateGas,
	// block gasLimit, tx gas, synthetic block gasUsed). The facade executes
	// transfers on Canton rather than an EVM, so gas is never metered — and the
	// gas price is fixed at 0 — making this value purely informational for wallets.
	DefaultGasLimit = 21_000
)

// NewService creates a new ethService. whitelist authorizes transaction senders
// in SendRawTransaction and must be non-nil; pass whitelist.New(store, true) to
// disable the gate (skip_whitelist_check).
func NewService(
	cfg *ethrpc.Config,
	evmStore Store,
	tokenSvc TokenService,
	whitelist whitelist.Checker,
) Service {
	if cfg == nil {
		panic("ethrpc: config is nil")
	}
	if whitelist == nil {
		panic("ethrpc: whitelist checker is required (use whitelist.New to construct one)")
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
		whitelist:    whitelist,
		erc20ABI:     parsedABI,
		chainID:      new(big.Int).SetUint64(cfg.ChainID),
	}
}

func (s *ethService) ChainID(_ context.Context) hexutil.Uint64 {
	return hexutil.Uint64(s.cfg.ChainID)
}

func (s *ethService) BlockNumber(ctx context.Context) (hexutil.Uint64, error) {
	head, err := s.store.GetChainHead(ctx)
	if err != nil {
		return 0, apperr.DependencyError(err, "get chain head")
	}
	return hexutil.Uint64(s.headNumber(head)), nil
}

// headNumber is the reported head: at least 12 blocks above the last sealed one
// so transfers show confirmations, and advancing one block per second from the
// persisted anchor so it is stable across restarts and replicas.
func (s *ethService) headNumber(head *ethrpc.ChainHead) uint64 {
	now := uint64(time.Now().Unix()) //nolint:gosec // time is always positive
	return max(head.LatestBlock+confirmationBufferBlocks, head.TimeHead(now))
}

// GasPrice always reports 0, and so do maxPriorityFeePerGas, block
// baseFeePerGas, and receipt effectiveGasPrice. The facade never charges gas,
// and MetaMask's client-side pre-flight check is
// `balance >= value + gasLimit*gasPrice`. Since eth_getBalance reports a zero
// native balance and this facade only accepts zero-value ERC-20 transfers, the
// check collapses to `0 >= 0` only while gas is 0 — any non-zero gas price would
// make MetaMask reject transfers with "insufficient funds for gas". Gas is
// therefore fixed at 0 in code rather than exposed as config.
func (*ethService) GasPrice(_ context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func (*ethService) MaxPriorityFeePerGas(_ context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func (*ethService) EstimateGas(_ context.Context, _ *ethrpc.CallArgs) (hexutil.Uint64, error) {
	return hexutil.Uint64(DefaultGasLimit), nil
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

// SendRawTransaction validates an EVM-encoded ERC-20 transfer, records the
// intent in the mempool as pending, and returns the tx hash immediately. The
// Canton transfer is executed asynchronously by the submitter worker, which
// transitions the entry to completed or failed. The miner then seals the entry
// into a synthetic EVM block, at which point eth_getTransactionReceipt yields a
// terminal status=0x1 (success) or status=0x0 (failure) receipt.
//
// This matches standard Ethereum semantics where the node accepts a signed
// transaction synchronously and clients poll the receipt — avoiding 5-15s+ HTTP
// stalls under Canton load that previously broke MetaMask compatibility.
// checkWhitelist authorizes a transaction sender against the registration
// whitelist.
func (s *ethService) checkWhitelist(ctx context.Context, from common.Address) error {
	addr := strings.ToLower(from.Hex())
	whitelisted, err := s.whitelist.IsWhitelisted(ctx, addr)
	if err != nil {
		return apperr.DependencyError(err, "whitelist check")
	}
	if !whitelisted {
		return apperr.ForbiddenError(nil, fmt.Sprintf("sender not whitelisted: %s", addr))
	}
	return nil
}

func (s *ethService) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, apperr.BadRequestError(err, "invalid transaction encoding")
	}

	signer := types.LatestSignerForChainID(s.chainID)
	from, err := types.Sender(signer, &tx)
	if err != nil {
		return common.Hash{}, apperr.BadRequestError(err, "invalid transaction signature")
	}

	// Authorize the sender before doing any work: only whitelisted addresses
	// may submit transfers through the facade. Rejected synchronously so the
	// wallet gets an immediate error instead of an eventual failed receipt.
	if err = s.checkWhitelist(ctx, from); err != nil {
		return common.Hash{}, err
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

	// Reject unsupported contracts synchronously so wallets surface the error
	// before they ever start polling for a receipt that will never arrive.
	if _, err = s.tokenService.ERC20(*contractAddress); err != nil {
		return common.Hash{}, apperr.BadRequestError(err, fmt.Sprintf("contract not supported: %s", contractAddress.Hex()))
	}

	txHash := tx.Hash()
	entry := &ethrpc.MempoolEntry{
		TxHash:           txHash.Bytes(),
		FromAddress:      from.Hex(),
		ContractAddress:  contractAddress.Hex(),
		RecipientAddress: toAddr.Hex(),
		Nonce:            tx.Nonce(),
		Input:            input,
		AmountData:       amount.Bytes(),
	}
	if err = s.store.InsertMempoolEntry(ctx, entry); err != nil {
		return common.Hash{}, apperr.DependencyError(err, "insert mempool entry")
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
			Address:        common.BytesToAddress(dbLog.Address),
			Data:           dbLog.Data,
			BlockNumber:    dbLog.BlockNumber,
			BlockTimestamp: dbLog.BlockTimestamp,
			TxHash:         hash,
			TxIndex:        dbLog.TxIndex,
			BlockHash:      common.BytesToHash(dbLog.BlockHash),
			Index:          dbLog.LogIndex,
			Removed:        dbLog.Removed,
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
		EffectiveGasPrice: hexutil.Uint64(0),
		Type:              hexutil.Uint64(2),
		// RevertReason is omitted when empty (omitempty), so successful
		// receipts keep the standard JSON shape.
		RevertReason:   row.ErrorMessage,
		CantonUpdateID: row.CantonUpdateID,
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
	return s.rpcTransaction(row), nil
}

// rpcTransaction renders a stored transaction. The facade keeps no signed
// transaction, so fees and the signature are zero.
func (s *ethService) rpcTransaction(row *ethrpc.EvmTransaction) *ethrpc.RPCTransaction {
	to := common.HexToAddress(row.ToAddress)
	blockHash := common.BytesToHash(row.BlockHash)
	blockNum := hexutil.Uint64(row.BlockNumber)
	txIndex := hexutil.Uint(row.TxIndex)
	zero := func() *hexutil.Big { return (*hexutil.Big)(big.NewInt(0)) }

	return &ethrpc.RPCTransaction{
		Hash:                 common.BytesToHash(row.TxHash),
		Nonce:                hexutil.Uint64(row.Nonce),
		BlockHash:            &blockHash,
		BlockNumber:          &blockNum,
		TransactionIndex:     &txIndex,
		From:                 common.HexToAddress(row.FromAddress),
		To:                   &to,
		Value:                zero(),
		GasPrice:             zero(),
		Gas:                  hexutil.Uint64(DefaultGasLimit),
		Input:                row.Input,
		V:                    zero(),
		R:                    zero(),
		S:                    zero(),
		Type:                 hexutil.Uint64(types.DynamicFeeTxType),
		ChainID:              (*hexutil.Big)(new(big.Int).Set(s.chainID)),
		MaxFeePerGas:         zero(),
		MaxPriorityFeePerGas: zero(),
		AccessList:           types.AccessList{},
	}
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
	latest, err := s.store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		return nil, apperr.DependencyError(err, "get latest EVM block number for logs")
	}
	// An omitted fromBlock covers the whole chain. Block tags and an omitted
	// toBlock resolve to the latest mined block, the last one with logs.
	var fromBlock uint64
	if query.FromBlock != nil {
		fromBlock = resolveBlockTag(query.FromBlock, latest)
	}
	toBlock := resolveBlockTag(query.ToBlock, latest)

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
			Address:        common.BytesToAddress(dbLog.Address),
			Data:           dbLog.Data,
			BlockNumber:    dbLog.BlockNumber,
			BlockTimestamp: dbLog.BlockTimestamp,
			TxHash:         common.BytesToHash(dbLog.TxHash),
			TxIndex:        dbLog.TxIndex,
			BlockHash:      common.BytesToHash(dbLog.BlockHash),
			Index:          dbLog.LogIndex,
			Removed:        dbLog.Removed,
		}
		for _, topic := range dbLog.Topics {
			log.Topics = append(log.Topics, common.BytesToHash(topic))
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// resolveBlockTag returns the block number of an eth_getLogs bound; tags and an
// omitted bound resolve to latest.
func resolveBlockTag(tag *ethrpc.BlockTag, latest uint64) uint64 {
	if tag == nil || tag.Number == nil {
		return latest
	}
	return *tag.Number
}

func (s *ethService) GetBlockByNumber(ctx context.Context, block ethrpc.BlockNumberOrHash, fullTx bool) (*ethrpc.RPCBlock, error) {
	head, err := s.store.GetChainHead(ctx)
	if err != nil {
		return nil, apperr.DependencyError(err, "get chain head")
	}
	headNum := s.headNumber(head)

	blockNum := headNum
	if block.BlockNumber != nil {
		blockNum = uint64(*block.BlockNumber)
	}
	if blockNum == 0 || blockNum > headNum {
		return nil, nil
	}
	return s.buildBlock(ctx, head, blockNum, fullTx)
}

func (s *ethService) GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error) {
	blockNum, err := s.store.GetBlockNumberByHash(ctx, hash.Bytes())
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get block number by hash %s", hash.Hex()))
	}

	head, err := s.store.GetChainHead(ctx)
	if err != nil {
		return nil, apperr.DependencyError(err, "get chain head")
	}
	if blockNum == 0 {
		// Unsealed blocks are not stored; their hashes derive from the number,
		// so match against the recent ones.
		blockNum = s.recentBlockNumberByHash(hash, s.headNumber(head))
		if blockNum == 0 {
			return nil, nil
		}
	}
	return s.buildBlock(ctx, head, blockNum, fullTx)
}

func (s *ethService) recentBlockNumberByHash(hash common.Hash, headNum uint64) uint64 {
	for n := headNum; n > 0 && headNum-n < recentBlockHashWindow; n-- {
		if common.BytesToHash(ethereum.ComputeBlockHash(s.chainID.Uint64(), n)) == hash {
			return n
		}
	}
	return 0
}

// buildBlock renders block blockNum. A sealed block lists its transactions, with
// roots and bloom derived from them and the time it was sealed. Any other block
// is empty, timed from the chain head anchor.
func (s *ethService) buildBlock(
	ctx context.Context, head *ethrpc.ChainHead, blockNum uint64, fullTx bool,
) (*ethrpc.RPCBlock, error) {
	blockHash := common.BytesToHash(ethereum.ComputeBlockHash(s.chainID.Uint64(), blockNum))
	parentHash := common.Hash{}
	if blockNum > 1 {
		parentHash = common.BytesToHash(ethereum.ComputeBlockHash(s.chainID.Uint64(), blockNum-1))
	}

	out := &ethrpc.RPCBlock{
		Number:           hexutil.Uint64(blockNum),
		Hash:             blockHash,
		ParentHash:       parentHash,
		Nonce:            types.BlockNonce{},
		Sha3Uncles:       types.EmptyUncleHash,
		LogsBloom:        types.Bloom{},
		TransactionsRoot: types.EmptyTxsHash,
		StateRoot:        types.EmptyRootHash,
		ReceiptsRoot:     types.EmptyReceiptsHash,
		Miner:            common.Address{},
		Difficulty:       (*hexutil.Big)(big.NewInt(0)),
		TotalDifficulty:  (*hexutil.Big)(big.NewInt(0)),
		ExtraData:        []byte{},
		Size:             hexutil.Uint64(0),
		GasLimit:         hexutil.Uint64(DefaultGasLimit),
		GasUsed:          hexutil.Uint64(0),
		Transactions:     []any{},
		Uncles:           []common.Hash{},
		BaseFeePerGas:    (*hexutil.Big)(big.NewInt(0)),
	}

	sealed, err := s.store.GetEvmBlock(ctx, blockNum)
	if err != nil {
		return nil, apperr.DependencyError(err, fmt.Sprintf("get block %d", blockNum))
	}
	if sealed == nil {
		ts, err := s.unsealedBlockTime(ctx, head, blockNum)
		if err != nil {
			return nil, err
		}
		out.Timestamp = hexutil.Uint64(ts)
		return out, nil
	}

	out.Timestamp = hexutil.Uint64(sealed.Timestamp)
	if len(sealed.Transactions) == 0 {
		return out, nil
	}

	logsByTx := make(map[common.Hash][]*types.Log, len(sealed.Transactions))
	for _, l := range sealed.Logs {
		h := common.BytesToHash(l.TxHash)
		logsByTx[h] = append(logsByTx[h], toTypesLog(l))
	}

	hashes := make(txHashList, 0, len(sealed.Transactions))
	receipts := make(types.Receipts, 0, len(sealed.Transactions))
	var gasUsed uint64
	for _, tx := range sealed.Transactions {
		h := common.BytesToHash(tx.TxHash)
		hashes = append(hashes, h)
		if fullTx {
			out.Transactions = append(out.Transactions, s.rpcTransaction(tx))
		} else {
			out.Transactions = append(out.Transactions, h)
		}
		gasUsed += tx.GasUsed
		r := &types.Receipt{
			Type:              types.DynamicFeeTxType,
			Status:            uint64(tx.Status),
			CumulativeGasUsed: gasUsed,
			Logs:              logsByTx[h],
		}
		r.Bloom = types.CreateBloom(r)
		receipts = append(receipts, r)
	}

	// The transactions root is a trie over the transaction hashes: the facade keeps
	// no signed transactions to build the canonical one from.
	out.TransactionsRoot = types.DeriveSha(hashes, trie.NewStackTrie(nil))
	out.ReceiptsRoot = types.DeriveSha(receipts, trie.NewStackTrie(nil))
	out.LogsBloom = types.MergeBloom(receipts)
	out.GasUsed = hexutil.Uint64(gasUsed)
	return out, nil
}

// unsealedBlockTime dates an empty block. Above the anchor the head advances one
// block per second. Below it, blocks fill the gap left by the upgrade between the
// last sealed block before them and the anchor, spread evenly.
func (s *ethService) unsealedBlockTime(ctx context.Context, head *ethrpc.ChainHead, blockNum uint64) (uint64, error) {
	if head.AnchorTime == 0 {
		return blockNum * syntheticBlockTimeSeconds, nil
	}
	if blockNum >= head.AnchorBlock {
		return head.AnchorTime + (blockNum - head.AnchorBlock), nil
	}

	prev, err := s.store.GetSealedBlockBefore(ctx, blockNum)
	if err != nil {
		return 0, apperr.DependencyError(err, fmt.Sprintf("get sealed block before %d", blockNum))
	}
	if prev == nil {
		return head.AnchorTime - min(head.AnchorTime, head.AnchorBlock-blockNum), nil
	}
	if prev.Timestamp >= head.AnchorTime {
		return prev.Timestamp, nil
	}
	span := head.AnchorTime - prev.Timestamp
	return prev.Timestamp + (blockNum-prev.Number)*span/(head.AnchorBlock-prev.Number), nil
}

// txHashList lets types.DeriveSha build a trie over transaction hashes.
type txHashList []common.Hash

func (l txHashList) Len() int { return len(l) }

func (l txHashList) EncodeIndex(i int, w *bytes.Buffer) { w.Write(l[i].Bytes()) }

func toTypesLog(l *ethrpc.EvmLog) *types.Log {
	topics := make([]common.Hash, 0, len(l.Topics))
	for _, t := range l.Topics {
		topics = append(topics, common.BytesToHash(t))
	}
	return &types.Log{
		Address:        common.BytesToAddress(l.Address),
		Topics:         topics,
		Data:           l.Data,
		BlockNumber:    l.BlockNumber,
		BlockTimestamp: l.BlockTimestamp,
		TxHash:         common.BytesToHash(l.TxHash),
		TxIndex:        l.TxIndex,
		BlockHash:      common.BytesToHash(l.BlockHash),
		Index:          l.LogIndex,
		Removed:        l.Removed,
	}
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
