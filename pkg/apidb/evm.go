package apidb

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
)

// EvmTransaction represents a synthetic EVM transaction for MetaMask compatibility
type EvmTransaction struct {
	TxHash       []byte
	FromAddress  string
	ToAddress    string
	Nonce        int64
	Input        []byte
	ValueWei     string
	Status       int16
	BlockNumber  int64
	BlockHash    []byte
	TxIndex      int
	GasUsed      int64
	ErrorMessage string
}

// EvmLog represents a synthetic EVM log entry for MetaMask compatibility
type EvmLog struct {
	TxHash      []byte
	LogIndex    int
	Address     []byte   // Contract address (20 bytes)
	Topics      [][]byte // Topic hashes (each 32 bytes)
	Data        []byte
	BlockNumber int64
	BlockHash   []byte
	TxIndex     int
	Removed     bool
}

// SaveEvmTransaction stores a synthetic EVM transaction
func (s *Store) SaveEvmTransaction(tx *EvmTransaction) error {
	query := `
		INSERT INTO evm_transactions (
			tx_hash, from_address, to_address, nonce, input, value_wei,
			status, block_number, block_hash, tx_index, gas_used, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (tx_hash) DO NOTHING
	`
	_, err := s.db.Exec(query,
		tx.TxHash,
		tx.FromAddress,
		tx.ToAddress,
		tx.Nonce,
		tx.Input,
		tx.ValueWei,
		tx.Status,
		tx.BlockNumber,
		tx.BlockHash,
		tx.TxIndex,
		tx.GasUsed,
		tx.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to save evm transaction: %w", err)
	}
	return nil
}

// GetEvmTransaction retrieves an EVM transaction by hash
func (s *Store) GetEvmTransaction(txHash []byte) (*EvmTransaction, error) {
	tx := &EvmTransaction{}
	var errorMsg sql.NullString

	query := `
		SELECT tx_hash, from_address, to_address, nonce, input, value_wei,
		       status, block_number, block_hash, tx_index, gas_used, error_message
		FROM evm_transactions
		WHERE tx_hash = $1
	`
	err := s.db.QueryRow(query, txHash).Scan(
		&tx.TxHash,
		&tx.FromAddress,
		&tx.ToAddress,
		&tx.Nonce,
		&tx.Input,
		&tx.ValueWei,
		&tx.Status,
		&tx.BlockNumber,
		&tx.BlockHash,
		&tx.TxIndex,
		&tx.GasUsed,
		&errorMsg,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get evm transaction: %w", err)
	}
	if errorMsg.Valid {
		tx.ErrorMessage = errorMsg.String
	}
	return tx, nil
}

// GetLatestEvmBlockNumber returns the current latest block number
func (s *Store) GetLatestEvmBlockNumber() (uint64, error) {
	var value string
	query := `SELECT value FROM evm_meta WHERE key = 'latest_block_number'`
	err := s.db.QueryRow(query).Scan(&value)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block number: %w", err)
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block number: %w", err)
	}
	return n, nil
}

// NextEvmBlock allocates the next block number and returns block metadata.
// Returns: blockNumber, blockHash, txIndex
func (s *Store) NextEvmBlock(chainID uint64) (uint64, []byte, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var currentValue string
	query := `SELECT value FROM evm_meta WHERE key = 'latest_block_number' FOR UPDATE`
	err = tx.QueryRow(query).Scan(&currentValue)
	if err != nil && err != sql.ErrNoRows {
		return 0, nil, 0, fmt.Errorf("failed to get current block number: %w", err)
	}

	currentBlock, _ := strconv.ParseUint(currentValue, 10, 64)
	nextBlock := currentBlock + 1

	updateQuery := `
		INSERT INTO evm_meta (key, value) VALUES ('latest_block_number', $1)
		ON CONFLICT (key) DO UPDATE SET value = $1
	`
	_, err = tx.Exec(updateQuery, strconv.FormatUint(nextBlock, 10))
	if err != nil {
		return 0, nil, 0, fmt.Errorf("failed to update block number: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, 0, fmt.Errorf("failed to commit: %w", err)
	}

	blockHash := ethereum.ComputeBlockHash(chainID, nextBlock)
	return nextBlock, blockHash, 0, nil
}

// GetEvmTransactionCount returns the next nonce for an address (max nonce + 1)
func (s *Store) GetEvmTransactionCount(fromAddress string) (uint64, error) {
	var maxNonce sql.NullInt64
	query := `SELECT MAX(nonce) FROM evm_transactions WHERE from_address = $1`
	err := s.db.QueryRow(query, fromAddress).Scan(&maxNonce)
	if err != nil {
		return 0, fmt.Errorf("failed to get transaction count: %w", err)
	}
	if !maxNonce.Valid {
		return 0, nil // No transactions yet
	}
	return uint64(maxNonce.Int64 + 1), nil
}

// SaveEvmLog stores a synthetic EVM log entry
func (s *Store) SaveEvmLog(log *EvmLog) error {
	query := `
		INSERT INTO evm_logs (
			tx_hash, log_index, address, topic0, topic1, topic2, topic3,
			data, block_number, block_hash, tx_index, removed
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (tx_hash, log_index) DO NOTHING
	`
	var topic0, topic1, topic2, topic3 []byte
	if len(log.Topics) > 0 {
		topic0 = log.Topics[0]
	}
	if len(log.Topics) > 1 {
		topic1 = log.Topics[1]
	}
	if len(log.Topics) > 2 {
		topic2 = log.Topics[2]
	}
	if len(log.Topics) > 3 {
		topic3 = log.Topics[3]
	}
	_, err := s.db.Exec(query,
		log.TxHash,
		log.LogIndex,
		log.Address,
		topic0,
		topic1,
		topic2,
		topic3,
		log.Data,
		log.BlockNumber,
		log.BlockHash,
		log.TxIndex,
		log.Removed,
	)
	if err != nil {
		return fmt.Errorf("failed to save evm log: %w", err)
	}
	return nil
}

// GetEvmLogsByTxHash retrieves all logs for a transaction
func (s *Store) GetEvmLogsByTxHash(txHash []byte) ([]*EvmLog, error) {
	query := `
		SELECT tx_hash, log_index, address, topic0, topic1, topic2, topic3,
		       data, block_number, block_hash, tx_index, removed
		FROM evm_logs
		WHERE tx_hash = $1
		ORDER BY log_index
	`
	rows, err := s.db.Query(query, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to query evm logs: %w", err)
	}
	defer rows.Close()

	var logs []*EvmLog
	for rows.Next() {
		log := &EvmLog{}
		var topic0, topic1, topic2, topic3 []byte
		err := rows.Scan(
			&log.TxHash,
			&log.LogIndex,
			&log.Address,
			&topic0,
			&topic1,
			&topic2,
			&topic3,
			&log.Data,
			&log.BlockNumber,
			&log.BlockHash,
			&log.TxIndex,
			&log.Removed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan evm log: %w", err)
		}
		// Check len() instead of nil because empty []byte{} is not nil
		if len(topic0) > 0 {
			log.Topics = append(log.Topics, topic0)
		}
		if len(topic1) > 0 {
			log.Topics = append(log.Topics, topic1)
		}
		if len(topic2) > 0 {
			log.Topics = append(log.Topics, topic2)
		}
		if len(topic3) > 0 {
			log.Topics = append(log.Topics, topic3)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// GetEvmLogs retrieves logs matching filter criteria
func (s *Store) GetEvmLogs(address []byte, topic0 []byte, fromBlock, toBlock int64) ([]*EvmLog, error) {
	query := `
		SELECT tx_hash, log_index, address, topic0, topic1, topic2, topic3,
		       data, block_number, block_hash, tx_index, removed
		FROM evm_logs
		WHERE ($1::bytea IS NULL OR address = $1)
		  AND ($2::bytea IS NULL OR topic0 = $2)
		  AND block_number >= $3
		  AND block_number <= $4
		ORDER BY block_number, tx_index, log_index
		LIMIT 10000
	`
	rows, err := s.db.Query(query, address, topic0, fromBlock, toBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to query evm logs: %w", err)
	}
	defer rows.Close()

	var logs []*EvmLog
	for rows.Next() {
		log := &EvmLog{}
		var topic0, topic1, topic2, topic3 []byte
		err := rows.Scan(
			&log.TxHash,
			&log.LogIndex,
			&log.Address,
			&topic0,
			&topic1,
			&topic2,
			&topic3,
			&log.Data,
			&log.BlockNumber,
			&log.BlockHash,
			&log.TxIndex,
			&log.Removed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan evm log: %w", err)
		}
		// Check len() instead of nil because empty []byte{} is not nil
		if len(topic0) > 0 {
			log.Topics = append(log.Topics, topic0)
		}
		if len(topic1) > 0 {
			log.Topics = append(log.Topics, topic1)
		}
		if len(topic2) > 0 {
			log.Topics = append(log.Topics, topic2)
		}
		if len(topic3) > 0 {
			log.Topics = append(log.Topics, topic3)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
