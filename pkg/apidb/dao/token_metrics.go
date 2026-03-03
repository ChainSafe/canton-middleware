package dao

import (
	"time"

	"github.com/uptrace/bun"
)

// TokenMetricsDao is a data access object that maps directly to the 'token_metrics' table in PostgreSQL.
type TokenMetricsDao struct {
	bun.BaseModel    `bun:"table:token_metrics"`
	TokenSymbol      string     `json:"token_symbol" bun:",pk,type:varchar(20)"`
	TotalSupply      string     `json:"total_supply" bun:",nullzero,type:numeric(38,18)"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty" bun:"last_reconciled_at"`
	UpdatedAt        time.Time  `json:"updated_at" bun:",nullzero,default:current_timestamp"`
}
