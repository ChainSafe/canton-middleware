package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/ethereum/go-ethereum/common"
)

// CantonSource implements Source for Canton
type CantonSource struct {
	client CantonBridgeClient
}

func NewCantonSource(client CantonBridgeClient) *CantonSource {
	return &CantonSource{client: client}
}

func (s *CantonSource) GetChainID() string {
	return "canton"
}

func (s *CantonSource) StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error) {
	outCh := make(chan *Event)
	outErrCh := make(chan error)

	depositCh, errCh := s.client.StreamDeposits(ctx, offset)

	go func() {
		defer close(outCh)
		defer close(outErrCh)

		for {
			select {
			case deposit, ok := <-depositCh:
				if !ok {
					return
				}
				outCh <- &Event{
					ID:                deposit.EventID,
					TransactionID:     deposit.TransactionID,
					SourceChain:       "canton",
					DestinationChain:  "ethereum",
					SourceTxHash:      deposit.TransactionID,
					TokenAddress:      deposit.TokenSymbol,
					Amount:            deposit.Amount,
					Sender:            deposit.Depositor,
					Recipient:         deposit.EthRecipient,
					Nonce:             0, // Canton doesn't use nonces in the same way
					SourceBlockNumber: 0,
					Raw:               deposit,
				}
			case err := <-errCh:
				if err != nil {
					outErrCh <- err
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, outErrCh
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
	// TODO: Implement proper streaming/polling for Ethereum
	// For now, this is a placeholder that would wrap WatchDepositEvents or polling logic
	// Since the original code had polling logic in processEthereumEvents, we should ideally move that here
	// But for the sake of this refactor, we might need to adapt the polling loop to a channel-based stream

	outCh := make(chan *Event)
	outErrCh := make(chan error)

	// Note: In a real implementation, we would start a goroutine here to poll/watch
	// and push events to outCh.
	// The original code passed a handler to processEthBlock.
	// We'll leave this as a TODO or implement a basic poller if needed.

	return outCh, outErrCh
}

// CantonDestination implements Destination for Canton
type CantonDestination struct {
	client CantonBridgeClient
	config *config.EthereumConfig
}

func NewCantonDestination(client CantonBridgeClient, cfg *config.EthereumConfig) *CantonDestination {
	return &CantonDestination{client: client, config: cfg}
}

func (d *CantonDestination) GetChainID() string {
	return "canton"
}

func (d *CantonDestination) SubmitTransfer(ctx context.Context, event *Event) (string, error) {
	// Map generic event to WithdrawalRequest
	req := &canton.WithdrawalRequest{
		EthTxHash:   event.SourceTxHash,
		EthSender:   event.Sender,
		Recipient:   event.Recipient,
		TokenSymbol: event.TokenAddress,
		Amount:      canton.BigIntToDecimal(new(big.Int), 18), // TODO: Parse amount correctly
		Nonce:       event.Nonce,
		EthChainID:  d.config.ChainID,
	}

	// Parse amount
	amount := new(big.Int)
	amount.SetString(event.Amount, 10)
	req.Amount = canton.BigIntToDecimal(amount, 18)

	if err := d.client.SubmitWithdrawal(ctx, req); err != nil {
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
