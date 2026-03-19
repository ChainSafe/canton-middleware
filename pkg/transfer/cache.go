package transfer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

var (
	ErrTransferNotFound = errors.New("transfer not found")
	ErrTransferExpired  = errors.New("transfer expired")
	ErrCacheFull        = errors.New("cache is full")
)

const defaultCleanupInterval = 30 * time.Second

// PreparedTransferCache is an in-memory cache for prepared transfers awaiting external signatures.
type PreparedTransferCache struct {
	mu      sync.RWMutex
	entries map[string]*token.PreparedTransfer
	ttl     time.Duration
	maxSize int
}

// NewPreparedTransferCache creates a new cache with the given TTL and a maximum number of entries.
func NewPreparedTransferCache(ttl time.Duration, maxSize int) *PreparedTransferCache {
	return &PreparedTransferCache{
		entries: make(map[string]*token.PreparedTransfer),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Put stores a prepared transfer in the cache. It sets ExpiresAt from the cache TTL.
// Returns ErrCacheFull if the maximum number of entries has been reached.
func (c *PreparedTransferCache) Put(transfer *token.PreparedTransfer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.maxSize > 0 && len(c.entries) >= c.maxSize {
		return ErrCacheFull
	}

	transfer.ExpiresAt = time.Now().Add(c.ttl)
	c.entries[transfer.TransferID] = transfer
	return nil
}

// GetAndDelete atomically retrieves and removes a prepared transfer.
// Returns ErrTransferNotFound if the ID doesn't exist, ErrTransferExpired if past TTL.
func (c *PreparedTransferCache) GetAndDelete(transferID string) (*token.PreparedTransfer, error) {
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
