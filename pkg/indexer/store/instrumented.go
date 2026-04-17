package store

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine"
)

// InstrumentedStore wraps a PGStore and records Prometheus metrics for every
// database operation. It satisfies both engine.Store (write path: processor)
// and service.Store (read path: HTTP API).
type InstrumentedStore struct {
	inner   *PGStore
	metrics *StoreMetrics
}

// NewInstrumentedStore returns a metrics-instrumented wrapper around the given PGStore.
func NewInstrumentedStore(inner *PGStore, metrics *StoreMetrics) *InstrumentedStore {
	return &InstrumentedStore{inner: inner, metrics: metrics}
}

// ── engine.Store (write path) ────────────────────────────────────────────────

func (s *InstrumentedStore) LatestOffset(ctx context.Context) (int64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpLatestOffset))
	defer timer.ObserveDuration()

	offset, err := s.inner.LatestOffset(ctx)
	if err != nil {
		s.metrics.IncErrors(OpLatestOffset)
	}
	return offset, err
}

// RunInTx wraps the inner RunInTx so that operations executed inside the
// transaction are also instrumented. The fn receives an *InstrumentedStore
// backed by the transaction-scoped *PGStore.
func (s *InstrumentedStore) RunInTx(ctx context.Context, fn func(ctx context.Context, tx engine.Store) error) error {
	return s.inner.RunInTx(ctx, func(ctx context.Context, txStore engine.Store) error {
		return fn(ctx, &InstrumentedStore{
			inner:   txStore.(*PGStore),
			metrics: s.metrics,
		})
	})
}

func (s *InstrumentedStore) InsertEvent(ctx context.Context, event *indexer.ParsedEvent) (bool, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpInsertEvent))
	defer timer.ObserveDuration()

	inserted, err := s.inner.InsertEvent(ctx, event)
	if err != nil {
		s.metrics.IncErrors(OpInsertEvent)
	}
	return inserted, err
}

func (s *InstrumentedStore) SaveOffset(ctx context.Context, offset int64) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpSaveOffset))
	defer timer.ObserveDuration()

	err := s.inner.SaveOffset(ctx, offset)
	if err != nil {
		s.metrics.IncErrors(OpSaveOffset)
	}
	return err
}

func (s *InstrumentedStore) UpsertToken(ctx context.Context, token *indexer.Token) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpUpsertToken))
	defer timer.ObserveDuration()

	err := s.inner.UpsertToken(ctx, token)
	if err != nil {
		s.metrics.IncErrors(OpUpsertToken)
	}
	return err
}

func (s *InstrumentedStore) ApplyBalanceDelta(ctx context.Context, partyID, instrumentAdmin, instrumentID, delta string) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpApplyBalanceDelta))
	defer timer.ObserveDuration()

	err := s.inner.ApplyBalanceDelta(ctx, partyID, instrumentAdmin, instrumentID, delta)
	if err != nil {
		s.metrics.IncErrors(OpApplyBalanceDelta)
	}
	return err
}

func (s *InstrumentedStore) ApplySupplyDelta(ctx context.Context, instrumentAdmin, instrumentID, delta string) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpApplySupplyDelta))
	defer timer.ObserveDuration()

	err := s.inner.ApplySupplyDelta(ctx, instrumentAdmin, instrumentID, delta)
	if err != nil {
		s.metrics.IncErrors(OpApplySupplyDelta)
	}
	return err
}

// ── service.Store (read path) ────────────────────────────────────────────────

func (s *InstrumentedStore) GetToken(ctx context.Context, admin, id string) (*indexer.Token, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetToken))
	defer timer.ObserveDuration()

	token, err := s.inner.GetToken(ctx, admin, id)
	if err != nil {
		s.metrics.IncErrors(OpGetToken)
	}
	return token, err
}

func (s *InstrumentedStore) ListTokens(ctx context.Context, p indexer.Pagination) ([]*indexer.Token, int64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpListTokens))
	defer timer.ObserveDuration()

	tokens, total, err := s.inner.ListTokens(ctx, p)
	if err != nil {
		s.metrics.IncErrors(OpListTokens)
	}
	return tokens, total, err
}

func (s *InstrumentedStore) GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetBalance))
	defer timer.ObserveDuration()

	balance, err := s.inner.GetBalance(ctx, partyID, admin, id)
	if err != nil {
		s.metrics.IncErrors(OpGetBalance)
	}
	return balance, err
}

func (s *InstrumentedStore) ListBalancesForParty(
	ctx context.Context, partyID string, p indexer.Pagination,
) ([]*indexer.Balance, int64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpListBalancesForParty))
	defer timer.ObserveDuration()

	balances, total, err := s.inner.ListBalancesForParty(ctx, partyID, p)
	if err != nil {
		s.metrics.IncErrors(OpListBalancesForParty)
	}
	return balances, total, err
}

func (s *InstrumentedStore) ListBalancesForToken(
	ctx context.Context, admin, id string, p indexer.Pagination,
) ([]*indexer.Balance, int64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpListBalancesForToken))
	defer timer.ObserveDuration()

	balances, total, err := s.inner.ListBalancesForToken(ctx, admin, id, p)
	if err != nil {
		s.metrics.IncErrors(OpListBalancesForToken)
	}
	return balances, total, err
}

func (s *InstrumentedStore) GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetEvent))
	defer timer.ObserveDuration()

	event, err := s.inner.GetEvent(ctx, contractID)
	if err != nil {
		s.metrics.IncErrors(OpGetEvent)
	}
	return event, err
}

func (s *InstrumentedStore) ListEvents(
	ctx context.Context, f indexer.EventFilter, p indexer.Pagination,
) ([]*indexer.ParsedEvent, int64, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpListEvents))
	defer timer.ObserveDuration()

	events, total, err := s.inner.ListEvents(ctx, f, p)
	if err != nil {
		s.metrics.IncErrors(OpListEvents)
	}
	return events, total, err
}
