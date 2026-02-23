package dao

import "time"

// ChainStateDao is a data access object that maps directly to the 'chain_state' table in PostgreSQL.
type ChainStateDao struct {
	tableName     struct{}  `bun:"table:chain_state"` // nolint
	ChainID       string    `json:"chain_id" bun:",pk,type:varchar(100)"`
	LastBlock     int64     `json:"last_block" bun:",notnull"`
	LastBlockHash string    `json:"last_block_hash" bun:",notnull,type:varchar(255)"`
	UpdatedAt     time.Time `json:"updated_at" bun:",notnull,nullzero,default:current_timestamp"`
}
