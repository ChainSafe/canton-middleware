package dao

import "time"

// NonceStateDao is a data access object that maps directly to the 'nonce_state' table in PostgreSQL.
type NonceStateDao struct {
	tableName struct{}  `pg:"nonce_state"` // nolint
	ChainID   string    `json:"chain_id" pg:",pk"`
	Address   string    `json:"address" pg:",pk"`
	Nonce     int64     `json:"nonce" pg:",notnull"`
	UpdatedAt time.Time `json:"updated_at" pg:"default:now()"`
}
