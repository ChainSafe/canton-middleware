package relayer

import (
	"context"
	"fmt"

	"github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/db"
	"go.uber.org/zap"
)

// Event represents a generic bridge event
type Event struct {
	ID                string
	TransactionID     string
	SourceChain       string
	DestinationChain  string
	SourceTxHash      string
	TokenAddress      string
	Amount            string
	Sender            string
	Recipient         string
	Nonce             int64
	SourceBlockNumber int64
	Raw               interface{} // Original event object
}

// Source defines the interface for fetching events from a chain
type Source interface {
	// StreamEvents streams events starting from the given offset
	StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error)
	// GetChainID returns the chain ID
	GetChainID() string
}

// Destination defines the interface for submitting transfers to a chain
type Destination interface {
	// SubmitTransfer submits a transfer to the destination chain
	SubmitTransfer(ctx context.Context, event *Event) (string, error)
	// GetChainID returns the chain ID
	GetChainID() string
}

// Processor orchestrates the transfer process from Source to Destination
type Processor struct {
	source      Source
	destination Destination
	store       BridgeStore
	logger      *zap.Logger
	metricsName string
}

// NewProcessor creates a new transfer processor
func NewProcessor(source Source, destination Destination, store BridgeStore, logger *zap.Logger, metricsName string) *Processor {
	return &Processor{
		source:      source,
		destination: destination,
		store:       store,
		logger:      logger,
		metricsName: metricsName,
	}
}

// Start starts the processor
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

// processEvent handles a single event
func (p *Processor) processEvent(ctx context.Context, event *Event) error {
	// Check if already processed
	existing, _ := p.store.GetTransfer(event.ID)
	if existing != nil {
		p.logger.Debug("Event already processed", zap.String("event_id", event.ID))
		return nil
	}

	// Create transfer record
	transfer := &db.Transfer{
		ID:                event.ID,
		Direction:         p.getDirection(),
		Status:            db.TransferStatusPending,
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

	if err := p.store.CreateTransfer(transfer); err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	p.logger.Info("Processing transfer",
		zap.String("id", event.ID),
		zap.String("direction", string(p.getDirection())),
		zap.String("amount", event.Amount))

	// Submit to destination
	destTxHash, err := p.destination.SubmitTransfer(ctx, event)
	if err != nil {
		p.logger.Error("Failed to submit transfer",
			zap.String("id", event.ID),
			zap.Error(err))

		// Update status to failed
		p.store.UpdateTransferStatus(event.ID, db.TransferStatusFailed, nil)
		return fmt.Errorf("submission failed: %w", err)
	}

	// Update status to completed
	p.store.UpdateTransferStatus(event.ID, db.TransferStatusCompleted, &destTxHash)

	metrics.TransfersTotal.WithLabelValues(string(p.getDirection()), "completed").Inc()

	p.logger.Info("Transfer completed",
		zap.String("id", event.ID),
		zap.String("dest_tx_hash", destTxHash))

	return nil
}

func (p *Processor) getDirection() db.TransferDirection {
	if p.source.GetChainID() == "canton" {
		return db.DirectionCantonToEthereum
	}
	return db.DirectionEthereumToCanton
}
