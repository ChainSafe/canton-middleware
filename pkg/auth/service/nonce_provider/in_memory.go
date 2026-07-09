// SPDX-License-Identifier: Apache-2.0

// Package nonceprovider contains implementations of the login nonce store.
package nonceprovider

import (
	"errors"
	"sync"
	"time"

	siwe "github.com/spruceid/siwe-go"
)

// maxAddresses caps how many distinct addresses can hold a live nonce at once.
// GET /auth/nonce is unauthenticated, so without a bound an attacker could flood
// the store and exhaust memory. Because each address maps to at most one live
// nonce (repeat requests reuse it), the map only grows with distinct addresses,
// and when the cap is hit the store rejects new issuance rather than evicting a
// live nonce — so an ongoing flood can never invalidate a nonce a real user is
// mid-login with. The cap is far above any legitimate concurrent-login volume.
const maxAddresses = 1 << 16 // 65536

// ErrStoreFull is returned by Issue when the store is at capacity and no expired
// entries can be reclaimed. It signals transient overload, not a client error.
var ErrStoreFull = errors.New("nonce store is at capacity")

type entry struct {
	nonce  string
	expiry time.Time
}

// InMemory is an in-process nonce store suitable for a single api-server replica.
// Nonces are keyed by the requesting address so a given address holds at most one
// live nonce. Nonces are short-lived, so losing them on restart is harmless; run
// more than one replica and this must be replaced with a shared (e.g. Postgres) store.
type InMemory struct {
	mu        sync.Mutex
	byAddr    map[string]entry  // address -> its live nonce + expiry
	byNonce   map[string]string // nonce -> address (reverse index for Consume)
	ttl       time.Duration
	max       int
	nextSweep time.Time
	now       func() time.Time
}

// NewInMemory creates an InMemory nonce store whose nonces expire after ttl.
func NewInMemory(ttl time.Duration) *InMemory {
	return &InMemory{
		byAddr:  make(map[string]entry),
		byNonce: make(map[string]string),
		ttl:     ttl,
		max:     maxAddresses,
		now:     time.Now,
	}
}

// Issue returns a nonce for address. If the address already holds a live nonce it
// is returned unchanged, so repeat requests (retries, an attacker replaying the
// same address) do not grow the store. Otherwise a fresh nonce is minted. It
// purges expired entries on a fixed cadence and, when the store is full of live
// entries, returns ErrStoreFull instead of evicting another address's live nonce.
func (s *InMemory) Issue(address string) (string, error) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if now.After(s.nextSweep) {
		s.purgeExpired(now)
		s.nextSweep = now.Add(s.ttl)
	}

	// Reuse the address's current nonce while it is still valid.
	if e, ok := s.byAddr[address]; ok && now.Before(e.expiry) {
		return e.nonce, nil
	}

	// No live nonce for this address: we are about to add (or replace) an entry.
	// If we would grow past the cap, try reclaiming expired entries first, then
	// refuse — never drop another address's live nonce.
	if _, exists := s.byAddr[address]; !exists && len(s.byAddr) >= s.max {
		s.purgeExpired(now)
		if len(s.byAddr) >= s.max {
			return "", ErrStoreFull
		}
	}

	// Drop any stale nonce this address held so its reverse index does not leak.
	if old, ok := s.byAddr[address]; ok {
		delete(s.byNonce, old.nonce)
	}

	nonce := siwe.GenerateNonce()
	s.byAddr[address] = entry{nonce: nonce, expiry: now.Add(s.ttl)}
	s.byNonce[nonce] = address
	return nonce, nil
}

// Consume removes the nonce and reports whether it was live at the moment of use.
func (s *InMemory) Consume(nonce string) bool {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	address, ok := s.byNonce[nonce]
	if !ok {
		return false
	}
	e := s.byAddr[address]
	delete(s.byNonce, nonce)
	delete(s.byAddr, address)
	return now.Before(e.expiry)
}

// purgeExpired removes all entries whose expiry has passed. Callers must hold s.mu.
func (s *InMemory) purgeExpired(now time.Time) {
	for address, e := range s.byAddr {
		if now.After(e.expiry) {
			delete(s.byAddr, address)
			delete(s.byNonce, e.nonce)
		}
	}
}
