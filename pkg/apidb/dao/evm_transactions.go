package dao

import "time"

// EvmTransactionDao is a data access object that maps directly to the 'evm_transactions' table in PostgreSQL.
type EvmTransactionDao struct {
	tableName    struct{}  `pg:"evm_transactions"` // nolint
	TxHash       []byte    `json:"tx_hash" pg:",pk,type:bytea"`
	FromAddress  string    `json:"from_address" pg:",notnull,type:VARCHAR(42)"`
	ToAddress    string    `json:"to_address" pg:",notnull,type:VARCHAR(42)"`
	Nonce        int64     `json:"nonce" pg:",notnull"`
	Input        []byte    `json:"input" pg:",notnull,type:bytea"`
	ValueWei     string    `json:"value_wei" pg:",notnull,use_zero,type:NUMERIC(78,0)"`
	Status       int16     `json:"status" pg:",notnull,default:1"`
	BlockNumber  int64     `json:"block_number" pg:",notnull"`
	BlockHash    []byte    `json:"block_hash" pg:",notnull,type:bytea"`
	TxIndex      int       `json:"tx_index" pg:",notnull,use_zero"`
	GasUsed      int64     `json:"gas_used" pg:",notnull,default:21000"`
	ErrorMessage *string   `json:"error_message,omitempty" pg:"error_message,type:TEXT"`
	CreatedAt    time.Time `json:"created_at" pg:"default:now()"`
}
