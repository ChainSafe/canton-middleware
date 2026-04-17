package engine

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

// Metrics holds Prometheus collectors for the indexer engine.
// Create with NewMetrics and inject into Processor via NewProcessor.
type Metrics struct {
	// ── Event processing ─────────────────────────────────────────────────────

	// EventsProcessedTotal counts events successfully indexed, partitioned by
	// event type (MINT, BURN, TRANSFER).
	EventsProcessedTotal *prometheus.CounterVec

	// BatchProcessingErrors counts batch processing failures that triggered a
	// retry. Each increment represents one failed attempt, not one lost batch.
	BatchProcessingErrors prometheus.Counter

	// ── Sync state ───────────────────────────────────────────────────────────

	// SyncLagSeconds reports how far behind real-time the indexer is, measured
	// as time.Since(lastEvent.EffectiveTime). Updated after each non-empty batch.
	SyncLagSeconds prometheus.Gauge

	// LastOffset is the ledger offset of the most recently committed batch.
	// Updated after every successful SaveOffset.
	LastOffset prometheus.Gauge
}

// NewMetrics registers indexer engine metrics against the given registerer.
func NewMetrics(reg sharedmetrics.NamespacedRegisterer) *Metrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "engine"

	return &Metrics{
		EventsProcessedTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "events_processed_total",
			Help: "Total number of ledger events successfully indexed, partitioned by event type",
		}, []string{"event_type"}),

		BatchProcessingErrors: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "batch_processing_errors_total",
			Help: "Total number of batch processing failures that triggered a retry",
		}),

		SyncLagSeconds: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "sync_lag_seconds",
			Help: "Seconds between last processed event's effective time and now; proxy for indexer lag",
		}),

		LastOffset: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "last_offset",
			Help: "Ledger offset of the most recently committed batch",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}

// ── Helper methods ───────────────────────────────────────────────────────────

// IncEventsProcessed increments the events-processed counter for the given type.
func (m *Metrics) IncEventsProcessed(eventType indexer.EventType) {
	m.EventsProcessedTotal.WithLabelValues(string(eventType)).Inc()
}
