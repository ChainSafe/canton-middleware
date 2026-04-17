package http

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
)

// HTTPMetrics holds Prometheus collectors for an HTTP server.
// Create with NewHTTPMetrics and pass to RequestMetricsMiddleware.
type HTTPMetrics struct {
	// RequestsTotal counts HTTP requests by method, route pattern, and status code.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration tracks HTTP request processing time by method and route pattern.
	RequestDuration *prometheus.HistogramVec

	// ActiveConnections tracks the current number of in-flight HTTP requests.
	ActiveConnections prometheus.Gauge
}

// NewHTTPMetrics registers HTTP server metrics against the given registerer.
// Metrics are named <namespace>_http_requests_total etc.
func NewHTTPMetrics(reg sharedmetrics.NamespacedRegisterer) *HTTPMetrics {
	f := promauto.With(reg)
	ns := reg.Namespace()
	sub := "http"

	return &HTTPMetrics{
		RequestsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub,
			Name: "requests_total",
			Help: "Total HTTP requests, partitioned by method, route, and status code",
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

// IncRequestsTotal increments the HTTP request counter.
func (m *HTTPMetrics) IncRequestsTotal(method, endpoint, statusCode string) {
	m.RequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
}

// ObserveRequestDuration returns the observer for a request's duration.
func (m *HTTPMetrics) ObserveRequestDuration(method, endpoint string) prometheus.Observer {
	return m.RequestDuration.WithLabelValues(method, endpoint)
}
