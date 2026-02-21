package dao

import "time"

// ChainStateDao is a data access object that maps directly to the 'chain_state' table in PostgreSQL.
type ChainStateDao struct {
	tableName     struct{}  `pg:"chain_state"` // nolint
	ChainID       string    `json:"chain_id" pg:",pk"`
	LastBlock     int64     `json:"last_block" pg:",notnull"`
	LastBlockHash string    `json:"last_block_hash" pg:",notnull"`
	UpdatedAt     time.Time `json:"updated_at" pg:"default:now()"`
}
