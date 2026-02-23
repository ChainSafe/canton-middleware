package dao

import "time"

// TokenMetricsDao is a data access object that maps directly to the 'token_metrics' table in PostgreSQL.
type TokenMetricsDao struct {
	tableName         struct{}   `pg:"token_metrics"` // nolint
	TokenSymbol       string     `json:"token_symbol" pg:",pk,type:VARCHAR(10)"`
	TotalSupply       string     `json:"total_supply" pg:",use_zero,type:NUMERIC(38,18)"`
	LastReconciledAt  *time.Time `json:"last_reconciled_at,omitempty" pg:"last_reconciled_at"`
	UpdatedAt         time.Time  `json:"updated_at" pg:"default:now()"`
}
