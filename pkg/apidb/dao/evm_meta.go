package dao

import "github.com/uptrace/bun"

// EvmMetaDao is a data access object that maps directly to the 'evm_meta' table in PostgreSQL.
type EvmMetaDao struct {
	bun.BaseModel `bun:"table:evm_meta"`
	Key           string `json:"key" bun:",pk,type:text"`
	Value         string `json:"value" bun:",notnull,type:text"`
}
