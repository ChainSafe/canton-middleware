package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/uptrace/bun"
)

const latestBlockNumberKey = "latest_block_number"
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
func (s *PGStore) SaveEvmTransaction(tx *ethrpc.EvmTransaction) error {
	if tx == nil {
		return fmt.Errorf("evm transaction is required")
	}

	dao := toEvmTransactionDao(tx)

	_, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash) DO NOTHING").
		Exec(context.Background())
	if err != nil {
		return fmt.Errorf("save evm transaction: %w", err)
	}
	return nil
}

// GetEvmTransaction retrieves an EVM transaction by hash.
func (s *PGStore) GetEvmTransaction(txHash []byte) (*ethrpc.EvmTransaction, error) {
	dao := new(EvmTransactionDao)
	err := s.db.NewSelect().
		Model(dao).
		Where("tx_hash = ?", txHash).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get evm transaction: %w", err)
	}

	return fromEvmTransactionDao(dao), nil
}

// GetLatestEvmBlockNumber returns the latest synthetic EVM block number.
func (s *PGStore) GetLatestEvmBlockNumber() (uint64, error) {
	meta := new(EvmMetaDao)
	err := s.db.NewSelect().
		Model(meta).
		Column("value").
		Where(`"key" = ?`, latestBlockNumberKey).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get latest block number: %w", err)
	}

	n, err := strconv.ParseUint(meta.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse latest block number %q: %w", meta.Value, err)
	}
	return n, nil
}

// NextEvmBlock allocates the next synthetic EVM block number.
// Returns block number, block hash, and tx index (always 0).
func (s *PGStore) NextEvmBlock(chainID uint64) (uint64, []byte, int, error) {
	ctx := context.Background()

	var nextBlock uint64
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		meta := new(EvmMetaDao)
		err := tx.NewSelect().
			Model(meta).
			Where(`"key" = ?`, latestBlockNumberKey).
			For("UPDATE").
			Limit(1).
			Scan(ctx)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("select current block number: %w", err)
			}
			meta.Value = "0"
		}

		currentBlock, parseErr := strconv.ParseUint(meta.Value, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("parse current block number %q: %w", meta.Value, parseErr)
		}
		nextBlock = currentBlock + 1

		_, err = tx.NewInsert().
			Model(&EvmMetaDao{Key: latestBlockNumberKey, Value: strconv.FormatUint(nextBlock, 10)}).
			On("CONFLICT (key) DO UPDATE").
			Set("value = EXCLUDED.value").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("upsert latest block number: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, nil, 0, fmt.Errorf("allocate next evm block: %w", err)
	}

	blockHash := ethereum.ComputeBlockHash(chainID, nextBlock)
	return nextBlock, blockHash, 0, nil
}

// GetBlockNumberByHash returns the block number for a block hash.
func (s *PGStore) GetBlockNumberByHash(blockHash []byte) (uint64, error) {
	dao := new(EvmTransactionDao)
	err := s.db.NewSelect().
		Model(dao).
		Column("block_number").
		Where("block_hash = ?", blockHash).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get block number by hash: %w", err)
	}
	if dao.BlockNumber < 0 {
		return 0, fmt.Errorf("invalid negative block number: %d", dao.BlockNumber)
	}
	return uint64(dao.BlockNumber), nil
}

// GetEvmTransactionCount returns the next nonce for the from address.
func (s *PGStore) GetEvmTransactionCount(fromAddress string) (uint64, error) {
	var maxNonce sql.NullInt64
	err := s.db.NewSelect().
		Model((*EvmTransactionDao)(nil)).
		ColumnExpr("MAX(nonce)").
		Where("from_address = ?", fromAddress).
		Scan(context.Background(), &maxNonce)
	if err != nil {
		return 0, fmt.Errorf("get transaction count for %s: %w", fromAddress, err)
	}
	if !maxNonce.Valid {
		return 0, nil
	}
	if maxNonce.Int64 < 0 {
		return 0, fmt.Errorf("invalid nonce for %s: %d", fromAddress, maxNonce.Int64)
	}
	nonce, convErr := strconv.ParseUint(strconv.FormatInt(maxNonce.Int64, 10), 10, 64)
	if convErr != nil {
		return 0, fmt.Errorf("parse max nonce for %s: %w", fromAddress, convErr)
	}
	return nonce + 1, nil
}

// SaveEvmLog stores a synthetic EVM log entry.
func (s *PGStore) SaveEvmLog(log *ethrpc.EvmLog) error {
	if log == nil {
		return fmt.Errorf("evm log is required")
	}

	dao := toEvmLogDao(log)

	_, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (tx_hash, log_index) DO NOTHING").
		Exec(context.Background())
	if err != nil {
		return fmt.Errorf("save evm log: %w", err)
	}
	return nil
}

// GetEvmLogsByTxHash retrieves all logs for a transaction hash.
func (s *PGStore) GetEvmLogsByTxHash(txHash []byte) ([]*ethrpc.EvmLog, error) {
	var daos []EvmLogDao
	err := s.db.NewSelect().
		Model(&daos).
		Where("tx_hash = ?", txHash).
		OrderExpr("log_index ASC").
		Scan(context.Background())
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
func (s *PGStore) GetEvmLogs(address []byte, topic0 []byte, fromBlock, toBlock int64) ([]*ethrpc.EvmLog, error) {
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

	err := query.Scan(context.Background())
	if err != nil {
		return nil, fmt.Errorf("get evm logs: %w", err)
	}

	logs := make([]*ethrpc.EvmLog, 0, len(daos))
	for i := range daos {
		logs = append(logs, fromEvmLogDao(&daos[i]))
	}
	return logs, nil
}
