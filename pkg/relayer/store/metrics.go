package store

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
)

// StoreMetrics holds Prometheus collectors for the relayer database layer.
type StoreMetrics struct {
	// QueryDuration tracks database query latency by operation.
	QueryDuration *prometheus.HistogramVec

	// Errors counts database errors by operation.
	Errors *prometheus.CounterVec
}

// NewStoreMetrics registers relayer store metrics against the given registerer.
func NewStoreMetrics(reg sharedmetrics.NamespacedRegisterer) *StoreMetrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "db"

	return &StoreMetrics{
		QueryDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: sharedmetrics.DBLatencyBuckets,
		}, []string{"operation"}),

		Errors: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "errors_total",
			Help: "Total number of database errors",
		}, []string{"operation"}),
	}
}

// NewNopStoreMetrics returns a StoreMetrics instance backed by a throwaway registry.
func NewNopStoreMetrics() *StoreMetrics {
	return NewStoreMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}

// ── Label value types ────────────────────────────────────────────────────────

// StoreOperation identifies a database operation for metrics labeling.
type StoreOperation string

const (
	OpCreateTransfer       StoreOperation = "create_transfer"
	OpGetTransfer          StoreOperation = "get_transfer"
	OpUpdateTransferStatus StoreOperation = "update_transfer_status"
	OpIncrementRetryCount  StoreOperation = "increment_retry_count"
	OpGetChainState        StoreOperation = "get_chain_state"
	OpSetChainState        StoreOperation = "set_chain_state"
	OpGetPendingTransfers  StoreOperation = "get_pending_transfers"
	OpListTransfers        StoreOperation = "list_transfers"
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
