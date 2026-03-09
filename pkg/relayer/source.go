package relayer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
)

// errCantonStreamClosed is returned when the Canton withdrawal stream closes without
// the context being canceled (indicating an unexpected disconnection).
var errCantonStreamClosed = errors.New("canton withdrawal stream closed unexpectedly")

// cantonSource implements Source for the Canton ledger.
type cantonSource struct {
	client        canton.Bridge
	tokenContract string
	chainID       string
}

// NewCantonSource creates a new Canton event source.
func NewCantonSource(client canton.Bridge, tokenContract, chainID string) Source {
	return &cantonSource{client: client, tokenContract: tokenContract, chainID: chainID}
}

// GetChainID returns the chain identifier.
func (s *cantonSource) GetChainID() string { return s.chainID }

// ExtractOffset extracts the Canton ledger offset from a processed event.
// Canton event IDs have the format "offset-nodeId" (e.g. "12345-0").
func (*cantonSource) ExtractOffset(event *Event) string {
	return cantonOffsetFromEventID(event.ID)
}

// StreamEvents streams Canton withdrawal events starting from offset.
// Reconnection and token refresh are handled internally by the Canton client.
func (s *cantonSource) StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error) {
	outCh := make(chan *Event)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		withdrawalCh := s.client.StreamWithdrawalEvents(ctx, offset)

		for {
			select {
			case withdrawal, ok := <-withdrawalCh:
				if !ok {
					// Only report an error when the stream closed without context cancellation.
					if ctx.Err() == nil {
						errCh <- errCantonStreamClosed
					}
					return
				}
				outCh <- &Event{
					ID:               withdrawal.EventID,
					TransactionID:    withdrawal.TransactionID,
					SourceChain:      ChainCanton,
					DestinationChain: ChainEthereum,
					SourceTxHash:     withdrawal.ContractID,
					SourceContractID: withdrawal.ContractID,
					TokenAddress:     s.tokenContract,
					Amount:           withdrawal.Amount,
					Sender:           withdrawal.UserParty,
					Recipient:        withdrawal.EvmDestination,
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, errCh
}

// ethereumSource implements Source for Ethereum.
type ethereumSource struct {
	client  EthereumBridgeClient
	config  *config.EthereumConfig
	chainID string
}

// NewEthereumSource creates a new Ethereum event source.
func NewEthereumSource(client EthereumBridgeClient, cfg *config.EthereumConfig, chainID string) Source {
	return &ethereumSource{client: client, config: cfg, chainID: chainID}
}

// GetChainID returns the chain identifier.
func (s *ethereumSource) GetChainID() string { return s.chainID }

// ExtractOffset returns the block number as a string offset.
// Returns "" when the event has no block number.
func (*ethereumSource) ExtractOffset(event *Event) string {
	if event.SourceBlockNumber <= 0 {
		return ""
	}
	return strconv.FormatInt(event.SourceBlockNumber, 10)
}

// StreamEvents streams Ethereum deposit events starting from the block encoded in offset.
func (s *ethereumSource) StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error) {
	outCh := make(chan *Event, 10) //nolint:mnd // small buffer to reduce blocking between poller and processor
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		var fromBlock uint64
		if offset != "" && offset != OffsetBegin {
			n, err := strconv.ParseUint(offset, 10, 64)
			if err != nil {
				errCh <- fmt.Errorf("invalid ethereum offset %q: %w", offset, err)
				return
			}
			fromBlock = n
		}

		err := s.client.WatchDepositEvents(ctx, fromBlock, func(event *ethereum.DepositEvent) error {
			relayerEvent := &Event{
				ID:                fmt.Sprintf("%s-%d", event.TxHash.Hex(), event.LogIndex),
				TransactionID:     event.TxHash.Hex(),
				SourceChain:       ChainEthereum,
				DestinationChain:  ChainCanton,
				SourceTxHash:      event.TxHash.Hex(),
				TokenAddress:      event.Token.Hex(),
				Amount:            event.Amount.String(),
				Sender:            event.Sender.Hex(),
				Recipient:         fmt.Sprintf("%x", event.CantonRecipient),
				Nonce:             event.Nonce.Int64(),
				SourceBlockNumber: int64(event.BlockNumber), //nolint:gosec // BlockNumber fits int64 in practice
			}

			select {
			case outCh <- relayerEvent:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		if err != nil && ctx.Err() == nil {
			errCh <- err
		}
	}()

	return outCh, errCh
}

// cantonOffsetFromEventID extracts the numeric ledger offset from a Canton event ID.
// Canton event IDs have the format "offset-nodeId" (e.g. "12345-0").
func cantonOffsetFromEventID(eventID string) string {
	parts := strings.SplitN(eventID, "-", 2)
	if len(parts) >= 1 {
		if _, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
			return parts[0]
		}
	}
	return ""
}
