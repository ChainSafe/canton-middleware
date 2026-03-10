package engine

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"go.uber.org/zap"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const decimalPlaces = 18

// CantonDestination implements Destination for the Canton ledger.
type CantonDestination struct {
	client  canton.Bridge
	chainID string
	logger  *zap.Logger
}

// NewCantonDestination creates a new Canton destination.
func NewCantonDestination(client canton.Bridge, chainID string, logger *zap.Logger) *CantonDestination {
	return &CantonDestination{
		client:  client,
		chainID: chainID,
		logger:  logger,
	}
}

// GetChainID returns the chain identifier.
func (d *CantonDestination) GetChainID() string { return d.chainID }

// SubmitTransfer deposits EVM tokens on Canton (creates and processes a PendingDeposit).
func (d *CantonDestination) SubmitTransfer(ctx context.Context, event *relayer.Event) (string, bool, error) {
	const base10 = 10
	amount, ok := new(big.Int).SetString(event.Amount, base10)
	if !ok {
		return "", false, fmt.Errorf("invalid amount %q", event.Amount)
	}
	amountStr := bigIntToDecimal(amount, decimalPlaces)

	alreadyProcessed, err := d.client.IsDepositProcessed(ctx, event.SourceTxHash)
	if err != nil {
		d.logger.Warn("Failed to check if deposit is already processed, continuing",
			zap.String("source_tx_hash", event.SourceTxHash), zap.Error(err))
	} else if alreadyProcessed {
		return "", true, nil
	}

	pendingDeposit, err := d.client.CreatePendingDeposit(ctx, canton.CreatePendingDepositRequest{
		Fingerprint: event.Recipient,
		Amount:      amountStr,
		EvmTxHash:   event.SourceTxHash,
	})
	if err != nil {
		return "", false, fmt.Errorf("create pending deposit: %w", err)
	}

	deposit, err := d.client.ProcessDepositAndMint(ctx, canton.ProcessDepositRequest{
		DepositCID: pendingDeposit.ContractID,
		MappingCID: pendingDeposit.MappingCID,
	})
	if err != nil {
		return "", false, fmt.Errorf("process deposit and mint: %w", err)
	}

	return deposit.ContractID, false, nil
}

// EthereumDestination implements Destination for Ethereum.
type EthereumDestination struct {
	client  EthereumBridgeClient
	chainID string
	logger  *zap.Logger
}

// NewEthereumDestination creates a new Ethereum destination.
func NewEthereumDestination(client EthereumBridgeClient, chainID string, logger *zap.Logger) *EthereumDestination {
	return &EthereumDestination{
		client:  client,
		chainID: chainID,
		logger:  logger,
	}
}

// GetChainID returns the chain identifier.
func (d *EthereumDestination) GetChainID() string { return d.chainID }

// SubmitTransfer releases tokens on Ethereum for a Canton withdrawal event.
func (d *EthereumDestination) SubmitTransfer(ctx context.Context, event *relayer.Event) (string, bool, error) {
	tokenAddress := common.HexToAddress(event.TokenAddress)
	recipientAddr := common.HexToAddress(event.Recipient)

	amount, err := decimalToBigInt(event.Amount, decimalPlaces)
	if err != nil {
		return "", false, fmt.Errorf("parse amount: %w", err)
	}

	cantonTxHashBytes, err := hex.DecodeString(event.SourceTxHash)
	if err != nil {
		return "", false, fmt.Errorf("decode source tx hash: %w", err)
	}
	var cantonTxHash [32]byte
	copy(cantonTxHash[:], cantonTxHashBytes)

	alreadyProcessed, err := d.client.IsWithdrawalProcessed(ctx, cantonTxHash)
	if err != nil {
		d.logger.Warn("Failed to check if withdrawal is already processed, continuing",
			zap.String("source_tx_hash", event.SourceTxHash), zap.Error(err))
	} else if alreadyProcessed {
		return "", true, nil
	}

	txHash, err := d.client.WithdrawFromCanton(
		ctx,
		tokenAddress,
		recipientAddr,
		amount,
		// Use the nonce carried by the Canton event. It is currently 0 because the source
		// does not populate it yet, but forwarding event.Nonce keeps us aligned with the
		// source payload once nonce population is enabled upstream.
		big.NewInt(event.Nonce),
		cantonTxHash,
	)
	if err != nil {
		return "", false, fmt.Errorf("withdraw from canton on EVM: %w", err)
	}

	return txHash.Hex(), false, nil
}

// bigIntToDecimal converts a wei big.Int to a Daml decimal string with the given decimal places.
func bigIntToDecimal(amount *big.Int, decimals int32) string {
	d := decimal.NewFromBigInt(amount, -decimals)
	return d.String()
}

// decimalToBigInt converts a Daml decimal string to a wei big.Int with the given decimal places.
func decimalToBigInt(s string, decimals int32) (*big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid decimal format: %w", err)
	}
	return d.Mul(decimal.New(1, int32(decimals))).BigInt(), nil
}
