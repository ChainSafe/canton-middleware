package miner

import (
	"context"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

// transferEventTopic is the keccak256 hash of the ERC-20 Transfer event signature.
var transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// Store is the narrow data-access interface the miner needs.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/ethrpc --name PendingBlock --output mocks --outpkg mocks --filename mock_pending_block.go --with-expecter
type Store interface {
	// NewBlock opens a DB transaction and acquires an exclusive lock on the
	// evm_state row, serializing concurrent miners. The lock is held until
	// Finalize or Abort is called on the returned PendingBlock.
	NewBlock(ctx context.Context, chainID uint64) (ethrpc.PendingBlock, error)
}

// Miner periodically claims completed mempool entries and commits them as a
// synthetic EVM block. Business logic for EVM data construction lives here;
// the Store only performs raw SQL operations.
type Miner struct {
	store    Store
	chainID  uint64
	gasLimit uint64
	interval time.Duration
	logger   *zap.Logger
}

// New creates a new Miner.
func New(store Store, chainID, gasLimit uint64, interval time.Duration, logger *zap.Logger) *Miner {
	return &Miner{
		store:    store,
		chainID:  chainID,
		gasLimit: gasLimit,
		interval: interval,
		logger:   logger,
	}
}

// Start runs the mining loop until ctx is cancelled.
func (m *Miner) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.mine(ctx); err != nil {
				m.logger.Error("ethrpc miner: mine failed", zap.Error(err))
			}
		}
	}
}

func (m *Miner) mine(ctx context.Context) error {
	block, err := m.store.NewBlock(ctx, m.chainID)
	if err != nil {
		return err
	}
	defer block.Abort(ctx) //nolint:errcheck // safe: Abort is a no-op after Finalize

	entries, err := block.ClaimMempoolEntries(ctx)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil // Abort via defer; block number is not consumed.
	}

	for i := range entries {
		e := &entries[i]
		txIndex := uint(i)

		evmTx := &ethrpc.EvmTransaction{
			TxHash:      e.TxHash,
			FromAddress: e.FromAddress,
			ToAddress:   e.ContractAddress,
			Nonce:       e.Nonce,
			Input:       e.Input,
			ValueWei:    "0",
			Status:      1,
			BlockNumber: block.Number(),
			BlockHash:   block.Hash(),
			TxIndex:     txIndex,
			GasUsed:     m.gasLimit,
		}
		if err = block.AddEvmTransaction(ctx, evmTx); err != nil {
			return err
		}

		if err = block.AddEvmLog(ctx, buildTransferLog(e, block, txIndex)); err != nil {
			return err
		}
	}

	if err = block.Finalize(ctx); err != nil {
		return err
	}

	m.logger.Info("ethrpc miner: mined block",
		zap.Uint64("number", block.Number()),
		zap.Int("txs", len(entries)),
	)
	return nil
}

// buildTransferLog constructs the synthetic ERC-20 Transfer event log for a
// completed mempool entry.
func buildTransferLog(e *ethrpc.MempoolEntry, block ethrpc.PendingBlock, txIndex uint) *ethrpc.EvmLog {
	fromAddr := common.HexToAddress(e.FromAddress)
	toAddr := common.HexToAddress(e.RecipientAddress)
	fromTopic := common.BytesToHash(common.LeftPadBytes(fromAddr.Bytes(), 32))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), 32))
	amountData := common.LeftPadBytes(e.AmountData, 32)
	contractAddr := common.HexToAddress(e.ContractAddress)

	return &ethrpc.EvmLog{
		TxHash:      e.TxHash,
		LogIndex:    txIndex,
		Address:     contractAddr.Bytes(),
		Topics:      [][]byte{transferEventTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:        amountData,
		BlockNumber: block.Number(),
		BlockHash:   block.Hash(),
		TxIndex:     txIndex,
		Removed:     false,
	}
}
