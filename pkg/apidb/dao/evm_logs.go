package dao

import "github.com/uptrace/bun"

// EvmLogDao is a data access object that maps directly to the 'evm_logs' table in PostgreSQL.
type EvmLogDao struct {
	bun.BaseModel `bun:"table:evm_logs"`
	TxHash        []byte  `json:"tx_hash" bun:",pk,type:bytea"`
	LogIndex      int     `json:"log_index" bun:",pk"`
	Address       []byte  `json:"address" bun:",notnull,type:bytea"`
	Topic0        *[]byte `json:"topic0,omitempty" bun:",type:bytea"`
	Topic1        *[]byte `json:"topic1,omitempty" bun:",type:bytea"`
	Topic2        *[]byte `json:"topic2,omitempty" bun:",type:bytea"`
	Topic3        *[]byte `json:"topic3,omitempty" bun:",type:bytea"`
	Data          *[]byte `json:"data,omitempty" bun:",type:bytea"`
	BlockNumber   int64   `json:"block_number" bun:",notnull"`
	BlockHash     []byte  `json:"block_hash" bun:",notnull,type:bytea"`
	TxIndex       int     `json:"tx_index" bun:",notnull"`
	Removed       bool    `json:"removed" bun:",notnull"`
}
