package dao

import "time"

// TransferDao is a data access object that maps directly to the 'transfers' table in PostgreSQL.
type TransferDao struct {
	tableName         struct{}   `pg:"transfers"` // nolint
	ID                string     `json:"id" pg:",pk"`
	Direction         string     `json:"direction" pg:",notnull"`
	Status            string     `json:"status" pg:",notnull"`
	SourceChain       string     `json:"source_chain" pg:",notnull"`
	DestinationChain  string     `json:"destination_chain" pg:",notnull"`
	SourceTxHash      string     `json:"source_tx_hash" pg:",notnull"`
	DestinationTxHash *string    `json:"destination_tx_hash,omitempty" pg:"destination_tx_hash"`
	TokenAddress      string     `json:"token_address" pg:",notnull"`
	Amount            string     `json:"amount" pg:",notnull"`
	Sender            string     `json:"sender" pg:",notnull"`
	Recipient         string     `json:"recipient" pg:",notnull"`
	Nonce             int64      `json:"nonce" pg:",notnull"`
	SourceBlockNumber int64      `json:"source_block_number" pg:",notnull"`
	ConfirmationCount int        `json:"confirmation_count" pg:",notnull,use_zero,default:0"`
	CreatedAt         time.Time  `json:"created_at" pg:"default:now()"`
	UpdatedAt         time.Time  `json:"updated_at" pg:"default:now()"`
	CompletedAt       *time.Time `json:"completed_at,omitempty" pg:"completed_at"`
	ErrorMessage      *string    `json:"error_message,omitempty" pg:"error_message"`
	RetryCount        int        `json:"retry_count" pg:",use_zero"`
}
