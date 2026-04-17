package miner

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus collectors for the ethrpc miner.
type Metrics struct {
	// MineDuration tracks the duration of each mine() cycle, including empty runs.
	MineDuration prometheus.Histogram

	// BlocksMined counts successfully finalized synthetic EVM blocks.
	BlocksMined prometheus.Counter

	// TransactionsMined counts total transactions committed across all mined blocks.
	TransactionsMined prometheus.Counter

	// ErrorsTotal counts mine() cycles that returned an error.
	ErrorsTotal prometheus.Counter

	// LatestBlock tracks the block number of the most recently finalized synthetic EVM block.
	LatestBlock prometheus.Gauge
}

// NewMetrics registers ethrpc miner metrics against the given registerer.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	f := promauto.With(reg)
	return &Metrics{
		MineDuration: f.NewHistogram(prometheus.HistogramOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "ethrpc_miner",
			Name:      "mine_duration_seconds",
			Help:      "Duration of each miner cycle in seconds",
			Buckets:   sharedmetrics.DefaultDurationBuckets,
		}),

		BlocksMined: f.NewCounter(prometheus.CounterOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "ethrpc_miner",
			Name:      "blocks_mined_total",
			Help:      "Total number of synthetic EVM blocks successfully mined",
		}),

		TransactionsMined: f.NewCounter(prometheus.CounterOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "ethrpc_miner",
			Name:      "transactions_mined_total",
			Help:      "Total number of transactions committed across all mined blocks",
		}),

		ErrorsTotal: f.NewCounter(prometheus.CounterOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "ethrpc_miner",
			Name:      "errors_total",
			Help:      "Total number of miner cycle errors",
		}),

		LatestBlock: f.NewGauge(prometheus.GaugeOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "ethrpc_miner",
			Name:      "latest_block",
			Help:      "Block number of the most recently finalized synthetic EVM block",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}
