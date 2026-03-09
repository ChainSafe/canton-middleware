package token

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrTransferNotFound = errors.New("transfer not found")
	ErrTransferExpired  = errors.New("transfer expired")
)

const defaultCleanupInterval = 30 * time.Second

// PreparedTransferCache is an in-memory cache for prepared transfers awaiting external signatures.
type PreparedTransferCache struct {
	mu      sync.RWMutex
	entries map[string]*PreparedTransfer
	ttl     time.Duration
}

// NewPreparedTransferCache creates a new cache with the given TTL.
func NewPreparedTransferCache(ttl time.Duration) *PreparedTransferCache {
	return &PreparedTransferCache{
		entries: make(map[string]*PreparedTransfer),
		ttl:     ttl,
	}
}

// Put stores a prepared transfer in the cache.
func (c *PreparedTransferCache) Put(transfer *PreparedTransfer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[transfer.TransferID] = transfer
}

// GetAndDelete atomically retrieves and removes a prepared transfer.
// Returns ErrTransferNotFound if the ID doesn't exist, ErrTransferExpired if past TTL.
func (c *PreparedTransferCache) GetAndDelete(transferID string) (*PreparedTransfer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[transferID]
	if !ok {
		return nil, ErrTransferNotFound
	}
	delete(c.entries, transferID)

	if time.Now().After(entry.ExpiresAt) {
		return nil, ErrTransferExpired
	}

	return entry, nil
}

// Start runs a background goroutine that periodically removes expired entries.
// It stops when the context is canceled.
func (c *PreparedTransferCache) Start(ctx context.Context) {
	ticker := time.NewTicker(defaultCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

func (c *PreparedTransferCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, id)
		}
	}
}
