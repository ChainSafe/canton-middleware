package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

const evmLogsQueryLimit = 10000

// PGStore is a PostgreSQL-backed EVM store for EthRPC.
type PGStore struct {
	db *bun.DB
}

// NewStore creates a new PostgreSQL-backed EVM store.
func NewStore(db *bun.DB) *PGStore {
	return &PGStore{db: db}
}

// NewBlock opens a DB transaction and takes an explicit exclusive row lock on
// the evm_state singleton (SELECT … FOR UPDATE). The lock is held until the
// caller calls Finalize or Abort on the returned PendingBlock, which serializes
// concurrent miner instances at the database level.
// If the transaction is rolled back the block number is skipped (gap) — an
// intentional tradeoff accepted in favour of simplicity.
func (s *PGStore) NewBlock(ctx context.Context, chainID uint64) (ethrpc.PendingBlock, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	// Acquire an exclusive row lock. Concurrent callers block here until the
	// previous miner's transaction commits or rolls back.
	// The singleton row is guaranteed to exist by migration 8_create_evm_state.
	state := new(EvmStateDao)
	if err = tx.NewSelect().
		Model(state).
		Where("id = 1").
		For("UPDATE").
		Scan(ctx); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("lock evm_state: %w", err)
	}

	// Pre-compute the next block number and hash. The counter is written to
	// evm_state only in Finalize, so a rolled-back block never consumes a number.
	nextBlock := state.LatestBlock + 1
	blockHash := ethereum.ComputeBlockHash(chainID, nextBlock)
	return &pendingBlock{
		tx:          tx,
		blockNumber: nextBlock,
		blockHash:   blockHash,
	}, nil
}

// pendingBlock holds an open DB transaction scoped to one synthetic EVM block.
type pendingBlock struct {
	tx          bun.Tx
	blockNumber uint64
	blockHash   []byte
	done        bool // guards against double-commit/rollback
}

func (b *pendingBlock) Number() uint64 { return b.blockNumber }
func (b *pendingBlock) Hash() []byte   { return b.blockHash }

func (b *pendingBlock) AddEvmTransaction(ctx context.Context, tx *ethrpc.EvmTransaction) error {
	dao := toEvmTransactionDao(tx)
	if _, err := b.tx.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash) DO NOTHING").
		Exec(ctx); err != nil {
		return fmt.Errorf("save evm transaction: %w", err)
	}
	return nil
}

func (b *pendingBlock) AddEvmLog(ctx context.Context, log *ethrpc.EvmLog) error {
	dao := toEvmLogDao(log)
	if _, err := b.tx.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash, log_index) DO NOTHING").
		Exec(ctx); err != nil {
		return fmt.Errorf("save evm log: %w", err)
	}
	return nil
}

// MarkMined transitions the given mempool entries to status=mined within the
// same transaction as the block writes, so the update is atomic with the commit.
func (b *pendingBlock) MarkMined(ctx context.Context, txHashes [][]byte) error {
	if len(txHashes) == 0 {
		return nil
	}
	_, err := b.tx.NewUpdate().
		TableExpr("mempool").
		Set("status = ?", string(ethrpc.MempoolMined)).
		Set("updated_at = current_timestamp").
		Where("tx_hash = ANY(?)", pgdialect.Array(txHashes)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark mempool entries mined: %w", err)
	}
	return nil
}

// Finalize increments evm_state.latest_block to the pre-computed block number
// and commits the transaction. Safe to call at most once.
func (b *pendingBlock) Finalize(ctx context.Context) error {
	if b.done {
		return nil
	}
	b.done = true
	if _, err := b.tx.NewUpdate().
		Model(&EvmStateDao{ID: 1, LatestBlock: b.blockNumber}).
		Where("id = 1").
		Exec(ctx); err != nil {
		_ = b.tx.Rollback()
		return fmt.Errorf("update evm_state block number: %w", err)
	}
	return b.tx.Commit()
}

// Abort rolls back the block transaction. No-op after Finalize; safe to defer.
func (b *pendingBlock) Abort(_ context.Context) error {
	if b.done {
		return nil
	}
	b.done = true
	return b.tx.Rollback()
}

// GetLatestEvmBlockNumber returns the latest committed synthetic EVM block number.
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

// GetEvmTransaction retrieves a synthetic EVM transaction by hash.
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

// GetBlockNumberByHash returns the block number for a given block hash.
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

// GetEvmTransactionCount returns the next nonce for the given from-address.
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

// GetEvmLogsByTxHash retrieves all logs for a transaction hash, ordered by log index.
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

	if err := query.Scan(ctx); err != nil {
		return nil, fmt.Errorf("get evm logs: %w", err)
	}
	logs := make([]*ethrpc.EvmLog, 0, len(daos))
	for i := range daos {
		logs = append(logs, fromEvmLogDao(&daos[i]))
	}
	return logs, nil
}

// InsertMempoolEntry records a new transfer intent with status=pending.
// DO NOTHING on tx_hash conflict so duplicate submissions are safe.
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

// UpdateMempoolStatus sets the status (and optional error message) for a mempool entry.
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

// GetMempoolEntriesByStatus returns all mempool entries with the given status,
// ordered by insertion ID.
func (s *PGStore) GetMempoolEntriesByStatus(ctx context.Context, status ethrpc.MempoolStatus) ([]ethrpc.MempoolEntry, error) {
	var daos []MempoolEntryDao
	err := s.db.NewSelect().
		Model(&daos).
		Where("status = ?", string(status)).
		OrderExpr("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get mempool entries by status %q: %w", status, err)
	}

	entries := make([]ethrpc.MempoolEntry, 0, len(daos))
	for _, dao := range daos {
		entries = append(entries, ethrpc.MempoolEntry{
			TxHash:           dao.TxHash,
			FromAddress:      dao.FromAddress,
			ContractAddress:  dao.ContractAddress,
			RecipientAddress: dao.RecipientAddress,
			Nonce:            dao.Nonce,
			Input:            dao.Input,
			AmountData:       dao.AmountData,
			Status:           status,
		})
	}
	return entries, nil
}
