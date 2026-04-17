package userstore

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
)

// StoreMetrics holds Prometheus collectors for the user store database layer.
type StoreMetrics struct {
	// QueryDuration tracks database query latency partitioned by operation.
	QueryDuration *prometheus.HistogramVec

	// Errors counts database errors partitioned by operation.
	Errors *prometheus.CounterVec
}

// NewStoreMetrics registers user store metrics against the given registerer.
func NewStoreMetrics(reg sharedmetrics.NamespacedRegisterer) *StoreMetrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "userstore_db"

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

// StoreOperation identifies a database operation for metrics labeling.
type StoreOperation string

const (
	OpCreateUser             StoreOperation = "create_user"
	OpGetUserByEVMAddress    StoreOperation = "get_user_by_evm_address"
	OpGetUserByCantonPartyID StoreOperation = "get_user_by_canton_party_id"
	OpGetUserByFingerprint   StoreOperation = "get_user_by_fingerprint"
	OpUserExists             StoreOperation = "user_exists"
	OpDeleteUser             StoreOperation = "delete_user"
	OpListUsers              StoreOperation = "list_users"
	OpIsWhitelisted          StoreOperation = "is_whitelisted"
	OpAddToWhitelist         StoreOperation = "add_to_whitelist"
	OpGetKeyByCantonPartyID  StoreOperation = "get_key_by_canton_party_id"
	OpGetKeyByEVMAddress     StoreOperation = "get_key_by_evm_address"
	OpGetKeyByFingerprint    StoreOperation = "get_key_by_fingerprint"
)

// ObserveQueryDuration returns the observer for the given operation.
func (m *StoreMetrics) ObserveQueryDuration(op StoreOperation) prometheus.Observer {
	return m.QueryDuration.WithLabelValues(string(op))
}

// IncErrors increments the error counter for the given operation.
func (m *StoreMetrics) IncErrors(op StoreOperation) {
	m.Errors.WithLabelValues(string(op)).Inc()
}
