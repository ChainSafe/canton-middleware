package miner

import (
	"context"
	"log/slog"
	"time"
)

// Store is the narrow data-access interface the miner needs.
type Store interface {
	MineBlock(ctx context.Context, chainID, gasLimit uint64) (int, error)
}

// Miner periodically attempts to mine a new synthetic EVM block from any
// completed mempool entries. Only one miner instance should run per process,
// but the underlying MineBlock implementation is safe for concurrent use.
type Miner struct {
	store    Store
	chainID  uint64
	gasLimit uint64
	interval time.Duration
}

// New creates a new Miner.
func New(store Store, chainID, gasLimit uint64, interval time.Duration) *Miner {
	return &Miner{
		store:    store,
		chainID:  chainID,
		gasLimit: gasLimit,
		interval: interval,
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
			n, err := m.store.MineBlock(ctx, m.chainID, m.gasLimit)
			if err != nil {
				slog.Error("ethrpc miner: MineBlock failed", "err", err)
				continue
			}
			if n > 0 {
				slog.Info("ethrpc miner: mined block", "txs", n)
			}
		}
	}
}
