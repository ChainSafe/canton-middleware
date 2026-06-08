// SPDX-License-Identifier: Apache-2.0

package miner

import (
	"context"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// transferEventTopic is the keccak256 hash of the ERC-20 Transfer event signature.
var transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// evmWordSize is the EVM word width (256 bits / 32 bytes). ABI-encoded topics
// and data segments are always left-padded to this size.
const evmWordSize = 32

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
	store          Store
	chainID        uint64
	gasLimit       uint64
	maxTxsPerBlock int
	interval       time.Duration
	metrics        *Metrics
	logger         *zap.Logger
}

// New creates a new Miner.
func New(store Store, chainID, gasLimit uint64, maxTxsPerBlock int, interval time.Duration, metrics *Metrics, logger *zap.Logger) *Miner {
	return &Miner{
		store:          store,
		chainID:        chainID,
		gasLimit:       gasLimit,
		maxTxsPerBlock: maxTxsPerBlock,
		interval:       interval,
		metrics:        metrics,
		logger:         logger,
	}
}

// Start runs the mining loop until ctx is canceled.
func (m *Miner) Start(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := m.mine(ctx); err != nil {
				m.logger.Error("ethrpc miner: mine failed", zap.Error(err))
			}
		}
	}
}

func (m *Miner) mine(ctx context.Context) (err error) {
	timer := prometheus.NewTimer(m.metrics.MineDuration)
	defer timer.ObserveDuration()
	defer func() {
		if err != nil {
			m.metrics.ErrorsTotal.Inc()
		}
	}()

	block, err := m.store.NewBlock(ctx, m.chainID)
	if err != nil {
		return err
	}
	defer block.Abort(ctx) //nolint:errcheck // safe: Abort is a no-op after Finalize

	entries, err := block.ClaimMempoolEntries(ctx, m.maxTxsPerBlock)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil // Abort via defer; block number is not consumed.
	}

	blockTimestamp := uint64(time.Now().Unix()) //nolint:gosec // time.Now() is always positive
	// logIndex is block-relative and contiguous across all logs in the block —
	// it must not skip when a failed tx emits zero logs. Track it separately
	// from txIndex so go-ethereum tooling that relies on contiguous indices
	// (e.g. for ordering or de-dup) behaves correctly.
	logIndex := uint(0)
	for i := range entries {
		e := &entries[i]
		txIndex := uint(i) //nolint:gosec // i is bounded by len(entries) which fits in uint
		succeeded := e.Status == ethrpc.MempoolCompleted

		evmTx := &ethrpc.EvmTransaction{
			TxHash:      e.TxHash,
			FromAddress: e.FromAddress,
			ToAddress:   e.ContractAddress,
			Nonce:       e.Nonce,
			Input:       e.Input,
			ValueWei:    "0",
			Status:      txStatus(succeeded),
			BlockNumber: block.Number(),
			BlockHash:   block.Hash(),
			TxIndex:     txIndex,
			GasUsed:     m.gasLimit,
		}
		if !succeeded {
			evmTx.ErrorMessage = e.ErrorMessage
		}
		if err = block.AddEvmTransaction(ctx, evmTx); err != nil {
			return err
		}

		// Failed transfers never executed on Canton, so they have no Transfer log.
		// Mining the EVM tx with status=0 surfaces the failure via getTransactionReceipt.
		if !succeeded {
			continue
		}
		if err = block.AddEvmLog(ctx, buildTransferLog(e, block, txIndex, logIndex, blockTimestamp)); err != nil {
			return err
		}
		logIndex++
	}

	if err = block.Finalize(ctx); err != nil {
		return err
	}

	m.metrics.BlocksMined.Inc()
	m.metrics.TransactionsMined.Add(float64(len(entries)))
	m.metrics.LatestBlock.Set(float64(block.Number()))
	m.logger.Info("ethrpc miner: mined block",
		zap.Uint64("number", block.Number()),
		zap.Int("txs", len(entries)),
	)
	return nil
}

// txStatus maps the entry outcome to the EVM-standard transaction receipt
// status (0x1 success, 0x0 failure) so MetaMask and other wallets render
// failed transfers correctly.
func txStatus(succeeded bool) uint8 {
	if succeeded {
		return 1
	}
	return 0
}

// buildTransferLog constructs the synthetic ERC-20 Transfer event log for a
// completed mempool entry. blockTimestamp is Unix seconds captured once per block.
// txIndex is the tx position in the block; logIndex is the *log* position in the
// block (block-relative, contiguous across all logs — the two diverge whenever
// a failed tx contributes a status=0 receipt with no logs).
func buildTransferLog(e *ethrpc.MempoolEntry, block ethrpc.PendingBlock, txIndex, logIndex uint, blockTimestamp uint64) *ethrpc.EvmLog {
	fromAddr := common.HexToAddress(e.FromAddress)
	toAddr := common.HexToAddress(e.RecipientAddress)
	fromTopic := common.BytesToHash(common.LeftPadBytes(fromAddr.Bytes(), evmWordSize))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), evmWordSize))
	amountData := common.LeftPadBytes(e.AmountData, evmWordSize)
	contractAddr := common.HexToAddress(e.ContractAddress)

	return &ethrpc.EvmLog{
		TxHash:         e.TxHash,
		LogIndex:       logIndex,
		Address:        contractAddr.Bytes(),
		Topics:         [][]byte{transferEventTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:           amountData,
		BlockNumber:    block.Number(),
		BlockHash:      block.Hash(),
		TxIndex:        txIndex,
		Removed:        false,
		BlockTimestamp: blockTimestamp,
	}
}
