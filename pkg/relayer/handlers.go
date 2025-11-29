package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// CantonSource implements Source for Canton
type CantonSource struct {
	client        CantonBridgeClient
	tokenContract string
}

func NewCantonSource(client CantonBridgeClient, tokenContract string) *CantonSource {
	return &CantonSource{
		client:        client,
		tokenContract: tokenContract,
	}
}

func (s *CantonSource) GetChainID() string {
	return "canton"
}

func (s *CantonSource) StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error) {
	outCh := make(chan *Event)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		burnCh, burnErrCh := s.client.StreamBurnEvents(ctx, offset)

		for {
			select {
			case burn, ok := <-burnCh:
				if !ok {
					return
				}
				outCh <- &Event{
					ID:                burn.EventID,
					TransactionID:     burn.TransactionID,
					SourceChain:       "canton",
					DestinationChain:  "ethereum",
					SourceTxHash:      burn.TransactionID,
					TokenAddress:      s.tokenContract,
					Amount:            burn.Amount,
					Sender:            burn.Owner,
					Recipient:         burn.Destination,
					Nonce:             0,
					SourceBlockNumber: 0,
					Raw:               burn,
				}
			case err := <-burnErrCh:
				select {
				case errCh <- err:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, errCh
}

// EthereumSource implements Source for Ethereum
type EthereumSource struct {
	client EthereumBridgeClient
	config *config.EthereumConfig
}

func NewEthereumSource(client EthereumBridgeClient, cfg *config.EthereumConfig) *EthereumSource {
	return &EthereumSource{client: client, config: cfg}
}

func (s *EthereumSource) GetChainID() string {
	return "ethereum"
}

func (s *EthereumSource) StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error) {
	outCh := make(chan *Event, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		var fromBlock uint64
		if offset != "" && offset != "BEGIN" {
			var err error
			fromBlock, err = strconv.ParseUint(offset, 10, 64)
			if err != nil {
				errCh <- fmt.Errorf("invalid offset: %w", err)
				return
			}
		}

		err := s.client.WatchDepositEvents(ctx, fromBlock, func(event *ethereum.DepositEvent) error {
			// Convert to generic Event
			relayerEvent := &Event{
				ID:                fmt.Sprintf("%s-%d", event.TxHash.Hex(), event.LogIndex),
				TransactionID:     event.TxHash.Hex(),
				SourceChain:       "ethereum",
				DestinationChain:  "canton",
				SourceTxHash:      event.TxHash.Hex(),
				TokenAddress:      event.Token.Hex(),
				Amount:            event.Amount.String(),
				Sender:            event.Sender.Hex(),
				Recipient:         fmt.Sprintf("%x", event.CantonRecipient),
				Nonce:             event.Nonce.Int64(),
				SourceBlockNumber: int64(event.BlockNumber),
				Raw:               event,
			}

			select {
			case outCh <- relayerEvent:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		if err != nil {
			// Check if it's just a context cancellation
			if ctx.Err() != nil {
				return
			}
			errCh <- err
		}
	}()

	return outCh, errCh
}

// CantonDestination implements Destination for Canton
type CantonDestination struct {
	client       CantonBridgeClient
	config       *config.EthereumConfig
	relayerParty string
}

func NewCantonDestination(client CantonBridgeClient, cfg *config.EthereumConfig, relayerParty string) *CantonDestination {
	return &CantonDestination{client: client, config: cfg, relayerParty: relayerParty}
}

func (d *CantonDestination) GetChainID() string {
	return "canton"
}

func (d *CantonDestination) SubmitTransfer(ctx context.Context, event *Event) (string, error) {
	// Map generic event to MintProposalRequest
	req := &canton.MintProposalRequest{
		Recipient: event.Recipient, // Canton party
		Amount:    canton.BigIntToDecimal(new(big.Int), 18),
		Reference: event.SourceTxHash,
	}

	// Parse amount
	amount := new(big.Int)
	amount.SetString(event.Amount, 10)
	req.Amount = canton.BigIntToDecimal(amount, 18)

	if err := d.client.SubmitMintProposal(ctx, req); err != nil {
		return "", err
	}

	// Canton doesn't return a tx hash for the submission itself in the same way,
	// but we can use the command ID or similar if available.
	// For now, return empty or a placeholder.
	return "submitted", nil
}

// EthereumDestination implements Destination for Ethereum
type EthereumDestination struct {
	client EthereumBridgeClient
}

func NewEthereumDestination(client EthereumBridgeClient) *EthereumDestination {
	return &EthereumDestination{client: client}
}

func (d *EthereumDestination) GetChainID() string {
	return "ethereum"
}

func (d *EthereumDestination) SubmitTransfer(ctx context.Context, event *Event) (string, error) {
	// Convert recipient
	cantonRecipientBytes := []byte(event.Recipient)
	var ethRecipient [32]byte
	copy(ethRecipient[:], cantonRecipientBytes)

	// Convert token
	tokenAddress := common.HexToAddress(event.TokenAddress)

	// Convert amount
	amount, err := canton.DecimalToBigInt(event.Amount, 18)
	if err != nil {
		return "", fmt.Errorf("failed to parse amount: %w", err)
	}

	// Convert tx hash
	cantonTxHashBytes, err := hex.DecodeString(event.SourceTxHash)
	if err != nil {
		return "", fmt.Errorf("failed to decode source tx hash: %w", err)
	}
	var cantonTxHash [32]byte
	copy(cantonTxHash[:], cantonTxHashBytes)

	// Submit
	ethRecipientAddr := common.BytesToAddress(ethRecipient[:20])
	txHash, err := d.client.WithdrawFromCanton(
		ctx,
		tokenAddress,
		ethRecipientAddr,
		amount,
		big.NewInt(0), // Nonce managed by contract/client
		cantonTxHash,
	)
	if err != nil {
		return "", err
	}

	return txHash.Hex(), nil
}
