package relayer

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/internal/metrics"
)

// OffsetUpdateFunc is called after successfully processing an event to persist the offset.
type OffsetUpdateFunc func(ctx context.Context, chainID string, offset string) error

// PostSubmitHook is called after a transfer is successfully submitted to the destination.
// It is best-effort: errors are logged but do not fail the transfer.
type PostSubmitHook func(ctx context.Context, event *Event, destTxHash string) error

// Source defines the interface for streaming events from a chain.
type Source interface {
	StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error)
	GetChainID() string
	// ExtractOffset returns the offset string to persist after processing event.
	// Returns "" when no offset should be saved for this event.
	ExtractOffset(event *Event) string
}

// Destination defines the interface for submitting transfers to a chain.
// SubmitTransfer returns (destTxHash, skipped, err). skipped=true means the transfer
// was already processed on the destination chain (idempotent, treat as success).
type Destination interface {
	SubmitTransfer(ctx context.Context, event *Event) (destTxHash string, skipped bool, err error)
	GetChainID() string
}

// Processor orchestrates the transfer process from Source to Destination.
type Processor struct {
	source          Source
	destination     Destination
	store           BridgeStore
	logger          *zap.Logger
	metricsName     string
	direction       TransferDirection
	onOffsetUpdate  OffsetUpdateFunc
	onPostSubmit    PostSubmitHook
	lastSavedOffset string
}

// NewProcessor creates a new transfer processor.
func NewProcessor(
	source Source,
	destination Destination,
	store BridgeStore,
	logger *zap.Logger,
	metricsName string,
	direction TransferDirection,
) *Processor {
	return &Processor{
		source:      source,
		destination: destination,
		store:       store,
		logger:      logger,
		metricsName: metricsName,
		direction:   direction,
	}
}

// WithOffsetUpdate sets the callback for persisting offsets after event processing.
func (p *Processor) WithOffsetUpdate(fn OffsetUpdateFunc) *Processor {
	p.onOffsetUpdate = fn
	return p
}

// WithPostSubmit sets a best-effort hook called after each successful transfer submission.
func (p *Processor) WithPostSubmit(fn PostSubmitHook) *Processor {
	p.onPostSubmit = fn
	return p
}

// Start starts the processor, streaming events from startOffset until ctx is canceled.
func (p *Processor) Start(ctx context.Context, startOffset string) error {
	p.logger.Info("Starting processor",
		zap.String("source", p.source.GetChainID()),
		zap.String("destination", p.destination.GetChainID()),
		zap.String("offset", startOffset))

	eventCh, errCh := p.source.StreamEvents(ctx, startOffset)

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if err := p.processEvent(ctx, event); err != nil {
				p.logger.Error("Failed to process event",
					zap.String("event_id", event.ID),
					zap.Error(err))
				metrics.ErrorsTotal.WithLabelValues(p.metricsName, "processing").Inc()
			}
		case err := <-errCh:
			if err != nil {
				return fmt.Errorf("source stream error: %w", err)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// processEvent handles a single bridge event end-to-end.
func (p *Processor) processEvent(ctx context.Context, event *Event) error {
	transfer := &Transfer{
		ID:                event.ID,
		Direction:         p.direction,
		Status:            TransferStatusPending,
		SourceChain:       p.source.GetChainID(),
		DestinationChain:  p.destination.GetChainID(),
		SourceTxHash:      event.SourceTxHash,
		TokenAddress:      event.TokenAddress,
		Amount:            event.Amount,
		Sender:            event.Sender,
		Recipient:         event.Recipient,
		Nonce:             event.Nonce,
		SourceBlockNumber: event.SourceBlockNumber,
	}

	inserted, err := p.store.CreateTransfer(ctx, transfer)
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}
	if !inserted {
		p.logger.Debug("Event already processed", zap.String("event_id", event.ID))
		p.persistOffset(ctx, event)
		return nil
	}

	p.logger.Info("Processing transfer",
		zap.String("id", event.ID),
		zap.String("direction", string(p.direction)),
		zap.String("amount", event.Amount))

	destTxHash, skipped, submitErr := p.destination.SubmitTransfer(ctx, event)
	if submitErr != nil {
		p.logger.Error("Failed to submit transfer", zap.String("id", event.ID), zap.Error(submitErr))
		errMsg := submitErr.Error()
		if updateErr := p.store.UpdateTransferStatus(ctx, event.ID, TransferStatusFailed, nil, &errMsg); updateErr != nil {
			p.logger.Warn("Failed to mark transfer as failed", zap.String("id", event.ID), zap.Error(updateErr))
		}
		return fmt.Errorf("submission failed: %w", submitErr)
	}

	if skipped {
		p.logger.Info("Transfer already processed on destination", zap.String("id", event.ID))
		if updateErr := p.store.UpdateTransferStatus(ctx, event.ID, TransferStatusCompleted, nil, nil); updateErr != nil {
			p.logger.Warn("Failed to mark skipped transfer as completed", zap.String("id", event.ID), zap.Error(updateErr))
		}
		p.persistOffset(ctx, event)
		return nil
	}

	if updateErr := p.store.UpdateTransferStatus(ctx, event.ID, TransferStatusCompleted, &destTxHash, nil); updateErr != nil {
		p.logger.Warn("Failed to mark transfer as completed", zap.String("id", event.ID), zap.Error(updateErr))
	}

	if p.onPostSubmit != nil {
		if hookErr := p.onPostSubmit(ctx, event, destTxHash); hookErr != nil {
			p.logger.Warn("Post-submit hook failed", zap.String("id", event.ID), zap.Error(hookErr))
		}
	}

	p.persistOffset(ctx, event)
	metrics.TransfersTotal.WithLabelValues(string(p.direction), "completed").Inc()

	p.logger.Info("Transfer completed",
		zap.String("id", event.ID),
		zap.String("dest_tx_hash", destTxHash))

	return nil
}

// persistOffset saves the current processing position to avoid replaying events on restart.
// The source determines the offset format; duplicate offsets (same value) are skipped.
func (p *Processor) persistOffset(ctx context.Context, event *Event) {
	if p.onOffsetUpdate == nil {
		return
	}

	offset := p.source.ExtractOffset(event)
	if offset == "" || offset == p.lastSavedOffset {
		return
	}
	p.lastSavedOffset = offset

	if err := p.onOffsetUpdate(ctx, p.source.GetChainID(), offset); err != nil {
		p.logger.Warn("Failed to persist offset",
			zap.String("offset", offset),
			zap.Error(err))
	}
}
