package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"go.uber.org/zap"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
)

// CantonDestination implements Destination for the Canton ledger.
type CantonDestination struct {
	client  canton.Bridge
	chainID string
	logger  *zap.Logger
}

// NewCantonDestination creates a new Canton destination.
func NewCantonDestination(client canton.Bridge, chainID string) *CantonDestination {
	return &CantonDestination{
		client:  client,
		chainID: chainID,
		logger:  zap.NewNop(),
	}
}

// SetLogger sets the logger for the destination.
func (d *CantonDestination) SetLogger(l *zap.Logger) {
	d.logger = l
}

// GetChainID returns the chain identifier.
func (d *CantonDestination) GetChainID() string { return d.chainID }

// SubmitTransfer deposits EVM tokens on Canton (creates and processes a PendingDeposit).
func (d *CantonDestination) SubmitTransfer(ctx context.Context, event *Event) (string, bool, error) {
	const base10 = 10
	amount := new(big.Int)
	amount.SetString(event.Amount, base10)
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
func NewEthereumDestination(client EthereumBridgeClient, chainID string) *EthereumDestination {
	return &EthereumDestination{
		client:  client,
		chainID: chainID,
		logger:  zap.NewNop(),
	}
}

// SetLogger sets the logger for the destination.
func (d *EthereumDestination) SetLogger(l *zap.Logger) {
	d.logger = l
}

// GetChainID returns the chain identifier.
func (d *EthereumDestination) GetChainID() string { return d.chainID }

// SubmitTransfer releases tokens on Ethereum for a Canton withdrawal event.
func (d *EthereumDestination) SubmitTransfer(ctx context.Context, event *Event) (string, bool, error) {
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
		big.NewInt(event.Nonce),
		cantonTxHash,
	)
	if err != nil {
		return "", false, fmt.Errorf("withdraw from canton on EVM: %w", err)
	}

	return txHash.Hex(), false, nil
}

// bigIntToDecimal converts a wei big.Int to a Daml decimal string with the given decimal places.
// decimals must fit in int32 (caller-enforced: always decimalPlaces = 18).
func bigIntToDecimal(amount *big.Int, decimals int) string {
	d := decimal.NewFromBigInt(amount, -int32(decimals)) //nolint:gosec // decimals is always 18, fits int32
	return d.String()
}

// decimalToBigInt converts a Daml decimal string to a wei big.Int with the given decimal places.
// decimals must fit in int32 (caller-enforced: always decimalPlaces = 18).
func decimalToBigInt(s string, decimals int) (*big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid decimal format: %w", err)
	}
	return d.Mul(decimal.New(1, int32(decimals))).BigInt(), nil //nolint:gosec // decimals is always 18, fits int32
}
