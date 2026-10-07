// SPDX-License-Identifier: Apache-2.0

package transfer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

func TestPreparedTransferCache_PutAndGetAndDelete(t *testing.T) {
	cache := NewPreparedTransferCache(5*time.Minute, 100)

	pt := &token.PreparedTransfer{
		TransferID:      "test-id-1",
		TransactionHash: []byte("hash"),
	}
	if err := cache.Put(pt); err != nil {
		t.Fatalf("Put() failed: %v", err)
	}
	if pt.ExpiresAt.IsZero() {
		t.Fatal("Put() should set ExpiresAt")
	}

	got, err := cache.GetAndDelete(pt.TransferID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TransferID != pt.TransferID {
		t.Fatalf("got transfer ID %q, want %q", got.TransferID, pt.TransferID)
	}

	// Second call should return not found (atomic delete)
	_, err = cache.GetAndDelete(pt.TransferID)
	if !errors.Is(err, ErrTransferNotFound) {
		t.Fatalf("expected ErrTransferNotFound, got %v", err)
	}
}

func TestPreparedTransferCache_Expired(t *testing.T) {
	cache := NewPreparedTransferCache(1*time.Millisecond, 100)

	pt := &token.PreparedTransfer{
		TransferID:      "test-id-2",
		TransactionHash: []byte("hash"),
	}
	if err := cache.Put(pt); err != nil {
		t.Fatalf("Put() failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	_, err := cache.GetAndDelete(pt.TransferID)
	if !errors.Is(err, ErrTransferExpired) {
		t.Fatalf("expected ErrTransferExpired, got %v", err)
	}
}

func TestPreparedTransferCache_NotFound(t *testing.T) {
	cache := NewPreparedTransferCache(5*time.Minute, 100)

	_, err := cache.GetAndDelete("nonexistent")
	if !errors.Is(err, ErrTransferNotFound) {
		t.Fatalf("expected ErrTransferNotFound, got %v", err)
	}
}

func TestPreparedTransferCache_MaxSize(t *testing.T) {
	cache := NewPreparedTransferCache(5*time.Minute, 2)

	if err := cache.Put(&token.PreparedTransfer{TransferID: "a"}); err != nil {
		t.Fatalf("Put(a) failed: %v", err)
	}
	if err := cache.Put(&token.PreparedTransfer{TransferID: "b"}); err != nil {
		t.Fatalf("Put(b) failed: %v", err)
	}

	err := cache.Put(&token.PreparedTransfer{TransferID: "c"})
	if !errors.Is(err, ErrCacheFull) {
		t.Fatalf("expected ErrCacheFull, got %v", err)
	}
}

func TestPreparedTransferCache_Cleanup(t *testing.T) {
	cache := NewPreparedTransferCache(1*time.Millisecond, 100)

	if err := cache.Put(&token.PreparedTransfer{TransferID: "will-expire"}); err != nil {
		t.Fatalf("Put() failed: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	if err := cache.Put(&token.PreparedTransfer{TransferID: "valid"}); err != nil {
		t.Fatalf("Put() failed: %v", err)
	}

	cache.cleanup()

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if _, ok := cache.entries["will-expire"]; ok {
		t.Fatal("expired entry should have been cleaned up")
	}
	if _, ok := cache.entries["valid"]; !ok {
		t.Fatal("valid entry should still exist")
	}
}

func TestPreparedTransferCache_StartStopsOnCancel(t *testing.T) {
	cache := NewPreparedTransferCache(1*time.Millisecond, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = cache.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// TestPreparedTransferCache_GetAndDeleteFor covers the owner-aware retrieval the
// Execute path relies on. The property that matters is not just that a foreign
// caller is refused, but that the refusal leaves the entry intact: the earlier
// shape deleted first and asked questions afterwards, so probing a transfer id
// destroyed it.
func TestPreparedTransferCache_GetAndDeleteFor(t *testing.T) {
	const owner = "alice::1220aa"
	const stranger = "mallory::1220bb"

	newCacheWith := func(t *testing.T, id string) *PreparedTransferCache {
		t.Helper()
		c := NewPreparedTransferCache(2*time.Minute, 10)
		if err := c.Put(&token.PreparedTransfer{TransferID: id, PartyID: owner}); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		return c
	}

	t.Run("owner retrieves and the entry is consumed", func(t *testing.T) {
		c := newCacheWith(t, "t1")

		pt, err := c.GetAndDeleteFor("t1", owner)
		if err != nil {
			t.Fatalf("owner retrieval failed: %v", err)
		}
		if pt.TransferID != "t1" {
			t.Fatalf("got transfer %q, want t1", pt.TransferID)
		}

		if _, err := c.GetAndDeleteFor("t1", owner); !errors.Is(err, ErrTransferNotFound) {
			t.Fatalf("second retrieval: got %v, want ErrTransferNotFound", err)
		}
	})

	t.Run("stranger is refused and the entry survives", func(t *testing.T) {
		c := newCacheWith(t, "t1")

		if _, err := c.GetAndDeleteFor("t1", stranger); !errors.Is(err, ErrTransferNotOwned) {
			t.Fatalf("stranger retrieval: got %v, want ErrTransferNotOwned", err)
		}

		// The whole point: the owner can still use it.
		if _, err := c.GetAndDeleteFor("t1", owner); err != nil {
			t.Fatalf("owner lost their transfer to a stranger's probe: %v", err)
		}
	})

	t.Run("a refusal does not extend the deadline", func(t *testing.T) {
		c := newCacheWith(t, "t2")
		c.mu.Lock()
		before := c.entries["t2"].ExpiresAt
		c.mu.Unlock()

		if _, err := c.GetAndDeleteFor("t2", stranger); !errors.Is(err, ErrTransferNotOwned) {
			t.Fatalf("stranger retrieval: got %v, want ErrTransferNotOwned", err)
		}

		// Restoring through Put would have reset this, letting a stranger keep
		// somebody else's transfer alive indefinitely by probing it.
		c.mu.Lock()
		after := c.entries["t2"].ExpiresAt
		c.mu.Unlock()
		if !after.Equal(before) {
			t.Fatalf("deadline moved from %v to %v on a refused probe", before, after)
		}
	})

	t.Run("unknown id is not found, whoever asks", func(t *testing.T) {
		c := newCacheWith(t, "t1")

		if _, err := c.GetAndDeleteFor("nope", stranger); !errors.Is(err, ErrTransferNotFound) {
			t.Fatalf("unknown id: got %v, want ErrTransferNotFound", err)
		}
	})

	t.Run("expired entry is reported expired to its owner", func(t *testing.T) {
		c := NewPreparedTransferCache(time.Nanosecond, 10)
		if err := c.Put(&token.PreparedTransfer{TransferID: "t3", PartyID: owner}); err != nil {
			t.Fatalf("seed cache: %v", err)
		}
		time.Sleep(2 * time.Millisecond)

		if _, err := c.GetAndDeleteFor("t3", owner); !errors.Is(err, ErrTransferExpired) {
			t.Fatalf("expired entry: got %v, want ErrTransferExpired", err)
		}
	})
}
