package store

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
)

// StoreMetrics holds Prometheus collectors for the indexer database layer.
type StoreMetrics struct {
	// QueryDuration tracks database query latency partitioned by operation.
	QueryDuration *prometheus.HistogramVec

	// Errors counts database errors partitioned by operation.
	Errors *prometheus.CounterVec
}

// NewStoreMetrics registers indexer store metrics against the given registerer.
// Pass prometheus.DefaultRegisterer in production; use prometheus.NewRegistry() in tests.
func NewStoreMetrics(reg prometheus.Registerer) *StoreMetrics {
	f := promauto.With(reg)
	ns := sharedmetrics.Namespace
	sub := "indexer_db"

	return &StoreMetrics{
		QueryDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "query_duration_seconds",
			Help:    "Database query duration in seconds, partitioned by operation",
			Buckets: sharedmetrics.DBLatencyBuckets,
		}, []string{"operation"}),

		Errors: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "errors_total",
			Help: "Total number of database errors, partitioned by operation",
		}, []string{"operation"}),
	}
}

// NewNopStoreMetrics returns a StoreMetrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopStoreMetrics() *StoreMetrics {
	return NewStoreMetrics(prometheus.NewRegistry())
}

// ── Label value types ────────────────────────────────────────────────────────

// StoreOperation identifies a database operation for metrics labeling.
type StoreOperation string

const (
	// Write-path operations (processor / engine.Store).
	OpLatestOffset      StoreOperation = "latest_offset"
	OpInsertEvent       StoreOperation = "insert_event"
	OpSaveOffset        StoreOperation = "save_offset"
	OpUpsertToken       StoreOperation = "upsert_token"
	OpApplyBalanceDelta StoreOperation = "apply_balance_delta"
	OpApplySupplyDelta  StoreOperation = "apply_supply_delta"

	// Read-path operations (HTTP API / service.Store).
	OpGetToken             StoreOperation = "get_token"
	OpListTokens           StoreOperation = "list_tokens"
	OpGetBalance           StoreOperation = "get_balance"
	OpListBalancesForParty StoreOperation = "list_balances_for_party"
	OpListBalancesForToken StoreOperation = "list_balances_for_token"
	OpGetEvent             StoreOperation = "get_event"
	OpListEvents           StoreOperation = "list_events"
)

// ── Helper methods ───────────────────────────────────────────────────────────

// ObserveQueryDuration returns the observer for the given operation's query duration.
func (m *StoreMetrics) ObserveQueryDuration(op StoreOperation) prometheus.Observer {
	return m.QueryDuration.WithLabelValues(string(op))
}

// IncErrors increments the error counter for the given operation.
func (m *StoreMetrics) IncErrors(op StoreOperation) {
	m.Errors.WithLabelValues(string(op)).Inc()
}
