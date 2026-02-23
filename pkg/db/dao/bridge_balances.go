package dao

import "time"

// BridgeBalanceDao is a data access object that maps directly to the 'bridge_balances' table in PostgreSQL.
type BridgeBalanceDao struct {
	tableName    struct{}  `bun:"table:bridge_balances"` // nolint
	ChainID      string    `json:"chain_id" bun:",pk,type:varchar(100)"`
	TokenAddress string    `json:"token_address" bun:",pk,type:varchar(255)"`
	Balance      string    `json:"balance" bun:",notnull,type:varchar(255)"`
	UpdatedAt    time.Time `json:"updated_at" bun:",notnull,nullzero,default:current_timestamp"`
}
