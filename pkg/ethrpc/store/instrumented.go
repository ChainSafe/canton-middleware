package store

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/prometheus/client_golang/prometheus"
)

// InstrumentedStore wraps a PGStore and records Prometheus metrics for every
// database operation. It satisfies both ethrpc/service.Store and ethrpc/miner.Store.
type InstrumentedStore struct {
	inner   *PGStore
	metrics *StoreMetrics
}

// NewInstrumentedStore returns a metrics-instrumented wrapper around the given PGStore.
func NewInstrumentedStore(inner *PGStore, metrics *StoreMetrics) *InstrumentedStore {
	return &InstrumentedStore{inner: inner, metrics: metrics}
}

func (s *InstrumentedStore) NewBlock(ctx context.Context, chainID uint64) (ethrpc.PendingBlock, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpNewBlock))
	defer timer.ObserveDuration()

	b, err := s.inner.NewBlock(ctx, chainID)
	if err != nil {
		s.metrics.IncErrors(OpNewBlock)
		return nil, err
	}
	return &instrumentedPendingBlock{inner: b, metrics: s.metrics}, nil
}

// instrumentedPendingBlock wraps ethrpc.PendingBlock and records metrics for
// each DB operation executed within the block's transaction.
type instrumentedPendingBlock struct {
	inner   ethrpc.PendingBlock
	metrics *StoreMetrics
}

func (b *instrumentedPendingBlock) Number() uint64 { return b.inner.Number() }
func (b *instrumentedPendingBlock) Hash() []byte   { return b.inner.Hash() }

func (b *instrumentedPendingBlock) ClaimMempoolEntries(ctx context.Context, maxTxsPerBlock int) ([]ethrpc.MempoolEntry, error) {
	timer := prometheus.NewTimer(b.metrics.ObserveQueryDuration(OpClaimMempoolEntries))
	defer timer.ObserveDuration()

	entries, err := b.inner.ClaimMempoolEntries(ctx, maxTxsPerBlock)
	if err != nil {
		b.metrics.IncErrors(OpClaimMempoolEntries)
	}
	return entries, err
}

func (b *instrumentedPendingBlock) AddEvmTransaction(ctx context.Context, tx *ethrpc.EvmTransaction) error {
	timer := prometheus.NewTimer(b.metrics.ObserveQueryDuration(OpAddEvmTransaction))
	defer timer.ObserveDuration()

	err := b.inner.AddEvmTransaction(ctx, tx)
	if err != nil {
		b.metrics.IncErrors(OpAddEvmTransaction)
	}
	return err
}

func (b *instrumentedPendingBlock) AddEvmLog(ctx context.Context, log *ethrpc.EvmLog) error {
	timer := prometheus.NewTimer(b.metrics.ObserveQueryDuration(OpAddEvmLog))
	defer timer.ObserveDuration()

	err := b.inner.AddEvmLog(ctx, log)
	if err != nil {
		b.metrics.IncErrors(OpAddEvmLog)
	}
	return err
}

func (b *instrumentedPendingBlock) Finalize(ctx context.Context) error {
	timer := prometheus.NewTimer(b.metrics.ObserveQueryDuration(OpFinalize))
	defer timer.ObserveDuration()

	err := b.inner.Finalize(ctx)
	if err != nil {
		b.metrics.IncErrors(OpFinalize)
	}
	return err
}

func (b *instrumentedPendingBlock) Abort(ctx context.Context) error {
	timer := prometheus.NewTimer(b.metrics.ObserveQueryDuration(OpAbort))
	defer timer.ObserveDuration()

	err := b.inner.Abort(ctx)
	if err != nil {
		b.metrics.IncErrors(OpAbort)
	}
	return err
}

func (s *InstrumentedStore) GetLatestEvmBlockNumber(ctx context.Context) (uint64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetLatestEvmBlockNumber))
	defer timer.ObserveDuration()

	n, err := s.inner.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		s.metrics.IncErrors(OpGetLatestEvmBlockNumber)
	}
	return n, err
}

func (s *InstrumentedStore) GetEvmTransactionCount(ctx context.Context, fromAddress string) (uint64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetEvmTransactionCount))
	defer timer.ObserveDuration()

	n, err := s.inner.GetEvmTransactionCount(ctx, fromAddress)
	if err != nil {
		s.metrics.IncErrors(OpGetEvmTransactionCount)
	}
	return n, err
}

func (s *InstrumentedStore) GetEvmTransaction(ctx context.Context, txHash []byte) (*ethrpc.EvmTransaction, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetEvmTransaction))
	defer timer.ObserveDuration()

	tx, err := s.inner.GetEvmTransaction(ctx, txHash)
	if err != nil {
		s.metrics.IncErrors(OpGetEvmTransaction)
	}
	return tx, err
}

func (s *InstrumentedStore) GetEvmLogsByTxHash(ctx context.Context, txHash []byte) ([]*ethrpc.EvmLog, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetEvmLogsByTxHash))
	defer timer.ObserveDuration()

	logs, err := s.inner.GetEvmLogsByTxHash(ctx, txHash)
	if err != nil {
		s.metrics.IncErrors(OpGetEvmLogsByTxHash)
	}
	return logs, err
}

func (s *InstrumentedStore) GetEvmLogs(
	ctx context.Context, address []byte, topic0 []byte, fromBlock, toBlock uint64,
) ([]*ethrpc.EvmLog, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetEvmLogs))
	defer timer.ObserveDuration()

	logs, err := s.inner.GetEvmLogs(ctx, address, topic0, fromBlock, toBlock)
	if err != nil {
		s.metrics.IncErrors(OpGetEvmLogs)
	}
	return logs, err
}

func (s *InstrumentedStore) GetBlockNumberByHash(ctx context.Context, blockHash []byte) (uint64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetBlockNumberByHash))
	defer timer.ObserveDuration()

	n, err := s.inner.GetBlockNumberByHash(ctx, blockHash)
	if err != nil {
		s.metrics.IncErrors(OpGetBlockNumberByHash)
	}
	return n, err
}

func (s *InstrumentedStore) InsertMempoolEntry(ctx context.Context, entry *ethrpc.MempoolEntry) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpInsertMempoolEntry))
	defer timer.ObserveDuration()

	err := s.inner.InsertMempoolEntry(ctx, entry)
	if err != nil {
		s.metrics.IncErrors(OpInsertMempoolEntry)
	}
	return err
}

func (s *InstrumentedStore) CompleteMempoolEntry(ctx context.Context, txHash []byte) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpCompleteMempoolEntry))
	defer timer.ObserveDuration()

	err := s.inner.CompleteMempoolEntry(ctx, txHash)
	if err != nil {
		s.metrics.IncErrors(OpCompleteMempoolEntry)
	}
	return err
}

func (s *InstrumentedStore) FailMempoolEntry(ctx context.Context, txHash []byte, errMsg string) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpFailMempoolEntry))
	defer timer.ObserveDuration()

	err := s.inner.FailMempoolEntry(ctx, txHash, errMsg)
	if err != nil {
		s.metrics.IncErrors(OpFailMempoolEntry)
	}
	return err
}
