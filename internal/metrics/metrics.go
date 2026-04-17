// Package metrics provides shared helpers and constants for Prometheus instrumentation.
//
// Module-specific metrics live next to their owning packages and are registered via
// dependency injection (see pkg/relayer/engine/metrics.go for the reference pattern).
// This file is intentionally kept minimal — see buckets.go for shared presets.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// NamespacedRegisterer extends prometheus.Registerer with a Namespace method.
// Callers implement this to supply both a registry and the metric name prefix,
// allowing each component to register its metrics under a caller-controlled namespace
// rather than a hardcoded constant.
type NamespacedRegisterer interface {
	prometheus.Registerer
	Namespace() string
}

// WithNamespace wraps any prometheus.Registerer together with a fixed namespace string,
// producing a NamespacedRegisterer that can be passed to SDK components.
func WithNamespace(reg prometheus.Registerer, ns string) NamespacedRegisterer {
	return &namespacedRegisterer{Registerer: reg, ns: ns}
}

type namespacedRegisterer struct {
	prometheus.Registerer
	ns string
}

func (r *namespacedRegisterer) Namespace() string { return r.ns }
