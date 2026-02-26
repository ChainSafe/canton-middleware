package relayer

// Todo: split this into multiple files (source & destination) that will implement source and destination respectively.
// Current name 'handlers' doesn't represent clear context

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// CantonSource implements Source for Canton
type CantonSource struct {
	client        canton.Bridge
	tokenContract string
	chainID       string
}

func NewCantonSource(client canton.Bridge, tokenContract string, chainID string) *CantonSource {
	return &CantonSource{
		client:        client,
		tokenContract: tokenContract,
		chainID:       chainID,
	}
}

func (s *CantonSource) GetChainID() string {
	return s.chainID
}

func (s *CantonSource) StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error) {
	outCh := make(chan *Event)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		// StreamWithdrawalEvents handles reconnection and token refresh internally,
		// so errors are not propagated to errCh. The channel will simply close when
		// the context is cancelled or the stream terminates gracefully.
		withdrawalCh := s.client.StreamWithdrawalEvents(ctx, offset)

		for {
			select {
			case withdrawal, ok := <-withdrawalCh:
				if !ok {
					return
				}
				outCh <- &Event{
					ID:                withdrawal.EventID,
					TransactionID:     withdrawal.TransactionID,
					SourceChain:       "canton",   // todo: use constant
					DestinationChain:  "ethereum", // todo: use constant
					SourceTxHash:      withdrawal.ContractID,
					TokenAddress:      s.tokenContract,
					Amount:            withdrawal.Amount,
					Sender:            withdrawal.UserParty,
					Recipient:         withdrawal.EvmDestination,
					Nonce:             0,
					SourceBlockNumber: 0,
					Raw:               withdrawal,
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
	client  EthereumBridgeClient
	config  *config.EthereumConfig
	chainID string
}

func NewEthereumSource(client EthereumBridgeClient, cfg *config.EthereumConfig, chainID string) *EthereumSource {
	return &EthereumSource{client: client, config: cfg, chainID: chainID}
}

func (s *EthereumSource) GetChainID() string {
	return s.chainID
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
				SourceChain:       "ethereum", // todo: use constant
				DestinationChain:  "canton",   // todo: use constant
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
	client            canton.Bridge
	config            *config.EthereumConfig
	relayerParty      string
	chainID           string
	apiDB             *apidb.Store      // Optional: for total supply cache updates
	userBalanceCacher UserBalanceCacher // Optional: for user balance cache updates
}

func NewCantonDestination(client canton.Bridge, cfg *config.EthereumConfig, relayerParty string, chainID string) *CantonDestination {
	return &CantonDestination{client: client, config: cfg, relayerParty: relayerParty, chainID: chainID}
}

// SetAPIDB sets the API database store for total supply cache updates.
func (d *CantonDestination) SetAPIDB(apiDB *apidb.Store) {
	d.apiDB = apiDB
}

// SetUserBalanceCacher sets the user balance cache store.
func (d *CantonDestination) SetUserBalanceCacher(ubc UserBalanceCacher) {
	d.userBalanceCacher = ubc
}

func (d *CantonDestination) GetChainID() string {
	return d.chainID
}

func (d *CantonDestination) SubmitTransfer(ctx context.Context, event *Event) (string, error) {
	// Parse amount
	amount := new(big.Int)
	amount.SetString(event.Amount, 10)
	amountStr := bigIntToDecimal(amount, 18)

	// Defense in depth: Check if this deposit was already processed on Canton
	// This prevents duplicate deposits if multiple relayer instances are running
	alreadyProcessed, err := d.client.IsDepositProcessed(ctx, event.SourceTxHash)
	if err != nil {
		// Log warning but continue - we'll create the deposit anyway
		// If it's a duplicate, Canton will handle it or we'll get an error
		_ = err // TODO: log this warning
	} else if alreadyProcessed {
		// Return successfully - this deposit was already processed
		return fmt.Sprintf("already-processed:%s", event.SourceTxHash), nil
	}

	// Step 1: Create Pending Deposit on canton Bridge
	depositReq := canton.CreatePendingDepositRequest{
		Fingerprint: event.Recipient,
		Amount:      amountStr,
		EvmTxHash:   event.SourceTxHash,
	}

	pendingDeposit, err := d.client.CreatePendingDeposit(ctx, depositReq)
	if err != nil {
		return "", fmt.Errorf("failed to create pending deposit: %w", err)
	}

	// Step 2: Process deposit and mint(unlock tokens on Canton side)
	processReq := canton.ProcessDepositRequest{
		DepositCID: pendingDeposit.ContractID,
		MappingCID: pendingDeposit.MappingCID,
	}

	deposit, err := d.client.ProcessDepositAndMint(ctx, processReq)
	if err != nil {
		return "", fmt.Errorf("failed to process deposit and mint: %w", err)
	}

	// Step 3: Update balance cache if configured
	if d.userBalanceCacher != nil {
		if err = d.userBalanceCacher.IncrementBalanceByFingerprint(ctx, pendingDeposit.Fingerprint, amountStr, token.Prompt); err != nil {
			// Log but don't fail - the deposit succeeded on Canton
			fmt.Printf("WARN: Failed to update prompt balance cache for %s: %v\n", pendingDeposit.Fingerprint, err)
		}
	}
	if d.apiDB != nil {
		if err = d.apiDB.IncrementTotalSupply("PROMPT", amountStr); err != nil {
			fmt.Printf("WARN: Failed to update total supply cache: %v\n", err)
		}
	}

	return deposit.ContractID, nil
}

// bigIntToDecimal converts big.Int to Daml decimal string
func bigIntToDecimal(amount *big.Int, decimals int) string {
	d := decimal.NewFromBigInt(amount, int32(-decimals))
	return d.String()
}

// EthereumDestination implements Destination for Ethereum
type EthereumDestination struct {
	client            EthereumBridgeClient
	cantonClient      canton.Bridge
	chainID           string
	apiDB             *apidb.Store      // Optional: for total supply cache updates
	userBalanceCacher UserBalanceCacher // Optional: for user balance cache updates
}

func NewEthereumDestination(client EthereumBridgeClient, cantonClient canton.Bridge, chainID string) *EthereumDestination {
	return &EthereumDestination{client: client, cantonClient: cantonClient, chainID: chainID}
}

// SetUserBalanceCacher sets the user balance cache store.
func (d *EthereumDestination) SetUserBalanceCacher(ubc UserBalanceCacher) {
	d.userBalanceCacher = ubc
}

// SetAPIDB sets the API database store for total supply cache updates.
func (d *EthereumDestination) SetAPIDB(apiDB *apidb.Store) {
	d.apiDB = apiDB
}

func (d *EthereumDestination) GetChainID() string {
	return d.chainID
}

func (d *EthereumDestination) SubmitTransfer(ctx context.Context, event *Event) (string, error) {
	// For withdrawal events, recipient is the EVM address
	ethRecipientAddr := common.HexToAddress(event.Recipient)

	// Convert token
	tokenAddress := common.HexToAddress(event.TokenAddress)

	// Convert amount
	amount, err := decimalToBigInt(event.Amount, 18)
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

	// Defense in depth: Check if this withdrawal was already processed on EVM
	// This prevents wasting gas on duplicate withdrawals that would revert anyway
	alreadyProcessed, err := d.client.IsWithdrawalProcessed(ctx, cantonTxHash)
	if err != nil {
		// Log warning but continue - the contract will reject duplicates anyway
		// This is just an optimization to avoid wasting gas
		_ = err // TODO: log this warning
	} else if alreadyProcessed {
		// Return successfully - this withdrawal was already processed
		// The contract ID serves as a pseudo tx hash since we don't have the original
		return fmt.Sprintf("already-processed:%s", event.SourceTxHash), nil
	}

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
		completeReq := canton.CompleteWithdrawalRequest{
			WithdrawalEventCID: withdrawal.ContractID,
			EvmTxHash:          txHash.Hex(),
		}
		if err = d.cantonClient.CompleteWithdrawal(ctx, completeReq); err != nil {
			// Log but don't fail - the EVM transfer succeeded
			// This can be reconciled later via cleanup script
			fmt.Printf("WARN: Failed to mark withdrawal complete on Canton (EVM succeeded): %v\n", err)
		}

		// Update balance cache if configured
		if d.userBalanceCacher != nil {
			// Decrement user PROMPT balance using EVM destination address from withdrawal event
			// Note: withdrawal.Fingerprint is the Canton party fingerprint, not the user's EVM fingerprint
			if err := d.userBalanceCacher.DecrementBalanceByEVMAddress(ctx, withdrawal.EvmDestination, event.Amount, token.Prompt); err != nil {
				// Log but don't fail - the withdrawal succeeded
				fmt.Printf("WARN: Failed to update prompt balance cache for %s: %v\n", withdrawal.EvmDestination, err)
			}
		}
		if d.apiDB != nil {
			// Decrement total supply for PROMPT (tokens leaving Canton system)
			if err := d.apiDB.DecrementTotalSupply("PROMPT", event.Amount); err != nil {
				fmt.Printf("WARN: Failed to update total supply cache: %v\n", err)
			}
		}
	}

	return txHash.Hex(), nil
}

// decimalToBigInt converts Daml decimal string to big.Int
func decimalToBigInt(s string, decimals int) (*big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid decimal format: %w", err)
	}
	d = d.Mul(decimal.New(1, int32(decimals)))
	return d.BigInt(), nil
}
