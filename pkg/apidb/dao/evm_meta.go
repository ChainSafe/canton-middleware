package dao

// EvmMetaDao is a data access object that maps directly to the 'evm_meta' table in PostgreSQL.
type EvmMetaDao struct {
	tableName struct{} `pg:"evm_meta"` // nolint
	Key       string   `json:"key" pg:",pk"`
	Value     string   `json:"value" pg:",notnull"`
}
