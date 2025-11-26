package db

import (
	"time"
)

// TransferStatus represents the current state of a cross-chain transfer
type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusConfirmed TransferStatus = "confirmed"
	TransferStatusProcessed TransferStatus = "processed"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusFailed    TransferStatus = "failed"
)

// TransferDirection indicates the direction of the transfer
type TransferDirection string

const (
	DirectionCantonToEthereum TransferDirection = "canton_to_ethereum"
	DirectionEthereumToCanton TransferDirection = "ethereum_to_canton"
)

// Transfer represents a cross-chain token transfer
type Transfer struct {
	ID                string            `db:"id"`
	Direction         TransferDirection `db:"direction"`
	Status            TransferStatus    `db:"status"`
	SourceChain       string            `db:"source_chain"`
	DestinationChain  string            `db:"destination_chain"`
	SourceTxHash      string            `db:"source_tx_hash"`
	DestinationTxHash *string           `db:"destination_tx_hash"`
	TokenAddress      string            `db:"token_address"`
	Amount            string            `db:"amount"`
	Sender            string            `db:"sender"`
	Recipient         string            `db:"recipient"`
	Nonce             int64             `db:"nonce"`
	SourceBlockNumber int64             `db:"source_block_number"`
	ConfirmationCount int               `db:"confirmation_count"`
	CreatedAt         time.Time         `db:"created_at"`
	UpdatedAt         time.Time         `db:"updated_at"`
	CompletedAt       *time.Time        `db:"completed_at"`
	ErrorMessage      *string           `db:"error_message"`
	RetryCount        int               `db:"retry_count"`
}

// ChainState tracks the last processed block for each chain
type ChainState struct {
	ChainID       string    `db:"chain_id"`
	LastBlock     int64     `db:"last_block"`
	LastBlockHash string    `db:"last_block_hash"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// Nonce tracks nonces for transaction submission on each chain
type NonceState struct {
	ChainID   string    `db:"chain_id"`
	Address   string    `db:"address"`
	Nonce     int64     `db:"nonce"`
	UpdatedAt time.Time `db:"updated_at"`
}

// BridgeBalance tracks token balances locked in bridge contracts
type BridgeBalance struct {
	ChainID      string    `db:"chain_id"`
	TokenAddress string    `db:"token_address"`
	Balance      string    `db:"balance"`
	UpdatedAt    time.Time `db:"updated_at"`
}
