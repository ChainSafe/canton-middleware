package transfer

import (
	"errors"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

// InstrumentedCache wraps a TransferCache and records Prometheus metrics for
// every Put and GetAndDelete call.
type InstrumentedCache struct {
	inner   TransferCache
	metrics *CacheMetrics
}

// Compile-time check that InstrumentedCache implements TransferCache.
var _ TransferCache = (*InstrumentedCache)(nil)

// NewInstrumentedCache returns a metrics-instrumented wrapper around the given TransferCache.
func NewInstrumentedCache(inner TransferCache, metrics *CacheMetrics) *InstrumentedCache {
	return &InstrumentedCache{inner: inner, metrics: metrics}
}

func (c *InstrumentedCache) Put(transfer *token.PreparedTransfer) error {
	err := c.inner.Put(transfer)
	if errors.Is(err, ErrCacheFull) {
		c.metrics.PutsTotal.WithLabelValues("full").Inc()
	} else {
		c.metrics.PutsTotal.WithLabelValues("ok").Inc()
	}
	return err
}

func (c *InstrumentedCache) GetAndDelete(transferID string) (*token.PreparedTransfer, error) {
	pt, err := c.inner.GetAndDelete(transferID)
	switch {
	case err == nil:
		c.metrics.GetsTotal.WithLabelValues("ok").Inc()
	case errors.Is(err, ErrTransferNotFound):
		c.metrics.GetsTotal.WithLabelValues("not_found").Inc()
	case errors.Is(err, ErrTransferExpired):
		c.metrics.GetsTotal.WithLabelValues("expired").Inc()
	default:
		c.metrics.GetsTotal.WithLabelValues("error").Inc()
	}
	return pt, err
}
