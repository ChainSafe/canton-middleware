package dao

import (
	"time"

	"github.com/uptrace/bun"
)

// EvmTransactionDao is a data access object that maps directly to the 'evm_transactions' table in PostgreSQL.
type EvmTransactionDao struct {
	bun.BaseModel `bun:"table:evm_transactions"`
	TxHash        []byte    `json:"tx_hash" bun:",pk,type:bytea"`
	FromAddress   string    `json:"from_address" bun:",notnull,type:text"`
	ToAddress     string    `json:"to_address" bun:",notnull,type:text"`
	Nonce         int64     `json:"nonce" bun:",notnull"`
	Input         []byte    `json:"input" bun:",notnull,type:bytea"`
	ValueWei      string    `json:"value_wei" bun:",notnull,default:'0',type:text"`
	Status        int16     `json:"status" bun:",notnull,default:1,type:smallint"`
	BlockNumber   int64     `json:"block_number" bun:",notnull"`
	BlockHash     []byte    `json:"block_hash" bun:",notnull,type:bytea"`
	TxIndex       int       `json:"tx_index" bun:",notnull,default:0"`
	GasUsed       int64     `json:"gas_used" bun:",notnull,default:21000"`
	ErrorMessage  *string   `json:"error_message,omitempty" bun:",type:text"`
	CreatedAt     time.Time `json:"created_at" bun:",notnull,default:current_timestamp"`
}
