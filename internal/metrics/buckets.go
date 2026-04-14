// Package metrics provides shared helpers and presets for Prometheus instrumentation.
//
// Module-specific metrics are defined next to their owning packages
// (e.g. pkg/relayer/engine/metrics.go) and injected at startup via
// NewMetrics(prometheus.Registerer). This package holds only cross-cutting
// constants used by multiple modules.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Common histogram bucket presets shared across modules.
var (
	// DBLatencyBuckets covers typical database round-trip times (1 ms → 1 s).
	DBLatencyBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

	// TransferAmountBuckets covers typical token transfer amounts.
	TransferAmountBuckets = []float64{0.001, 0.01, 0.1, 1, 10, 100, 1000, 10000}

	// ReconciliationBuckets covers reconciliation durations (100 ms → 60 s).
	ReconciliationBuckets = []float64{0.1, 0.5, 1, 5, 10, 30, 60}

	// TransferAgeBuckets covers full transfer lifecycle (1 s → 1 h).
	TransferAgeBuckets = []float64{1, 5, 30, 60, 300, 600, 1800, 3600}

	// DefaultDurationBuckets is the standard prometheus default buckets.
	DefaultDurationBuckets = prometheus.DefBuckets
)

// Namespace is the common top-level namespace for all canton middleware metrics.
const Namespace = "canton"
