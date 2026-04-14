package engine

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
)

// Metrics holds all Prometheus collectors for the relayer engine.
// Create with NewMetrics and inject into Engine / Processor / Source via constructors.
type Metrics struct {
	// ── Transfer pipeline ────────────────────────────────────────────────────

	// TransfersTotal counts completed/failed transfers by direction and status.
	TransfersTotal *prometheus.CounterVec

	// TransferDuration tracks transfer processing time by direction.
	TransferDuration *prometheus.HistogramVec

	// TransferAmount observes token amounts per transfer.
	TransferAmount *prometheus.HistogramVec

	// TransferVolumeTotal tracks cumulative token volume bridged.
	TransferVolumeTotal *prometheus.CounterVec

	// UniqueSenders counts distinct senders observed per chain (monotonic approximation).
	UniqueSenders *prometheus.CounterVec

	// TransactionsSent counts transactions sent to each chain by status.
	TransactionsSent *prometheus.CounterVec

	// ── Block / event processing ─────────────────────────────────────────────

	// BlocksProcessed counts blocks processed on each chain.
	BlocksProcessed *prometheus.CounterVec

	// EventsDetected counts events detected on each chain by event type.
	EventsDetected *prometheus.CounterVec

	// LastProcessedBlock tracks the last processed block number per chain.
	LastProcessedBlock *prometheus.GaugeVec

	// ── Stream / processor reliability ───────────────────────────────────────

	// CantonStreamReconnects counts Canton gRPC stream reconnections.
	CantonStreamReconnects prometheus.Counter

	// ProcessorRestarts counts processor restart events by chain and reason.
	ProcessorRestarts *prometheus.CounterVec

	// EventProcessingErrors counts errors at each processing stage.
	EventProcessingErrors *prometheus.CounterVec

	// ErrorsTotal counts errors by component and error type.
	ErrorsTotal *prometheus.CounterVec

	// ── Reconciliation & retries ─────────────────────────────────────────────

	// PendingTransfers tracks number of pending transfers by direction (gauge).
	PendingTransfers *prometheus.GaugeVec

	// ReconciliationDuration tracks how long each reconciliation run takes.
	ReconciliationDuration prometheus.Histogram

	// ReconciliationRuns counts reconciliation runs by result.
	ReconciliationRuns *prometheus.CounterVec

	// TransferRetries counts transfer retry attempts by direction and outcome.
	TransferRetries *prometheus.CounterVec

	// TransferAge tracks the full lifecycle duration of a transfer (created → completed).
	TransferAge *prometheus.HistogramVec

	// ── Chain sync & readiness ───────────────────────────────────────────────

	// ChainHeadBlock tracks the latest known head block for each chain.
	ChainHeadBlock *prometheus.GaugeVec

	// ReadinessSyncDuration tracks how long initial sync took per chain.
	ReadinessSyncDuration *prometheus.GaugeVec

	// Ready indicates whether the relayer engine is fully synced (1=yes, 0=no).
	Ready prometheus.Gauge
}

// NewMetrics registers relayer engine metrics against the given registerer.
// Pass prometheus.DefaultRegisterer in production; use prometheus.NewRegistry() in tests.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	f := promauto.With(reg)
	ns := sharedmetrics.Namespace
	sub := "relayer"

	return &Metrics{
		// Transfer pipeline
		TransfersTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "transfers_total",
			Help: "Total number of bridge transfers",
		}, []string{"direction", "status"}),

		TransferDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "transfer_duration_seconds",
			Help:    "Transfer processing duration in seconds",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}, []string{"direction"}),

		TransferAmount: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "transfer_amount",
			Help:    "Amount of tokens transferred",
			Buckets: sharedmetrics.TransferAmountBuckets,
		}, []string{"direction", "token"}),

		TransferVolumeTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "transfer_volume_total",
			Help: "Cumulative token volume bridged",
		}, []string{"direction", "token"}),

		UniqueSenders: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "unique_senders_total",
			Help: "Total unique senders observed (monotonic approximation)",
		}, []string{"chain"}),

		TransactionsSent: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "transactions_sent_total",
			Help: "Total number of transactions sent",
		}, []string{"chain", "status"}),

		// Block / event processing
		BlocksProcessed: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "blocks_processed_total",
			Help: "Total number of blocks processed",
		}, []string{"chain"}),

		EventsDetected: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "events_detected_total",
			Help: "Total number of bridge events detected",
		}, []string{"chain", "event_type"}),

		LastProcessedBlock: f.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "last_processed_block",
			Help: "Last processed block number by chain",
		}, []string{"chain"}),

		// Stream / processor reliability
		CantonStreamReconnects: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "canton_stream_reconnects_total",
			Help: "Total number of Canton stream reconnections",
		}),

		ProcessorRestarts: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "processor_restarts_total",
			Help: "Total number of processor restarts",
		}, []string{"chain", "reason"}),

		EventProcessingErrors: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "event_processing_errors_total",
			Help: "Total processing errors by chain and stage",
		}, []string{"chain", "stage"}),

		ErrorsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "errors_total",
			Help: "Total number of errors",
		}, []string{"component", "error_type"}),

		// Reconciliation & retries
		PendingTransfers: f.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "pending_transfers",
			Help: "Number of pending transfers by direction",
		}, []string{"direction"}),

		ReconciliationDuration: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "reconciliation_duration_seconds",
			Help:    "Duration of reconciliation runs in seconds",
			Buckets: sharedmetrics.ReconciliationBuckets,
		}),

		ReconciliationRuns: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "reconciliation_runs_total",
			Help: "Total number of reconciliation runs",
		}, []string{"result"}),

		TransferRetries: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "transfer_retries_total",
			Help: "Total number of transfer retry attempts",
		}, []string{"direction", "outcome"}),

		TransferAge: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "transfer_age_seconds",
			Help:    "Total age of transfers from creation to completion in seconds",
			Buckets: sharedmetrics.TransferAgeBuckets,
		}, []string{"direction"}),

		// Chain sync & readiness
		ChainHeadBlock: f.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "chain_head_block",
			Help: "Latest known chain head block/offset",
		}, []string{"chain"}),

		ReadinessSyncDuration: f.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "readiness_sync_duration_seconds",
			Help: "Time taken for initial sync to complete by chain",
		}, []string{"chain"}),

		Ready: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "ready",
			Help: "Whether the relayer is fully synced and ready (1=ready, 0=not ready)",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}
