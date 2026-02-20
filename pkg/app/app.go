// Package app defines common runtime contracts shared by different
// executable entrypoints (e.g., API server, relayer, migration runner).
//
// It provides minimal abstractions that allow cmd/* binaries to start
// application components without depending on their concrete implementations.
package app

// Runner represents a runnable application component.
type Runner interface {
	Run() error
}
