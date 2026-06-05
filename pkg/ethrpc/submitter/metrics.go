// SPDX-License-Identifier: Apache-2.0

package submitter

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus collectors for the ethrpc submitter.
type Metrics struct {
	// DrainDuration tracks the wall-clock duration of one drain tick, including
	// empty ticks. Successful and errored ticks both contribute so the
	// histogram reflects the loop's actual cadence.
	DrainDuration prometheus.Histogram

	// DrainErrorsTotal counts drain ticks that aborted because the store fetch
	// for pending entries failed. Successful and empty ticks are not counted.
	DrainErrorsTotal prometheus.Counter

	// EntriesFetched is the distribution of pending-entry counts returned by
	// one drain tick. Compare against the configured batch size to detect a
	// backlog that's pinned at the cap.
	EntriesFetched prometheus.Histogram

	// PendingBacklog is the last observed count of pending mempool entries
	// returned by the store. Updated each drain after the fetch; reflects the
	// queue depth at sample time, not a running total.
	PendingBacklog prometheus.Gauge

	// EntriesProcessedTotal counts terminal per-entry outcomes:
	//   completed         – Canton accepted, mempool row → completed
	//   failed_permanent  – permanent (client-side) error, mempool row → failed
	//   transient_retry   – transient error, row left pending for next tick
	EntriesProcessedTotal *prometheus.CounterVec

	// EntryProcessDuration is per-entry total processing time, labeled by the
	// same outcome as EntriesProcessedTotal. Includes the Canton call plus the
	// surrounding orchestration and any follow-up DB write.
	EntryProcessDuration *prometheus.HistogramVec

	// CantonTransferDuration isolates the Canton TransferFrom RPC latency from
	// the rest of process(). Buckets extend to cantonCallTimeout (60s) so
	// budget consumption is visible at the high end.
	CantonTransferDuration *prometheus.HistogramVec

	// CantonTransferErrorsTotal counts Canton call errors classified by the
	// apperr.Category taxonomy, plus a "timeout" / "transient" fallback for
	// uncategorised cases. Mirrors the labels used elsewhere in
	// canton-middleware so dashboards stay consistent.
	CantonTransferErrorsTotal *prometheus.CounterVec

	// DBWriteErrorsTotal counts post-Canton store writes that failed.
	//   kind=complete  – CompleteMempoolEntry failed
	//   kind=fail      – FailMempoolEntry failed
	// Both leave the entry in an inconsistent state; this surfaces what the
	// existing log.Error sites do not.
	DBWriteErrorsTotal *prometheus.CounterVec

	// WorkerPoolBlockedTotal counts the times the drain loop had to block on a
	// saturated worker pool before launching the next goroutine. Non-zero
	// means concurrency is undersized for the arrival rate.
	WorkerPoolBlockedTotal prometheus.Counter

	// LastSuccessfulDrainTimestamp is the UNIX timestamp of the most recent
	// drain tick that completed without a store error. Powers a staleness
	// alert: if (time() - this) > threshold, the submitter is stuck.
	LastSuccessfulDrainTimestamp prometheus.Gauge
}

// cantonCallBuckets sits across the package-level cantonCallTimeout (60s) so
// the budget tail is visible rather than collapsing into the histogram's last
// bucket. Lower boundaries cover the normal 5-15s Canton commit window.
var cantonCallBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 30, 60}

// entriesFetchedBuckets cover empty ticks through a large backlog draining at
// the configured batch-size cap. Log-spaced so a small batch and a saturated
// batch land in distinct buckets.
var entriesFetchedBuckets = []float64{0, 1, 5, 10, 25, 50, 100, 500, 1000}

// NewMetrics registers ethrpc submitter metrics against the given registerer.
func NewMetrics(reg sharedmetrics.NamespacedRegisterer) *Metrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "ethrpc_submitter"
	return &Metrics{
		DrainDuration: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "drain_duration_seconds",
			Help:    "Duration of each submitter drain tick in seconds",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}),

		DrainErrorsTotal: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "drain_errors_total",
			Help: "Drain ticks aborted because the store fetch for pending entries failed",
		}),

		EntriesFetched: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "entries_fetched",
			Help:    "Number of pending entries fetched per drain tick",
			Buckets: entriesFetchedBuckets,
		}),

		PendingBacklog: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "pending_backlog",
			Help: "Last observed count of pending mempool entries returned by the store",
		}),

		EntriesProcessedTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "entries_processed_total",
			Help: "Total mempool entries processed by terminal outcome",
		}, []string{"outcome"}),

		EntryProcessDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "entry_process_duration_seconds",
			Help:    "Per-entry processing time including Canton call and follow-up store write",
			Buckets: cantonCallBuckets,
		}, []string{"outcome"}),

		CantonTransferDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "canton_transfer_duration_seconds",
			Help:    "Duration of the Canton TransferFrom RPC call, isolated from orchestration overhead",
			Buckets: cantonCallBuckets,
		}, []string{"outcome"}),

		CantonTransferErrorsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "canton_transfer_errors_total",
			Help: "Canton TransferFrom errors classified by apperr.Category (plus timeout / transient)",
		}, []string{"category"}),

		DBWriteErrorsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "db_write_errors_total",
			Help: "Post-Canton store write failures",
		}, []string{"kind"}),

		WorkerPoolBlockedTotal: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "worker_pool_blocked_total",
			Help: "Number of times the drain loop blocked on a saturated worker pool",
		}),

		LastSuccessfulDrainTimestamp: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "last_successful_drain_timestamp",
			Help: "UNIX timestamp of the most recent drain tick that completed without store error",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}
