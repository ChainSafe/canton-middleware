// Package metrics provides shared helpers and constants for Prometheus instrumentation.
//
// Module-specific metrics live next to their owning packages and are registered via
// dependency injection (see pkg/relayer/engine/metrics.go for the reference pattern).
// This file is intentionally kept minimal — see buckets.go for shared presets.
package metrics
