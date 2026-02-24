package dao

import "time"

// TransferDao is a data access object that maps directly to the 'transfers' table in PostgreSQL.
type TransferDao struct {
	tableName         struct{}   `bun:"table:transfers"` // nolint
	ID                string     `json:"id" bun:",pk,type:varchar(255)"`
	Direction         string     `json:"direction" bun:",notnull,type:varchar(50)"`
	Status            string     `json:"status" bun:",notnull,type:varchar(50)"`
	SourceChain       string     `json:"source_chain" bun:",notnull,type:varchar(100)"`
	DestinationChain  string     `json:"destination_chain" bun:",notnull,type:varchar(100)"`
	SourceTxHash      string     `json:"source_tx_hash" bun:",notnull,type:varchar(255)"`
	DestinationTxHash *string    `json:"destination_tx_hash,omitempty" bun:",type:varchar(255)"`
	TokenAddress      string     `json:"token_address" bun:",notnull,type:varchar(255)"`
	Amount            string     `json:"amount" bun:",notnull,type:varchar(255)"`
	Sender            string     `json:"sender" bun:",notnull,type:varchar(255)"`
	Recipient         string     `json:"recipient" bun:",notnull,type:varchar(255)"`
	Nonce             int64      `json:"nonce" bun:",notnull"`
	SourceBlockNumber int64      `json:"source_block_number" bun:",notnull"`
	ConfirmationCount int        `json:"confirmation_count" bun:",notnull,default:0"`
	CreatedAt         time.Time  `json:"created_at" bun:",notnull,default:current_timestamp"`
	UpdatedAt         time.Time  `json:"updated_at" bun:",notnull,default:current_timestamp"`
	CompletedAt       *time.Time `json:"completed_at,omitempty" bun:"completed_at"`
	ErrorMessage      *string    `json:"error_message,omitempty" bun:",type:text"`
	RetryCount        int        `json:"retry_count" bun:",notnull,default:0"`
}
