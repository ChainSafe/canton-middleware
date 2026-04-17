package store

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// StoreOperation identifies a database operation for metrics labeling.
type StoreOperation string

const (
	// Store operations.
	OpNewBlock                StoreOperation = "new_block"
	OpGetLatestEvmBlockNumber StoreOperation = "get_latest_evm_block_number"
	OpGetEvmTransactionCount  StoreOperation = "get_evm_transaction_count"
	OpGetEvmTransaction       StoreOperation = "get_evm_transaction"
	OpGetEvmLogsByTxHash      StoreOperation = "get_evm_logs_by_tx_hash"
	OpGetEvmLogs              StoreOperation = "get_evm_logs"
	OpGetBlockNumberByHash    StoreOperation = "get_block_number_by_hash"
	OpInsertMempoolEntry      StoreOperation = "insert_mempool_entry"
	OpCompleteMempoolEntry    StoreOperation = "complete_mempool_entry"
	OpFailMempoolEntry        StoreOperation = "fail_mempool_entry"

	// PendingBlock operations.
	OpClaimMempoolEntries StoreOperation = "claim_mempool_entries"
	OpAddEvmTransaction   StoreOperation = "add_evm_transaction"
	OpAddEvmLog           StoreOperation = "add_evm_log"
	OpFinalize            StoreOperation = "finalize"
	OpAbort               StoreOperation = "abort"
)

// StoreMetrics holds Prometheus collectors for the ethrpc store.
type StoreMetrics struct {
	QueryDuration *prometheus.HistogramVec
	Errors        *prometheus.CounterVec
}

// NewStoreMetrics registers ethrpc store metrics against the given registerer.
func NewStoreMetrics(reg sharedmetrics.NamespacedRegisterer) *StoreMetrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "ethrpc_db"
	return &StoreMetrics{
		QueryDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "query_duration_seconds",
			Help:    "Duration of ethrpc store database queries in seconds",
			Buckets: sharedmetrics.DBLatencyBuckets,
		}, []string{"operation"}),

		Errors: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "errors_total",
			Help: "Total number of ethrpc store errors by operation",
		}, []string{"operation"}),
	}
}

// NewNopStoreMetrics returns a StoreMetrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopStoreMetrics() *StoreMetrics {
	return NewStoreMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}

// ObserveQueryDuration returns the observer for the given operation.
func (m *StoreMetrics) ObserveQueryDuration(op StoreOperation) prometheus.Observer {
	return m.QueryDuration.WithLabelValues(string(op))
}

// IncErrors increments the error counter for the given operation.
func (m *StoreMetrics) IncErrors(op StoreOperation) {
	m.Errors.WithLabelValues(string(op)).Inc()
}
