package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"go.uber.org/zap"
)

var (
	processorRetryBaseDelay = 5 * time.Second
	processorRetryMaxDelay  = 60 * time.Second
)

// EventFetcher is the interface the Processor uses to start and consume the ledger stream.
//
//go:generate mockery --name EventFetcher --output mocks --outpkg mocks --filename mock_event_fetcher.go --with-expecter
type EventFetcher interface {
	// Start begins streaming from offset in a background goroutine.
	// Must be called exactly once before Events is used.
	Start(ctx context.Context, offset int64)

	// Events returns the read-only channel of decoded batches.
	// The channel is closed when the stream terminates.
	Events() <-chan *streaming.Batch[*indexer.ParsedEvent]
}

// Store defines the persistence contract for the indexer Processor.
//
// The key invariant: offset and events from the same LedgerTransaction must be
// written atomically. This guarantees that after a restart the processor resumes
// from a consistent point — no event is lost and no event is double-written.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	// LatestOffset returns the last successfully persisted ledger offset.
	// Returns 0 and no error when no offset has been stored yet (fresh start).
	// Called once at startup, outside any transaction.
	LatestOffset(ctx context.Context) (int64, error)

	// RunInTx executes fn inside a single database transaction.
	// On success fn's return value is nil and the transaction is committed.
	// On any error the transaction is rolled back and the error is returned.
	// The Store passed to fn is scoped to the transaction — all methods on it
	// participate in the same underlying DB transaction.
	RunInTx(ctx context.Context, fn func(ctx context.Context, tx Store) error) error

	// SaveBatch persists a batch of ParsedEvents and advances the stored ledger offset.
	// Duplicate events (same ContractID) are silently skipped via ON CONFLICT DO NOTHING.
	// When events is empty the offset is still advanced to skip no-op transactions on restart.
	SaveBatch(ctx context.Context, offset int64, events []*indexer.ParsedEvent) error

	// UpsertToken records a token deployment on first observation.
	// Subsequent calls for the same {InstrumentAdmin, InstrumentID} are no-ops
	// (ON CONFLICT DO NOTHING).
	UpsertToken(ctx context.Context, token *indexer.Token) error

	// ApplyBalanceDelta adjusts a party's token balance by delta (signed decimal string).
	// The balance row is created at zero if it does not yet exist, then delta is added.
	// The store must also update Token.HolderCount atomically:
	//   - increment when a party's balance transitions from zero to positive
	//   - decrement when a party's balance transitions from positive to zero
	ApplyBalanceDelta(ctx context.Context, partyID, instrumentAdmin, instrumentID, delta string) error

	// ApplySupplyDelta adjusts a token's TotalSupply by delta (signed decimal string).
	// Called once per mint (+amount) or burn (-amount). Transfer events must not call this.
	ApplySupplyDelta(ctx context.Context, instrumentAdmin, instrumentID, delta string) error
}

// Processor is the main run loop of the indexer. It wires the EventFetcher to the
// Store and writes decoded events atomically.
//
// Processing is sequential — one batch at a time. The ordering guarantee comes from
// the Canton ledger: transactions within a party's projection are delivered in
// strictly increasing offset order.
type Processor struct {
	fetcher EventFetcher
	store   Store
	logger  *zap.Logger
}

// NewProcessor creates a Processor.
func NewProcessor(fetcher EventFetcher, store Store, logger *zap.Logger) *Processor {
	return &Processor{
		fetcher: fetcher,
		store:   store,
		logger:  logger,
	}
}

// Run starts the indexer loop. It blocks until ctx is canceled or the fetcher
// channel closes, then returns ctx.Err() or nil respectively.
//
// On startup Run loads the resume offset from the store and passes it to the fetcher,
// so callers do not need to track offsets themselves.
//
// If processBatch fails (store error) Run retries the same batch with exponential
// backoff (5s → 60s) until it succeeds or ctx is canceled. The offset is never
// advanced past a failed batch — no event is silently dropped.
func (p *Processor) Run(ctx context.Context) error {
	offset, err := p.store.LatestOffset(ctx)
	if err != nil {
		return fmt.Errorf("load resume offset: %w", err)
	}

	p.logger.Info("indexer processor starting", zap.Int64("resume_offset", offset))
	p.fetcher.Start(ctx, offset)

	for {
		select {
		case batch, ok := <-p.fetcher.Events():
			if !ok {
				p.logger.Info("indexer stream closed")
				return nil
			}
			if err := p.processBatchWithRetry(ctx, batch); err != nil {
				// Only reachable when ctx is canceled.
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// processBatchWithRetry calls processBatch and retries with exponential backoff on failure.
// It returns only when the batch is successfully persisted or ctx is canceled.
func (p *Processor) processBatchWithRetry(ctx context.Context, batch *streaming.Batch[*indexer.ParsedEvent]) error {
	delay := processorRetryBaseDelay

	for {
		err := p.processBatch(ctx, batch)
		if err == nil {
			return nil
		}

		p.logger.Error("failed to process batch, retrying",
			zap.String("update_id", batch.UpdateID),
			zap.Int64("offset", batch.Offset),
			zap.Duration("backoff", delay),
			zap.Error(err),
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay = min(delay*2, processorRetryMaxDelay)
	}
}

// processBatch persists a single decoded batch inside a single database transaction.
// All writes — token upserts, supply/balance deltas, events, and offset advance — are
// committed atomically. On any error the transaction is rolled back and the caller retries.
func (p *Processor) processBatch(ctx context.Context, batch *streaming.Batch[*indexer.ParsedEvent]) error {
	err := p.store.RunInTx(ctx, func(ctx context.Context, tx Store) error {
		for _, e := range batch.Items {
			if err := tx.UpsertToken(ctx, tokenFromEvent(e)); err != nil {
				return fmt.Errorf("upsert token: %w", err)
			}

			if admin, id, delta, ok := supplyDeltaFromEvent(e); ok {
				if err := tx.ApplySupplyDelta(ctx, admin, id, delta); err != nil {
					return fmt.Errorf("apply supply delta: %w", err)
				}
			}

			for _, u := range balanceUpdatesFromEvent(e) {
				if err := tx.ApplyBalanceDelta(ctx, u[0], e.InstrumentAdmin, e.InstrumentID, u[1]); err != nil {
					return fmt.Errorf("apply balance delta: %w", err)
				}
			}
		}

		return tx.SaveBatch(ctx, batch.Offset, batch.Items)
	})
	if err != nil {
		return fmt.Errorf("tx at offset %d: %w", batch.Offset, err)
	}

	if len(batch.Items) > 0 {
		p.logger.Debug("indexed batch",
			zap.String("update_id", batch.UpdateID),
			zap.Int64("offset", batch.Offset),
			zap.Int("events", len(batch.Items)),
		)
	}

	return nil
}

// tokenFromEvent constructs a Token from a ParsedEvent for UpsertToken.
// TotalSupply and HolderCount are left zero — the store initializes them on first
// insert and maintains them via ApplySupplyDelta / UpsertBalance thereafter.
func tokenFromEvent(e *indexer.ParsedEvent) *indexer.Token {
	return &indexer.Token{
		InstrumentAdmin: e.InstrumentAdmin,
		InstrumentID:    e.InstrumentID,
		Issuer:          e.Issuer,
		FirstSeenOffset: e.LedgerOffset,
		FirstSeenAt:     e.EffectiveTime,
	}
}

// supplyDeltaFromEvent returns the signed supply delta for MINT (+amount) and
// BURN (-amount). Returns ok=false for TRANSFER, which leaves total supply unchanged.
func supplyDeltaFromEvent(e *indexer.ParsedEvent) (instrumentAdmin, instrumentID, delta string, ok bool) {
	switch e.EventType {
	case indexer.EventMint:
		return e.InstrumentAdmin, e.InstrumentID, e.Amount, true
	case indexer.EventBurn:
		return e.InstrumentAdmin, e.InstrumentID, "-" + e.Amount, true
	default:
		return "", "", "", false
	}
}

// balanceUpdatesFromEvent returns [partyID, signedDelta] pairs for each balance
// affected by an event. Mirrors supplyDeltaFromEvent but at the per-party level.
//
//	MINT:     toParty   +amount
//	BURN:     fromParty −amount
//	TRANSFER: fromParty −amount, toParty +amount
func balanceUpdatesFromEvent(e *indexer.ParsedEvent) [][2]string {
	neg := "-" + e.Amount
	switch e.EventType {
	case indexer.EventMint:
		if e.ToPartyID == nil {
			return nil
		}
		return [][2]string{{*e.ToPartyID, e.Amount}}
	case indexer.EventBurn:
		if e.FromPartyID == nil {
			return nil
		}
		return [][2]string{{*e.FromPartyID, neg}}
	case indexer.EventTransfer:
		if e.FromPartyID == nil || e.ToPartyID == nil {
			return nil
		}
		return [][2]string{{*e.FromPartyID, neg}, {*e.ToPartyID, e.Amount}}
	default:
		return nil
	}
}
