package dao

// EvmLogDao is a data access object that maps directly to the 'evm_logs' table in PostgreSQL.
type EvmLogDao struct {
	tableName   struct{} `pg:"evm_logs"` // nolint
	TxHash      []byte   `json:"tx_hash" pg:",pk,type:bytea"`
	LogIndex    int      `json:"log_index" pg:",pk"`
	Address     []byte   `json:"address" pg:",notnull,type:bytea"`
	Topic0      *[]byte  `json:"topic0,omitempty" pg:"topic0,type:bytea"`
	Topic1      *[]byte  `json:"topic1,omitempty" pg:"topic1,type:bytea"`
	Topic2      *[]byte  `json:"topic2,omitempty" pg:"topic2,type:bytea"`
	Topic3      *[]byte  `json:"topic3,omitempty" pg:"topic3,type:bytea"`
	Data        *[]byte  `json:"data,omitempty" pg:"data,type:bytea"`
	BlockNumber int64    `json:"block_number" pg:",notnull"`
	BlockHash   []byte   `json:"block_hash" pg:",notnull,type:bytea"`
	TxIndex     int      `json:"tx_index" pg:",notnull,use_zero"`
	Removed     bool     `json:"removed" pg:",notnull,use_zero"`
}
