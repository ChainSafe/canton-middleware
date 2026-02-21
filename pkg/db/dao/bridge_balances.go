package dao

import "time"

// BridgeBalanceDao is a data access object that maps directly to the 'bridge_balances' table in PostgreSQL.
type BridgeBalanceDao struct {
	tableName    struct{}  `pg:"bridge_balances"` // nolint
	ChainID      string    `json:"chain_id" pg:",pk"`
	TokenAddress string    `json:"token_address" pg:",pk"`
	Balance      string    `json:"balance" pg:",notnull"`
	UpdatedAt    time.Time `json:"updated_at" pg:"default:now()"`
}
