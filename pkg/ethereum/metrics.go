// SPDX-License-Identifier: Apache-2.0

package ethereum

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus collectors for the Ethereum RPC client.
//
// The collectors here cover two distinct surfaces that benefit from separate
// observation:
//
//  1. Outbound RPC calls — every method this client invokes against the
//     Ethereum node (eth_blockNumber, eth_gasPrice, contract calls/sends,
//     log filtering). Latency and error rate live here.
//  2. The deposit-event poll loop — a separate, dominant code path with its
//     own cycle-level metrics: cycle duration, where in the cycle failures
//     cluster, and how many events each cycle pulls in.
//
// Both surfaces share a namespaced registerer so a deployment can scrape one
// /metrics endpoint and discriminate by metric name.
type Metrics struct {
	// RPCCallsTotal counts outbound Ethereum RPC operations by method name and
	// terminal status. method is a stable lowercase identifier (see the
	// method=... call sites in client.go); status is "ok" or "error".
	RPCCallsTotal *prometheus.CounterVec

	// RPCDuration is the per-method latency distribution for RPC calls. Paired
	// with RPCCallsTotal so dashboards can show p95 latency next to error rate
	// for the same operation.
	RPCDuration *prometheus.HistogramVec

	// EventPollDuration is the wall-clock duration of one deposit-event poll
	// cycle (one tick of WatchDepositEvents). Empty cycles still contribute.
	EventPollDuration prometheus.Histogram

	// EventPollFailuresTotal counts poll cycles that hit an error at a
	// specific phase. reason ∈
	//   get_latest_block – HeaderByNumber failed at the start of the cycle
	//   filter_events    – FilterDepositToCanton failed
	//   iterator         – iter.Error() reported a problem while ranging
	// A single cycle can contribute to more than one reason if multiple phases
	// fail (rare but possible).
	EventPollFailuresTotal *prometheus.CounterVec

	// EventsFetchedTotal is the running count of deposit events the poll loop
	// has consumed (one increment per iter.Next() returning true). Pair with
	// rate() to get throughput.
	EventsFetchedTotal prometheus.Counter

	// LatestBlockSeen is the most recent block number the client has observed
	// from HeaderByNumber. Stale = the Ethereum node is unreachable or behind.
	LatestBlockSeen prometheus.Gauge

	// LastScannedBlock is the highest block number the poll loop has scanned
	// for deposit events. Distance from LatestBlockSeen = backlog the poller
	// is working through.
	LastScannedBlock prometheus.Gauge
}

// NewMetrics registers ethereum client metrics against the given registerer.
func NewMetrics(reg sharedmetrics.NamespacedRegisterer) *Metrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "ethereum_client"
	return &Metrics{
		RPCCallsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "rpc_calls_total",
			Help: "Total outbound Ethereum RPC calls by method and terminal status",
		}, []string{"method", "status"}),

		RPCDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "rpc_duration_seconds",
			Help:    "Latency of outbound Ethereum RPC calls",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}, []string{"method"}),

		EventPollDuration: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "event_poll_duration_seconds",
			Help:    "Duration of one deposit-event poll cycle",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}),

		EventPollFailuresTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "event_poll_failures_total",
			Help: "Deposit-event poll cycle failures by phase (get_latest_block, filter_events, iterator)",
		}, []string{"reason"}),

		EventsFetchedTotal: f.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "events_fetched_total",
			Help: "Total deposit events consumed by the poll loop",
		}),

		LatestBlockSeen: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "latest_block_seen",
			Help: "Most recent block number observed via HeaderByNumber",
		}),

		LastScannedBlock: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "last_scanned_block",
			Help: "Highest block number the deposit-event poll loop has scanned",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests or contexts where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(sharedmetrics.WithNamespace(prometheus.NewRegistry(), "nop"))
}
