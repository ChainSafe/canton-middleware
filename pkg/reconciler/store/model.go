package store

import (
	"time"

	"github.com/uptrace/bun"
)

// ReconciliationState tracks reconciliation progress.
type ReconciliationState struct {
	LastProcessedOffset int64      `json:"last_processed_offset"`
	LastFullReconcileAt *time.Time `json:"last_full_reconcile_at,omitempty"`
	EventsProcessed     int        `json:"events_processed"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// UserTokenBalanceDao maps to user_token_balances table in PostgreSQL.
type UserTokenBalanceDao struct {
	bun.BaseModel `bun:"table:user_token_balances,alias:utb"`
	ID            int64     `bun:"id,pk,autoincrement"`
	Fingerprint   *string   `bun:"fingerprint,type:varchar(128)"`
	TokenSymbol   string    `bun:"token_symbol,notnull,type:varchar(20)"`
	EVMAddress    *string   `bun:"evm_address,type:varchar(42)"`
	CantonPartyID *string   `bun:"canton_party_id,type:varchar(255)"`
	Balance       string    `bun:",notnull,type:numeric(38,18),default:'0'"`
	UpdatedAt     time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

// TokenMetricsDao maps to token_metrics table in PostgreSQL.
type TokenMetricsDao struct {
	bun.BaseModel    `bun:"table:token_metrics"`
	TokenSymbol      string     `bun:",pk,notnull,type:varchar(20)"`
	TotalSupply      string     `bun:",notnull,type:numeric(38,18),default:'0'"`
	LastReconciledAt *time.Time `bun:"last_reconciled_at"`
	UpdatedAt        time.Time  `bun:",nullzero,notnull,default:current_timestamp"`
}

// BridgeEventDao maps to bridge_events table in PostgreSQL.
type BridgeEventDao struct {
	bun.BaseModel        `bun:"table:bridge_events"`
	ID                   int64      `bun:",pk,autoincrement"`
	EventType            string     `bun:",notnull,type:varchar(20)"`
	ContractID           string     `bun:",unique,notnull,type:varchar(255)"`
	Fingerprint          *string    `bun:",type:varchar(128)"`
	RecipientFingerprint *string    `bun:"recipient_fingerprint,type:varchar(128)"`
	Amount               string     `bun:",notnull,type:numeric(38,18)"`
	EvmTxHash            *string    `bun:"evm_tx_hash,type:varchar(66)"`
	EvmDestination       *string    `bun:"evm_destination,type:varchar(42)"`
	TokenSymbol          *string    `bun:"token_symbol,type:varchar(20)"`
	CantonTimestamp      *time.Time `bun:"canton_timestamp"`
	ProcessedAt          time.Time  `bun:",nullzero,notnull,default:current_timestamp"`
}

// ReconciliationStateDao maps to reconciliation_state table in PostgreSQL.
type ReconciliationStateDao struct {
	bun.BaseModel       `bun:"table:reconciliation_state"`
	ID                  int        `bun:",pk,notnull"`
	LastProcessedOffset int64      `bun:"last_processed_offset,notnull,default:0"`
	LastFullReconcileAt *time.Time `bun:"last_full_reconcile_at"`
	EventsProcessed     int        `bun:"events_processed,notnull,default:0"`
	UpdatedAt           time.Time  `bun:",nullzero,notnull,default:current_timestamp"`
}
