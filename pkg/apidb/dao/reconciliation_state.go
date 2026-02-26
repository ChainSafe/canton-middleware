package dao

import (
	"time"

	"github.com/uptrace/bun"
)

// ReconciliationStateDao is a data access object that maps directly to the 'reconciliation_state' table in PostgreSQL.
type ReconciliationStateDao struct {
	bun.BaseModel       `bun:"table:reconciliation_state"`
	ID                  int        `json:"id" bun:",pk"`
	LastProcessedOffset int64      `json:"last_processed_offset" bun:",nullzero"`
	LastFullReconcileAt *time.Time `json:"last_full_reconcile_at,omitempty" bun:"last_full_reconcile_at"`
	EventsProcessed     int        `json:"events_processed" bun:",nullzero"`
	UpdatedAt           time.Time  `json:"updated_at" bun:",nullzero,default:current_timestamp"`
}
