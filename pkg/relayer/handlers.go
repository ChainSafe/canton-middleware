package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"time"

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

		// Use the new issuer-centric StreamWithdrawalEvents
		withdrawalCh, withdrawalErrCh := s.client.StreamWithdrawalEvents(ctx, offset)

		for {
			select {
			case withdrawal, ok := <-withdrawalCh:
				if !ok {
					return
				}
				outCh <- &Event{
					ID:                withdrawal.EventID,
					TransactionID:     withdrawal.TransactionID,
					SourceChain:       "canton",
					DestinationChain:  "ethereum",
					SourceTxHash:      withdrawal.TransactionID,
					TokenAddress:      s.tokenContract,
					Amount:            withdrawal.Amount,
					Sender:            withdrawal.UserParty,
					Recipient:         withdrawal.EvmDestination,
					Nonce:             0,
					SourceBlockNumber: 0,
					Raw:               withdrawal,
				}
			case err := <-withdrawalErrCh:
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
	// The recipient from EVM is a fingerprint (bytes32 as hex)
	fingerprint := event.Recipient

	// Parse amount
	amount := new(big.Int)
	amount.SetString(event.Amount, 10)
	amountStr := canton.BigIntToDecimal(amount, 18)

	// Step 1: Create PendingDeposit from EVM event
	depositReq := &canton.CreatePendingDepositRequest{
		Fingerprint: fingerprint,
		Amount:      amountStr,
		EvmTxHash:   event.SourceTxHash,
		Timestamp:   time.Now(),
	}

	depositCid, err := d.client.CreatePendingDeposit(ctx, depositReq)
	if err != nil {
		return "", fmt.Errorf("failed to create pending deposit: %w", err)
	}

	// Step 2: Look up FingerprintMapping by fingerprint
	mapping, err := d.client.GetFingerprintMapping(ctx, fingerprint)
	if err != nil {
		return "", fmt.Errorf("failed to get fingerprint mapping for %s: %w", fingerprint, err)
	}

	// Step 3: Process deposit (unlock tokens on Canton side)
	processReq := &canton.ProcessDepositRequest{
		DepositCid: depositCid,
		MappingCid: mapping.ContractID,
	}

	holdingCid, err := d.client.ProcessDeposit(ctx, processReq)
	if err != nil {
		return "", fmt.Errorf("failed to process deposit: %w", err)
	}

	return holdingCid, nil
}

// EthereumDestination implements Destination for Ethereum
type EthereumDestination struct {
	client       EthereumBridgeClient
	cantonClient CantonBridgeClient
}

func NewEthereumDestination(client EthereumBridgeClient, cantonClient CantonBridgeClient) *EthereumDestination {
	return &EthereumDestination{client: client, cantonClient: cantonClient}
}

func (d *EthereumDestination) GetChainID() string {
	return "ethereum"
}

func (d *EthereumDestination) SubmitTransfer(ctx context.Context, event *Event) (string, error) {
	// For withdrawal events, recipient is the EVM address
	ethRecipientAddr := common.HexToAddress(event.Recipient)

	// Convert token
	tokenAddress := common.HexToAddress(event.TokenAddress)

	// Convert amount
	amount, err := canton.DecimalToBigInt(event.Amount, 18)
	if err != nil {
		return "", fmt.Errorf("failed to parse amount: %w", err)
	}

	// Convert Canton tx hash to bytes32 for idempotency
	cantonTxHashBytes, err := hex.DecodeString(event.SourceTxHash)
	if err != nil {
		return "", fmt.Errorf("failed to decode source tx hash: %w", err)
	}
	var cantonTxHash [32]byte
	copy(cantonTxHash[:], cantonTxHashBytes)

	// Submit withdrawal to EVM
	txHash, err := d.client.WithdrawFromCanton(
		ctx,
		tokenAddress,
		ethRecipientAddr,
		amount,
		big.NewInt(0), // Nonce managed by contract/client
		cantonTxHash,
	)
	if err != nil {
		return "", fmt.Errorf("failed to withdraw from Canton on EVM: %w", err)
	}

	// Mark withdrawal as complete on Canton (if we have the withdrawal event)
	if withdrawal, ok := event.Raw.(*canton.WithdrawalEvent); ok && d.cantonClient != nil {
		completeReq := &canton.CompleteWithdrawalRequest{
			WithdrawalEventCid: withdrawal.ContractID,
			EvmTxHash:          txHash.Hex(),
		}
		if err := d.cantonClient.CompleteWithdrawal(ctx, completeReq); err != nil {
			// Log but don't fail - the EVM transfer succeeded
			// This can be reconciled later
			_ = err // TODO: log this error
		}
	}

	return txHash.Hex(), nil
}
