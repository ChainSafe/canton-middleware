package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
)

var (
	ErrTopologyNotFound = errors.New("topology not found")
	ErrTopologyExpired  = errors.New("topology expired")
)

const (
	defaultTopologyTTL      = 5 * time.Minute
	topologyCleanupInterval = 30 * time.Second
)

type pendingTopology struct {
	Topology  *identity.ExternalPartyTopology
	PublicKey []byte // SPKI public key bytes
	ExpiresAt time.Time
}

// TopologyCache stores pending topology data for two-step external user registration.
type TopologyCache struct {
	mu      sync.RWMutex
	entries map[string]*pendingTopology
	ttl     time.Duration
}

// NewTopologyCache creates a new topology cache with the given TTL.
func NewTopologyCache(ttl time.Duration) *TopologyCache {
	return &TopologyCache{
		entries: make(map[string]*pendingTopology),
		ttl:     ttl,
	}
}

// Put stores a pending topology keyed by registration token.
func (c *TopologyCache) Put(token string, topo *identity.ExternalPartyTopology, spkiKey []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[token] = &pendingTopology{
		Topology:  topo,
		PublicKey: spkiKey,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// GetAndDelete atomically retrieves and removes a pending topology.
func (c *TopologyCache) GetAndDelete(token string) (*pendingTopology, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[token]
	if !ok {
		return nil, ErrTopologyNotFound
	}
	delete(c.entries, token)

	if time.Now().After(entry.ExpiresAt) {
		return nil, ErrTopologyExpired
	}

	return entry, nil
}

// Start runs a background goroutine that periodically removes expired entries.
func (c *TopologyCache) Start(ctx context.Context) {
	ticker := time.NewTicker(topologyCleanupInterval)
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

func (c *TopologyCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, id)
		}
	}
}
