package transfer

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// CacheMetrics holds Prometheus collectors for the prepared-transfer cache.
type CacheMetrics struct {
	// PutsTotal counts Put calls by result: "ok" or "full".
	PutsTotal *prometheus.CounterVec

	// GetsTotal counts GetAndDelete calls by result: "ok", "not_found", or "expired".
	GetsTotal *prometheus.CounterVec
}

// NewCacheMetrics registers transfer cache metrics against the given registerer.
func NewCacheMetrics(reg prometheus.Registerer) *CacheMetrics {
	f := promauto.With(reg)
	return &CacheMetrics{
		PutsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "transfer_cache",
			Name:      "puts_total",
			Help:      "Total number of prepared-transfer cache Put calls by result",
		}, []string{"result"}),

		GetsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "transfer_cache",
			Name:      "gets_total",
			Help:      "Total number of prepared-transfer cache GetAndDelete calls by result",
		}, []string{"result"}),
	}
}

// NewNopCacheMetrics returns a CacheMetrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopCacheMetrics() *CacheMetrics {
	return NewCacheMetrics(prometheus.NewRegistry())
}
