package client

import (
	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// sdkMetrics holds Prometheus collectors for the Canton SDK client.
type sdkMetrics struct {
	// RPCDuration tracks the duration of Canton ledger gRPC calls by method.
	RPCDuration *prometheus.HistogramVec
}

// newSDKMetrics registers Canton client metrics against the given registerer.
// The metric namespace is taken from reg.Namespace() rather than a hardcoded constant,
// allowing each caller to place metrics under its own top-level prefix.
func newSDKMetrics(reg sharedmetrics.NamespacedRegisterer) *sdkMetrics {
	f := promauto.With(reg)
	return &sdkMetrics{
		RPCDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: reg.Namespace(),
			Subsystem: "canton_client",
			Name:      "grpc_duration_seconds",
			Help:      "Duration of Canton ledger gRPC calls in seconds",
			Buckets:   sharedmetrics.DefaultDurationBuckets,
		}, []string{"method"}),
	}
}
