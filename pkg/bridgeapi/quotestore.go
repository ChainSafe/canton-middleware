// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"sync"
	"time"
)

// storedQuote is everything registration needs to validate and forward a
// reported deposit without trusting the caller's parameters.
type storedQuote struct {
	QuoteID        string
	Owner          string // authenticated EVM address the quote was issued to
	TokenSymbol    string
	Mechanism      string
	TokenAddress   string
	Amount         string // token units, decimal string
	RecipientParty string
	ExpiresAt      time.Time
}

// quoteStore is an in-memory TTL store for issued quotes. Quotes are
// short-lived and advisory — losing them on restart only means the dapp
// re-quotes — so process-local storage is sufficient for a single-replica
// api-server.
type quoteStore struct {
	mu     sync.Mutex
	quotes map[string]*storedQuote
	now    func() time.Time
}

func newQuoteStore() *quoteStore {
	return &quoteStore{quotes: make(map[string]*storedQuote), now: time.Now}
}

// Put stores a quote and opportunistically prunes expired ones.
func (s *quoteStore) Put(q *storedQuote) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	for id, stored := range s.quotes {
		if now.After(stored.ExpiresAt) {
			delete(s.quotes, id)
		}
	}
	s.quotes[q.QuoteID] = q
}

// Get returns the quote if it exists and has not expired.
func (s *quoteStore) Get(quoteID string) (*storedQuote, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.quotes[quoteID]
	if !ok || s.now().After(q.ExpiresAt) {
		return nil, false
	}
	return q, true
}
