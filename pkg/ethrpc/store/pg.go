// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/uptrace/bun"
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
// The block number is only persisted to evm_state inside Finalize, so a
// rolled-back transaction does not consume a number and creates no gaps.
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

// ClaimMempoolEntries atomically claims up to maxTxsPerBlock terminal mempool
// entries (status = completed or failed) and flips them to mined within the
// block's transaction. The limit caps block size to prevent excessively large
// blocks after traffic spikes or Canton downtime.
//
// The claim is performed as SELECT … FOR UPDATE followed by an UPDATE by ID so
// the returned entries retain their *pre-mined* status (completed vs failed).
// The miner relies on that distinction to synthesize EVM tx status=1 (success)
// vs status=0 (failure) and to skip the Transfer event log for failures.
//
// Because the transaction already holds the evm_state row lock, concurrent
// miners are serialized: by the time a second miner acquires the lock, the
// first miner's commit has already flipped these rows to mined.
func (b *pendingBlock) ClaimMempoolEntries(ctx context.Context, maxTxsPerBlock int) ([]ethrpc.MempoolEntry, error) {
	var daos []MempoolEntryDao
	if err := b.tx.NewSelect().
		Model(&daos).
		Where("status IN (?)", bun.List([]string{
			string(ethrpc.MempoolCompleted),
			string(ethrpc.MempoolFailed),
		})).
		OrderExpr("id ASC").
		Limit(maxTxsPerBlock).
		For("UPDATE").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("select claimable mempool entries: %w", err)
	}
	if len(daos) == 0 {
		return nil, nil
	}

	ids := make([]int64, len(daos))
	for i := range daos {
		ids[i] = daos[i].ID
	}
	if _, err := b.tx.NewUpdate().
		Model((*MempoolEntryDao)(nil)).
		Set("status = ?", string(ethrpc.MempoolMined)).
		Set("updated_at = current_timestamp").
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("mark mempool entries mined: %w", err)
	}

	entries := make([]ethrpc.MempoolEntry, 0, len(daos))
	for i := range daos {
		dao := &daos[i]
		entry := ethrpc.MempoolEntry{
			ID:               dao.ID,
			TxHash:           dao.TxHash,
			FromAddress:      dao.FromAddress,
			ContractAddress:  dao.ContractAddress,
			RecipientAddress: dao.RecipientAddress,
			Nonce:            dao.Nonce,
			Input:            dao.Input,
			AmountData:       dao.AmountData,
			Status:           ethrpc.MempoolStatus(dao.Status),
		}
		if dao.ErrorMessage != nil {
			entry.ErrorMessage = *dao.ErrorMessage
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

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

// CompleteMempoolEntry transitions a mempool entry from pending → completed after
// a successful Canton transfer.  Only entries with status=pending are affected;
// entries already completed, failed, or mined are left untouched.
func (s *PGStore) CompleteMempoolEntry(ctx context.Context, txHash []byte) error {
	_, err := s.db.NewUpdate().
		Model((*MempoolEntryDao)(nil)).
		Set("status = ?", string(ethrpc.MempoolCompleted)).
		Set("updated_at = current_timestamp").
		Where("tx_hash = ?", txHash).
		Where("status = ?", string(ethrpc.MempoolPending)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("complete mempool entry: %w", err)
	}
	return nil
}

// FailMempoolEntry transitions a mempool entry from pending → failed after a
// Canton transfer error, recording the error message for diagnostics.  Only
// entries with status=pending are affected.
func (s *PGStore) FailMempoolEntry(ctx context.Context, txHash []byte, errMsg string) error {
	_, err := s.db.NewUpdate().
		Model((*MempoolEntryDao)(nil)).
		Set("status = ?", string(ethrpc.MempoolFailed)).
		Set("error_message = ?", errMsg).
		Set("updated_at = current_timestamp").
		Where("tx_hash = ?", txHash).
		Where("status = ?", string(ethrpc.MempoolPending)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("fail mempool entry: %w", err)
	}
	return nil
}

// GetMempoolEntriesByStatus returns mempool entries with the given status,
// ordered by insertion ID. limit caps how many rows are returned (limit <= 0
// means no limit). The submitter passes its batch size so a backlog after
// Canton downtime never loads the entire pending queue into memory.
func (s *PGStore) GetMempoolEntriesByStatus(ctx context.Context, status ethrpc.MempoolStatus, limit int) ([]ethrpc.MempoolEntry, error) {
	var daos []MempoolEntryDao
	query := s.db.NewSelect().
		Model(&daos).
		Where("status = ?", string(status)).
		OrderExpr("id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, fmt.Errorf("get mempool entries by status %q: %w", status, err)
	}

	entries := make([]ethrpc.MempoolEntry, 0, len(daos))
	for i := range daos {
		dao := &daos[i]
		entry := ethrpc.MempoolEntry{
			ID:               dao.ID,
			TxHash:           dao.TxHash,
			FromAddress:      dao.FromAddress,
			ContractAddress:  dao.ContractAddress,
			RecipientAddress: dao.RecipientAddress,
			Nonce:            dao.Nonce,
			Input:            dao.Input,
			AmountData:       dao.AmountData,
			Status:           status,
		}
		if dao.ErrorMessage != nil {
			entry.ErrorMessage = *dao.ErrorMessage
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
