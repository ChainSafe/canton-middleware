// SPDX-License-Identifier: Apache-2.0

package transfer

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

// newInstrumented returns an instrumented cache over a real one, plus the
// registry so a test can read the counters back.
func newInstrumented(t *testing.T, ttl time.Duration) (*InstrumentedCache, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics := NewCacheMetrics(sharedmetrics.WithNamespace(reg, "test"))
	return NewInstrumentedCache(NewPreparedTransferCache(ttl, 10), metrics), reg
}

// getCount reads one labeled value of the gets counter out of the registry.
// Gathering rather than reaching into the counter keeps the test honest about
// what an operator would actually scrape.
func getCount(t *testing.T, reg *prometheus.Registry, result string) float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "test_transfer_cache_gets_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "result" && l.GetValue() == result {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	// A label that has never been incremented is absent, which is zero.
	return 0
}

// TestInstrumentedCache_GetAndDeleteFor_Labels checks the wrapper attributes
// each outcome to the right label. The not_owned label is the one worth having:
// a sustained rate of it means callers are submitting transfer ids they do not
// own, which is either a client bug or a probe, and neither is visible if the
// outcome is folded into the generic error bucket.
func TestInstrumentedCache_GetAndDeleteFor_Labels(t *testing.T) {
	const owner = "alice::1220aa"
	const stranger = "mallory::1220bb"

	t.Run("owner retrieval counts ok", func(t *testing.T) {
		c, reg := newInstrumented(t, 2*time.Minute)
		if err := c.Put(&token.PreparedTransfer{TransferID: "t1", PartyID: owner}); err != nil {
			t.Fatalf("put: %v", err)
		}

		if _, err := c.GetAndDeleteFor("t1", owner); err != nil {
			t.Fatalf("owner retrieval: %v", err)
		}

		if got := getCount(t, reg, "ok"); got != 1 {
			t.Fatalf("ok counter = %v, want 1", got)
		}
	})

	t.Run("foreign caller counts not_owned, not error", func(t *testing.T) {
		c, reg := newInstrumented(t, 2*time.Minute)
		if err := c.Put(&token.PreparedTransfer{TransferID: "t1", PartyID: owner}); err != nil {
			t.Fatalf("put: %v", err)
		}

		if _, err := c.GetAndDeleteFor("t1", stranger); err == nil {
			t.Fatal("expected a refusal for a foreign caller")
		}

		if got := getCount(t, reg, "not_owned"); got != 1 {
			t.Fatalf("not_owned counter = %v, want 1", got)
		}
		if got := getCount(t, reg, "error"); got != 0 {
			t.Fatalf("error counter = %v, want 0: a refusal is not a failure", got)
		}
	})

	t.Run("unknown id counts not_found", func(t *testing.T) {
		c, reg := newInstrumented(t, 2*time.Minute)

		if _, err := c.GetAndDeleteFor("nope", owner); err == nil {
			t.Fatal("expected not found")
		}

		if got := getCount(t, reg, "not_found"); got != 1 {
			t.Fatalf("not_found counter = %v, want 1", got)
		}
	})

	t.Run("expired entry counts expired", func(t *testing.T) {
		c, reg := newInstrumented(t, time.Nanosecond)
		if err := c.Put(&token.PreparedTransfer{TransferID: "t2", PartyID: owner}); err != nil {
			t.Fatalf("put: %v", err)
		}
		time.Sleep(2 * time.Millisecond)

		if _, err := c.GetAndDeleteFor("t2", owner); err == nil {
			t.Fatal("expected expiry")
		}

		if got := getCount(t, reg, "expired"); got != 1 {
			t.Fatalf("expired counter = %v, want 1", got)
		}
	})

	t.Run("delegates to the wrapped cache", func(t *testing.T) {
		c, _ := newInstrumented(t, 2*time.Minute)
		if err := c.Put(&token.PreparedTransfer{TransferID: "t3", PartyID: owner}); err != nil {
			t.Fatalf("put: %v", err)
		}

		// A refused probe must leave the entry usable by its owner, which is the
		// property the wrapper must not break.
		if _, err := c.GetAndDeleteFor("t3", stranger); err == nil {
			t.Fatal("expected a refusal")
		}
		if _, err := c.GetAndDeleteFor("t3", owner); err != nil {
			t.Fatalf("owner lost their transfer through the wrapper: %v", err)
		}
	})
}
