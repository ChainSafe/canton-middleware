package dao

// EvmMetaDao is a data access object that maps directly to the 'evm_meta' table in PostgreSQL.
type EvmMetaDao struct {
	tableName struct{} `bun:"table:evm_meta"` // nolint
	Key       string   `json:"key" bun:",pk,type:text"`
	Value     string   `json:"value" bun:",notnull,type:text"`
}
