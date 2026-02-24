package dao

import "time"

// TokenMetricsDao is a data access object that maps directly to the 'token_metrics' table in PostgreSQL.
type TokenMetricsDao struct {
	tableName        struct{}   `bun:"table:token_metrics"` // nolint
	TokenSymbol      string     `json:"token_symbol" bun:",pk,type:varchar(20)"`
	TotalSupply      string     `json:"total_supply" bun:",nullzero,type:numeric(38,18)"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty" bun:"last_reconciled_at"`
	UpdatedAt        time.Time  `json:"updated_at" bun:",nullzero,default:current_timestamp"`
}
