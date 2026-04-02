package engine

import "time"

// SetRetryBaseDelay overrides processorRetryBaseDelay for the duration of a test.
func SetRetryBaseDelay(t interface{ Cleanup(func()) }, d time.Duration) {
	orig := processorRetryBaseDelay
	processorRetryBaseDelay = d
	t.Cleanup(func() { processorRetryBaseDelay = orig })
}
