package store

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// InstrumentedStore wraps a PGStore and records Prometheus metrics for every operation.
type InstrumentedStore struct {
	inner   *PGStore
	metrics *StoreMetrics
}

// NewInstrumentedStore returns a metrics-instrumented wrapper around the given PGStore.
func NewInstrumentedStore(inner *PGStore, metrics *StoreMetrics) *InstrumentedStore {
	return &InstrumentedStore{inner: inner, metrics: metrics}
}

func (s *InstrumentedStore) CreateTransfer(ctx context.Context, t *relayer.Transfer) (bool, error) {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("create_transfer"))
	defer timer.ObserveDuration()

	inserted, err := s.inner.CreateTransfer(ctx, t)
	if err != nil {
		s.metrics.Errors.WithLabelValues("create_transfer").Inc()
	}
	return inserted, err
}

func (s *InstrumentedStore) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("get_transfer"))
	defer timer.ObserveDuration()

	transfer, err := s.inner.GetTransfer(ctx, id)
	if err != nil {
		s.metrics.Errors.WithLabelValues("get_transfer").Inc()
	}
	return transfer, err
}

func (s *InstrumentedStore) UpdateTransferStatus(
	ctx context.Context,
	id string,
	status relayer.TransferStatus,
	destTxHash *string,
	errMsg *string,
) error {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("update_transfer_status"))
	defer timer.ObserveDuration()

	err := s.inner.UpdateTransferStatus(ctx, id, status, destTxHash, errMsg)
	if err != nil {
		s.metrics.Errors.WithLabelValues("update_transfer_status").Inc()
	}
	return err
}

func (s *InstrumentedStore) IncrementRetryCount(ctx context.Context, id string) error {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("increment_retry_count"))
	defer timer.ObserveDuration()

	err := s.inner.IncrementRetryCount(ctx, id)
	if err != nil {
		s.metrics.Errors.WithLabelValues("increment_retry_count").Inc()
	}
	return err
}

func (s *InstrumentedStore) GetChainState(ctx context.Context, chainID string) (*relayer.ChainState, error) {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("get_chain_state"))
	defer timer.ObserveDuration()

	state, err := s.inner.GetChainState(ctx, chainID)
	if err != nil {
		s.metrics.Errors.WithLabelValues("get_chain_state").Inc()
	}
	return state, err
}

func (s *InstrumentedStore) SetChainState(ctx context.Context, chainID string, blockNumber uint64, offset string) error {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("set_chain_state"))
	defer timer.ObserveDuration()

	err := s.inner.SetChainState(ctx, chainID, blockNumber, offset)
	if err != nil {
		s.metrics.Errors.WithLabelValues("set_chain_state").Inc()
	}
	return err
}

func (s *InstrumentedStore) GetPendingTransfers(ctx context.Context, direction relayer.TransferDirection) ([]*relayer.Transfer, error) {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("get_pending_transfers"))
	defer timer.ObserveDuration()

	transfers, err := s.inner.GetPendingTransfers(ctx, direction)
	if err != nil {
		s.metrics.Errors.WithLabelValues("get_pending_transfers").Inc()
	}
	return transfers, err
}

func (s *InstrumentedStore) ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error) {
	timer := prometheus.NewTimer(s.metrics.QueryDuration.WithLabelValues("list_transfers"))
	defer timer.ObserveDuration()

	transfers, err := s.inner.ListTransfers(ctx, limit)
	if err != nil {
		s.metrics.Errors.WithLabelValues("list_transfers").Inc()
	}
	return transfers, err
}
