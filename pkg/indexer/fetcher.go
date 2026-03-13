package indexer

import (
	"context"
	"sync/atomic"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"

	"go.uber.org/zap"
)

const txChannelCap = 100

// Fetcher opens a live Canton stream from a caller-supplied resume offset and
// exposes the resulting transactions via Events.
//
// Typical usage:
//
//	f := indexer.NewFetcher(streamClient, templateID, logger)
//	f.Start(ctx, lastProcessedOffset)
//	for tx := range f.Events() { ... }
type Fetcher struct {
	stream     streaming.Streamer
	templateID streaming.TemplateID
	out        chan *streaming.LedgerTransaction
	logger     *zap.Logger
}

// NewFetcher creates a new Fetcher.
//
//   - stream:     Canton streaming client (handles reconnection, auth, backoff)
//   - templateID: DAML template to subscribe to (e.g. TokenTransferEvent)
//   - logger:     caller-provided logger
func NewFetcher(stream streaming.Streamer, templateID streaming.TemplateID, logger *zap.Logger) *Fetcher {
	return &Fetcher{
		stream:     stream,
		templateID: templateID,
		out:        make(chan *streaming.LedgerTransaction, txChannelCap),
		logger:     logger,
	}
}

// Start begins streaming from offset in a background goroutine. It is non-blocking.
// The goroutine exits when ctx is cancelled or the underlying stream closes.
//
// Start must be called exactly once before Events is used.
func (f *Fetcher) Start(ctx context.Context, offset int64) {
	f.logger.Info("fetcher starting", zap.Int64("resume_offset", offset))

	// lastOffset is updated atomically by the streaming.Client goroutine as
	// transactions arrive, and read back by its reconnect loop on each new
	// connection attempt, ensuring exactly-once resumption from the right point.
	var lastOffset int64
	atomic.StoreInt64(&lastOffset, offset)

	txCh := f.stream.Subscribe(ctx, streaming.SubscribeRequest{
		FromOffset:  offset,
		TemplateIDs: []streaming.TemplateID{f.templateID},
	}, &lastOffset)

	go func() {
		defer close(f.out)
		for {
			select {
			case tx, ok := <-txCh:
				if !ok {
					return
				}
				select {
				case f.out <- tx:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Events returns the read-only channel of LedgerTransactions.
// Must be called after Start. The channel is closed when the stream terminates.
func (f *Fetcher) Events() <-chan *streaming.LedgerTransaction {
	return f.out
}
