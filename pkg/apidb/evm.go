package apidb

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"strconv"
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

	blockHash := computeBlockHash(chainID, nextBlock)
	return nextBlock, blockHash, 0, nil
}

// computeBlockHash generates a deterministic block hash from chainID and block number
func computeBlockHash(chainID, blockNumber uint64) []byte {
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data[0:8], chainID)
	binary.BigEndian.PutUint64(data[8:16], blockNumber)
	hash := sha256.Sum256(data)
	return hash[:]
}

// GetEvmTransactionCount returns the nonce for an address (count of transactions sent)
func (s *Store) GetEvmTransactionCount(fromAddress string) (uint64, error) {
	var count uint64
	query := `SELECT COUNT(*) FROM evm_transactions WHERE from_address = $1`
	err := s.db.QueryRow(query, fromAddress).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get transaction count: %w", err)
	}
	return count, nil
}
