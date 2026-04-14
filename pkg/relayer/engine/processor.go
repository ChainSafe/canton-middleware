package engine

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// OffsetUpdateFunc is called after successfully processing an event to persist the offset.
type OffsetUpdateFunc func(ctx context.Context, chainID string, offset string) error

// PostSubmitHook is called after a transfer is successfully submitted to the destination.
// It is best-effort: errors are logged but do not fail the transfer.
type PostSubmitHook func(ctx context.Context, event *relayer.Event, destTxHash string) error

// Source defines the interface for streaming events from a chain.
//
//go:generate mockery --name Source --output mocks --outpkg mocks --filename mock_source.go --with-expecter
type Source interface {
	StreamEvents(ctx context.Context, offset string) (<-chan *relayer.Event, <-chan error)
	GetChainID() string
	// ExtractOffset returns the offset string to persist after processing event.
	// Returns "" when no offset should be saved for this event.
	ExtractOffset(event *relayer.Event) string
}

// Destination defines the interface for submitting transfers to a chain.
// SubmitTransfer returns (destTxHash, skipped, err). skipped=true means the transfer
// was already processed on the destination chain (idempotent, treat as success).
//
//go:generate mockery --name Destination --output mocks --outpkg mocks --filename mock_destination.go --with-expecter
type Destination interface {
	SubmitTransfer(ctx context.Context, event *relayer.Event) (destTxHash string, skipped bool, err error)
	GetChainID() string
}

// Processor orchestrates the transfer process from Source to Destination.
type Processor struct {
	source          Source
	destination     Destination
	store           BridgeStore
	metrics         *Metrics
	logger          *zap.Logger
	metricsName     string
	direction       relayer.TransferDirection
	onOffsetUpdate  OffsetUpdateFunc
	onPostSubmit    PostSubmitHook
	lastSavedOffset string
}

// NewProcessor creates a new transfer processor.
func NewProcessor(
	source Source,
	destination Destination,
	store BridgeStore,
	metrics *Metrics,
	logger *zap.Logger,
	metricsName string,
	direction relayer.TransferDirection,
) *Processor {
	return &Processor{
		source:      source,
		destination: destination,
		store:       store,
		metrics:     metrics,
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
				p.metrics.ErrorsTotal.WithLabelValues(p.metricsName, "processing").Inc()
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
func (p *Processor) processEvent(ctx context.Context, event *relayer.Event) error {
	timer := prometheus.NewTimer(p.metrics.TransferDuration.WithLabelValues(string(p.direction)))
	defer timer.ObserveDuration()

	transfer := &relayer.Transfer{
		ID:                event.ID,
		Direction:         p.direction,
		Status:            relayer.TransferStatusPending,
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
		p.metrics.EventProcessingErrors.WithLabelValues(p.source.GetChainID(), "create_transfer").Inc()
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
		p.metrics.TransactionsSent.WithLabelValues(p.destination.GetChainID(), "failed").Inc()
		p.metrics.EventProcessingErrors.WithLabelValues(p.source.GetChainID(), "submit").Inc()
		errMsg := submitErr.Error()
		// Keep failed submissions pending so reconciliation can retry them.
		if updateErr := p.store.UpdateTransferStatus(ctx, event.ID, relayer.TransferStatusPending, nil, &errMsg); updateErr != nil {
			p.logger.Warn("Failed to keep transfer pending after submission error", zap.String("id", event.ID), zap.Error(updateErr))
		}
		return fmt.Errorf("submission failed: %w", submitErr)
	}

	if skipped {
		p.logger.Info("Transfer already processed on destination", zap.String("id", event.ID))
		if updateErr := p.store.UpdateTransferStatus(ctx, event.ID, relayer.TransferStatusCompleted, nil, nil); updateErr != nil {
			p.logger.Warn("Failed to mark skipped transfer as completed", zap.String("id", event.ID), zap.Error(updateErr))
		}
		p.persistOffset(ctx, event)
		return nil
	}

	if updateErr := p.store.UpdateTransferStatus(ctx, event.ID, relayer.TransferStatusCompleted, &destTxHash, nil); updateErr != nil {
		p.logger.Warn("Failed to mark transfer as completed", zap.String("id", event.ID), zap.Error(updateErr))
	}

	if p.onPostSubmit != nil {
		if hookErr := p.onPostSubmit(ctx, event, destTxHash); hookErr != nil {
			p.metrics.EventProcessingErrors.WithLabelValues(p.source.GetChainID(), "post_submit_hook").Inc()
			p.logger.Warn("Post-submit hook failed", zap.String("id", event.ID), zap.Error(hookErr))
		}
	}

	p.persistOffset(ctx, event)
	p.metrics.TransfersTotal.WithLabelValues(string(p.direction), "completed").Inc()
	p.metrics.TransactionsSent.WithLabelValues(p.destination.GetChainID(), "success").Inc()

	// Record the transfer amount for distribution tracking.
	if amount, err := strconv.ParseFloat(event.Amount, 64); err == nil {
		p.metrics.TransferAmount.WithLabelValues(string(p.direction), event.TokenAddress).Observe(amount)
		p.metrics.TransferVolumeTotal.WithLabelValues(string(p.direction), event.TokenAddress).Add(amount)
	}

	p.logger.Info("Transfer completed",
		zap.String("id", event.ID),
		zap.String("dest_tx_hash", destTxHash))

	return nil
}

// persistOffset saves the current processing position to avoid replaying events on restart.
// The source determines the offset format; duplicate offsets (same value) are skipped.
func (p *Processor) persistOffset(ctx context.Context, event *relayer.Event) {
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
