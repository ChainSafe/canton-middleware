// SPDX-License-Identifier: Apache-2.0

package custodial

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus collectors for the AcceptWorker.
//
// The worker's cycle ("acceptPending") has three observable phases:
//
//  1. Listing custodial users (one DB call to UserLister).
//  2. Paginating pending TransferOffers from the indexer (N indexer calls).
//  3. Accepting each custodial-owned offer (M Canton RPC calls).
//
// Phase-1 and phase-2 failures abort the cycle and are counted in
// ErrorsTotal with the appropriate phase label. Phase-3 (per-offer) failures
// are *not* fatal — the worker logs and continues to the next offer — so they
// live in their own OffersAcceptedTotal{result="error"} series rather than
// the cycle-level ErrorsTotal.
type Metrics struct {
	// RunsTotal counts every invocation of acceptPending — successful, empty,
	// and errored alike. Pair with rate() to check the worker's tick cadence.
	RunsTotal prometheus.Counter

	// RunDuration is the per-cycle wall-clock duration including all paginated
	// indexer fetches and all per-offer accept calls in that cycle.
	RunDuration prometheus.Histogram

	// ErrorsTotal counts cycles that aborted early.
	//   phase=list_users   – ListCustodialUsers failed
	//   phase=fetch_offers – GetPendingTransfers failed on some page
	// Per-offer accept failures are *not* counted here (see OffersAccepted).
	ErrorsTotal *prometheus.CounterVec

	// CustodialUsers is the last observed count of users returned by
	// ListCustodialUsers — the size of the operating set the worker checks
	// each cycle.
	CustodialUsers prometheus.Gauge

	// PendingOffers is the last observed total count of pending TransferOffers
	// from the indexer (taken from result.Total on the first page). Operational
	// signal: if this grows unbounded the worker is falling behind.
	PendingOffers prometheus.Gauge

	// OffersFetchedTotal is the running count of offers the indexer has
	// returned across all pages and cycles, regardless of receiver. Counts
	// indexer throughput from the worker's perspective.
	OffersFetchedTotal prometheus.Counter

	// OffersAcceptedTotal counts per-offer accept attempts (only offers whose
	// receiver matches a custodial party — others are skipped without
	// incrementing). result ∈ "success" / "error".
	OffersAcceptedTotal *prometheus.CounterVec

	// OfferAcceptDuration is the per-AcceptTransferInstruction call latency.
	// Isolated from RunDuration so dashboards can show Canton RPC cost
	// separately from cycle orchestration cost.
	OfferAcceptDuration prometheus.Histogram

	// LastSuccessfulRunTimestamp is the UNIX timestamp of the most recent
	// cycle that completed without an abort-on-error. Powers a staleness
	// alert: if (time() - this) >> pollInterval, the worker is stuck.
	LastSuccessfulRunTimestamp prometheus.Gauge
}

// NewMetrics registers AcceptWorker metrics against the given registerer.
func NewMetrics(reg sharedmetrics.NamespacedRegisterer) *Metrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "custodial_accept_worker"
	return &Metrics{
		RunsTotal: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "runs_total",
			Help: "Total acceptPending cycle invocations (regardless of outcome)",
		}),

		RunDuration: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "run_duration_seconds",
			Help:    "Per-cycle wall-clock duration of acceptPending",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}),

		ErrorsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "errors_total",
			Help: "AcceptPending cycles that aborted early, labeled by phase (list_users, fetch_offers)",
		}, []string{"phase"}),

		CustodialUsers: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "custodial_users",
			Help: "Last observed count of custodial users returned by ListCustodialUsers",
		}),

		PendingOffers: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "pending_offers",
			Help: "Last observed total count of pending TransferOffers reported by the indexer",
		}),

		OffersFetchedTotal: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "offers_fetched_total",
			Help: "Total TransferOffers fetched from the indexer across pages and cycles",
		}),

		OffersAcceptedTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "offers_accepted_total",
			Help: "Per-offer AcceptTransferInstruction outcomes; non-custodial receivers are skipped silently.",
		}, []string{"result"}),

		OfferAcceptDuration: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "offer_accept_duration_seconds",
			Help:    "Per-AcceptTransferInstruction Canton RPC latency",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}),

		LastSuccessfulRunTimestamp: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "last_successful_run_timestamp",
			Help: "UNIX timestamp of the most recent acceptPending cycle that completed without an abort-on-error",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}
