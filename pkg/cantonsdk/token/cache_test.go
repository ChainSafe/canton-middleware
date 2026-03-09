package token

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPreparedTransferCache_PutAndGetAndDelete(t *testing.T) {
	cache := NewPreparedTransferCache(5 * time.Minute)

	pt := &PreparedTransfer{
		TransferID:      "test-id-1",
		TransactionHash: []byte("hash"),
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}
	cache.Put(pt)

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
	cache := NewPreparedTransferCache(1 * time.Millisecond)

	pt := &PreparedTransfer{
		TransferID:      "test-id-2",
		TransactionHash: []byte("hash"),
		ExpiresAt:       time.Now().Add(-1 * time.Second), // already expired
	}
	cache.Put(pt)

	_, err := cache.GetAndDelete(pt.TransferID)
	if !errors.Is(err, ErrTransferExpired) {
		t.Fatalf("expected ErrTransferExpired, got %v", err)
	}
}

func TestPreparedTransferCache_NotFound(t *testing.T) {
	cache := NewPreparedTransferCache(5 * time.Minute)

	_, err := cache.GetAndDelete("nonexistent")
	if !errors.Is(err, ErrTransferNotFound) {
		t.Fatalf("expected ErrTransferNotFound, got %v", err)
	}
}

func TestPreparedTransferCache_Cleanup(t *testing.T) {
	cache := NewPreparedTransferCache(1 * time.Millisecond)

	cache.Put(&PreparedTransfer{
		TransferID: "expired",
		ExpiresAt:  time.Now().Add(-1 * time.Second),
	})
	cache.Put(&PreparedTransfer{
		TransferID: "valid",
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	})

	cache.cleanup()

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if _, ok := cache.entries["expired"]; ok {
		t.Fatal("expired entry should have been cleaned up")
	}
	if _, ok := cache.entries["valid"]; !ok {
		t.Fatal("valid entry should still exist")
	}
}

func TestPreparedTransferCache_StartStopsOnCancel(t *testing.T) {
	cache := NewPreparedTransferCache(1 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		cache.Start(ctx)
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
