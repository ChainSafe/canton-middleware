// SPDX-License-Identifier: Apache-2.0

package nonceprovider

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestInMemory_SingleUse(t *testing.T) {
	store := NewInMemory(time.Minute)
	nonce, err := store.Issue("0xabc")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	if !store.Consume(nonce) {
		t.Fatal("first consume should succeed")
	}
	if store.Consume(nonce) {
		t.Fatal("second consume of same nonce must fail")
	}
	if store.Consume("never-issued") {
		t.Fatal("consuming an unknown nonce must fail")
	}
}

func TestInMemory_ReusesLiveNoncePerAddress(t *testing.T) {
	store := NewInMemory(time.Minute)

	first, err := store.Issue("0xabc")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	second, err := store.Issue("0xabc")
	if err != nil {
		t.Fatalf("re-issue: %v", err)
	}
	if first != second {
		t.Fatalf("expected the same live nonce to be reused, got %q then %q", first, second)
	}

	// A different address gets its own nonce.
	other, _ := store.Issue("0xdef")
	if other == first {
		t.Fatal("different addresses must get different nonces")
	}
}

func TestInMemory_Expiry(t *testing.T) {
	store := NewInMemory(time.Minute)
	base := time.Unix(1_700_000_000, 0)
	store.now = func() time.Time { return base }

	nonce, _ := store.Issue("0xabc")

	// Advance past the TTL before consuming.
	store.now = func() time.Time { return base.Add(2 * time.Minute) }
	if store.Consume(nonce) {
		t.Fatal("expired nonce must not be consumable")
	}
}

func TestInMemory_RejectsWhenFull_WithoutEvictingLive(t *testing.T) {
	store := NewInMemory(time.Hour) // long TTL so nothing expires during the test
	store.max = 4

	issued := make([]string, 0, store.max)
	for i := 0; i < store.max; i++ {
		n, err := store.Issue("addr-" + strconv.Itoa(i))
		if err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
		issued = append(issued, n)
	}

	// A new distinct address is rejected — never at the cost of a live nonce.
	if _, err := store.Issue("addr-overflow"); !errors.Is(err, ErrStoreFull) {
		t.Fatalf("expected ErrStoreFull, got %v", err)
	}

	// Every previously-issued nonce is still valid (none were evicted).
	for i, n := range issued {
		if !store.Consume(n) {
			t.Fatalf("nonce %d was evicted but should have survived", i)
		}
	}
}

func TestInMemory_ReclaimsExpiredBeforeRejecting(t *testing.T) {
	store := NewInMemory(time.Minute)
	base := time.Unix(1_700_000_000, 0)
	store.now = func() time.Time { return base }
	store.max = 2

	_, _ = store.Issue("a")
	_, _ = store.Issue("b") // now full

	// After the TTL, those entries are reclaimable, so a new address succeeds.
	store.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := store.Issue("c"); err != nil {
		t.Fatalf("expected expired entries to be reclaimed, got %v", err)
	}
}
