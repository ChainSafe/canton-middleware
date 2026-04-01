package streaming

import "context"

// Batch carries decoded items from one LedgerTransaction, preserving the
// transaction boundary for atomic offset writes.
type Batch[T any] struct {
	Offset   int64
	UpdateID string
	Items    []T
}

// Stream[T] wraps a Streamer and applies a per-event decode function.
// Use when subscribing to a single homogeneous template.
type Stream[T any] struct {
	streamer Streamer
	decode   func(*LedgerTransaction, *LedgerEvent) (T, bool)
}

// NewStream creates a Stream[T] that decodes events using the provided function.
func NewStream[T any](streamer Streamer, decode func(*LedgerTransaction, *LedgerEvent) (T, bool)) *Stream[T] {
	return &Stream[T]{streamer: streamer, decode: decode}
}

// Subscribe passes lastOffset to streamer.Subscribe, iterates each tx's events
// through decode, and emits *Batch[T] for every tx. Items may be empty —
// offset must still advance for no-op transactions.
func (s *Stream[T]) Subscribe(ctx context.Context, req SubscribeRequest, lastOffset *int64) <-chan *Batch[T] {
	txCh := s.streamer.Subscribe(ctx, req, lastOffset)
	out := make(chan *Batch[T], txChannelCap)

	go func() {
		defer close(out)
		for {
			select {
			case tx, ok := <-txCh:
				if !ok {
					return
				}
				batch := &Batch[T]{
					Offset:   tx.Offset,
					UpdateID: tx.UpdateID,
					Items:    make([]T, 0, len(tx.Events)),
				}
				for _, ev := range tx.Events {
					if item, ok := s.decode(tx, ev); ok {
						batch.Items = append(batch.Items, item)
					}
				}
				select {
				case out <- batch:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}
