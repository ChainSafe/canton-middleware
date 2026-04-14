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
func NewStoreMetrics(reg prometheus.Registerer) *StoreMetrics {
	f := promauto.With(reg)
	ns := sharedmetrics.Namespace
	sub := "relayer_db"

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
	return NewStoreMetrics(prometheus.NewRegistry())
}
