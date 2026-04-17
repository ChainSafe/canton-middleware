package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
)

// Metrics holds Prometheus collectors for the API server.
// Create with NewMetrics and inject into the server at startup.
type Metrics struct {
	// ── HTTP layer ───────────────────────────────────────────────────────────

	// RequestsTotal counts HTTP requests by method, route pattern, and status code.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration tracks HTTP request processing time by method and route pattern.
	RequestDuration *prometheus.HistogramVec

	// ActiveConnections tracks the current number of in-flight HTTP requests.
	ActiveConnections prometheus.Gauge
}

// NewMetrics registers API server metrics against the given registerer.
// Pass prometheus.DefaultRegisterer in production; use prometheus.NewRegistry() in tests.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	f := promauto.With(reg)
	ns := sharedmetrics.Namespace
	sub := "api"

	return &Metrics{
		RequestsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "requests_total",
			Help: "Total number of HTTP requests, partitioned by method, route, and status code",
		}, []string{"method", "endpoint", "status_code"}),

		RequestDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub,
			Name:    "request_duration_seconds",
			Help:    "HTTP request processing duration in seconds",
			Buckets: sharedmetrics.DefaultDurationBuckets,
		}, []string{"method", "endpoint"}),

		ActiveConnections: f.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub,
			Name: "active_connections",
			Help: "Number of HTTP requests currently being processed",
		}),
	}
}

// NewNopMetrics returns a Metrics instance backed by a throwaway registry.
// Use in tests where metric values are not asserted.
func NewNopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}

// ── Helper methods ───────────────────────────────────────────────────────────

// IncRequestsTotal increments the HTTP request counter.
func (m *Metrics) IncRequestsTotal(method, endpoint, statusCode string) {
	m.RequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
}

// ObserveRequestDuration returns the observer for a request's duration.
func (m *Metrics) ObserveRequestDuration(method, endpoint string) prometheus.Observer {
	return m.RequestDuration.WithLabelValues(method, endpoint)
}
