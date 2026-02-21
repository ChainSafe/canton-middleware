package dao

import "time"

// ReconciliationStateDao is a data access object that maps directly to the 'reconciliation_state' table in PostgreSQL.
type ReconciliationStateDao struct {
	tableName           struct{}   `pg:"reconciliation_state"` // nolint
	ID                  int        `json:"id" pg:",pk"`
	LastProcessedOffset int64      `json:"last_processed_offset" pg:",use_zero"`
	LastFullReconcileAt *time.Time `json:"last_full_reconcile_at,omitempty" pg:"last_full_reconcile_at"`
	EventsProcessed     int        `json:"events_processed" pg:",use_zero"`
	UpdatedAt           time.Time  `json:"updated_at" pg:"default:now()"`
}
