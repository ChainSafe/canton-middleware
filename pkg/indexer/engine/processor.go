package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"go.uber.org/zap"
)

// ErrNegativeBalance is returned by Store.ApplyBalanceDelta when the delta
// would make the balance negative. Exposed so the processor can distinguish
// incomplete cross-participant history from actual store errors.
var ErrNegativeBalance = errors.New("negative balance")

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
	Events() <-chan *streaming.Batch[any]
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

	// InsertEvent persists one ParsedEvent by ContractID.
	// Returns inserted=false when the event already exists and should therefore not
	// mutate any derived state a second time.
	InsertEvent(ctx context.Context, event *indexer.ParsedEvent) (inserted bool, err error)

	// SaveOffset advances the stored ledger offset after all newly inserted events in
	// the transaction have updated derived state. It must be safe to call even when the
	// batch was empty or every event was already present.
	SaveOffset(ctx context.Context, offset int64) error

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

	// InsertPendingOffer records a new TransferOffer (idempotent by ContractID).
	// Status is set to PENDING on insert.
	InsertPendingOffer(ctx context.Context, offer *indexer.PendingOffer) error

	// MarkOfferAccepted transitions a TransferOffer to ACCEPTED status when the Canton
	// ledger emits an ARCHIVED event for the contract (receiver exercised Accept, or the
	// offer was rejected/expired). The row is kept for audit history; no-op when not found.
	MarkOfferAccepted(ctx context.Context, contractID string) error

	// InsertHolding records an active Utility.Registry.Holding contract so its amount
	// and owner can be recovered when the contract is later archived (archive events
	// carry only the contract ID). Idempotent on ContractID.
	InsertHolding(ctx context.Context, h *indexer.HoldingChange) error

	// TakeHolding deletes the row for contractID and returns the stored owner/
	// instrument/amount needed to decrement the matching balance on archive.
	// Returns ok=false on missing rows so replayed ARCHIVED events become no-ops
	// instead of errors.
	TakeHolding(ctx context.Context, contractID string) (h indexer.HoldingChange, ok bool, err error)
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
func (p *Processor) processBatchWithRetry(ctx context.Context, batch *streaming.Batch[any]) error {
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
// Each event is inserted before its derived state is mutated so replayed transactions
// can skip already-indexed events without double-applying balances or supply.
// All writes — event inserts, token upserts, supply/balance deltas, and offset advance —
// are committed atomically. On any error the transaction is rolled back and the caller retries.
func (p *Processor) processBatch(ctx context.Context, batch *streaming.Batch[any]) error {
	err := p.store.RunInTx(ctx, func(ctx context.Context, tx Store) error {
		for _, raw := range batch.Items {
			switch item := raw.(type) {
			case *indexer.ParsedEvent:
				if err := p.processTransferEvent(ctx, tx, item, batch.Offset); err != nil {
					return err
				}
			case *indexer.PendingOffer:
				if item.IsArchived {
					if err := tx.MarkOfferAccepted(ctx, item.ContractID); err != nil {
						return fmt.Errorf("mark offer accepted %s: %w", item.ContractID, err)
					}
				} else {
					if err := tx.InsertPendingOffer(ctx, item); err != nil {
						return fmt.Errorf("insert pending offer %s: %w", item.ContractID, err)
					}
				}
			case *indexer.HoldingChange:
				if err := p.processHoldingChange(ctx, tx, item); err != nil {
					return err
				}
			default:
				p.logger.Error("processBatch: unrecognized item type, skipping",
					zap.String("type", fmt.Sprintf("%T", raw)),
					zap.Int64("offset", batch.Offset),
				)
			}
		}

		if err := tx.SaveOffset(ctx, batch.Offset); err != nil {
			return fmt.Errorf("save offset: %w", err)
		}

		return nil
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

// processTransferEvent handles a single *indexer.ParsedEvent within a transaction.
func (p *Processor) processTransferEvent(ctx context.Context, tx Store, e *indexer.ParsedEvent, batchOffset int64) error {
	inserted, err := tx.InsertEvent(ctx, e)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if !inserted {
		return nil
	}

	if err = tx.UpsertToken(ctx, tokenFromEvent(e)); err != nil {
		return fmt.Errorf("upsert token: %w", err)
	}

	if admin, id, delta, ok := supplyDeltaFromEvent(e); ok {
		if err = tx.ApplySupplyDelta(ctx, admin, id, delta); err != nil {
			return fmt.Errorf("apply supply delta: %w", err)
		}
	}

	for _, u := range balanceUpdatesFromEvent(e) {
		err = tx.ApplyBalanceDelta(ctx, u[0], e.InstrumentAdmin, e.InstrumentID, u[1])
		if err == nil {
			continue
		}
		// For a TRANSFER where the sender delta is negative and the store reports
		// ErrNegativeBalance: the sender's full mint history is on another participant
		// and was never delivered to P1. Log a warning and skip the sender deduction —
		// the receiver's credit is still applied correctly.
		if e.EventType == indexer.EventTransfer && isNegativeDelta(u[1]) && errors.Is(err, ErrNegativeBalance) {
			p.logger.Warn("sender balance underflow — mint history on another participant; skipping sender deduction",
				zap.String("party", u[0]),
				zap.String("instrument_admin", e.InstrumentAdmin),
				zap.String("instrument_id", e.InstrumentID),
				zap.String("delta", u[1]),
				zap.Int64("offset", batchOffset),
			)
			continue
		}
		return fmt.Errorf("apply balance delta: %w", err)
	}
	return nil
}

// processHoldingChange handles a Utility.Registry.Holding lifecycle event by
// applying the matching balance delta:
//   - CREATED: store the holding row + increment balance for owner by amount.
//   - ARCHIVED: take (lookup + delete) the stored row + decrement balance by
//     the stored amount. Missing rows are treated as already-processed and skipped.
//
// We deliberately do not advance total supply here — internal transfers (split/
// lock/unlock) routinely archive one Holding and create another for the same total,
// so attributing every create as a mint would inflate supply. Supply for Utility.
// Registry instruments stays at 0 until we add explicit mint/burn-event tracking.
func (p *Processor) processHoldingChange(ctx context.Context, tx Store, h *indexer.HoldingChange) error {
	if h.IsArchived {
		stored, ok, err := tx.TakeHolding(ctx, h.ContractID)
		if err != nil {
			return fmt.Errorf("take holding %s: %w", h.ContractID, err)
		}
		if !ok {
			return nil
		}
		if err := tx.ApplyBalanceDelta(ctx, stored.Owner, stored.InstrumentAdmin, stored.InstrumentID, "-"+stored.Amount); err != nil {
			if errors.Is(err, ErrNegativeBalance) {
				p.logger.Warn("holding archive would underflow balance — mint history on another participant; skipping",
					zap.String("contract_id", h.ContractID),
					zap.String("party", stored.Owner),
					zap.String("amount", stored.Amount),
				)
				return nil
			}
			return fmt.Errorf("apply holding archive delta: %w", err)
		}
		return nil
	}
	if h.Owner == "" || h.InstrumentID == "" || h.InstrumentAdmin == "" {
		// Malformed CREATED event already logged by decoder; skip silently.
		return nil
	}
	if err := tx.InsertHolding(ctx, h); err != nil {
		return fmt.Errorf("insert holding %s: %w", h.ContractID, err)
	}
	if err := tx.ApplyBalanceDelta(ctx, h.Owner, h.InstrumentAdmin, h.InstrumentID, h.Amount); err != nil {
		return fmt.Errorf("apply holding create delta: %w", err)
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

// isNegativeDelta reports whether a signed decimal delta string is negative.
func isNegativeDelta(delta string) bool {
	return len(delta) > 0 && delta[0] == '-'
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
