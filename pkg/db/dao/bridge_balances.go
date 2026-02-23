package dao

import "time"

// BridgeBalanceDao is a data access object that maps directly to the 'bridge_balances' table in PostgreSQL.
type BridgeBalanceDao struct {
	tableName    struct{}  `pg:"bridge_balances"` // nolint
	ChainID      string    `json:"chain_id" pg:",pk,type:VARCHAR(32)"`
	TokenAddress string    `json:"token_address" pg:",pk,type:VARCHAR(42)"`
	Balance      string    `json:"balance" pg:",notnull,type:NUMERIC(38,18)"`
	UpdatedAt    time.Time `json:"updated_at" pg:"default:now()"`
}
