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

	// GetsTotal counts GetAndDelete and GetAndDeleteFor calls by result:
	// "ok", "not_found", "expired", or "not_owned". Only the owner-aware
	// retrieval can produce "not_owned"; a sustained rate of it means callers
	// are submitting transfer ids they do not own.
	GetsTotal *prometheus.CounterVec
}

// NewCacheMetrics registers transfer cache metrics against the given registerer.
func NewCacheMetrics(reg sharedmetrics.NamespacedRegisterer) *CacheMetrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "transfer_cache"
	return &CacheMetrics{
		PutsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "puts_total",
			Help: "Total number of prepared-transfer cache Put calls by result",
		}, []string{"result"}),

		GetsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "gets_total",
			Help: "Total number of prepared-transfer cache GetAndDelete calls by result",
		}, []string{"result"}),
	}
}

// NewNopCacheMetrics returns a CacheMetrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopCacheMetrics() *CacheMetrics {
	return NewCacheMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}
