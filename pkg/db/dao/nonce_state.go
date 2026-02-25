package dao

import (
	"time"

	"github.com/uptrace/bun"
)

// NonceStateDao is a data access object that maps directly to the 'nonce_state' table in PostgreSQL.
type NonceStateDao struct {
	bun.BaseModel `bun:"table:nonce_state"`
	ChainID       string    `json:"chain_id" bun:",pk,type:varchar(100)"`
	Address       string    `json:"address" bun:",pk,type:varchar(255)"`
	Nonce         int64     `json:"nonce" bun:",notnull"`
	UpdatedAt     time.Time `json:"updated_at" bun:",notnull,default:current_timestamp"`
}
