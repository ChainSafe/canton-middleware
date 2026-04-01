package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/uptrace/bun"
)

// transferEventTopic is the keccak256 hash of the ERC-20 Transfer event signature.
var transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

const evmLogsQueryLimit = 10000

// PGStore is a PostgreSQL-backed EVM store for EthRPC.
type PGStore struct {
	db *bun.DB
}

// NewStore creates a new PostgreSQL-backed EVM store.
func NewStore(db *bun.DB) *PGStore {
	return &PGStore{db: db}
}

// SaveEvmTransaction stores a synthetic EVM transaction.
func (s *PGStore) SaveEvmTransaction(ctx context.Context, tx *ethrpc.EvmTransaction) error {
	dao := toEvmTransactionDao(tx)

	_, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("save evm transaction: %w", err)
	}
	return nil
}

// GetEvmTransaction retrieves an EVM transaction by hash.
func (s *PGStore) GetEvmTransaction(ctx context.Context, txHash []byte) (*ethrpc.EvmTransaction, error) {
	dao := new(EvmTransactionDao)
	err := s.db.NewSelect().
		Model(dao).
		Where("tx_hash = ?", txHash).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get evm transaction: %w", err)
	}

	return fromEvmTransactionDao(dao), nil
}

// GetLatestEvmBlockNumber returns the latest synthetic EVM block number.
func (s *PGStore) GetLatestEvmBlockNumber(ctx context.Context) (uint64, error) {
	state := new(EvmStateDao)
	err := s.db.NewSelect().Model(state).Where("id = 1").Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get latest block number: %w", err)
	}
	return state.LatestBlock, nil
}

// NextEvmBlock allocates the next synthetic EVM block number.
// Returns block number, block hash, and tx index (always 0).
func (s *PGStore) NextEvmBlock(ctx context.Context, chainID uint64) (uint64, []byte, uint, error) {
	state := new(EvmStateDao)
	err := s.db.NewInsert().
		Model(&EvmStateDao{ID: 1, LatestBlock: 1}).
		On("CONFLICT (id) DO UPDATE").
		Set("latest_block = ?TableAlias.latest_block + 1").
		Returning("*").
		Scan(ctx, state)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("allocate next evm block: %w", err)
	}

	blockHash := ethereum.ComputeBlockHash(chainID, state.LatestBlock)
	return state.LatestBlock, blockHash, 0, nil
}

// GetBlockNumberByHash returns the block number for a block hash.
func (s *PGStore) GetBlockNumberByHash(ctx context.Context, blockHash []byte) (uint64, error) {
	dao := new(EvmTransactionDao)
	err := s.db.NewSelect().
		Model(dao).
		Column("block_number").
		Where("block_hash = ?", blockHash).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get block number by hash: %w", err)
	}
	return dao.BlockNumber, nil
}

// GetEvmTransactionCount returns the next nonce for the from address.
func (s *PGStore) GetEvmTransactionCount(ctx context.Context, fromAddress string) (uint64, error) {
	var nextNonce uint64
	err := s.db.NewSelect().
		Model((*EvmTransactionDao)(nil)).
		ColumnExpr("COALESCE(MAX(nonce) + 1, 0)").
		Where("from_address = ?", fromAddress).
		Scan(ctx, &nextNonce)
	if err != nil {
		return 0, fmt.Errorf("get transaction count for %s: %w", fromAddress, err)
	}
	return nextNonce, nil
}

// SaveEvmLog stores a synthetic EVM log entry.
func (s *PGStore) SaveEvmLog(ctx context.Context, log *ethrpc.EvmLog) error {
	dao := toEvmLogDao(log)

	_, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash, log_index) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("save evm log: %w", err)
	}
	return nil
}

// GetEvmLogsByTxHash retrieves all logs for a transaction hash.
func (s *PGStore) GetEvmLogsByTxHash(ctx context.Context, txHash []byte) ([]*ethrpc.EvmLog, error) {
	var daos []EvmLogDao
	err := s.db.NewSelect().
		Model(&daos).
		Where("tx_hash = ?", txHash).
		OrderExpr("log_index ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get evm logs by tx hash: %w", err)
	}

	logs := make([]*ethrpc.EvmLog, 0, len(daos))
	for i := range daos {
		logs = append(logs, fromEvmLogDao(&daos[i]))
	}
	return logs, nil
}

// GetEvmLogs retrieves logs matching address/topic0 and block range.
func (s *PGStore) GetEvmLogs(ctx context.Context, address []byte, topic0 []byte, fromBlock, toBlock uint64) ([]*ethrpc.EvmLog, error) {
	var daos []EvmLogDao
	query := s.db.NewSelect().
		Model(&daos).
		Where("block_number >= ?", fromBlock).
		Where("block_number <= ?", toBlock).
		OrderExpr("block_number ASC, tx_index ASC, log_index ASC").
		Limit(evmLogsQueryLimit)

	if address != nil {
		query = query.Where("address = ?", address)
	}
	if topic0 != nil {
		query = query.Where("topic0 = ?", topic0)
	}

	err := query.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get evm logs: %w", err)
	}

	logs := make([]*ethrpc.EvmLog, 0, len(daos))
	for i := range daos {
		logs = append(logs, fromEvmLogDao(&daos[i]))
	}
	return logs, nil
}

// InsertMempoolEntry records a new transfer intent with status=pending.
// Uses DO NOTHING on tx_hash conflict so duplicate submissions are safe.
func (s *PGStore) InsertMempoolEntry(ctx context.Context, entry *ethrpc.MempoolEntry) error {
	dao := &MempoolEntryDao{
		TxHash:           entry.TxHash,
		FromAddress:      entry.FromAddress,
		ContractAddress:  entry.ContractAddress,
		RecipientAddress: entry.RecipientAddress,
		Nonce:            entry.Nonce,
		Input:            entry.Input,
		AmountData:       entry.AmountData,
		Status:           string(ethrpc.MempoolPending),
	}
	_, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("insert mempool entry: %w", err)
	}
	return nil
}

// UpdateMempoolStatus transitions a mempool entry to completed, failed, or mined.
func (s *PGStore) UpdateMempoolStatus(ctx context.Context, txHash []byte, status ethrpc.MempoolStatus, errMsg string) error {
	q := s.db.NewUpdate().
		TableExpr("mempool").
		Set("status = ?", string(status)).
		Set("updated_at = current_timestamp").
		Where("tx_hash = ?", txHash)
	if errMsg != "" {
		q = q.Set("error_message = ?", errMsg)
	}
	_, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("update mempool status: %w", err)
	}
	return nil
}

// MineBlock atomically:
//  1. Claims the next EVM block number (serializes concurrent miners via row lock).
//  2. Fetches all completed mempool entries inside the same transaction.
//  3. Inserts synthetic evm_transactions and evm_logs for each entry.
//  4. Marks those mempool entries as mined.
//
// Returns the number of transactions mined (0 if there was nothing to mine).
func (s *PGStore) MineBlock(ctx context.Context, chainID, gasLimit uint64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}

	// Upsert evm_state to claim the next block. The DO UPDATE takes a row-level
	// exclusive lock, serializing concurrent miner instances automatically.
	state := new(EvmStateDao)
	if err = tx.NewInsert().
		Model(&EvmStateDao{ID: 1, LatestBlock: 1}).
		On("CONFLICT (id) DO UPDATE").
		Set("latest_block = ?TableAlias.latest_block + 1").
		Returning("*").
		Scan(ctx, state); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("claim next block: %w", err)
	}

	// Fetch all completed entries while holding the lock.
	var entries []MempoolEntryDao
	if err = tx.NewSelect().
		Model(&entries).
		Where("status = ?", string(ethrpc.MempoolCompleted)).
		OrderExpr("id ASC").
		Scan(ctx); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("fetch completed mempool entries: %w", err)
	}

	if len(entries) == 0 {
		_ = tx.Rollback()
		return 0, nil
	}

	blockHash := ethereum.ComputeBlockHash(chainID, state.LatestBlock)
	txHashes := make([][]byte, 0, len(entries))

	for i := range entries {
		e := &entries[i]
		txIndex := uint(i)

		// Insert synthetic EVM transaction.
		evmTx := &EvmTransactionDao{
			TxHash:      e.TxHash,
			FromAddress: e.FromAddress,
			ToAddress:   e.ContractAddress,
			Nonce:       e.Nonce,
			Input:       e.Input,
			ValueWei:    "0",
			Status:      1,
			BlockNumber: state.LatestBlock,
			BlockHash:   blockHash,
			TxIndex:     txIndex,
			GasUsed:     gasLimit,
		}
		if _, err = tx.NewInsert().
			Model(evmTx).
			On("CONFLICT (tx_hash) DO NOTHING").
			Exec(ctx); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("insert evm transaction: %w", err)
		}

		// Build ERC-20 Transfer log topics and data.
		fromAddr := common.HexToAddress(e.FromAddress)
		toAddr := common.HexToAddress(e.RecipientAddress)
		fromTopic := common.BytesToHash(common.LeftPadBytes(fromAddr.Bytes(), 32))
		toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), 32))
		amount := new(big.Int).SetBytes(e.AmountData)
		amountData := common.LeftPadBytes(amount.Bytes(), 32)
		contractAddr := common.HexToAddress(e.ContractAddress)

		evmLog := &EvmLogDao{
			TxHash:      e.TxHash,
			LogIndex:    0,
			Address:     contractAddr.Bytes(),
			Topic0:      bytesPtr(transferEventTopic.Bytes()),
			Topic1:      bytesPtr(fromTopic.Bytes()),
			Topic2:      bytesPtr(toTopic.Bytes()),
			Data:        bytesPtr(amountData),
			BlockNumber: state.LatestBlock,
			BlockHash:   blockHash,
			TxIndex:     txIndex,
			Removed:     false,
		}
		if _, err = tx.NewInsert().
			Model(evmLog).
			On("CONFLICT (tx_hash, log_index) DO NOTHING").
			Exec(ctx); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("insert evm log: %w", err)
		}

		txHashes = append(txHashes, e.TxHash)
	}

	// Mark all as mined.
	if _, err = tx.NewUpdate().
		TableExpr("mempool").
		Set("status = ?", string(ethrpc.MempoolMined)).
		Set("updated_at = current_timestamp").
		Where("tx_hash IN (?)", bun.In(txHashes)).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("mark mempool entries mined: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit mining block: %w", err)
	}

	return len(entries), nil
}
