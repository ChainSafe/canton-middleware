package engine

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"go.uber.org/zap"
)

// Fetcher opens a live Canton stream from a caller-supplied resume offset and
// exposes the resulting batches via Events.
//
// Typical usage:
//
//	decode := engine.NewTokenTransferDecoder(mode, allowed, logger)
//	f := engine.NewFetcher(streamClient, templateID, decode, logger)
//	f.Start(ctx, lastProcessedOffset)
//	for batch := range f.Events() { ... }
type Fetcher struct {
	stream     *streaming.Stream[*indexer.ParsedEvent]
	templateID streaming.TemplateID
	out        <-chan *streaming.Batch[*indexer.ParsedEvent]
	once       sync.Once
	logger     *zap.Logger
}

// NewFetcher creates a new Fetcher.
//
//   - streamer:   Canton streaming client (handles reconnection, auth, backoff)
//   - templateID: DAML template to subscribe to (e.g. TokenTransferEvent)
//   - decode:     per-event decode function (see NewTokenTransferDecoder)
//   - logger:     caller-provided logger
func NewFetcher(
	streamer streaming.Streamer,
	templateID streaming.TemplateID,
	decode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.ParsedEvent, bool),
	logger *zap.Logger,
) *Fetcher {
	return &Fetcher{
		stream:     streaming.NewStream(streamer, decode),
		templateID: templateID,
		logger:     logger,
	}
}

// Start begins streaming from offset. It is non-blocking; the underlying goroutine
// exits when ctx is canceled or the stream closes.
//
// Start must be called exactly once before Events is used. Subsequent calls are no-ops.
func (f *Fetcher) Start(ctx context.Context, offset int64) {
	f.once.Do(func() {
		f.logger.Info("fetcher starting", zap.Int64("resume_offset", offset))

		// lastOffset is updated atomically by the streaming.Client goroutine as
		// transactions arrive, and read back by its reconnect loop on each new
		// connection attempt, ensuring exactly-once resumption from the right point.
		var lastOffset int64
		atomic.StoreInt64(&lastOffset, offset)

		f.out = f.stream.Subscribe(ctx, streaming.SubscribeRequest{
			FromOffset:  offset,
			TemplateIDs: []streaming.TemplateID{f.templateID},
		}, &lastOffset)
	})
}

// Events returns the read-only channel of decoded batches.
// Must be called after Start. The channel is closed when the stream terminates.
func (f *Fetcher) Events() <-chan *streaming.Batch[*indexer.ParsedEvent] {
	return f.out
}
