package dao

import "time"

// TransferDao is a data access object that maps directly to the 'transfers' table in PostgreSQL.
type TransferDao struct {
	tableName         struct{}   `pg:"transfers"` // nolint
	ID                string     `json:"id" pg:",pk,type:VARCHAR(128)"`
	Direction         string     `json:"direction" pg:",notnull,type:VARCHAR(20)"`
	Status            string     `json:"status" pg:",notnull,type:VARCHAR(20)"`
	SourceChain       string     `json:"source_chain" pg:",notnull,type:VARCHAR(32)"`
	DestinationChain  string     `json:"destination_chain" pg:",notnull,type:VARCHAR(32)"`
	SourceTxHash      string     `json:"source_tx_hash" pg:",notnull,type:VARCHAR(66)"`
	DestinationTxHash *string    `json:"destination_tx_hash,omitempty" pg:"destination_tx_hash,type:VARCHAR(66)"`
	TokenAddress      string     `json:"token_address" pg:",notnull,type:VARCHAR(42)"`
	Amount            string     `json:"amount" pg:",notnull,type:NUMERIC(38,18)"`
	Sender            string     `json:"sender" pg:",notnull,type:VARCHAR(255)"`
	Recipient         string     `json:"recipient" pg:",notnull,type:VARCHAR(255)"`
	Nonce             int64      `json:"nonce" pg:",notnull"`
	SourceBlockNumber int64      `json:"source_block_number" pg:",notnull"`
	ConfirmationCount int        `json:"confirmation_count" pg:",notnull,use_zero,default:0"`
	CreatedAt         time.Time  `json:"created_at" pg:"default:now()"`
	UpdatedAt         time.Time  `json:"updated_at" pg:"default:now()"`
	CompletedAt       *time.Time `json:"completed_at,omitempty" pg:"completed_at"`
	ErrorMessage      *string    `json:"error_message,omitempty" pg:"error_message,type:TEXT"`
	RetryCount        int        `json:"retry_count" pg:",use_zero"`
}
