package engine

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
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

// ── Label value types ────────────────────────────────────────────────────────
// Typed constants prevent positional string mistakes at call sites.

// TxStatus represents the outcome status of a transaction or transfer.
type TxStatus string

const (
	TxStatusSuccess TxStatus = "success"
	TxStatusFailed  TxStatus = "failed"
)

// TransferResult represents the final status of a completed transfer.
type TransferResult string

const (
	TransferResultCompleted TransferResult = "completed"
)

// RetryOutcome represents the result of a transfer retry attempt.
type RetryOutcome string

const (
	RetryOutcomeSuccess     RetryOutcome = "success"
	RetryOutcomeFailed      RetryOutcome = "failed"
	RetryOutcomeMaxExceeded RetryOutcome = "max_exceeded"
)

// ReconciliationResult represents the result of a reconciliation run.
type ReconciliationResult string

const (
	ReconciliationSuccess ReconciliationResult = "success"
	ReconciliationError   ReconciliationResult = "error"
)

// RestartReason represents the reason a processor was restarted.
type RestartReason string

const (
	RestartReasonError        RestartReason = "error"
	RestartReasonStreamClosed RestartReason = "stream_closed"
)

// ProcessingStage represents the stage at which a processing error occurred.
type ProcessingStage string

const (
	StageCreateTransfer ProcessingStage = "create_transfer"
	StageSubmit         ProcessingStage = "submit"
	StagePostSubmitHook ProcessingStage = "post_submit_hook"
)

// EventType represents the type of a bridge event.
type EventType string

const (
	EventTypeWithdrawal EventType = "withdrawal"
	EventTypeDeposit    EventType = "deposit"
)

// ErrorCategory represents the category of an error in ErrorsTotal.
type ErrorCategory string

const (
	ErrorCategoryProcessing ErrorCategory = "processing"
)

// ── Helper methods ───────────────────────────────────────────────────────────
// These wrap WithLabelValues so call sites are explicit about which label is which.

// IncTransfersTotal increments the transfer counter for the given direction and result.
func (m *Metrics) IncTransfersTotal(direction relayer.TransferDirection, result TransferResult) {
	m.TransfersTotal.WithLabelValues(string(direction), string(result)).Inc()
}

// ObserveTransferDuration returns the observer for transfer duration by direction.
func (m *Metrics) ObserveTransferDuration(direction relayer.TransferDirection) prometheus.Observer {
	return m.TransferDuration.WithLabelValues(string(direction))
}

// ObserveTransferAmount records a transfer amount for the given direction and token.
func (m *Metrics) ObserveTransferAmount(direction relayer.TransferDirection, token string, amount float64) {
	m.TransferAmount.WithLabelValues(string(direction), token).Observe(amount)
}

// AddTransferVolume adds to cumulative volume for the given direction and token.
func (m *Metrics) AddTransferVolume(direction relayer.TransferDirection, token string, amount float64) {
	m.TransferVolumeTotal.WithLabelValues(string(direction), token).Add(amount)
}

// IncTransactionsSent increments the transaction counter for the given chain and status.
func (m *Metrics) IncTransactionsSent(chain string, status TxStatus) {
	m.TransactionsSent.WithLabelValues(chain, string(status)).Inc()
}

// IncBlocksProcessed increments the block counter for the given chain.
func (m *Metrics) IncBlocksProcessed(chain string) {
	m.BlocksProcessed.WithLabelValues(chain).Inc()
}

// IncEventsDetected increments the event counter for the given chain and event type.
func (m *Metrics) IncEventsDetected(chain string, eventType EventType) {
	m.EventsDetected.WithLabelValues(chain, string(eventType)).Inc()
}

// SetLastProcessedBlock sets the last processed block gauge for the given chain.
func (m *Metrics) SetLastProcessedBlock(chain string, block float64) {
	m.LastProcessedBlock.WithLabelValues(chain).Set(block)
}

// IncProcessorRestarts increments the processor restart counter for the given chain and reason.
func (m *Metrics) IncProcessorRestarts(chain string, reason RestartReason) {
	m.ProcessorRestarts.WithLabelValues(chain, string(reason)).Inc()
}

// IncEventProcessingErrors increments the processing error counter for the given chain and stage.
func (m *Metrics) IncEventProcessingErrors(chain string, stage ProcessingStage) {
	m.EventProcessingErrors.WithLabelValues(chain, string(stage)).Inc()
}

// IncErrorsTotal increments the error counter for the given component and error category.
func (m *Metrics) IncErrorsTotal(component string, category ErrorCategory) {
	m.ErrorsTotal.WithLabelValues(component, string(category)).Inc()
}

// SetPendingTransfers sets the pending transfer gauge for the given direction.
func (m *Metrics) SetPendingTransfers(direction relayer.TransferDirection, count float64) {
	m.PendingTransfers.WithLabelValues(string(direction)).Set(count)
}

// IncReconciliationRuns increments the reconciliation run counter for the given result.
func (m *Metrics) IncReconciliationRuns(result ReconciliationResult) {
	m.ReconciliationRuns.WithLabelValues(string(result)).Inc()
}

// IncTransferRetries increments the retry counter for the given direction and outcome.
func (m *Metrics) IncTransferRetries(direction relayer.TransferDirection, outcome RetryOutcome) {
	m.TransferRetries.WithLabelValues(string(direction), string(outcome)).Inc()
}

// ObserveTransferAge records a transfer's lifecycle duration for the given direction.
func (m *Metrics) ObserveTransferAge(direction relayer.TransferDirection, seconds float64) {
	m.TransferAge.WithLabelValues(string(direction)).Observe(seconds)
}

// SetChainHeadBlock sets the chain head block gauge for the given chain.
func (m *Metrics) SetChainHeadBlock(chain string, block float64) {
	m.ChainHeadBlock.WithLabelValues(chain).Set(block)
}

// SetReadinessSyncDuration sets the sync duration gauge for the given chain.
func (m *Metrics) SetReadinessSyncDuration(chain string, seconds float64) {
	m.ReadinessSyncDuration.WithLabelValues(chain).Set(seconds)
}
