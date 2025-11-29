package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Store provides database operations for the bridge
type Store struct {
	db *sql.DB
}

// NewStore creates a new database store
func NewStore(connString string) (*Store, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateTransfer creates a new transfer record
func (s *Store) CreateTransfer(transfer *Transfer) error {
	query := `
		INSERT INTO transfers (
			id, direction, status, source_chain, destination_chain,
			source_tx_hash, token_address, amount, sender, recipient,
			nonce, source_block_number, confirmation_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := s.db.Exec(query,
		transfer.ID, transfer.Direction, transfer.Status,
		transfer.SourceChain, transfer.DestinationChain,
		transfer.SourceTxHash, transfer.TokenAddress, transfer.Amount,
		transfer.Sender, transfer.Recipient, transfer.Nonce,
		transfer.SourceBlockNumber, transfer.ConfirmationCount,
	)
	return err
}

// GetTransfer retrieves a transfer by ID
func (s *Store) GetTransfer(id string) (*Transfer, error) {
	transfer := &Transfer{}
	query := `
		SELECT id, direction, status, source_chain, destination_chain,
			source_tx_hash, destination_tx_hash, token_address, amount,
			sender, recipient, nonce, source_block_number, confirmation_count,
			created_at, updated_at, completed_at, error_message, retry_count
		FROM transfers WHERE id = $1
	`
	err := s.db.QueryRow(query, id).Scan(
		&transfer.ID, &transfer.Direction, &transfer.Status,
		&transfer.SourceChain, &transfer.DestinationChain,
		&transfer.SourceTxHash, &transfer.DestinationTxHash,
		&transfer.TokenAddress, &transfer.Amount, &transfer.Sender,
		&transfer.Recipient, &transfer.Nonce, &transfer.SourceBlockNumber,
		&transfer.ConfirmationCount, &transfer.CreatedAt, &transfer.UpdatedAt,
		&transfer.CompletedAt, &transfer.ErrorMessage, &transfer.RetryCount,
	)
	if err != nil {
		return nil, err
	}
	return transfer, nil
}

// UpdateTransferStatus updates the status of a transfer
func (s *Store) UpdateTransferStatus(id string, status TransferStatus, destTxHash *string) error {
	query := `
		UPDATE transfers 
		SET status = $1, destination_tx_hash = $2, updated_at = $3
		WHERE id = $4
	`
	_, err := s.db.Exec(query, status, destTxHash, time.Now(), id)
	return err
}

// GetPendingTransfers retrieves all pending transfers for a direction
func (s *Store) GetPendingTransfers(direction TransferDirection) ([]*Transfer, error) {
	query := `
		SELECT id, direction, status, source_chain, destination_chain,
			source_tx_hash, destination_tx_hash, token_address, amount,
			sender, recipient, nonce, source_block_number, confirmation_count,
			created_at, updated_at, completed_at, error_message, retry_count
		FROM transfers 
		WHERE direction = $1 AND status IN ('pending', 'confirmed', 'processed')
		ORDER BY created_at ASC
	`
	rows, err := s.db.Query(query, direction)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*Transfer
	for rows.Next() {
		transfer := &Transfer{}
		err := rows.Scan(
			&transfer.ID, &transfer.Direction, &transfer.Status,
			&transfer.SourceChain, &transfer.DestinationChain,
			&transfer.SourceTxHash, &transfer.DestinationTxHash,
			&transfer.TokenAddress, &transfer.Amount, &transfer.Sender,
			&transfer.Recipient, &transfer.Nonce, &transfer.SourceBlockNumber,
			&transfer.ConfirmationCount, &transfer.CreatedAt, &transfer.UpdatedAt,
			&transfer.CompletedAt, &transfer.ErrorMessage, &transfer.RetryCount,
		)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, transfer)
	}
	return transfers, rows.Err()
}

// ListTransfers retrieves the most recent transfers
func (s *Store) ListTransfers(limit int) ([]*Transfer, error) {
	query := `
		SELECT id, direction, status, source_chain, destination_chain,
			source_tx_hash, destination_tx_hash, token_address, amount,
			sender, recipient, nonce, source_block_number, confirmation_count,
			created_at, updated_at, completed_at, error_message, retry_count
		FROM transfers 
		ORDER BY created_at DESC
		LIMIT $1
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*Transfer
	for rows.Next() {
		transfer := &Transfer{}
		err := rows.Scan(
			&transfer.ID, &transfer.Direction, &transfer.Status,
			&transfer.SourceChain, &transfer.DestinationChain,
			&transfer.SourceTxHash, &transfer.DestinationTxHash,
			&transfer.TokenAddress, &transfer.Amount, &transfer.Sender,
			&transfer.Recipient, &transfer.Nonce, &transfer.SourceBlockNumber,
			&transfer.ConfirmationCount, &transfer.CreatedAt, &transfer.UpdatedAt,
			&transfer.CompletedAt, &transfer.ErrorMessage, &transfer.RetryCount,
		)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, transfer)
	}
	return transfers, rows.Err()
}

// SetChainState updates the last processed block for a chain
func (s *Store) SetChainState(chainID string, blockNumber int64, blockHash string) error {
	query := `
		INSERT INTO chain_state (chain_id, last_block, last_block_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (chain_id) 
		DO UPDATE SET last_block = $2, last_block_hash = $3, updated_at = NOW()
	`
	_, err := s.db.Exec(query, chainID, blockNumber, blockHash)
	return err
}

// GetChainState retrieves the last processed block for a chain
func (s *Store) GetChainState(chainID string) (*ChainState, error) {
	state := &ChainState{}
	query := `SELECT chain_id, last_block, last_block_hash, updated_at FROM chain_state WHERE chain_id = $1`
	err := s.db.QueryRow(query, chainID).Scan(&state.ChainID, &state.LastBlock, &state.LastBlockHash, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return state, nil
}

// GetNonce retrieves the current nonce for an address on a chain
func (s *Store) GetNonce(chainID, address string) (int64, error) {
	var nonce int64
	query := `SELECT nonce FROM nonce_state WHERE chain_id = $1 AND address = $2`
	err := s.db.QueryRow(query, chainID, address).Scan(&nonce)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return nonce, err
}

// IncrementNonce increments and returns the next nonce for an address on a chain
func (s *Store) IncrementNonce(chainID, address string) (int64, error) {
	query := `
		INSERT INTO nonce_state (chain_id, address, nonce)
		VALUES ($1, $2, 1)
		ON CONFLICT (chain_id, address)
		DO UPDATE SET nonce = nonce_state.nonce + 1, updated_at = NOW()
		RETURNING nonce
	`
	var nonce int64
	err := s.db.QueryRow(query, chainID, address).Scan(&nonce)
	return nonce, err
}
