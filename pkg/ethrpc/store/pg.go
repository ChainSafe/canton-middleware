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
