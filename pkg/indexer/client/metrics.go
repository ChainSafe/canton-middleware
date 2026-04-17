package client

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ClientOperation identifies an indexer client method for metrics labeling.
type ClientOperation string

const (
	OpGetToken             ClientOperation = "get_token"
	OpListTokens           ClientOperation = "list_tokens"
	OpTotalSupply          ClientOperation = "total_supply"
	OpGetBalance           ClientOperation = "get_balance"
	OpListBalancesForParty ClientOperation = "list_balances_for_party"
	OpListBalancesForToken ClientOperation = "list_balances_for_token"
	OpGetEvent             ClientOperation = "get_event"
	OpListTokenEvents      ClientOperation = "list_token_events"
	OpListPartyEvents      ClientOperation = "list_party_events"
)

// Metrics holds Prometheus collectors for the indexer HTTP client.
type Metrics struct {
	// RequestDuration tracks the duration of indexer HTTP calls by method.
	RequestDuration *prometheus.HistogramVec

	// RequestErrors counts indexer HTTP calls that returned an error, by method.
	RequestErrors *prometheus.CounterVec
}

// NewMetrics registers indexer client metrics against the given registerer.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	f := promauto.With(reg)
	return &Metrics{
		RequestDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "indexer_client",
			Name:      "request_duration_seconds",
			Help:      "Duration of indexer HTTP client calls in seconds",
			Buckets:   sharedmetrics.DefaultDurationBuckets,
		}, []string{"method"}),

		RequestErrors: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: sharedmetrics.Namespace,
			Subsystem: "indexer_client",
			Name:      "request_errors_total",
			Help:      "Total number of indexer HTTP client errors by method",
		}, []string{"method"}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}
