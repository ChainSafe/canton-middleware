package token

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPreparedTransferCache_PutAndGetAndDelete(t *testing.T) {
	cache := NewPreparedTransferCache(5*time.Minute, 100)

	pt := &PreparedTransfer{
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

	pt := &PreparedTransfer{
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

	if err := cache.Put(&PreparedTransfer{TransferID: "a"}); err != nil {
		t.Fatalf("Put(a) failed: %v", err)
	}
	if err := cache.Put(&PreparedTransfer{TransferID: "b"}); err != nil {
		t.Fatalf("Put(b) failed: %v", err)
	}

	err := cache.Put(&PreparedTransfer{TransferID: "c"})
	if !errors.Is(err, ErrCacheFull) {
		t.Fatalf("expected ErrCacheFull, got %v", err)
	}
}

func TestPreparedTransferCache_Cleanup(t *testing.T) {
	cache := NewPreparedTransferCache(1*time.Millisecond, 100)

	if err := cache.Put(&PreparedTransfer{TransferID: "will-expire"}); err != nil {
		t.Fatalf("Put() failed: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	if err := cache.Put(&PreparedTransfer{TransferID: "valid"}); err != nil {
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
