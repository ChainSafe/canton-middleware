package dao

import (
	"time"

	"github.com/uptrace/bun"
)

// ChainStateDao is a data access object that maps directly to the 'chain_state' table in PostgreSQL.
type ChainStateDao struct {
	bun.BaseModel `bun:"table:chain_state"`
	ChainID       string    `json:"chain_id" bun:",pk,type:varchar(100)"`
	LastBlock     int64     `json:"last_block" bun:",notnull"`
	LastBlockHash string    `json:"last_block_hash" bun:",notnull,type:varchar(255)"`
	UpdatedAt     time.Time `json:"updated_at" bun:",notnull,default:current_timestamp"`
}
