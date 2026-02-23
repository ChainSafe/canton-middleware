package dao

import "time"

// NonceStateDao is a data access object that maps directly to the 'nonce_state' table in PostgreSQL.
type NonceStateDao struct {
	tableName struct{}  `bun:"table:nonce_state"` // nolint
	ChainID   string    `json:"chain_id" bun:",pk,type:varchar(100)"`
	Address   string    `json:"address" bun:",pk,type:varchar(255)"`
	Nonce     int64     `json:"nonce" bun:",notnull"`
	UpdatedAt time.Time `json:"updated_at" bun:",notnull,nullzero,default:current_timestamp"`
}
