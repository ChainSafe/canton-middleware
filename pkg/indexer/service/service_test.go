package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/service"
	"github.com/chainsafe/canton-middleware/pkg/indexer/service/mocks"
)

const (
	alice = "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7"
	admin = "admin-party::deadbeef"
)

func newSvc(t *testing.T) (service.Service, *mocks.Store) {
	t.Helper()
	store := mocks.NewStore(t)
	return service.NewService(store, zap.NewNop()), store
}

// ─── GetToken ─────────────────────────────────────────────────────────────────

func TestSvc_GetToken(t *testing.T) {
	token := &indexer.Token{InstrumentAdmin: admin, InstrumentID: "DEMO", TotalSupply: "100.0"}

	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(token, nil)

		got, err := svc.GetToken(context.Background(), admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, token, got)
	})

	t.Run("store returns nil → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(nil, nil)

		_, err := svc.GetToken(context.Background(), admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(nil, errors.New("db error"))

		_, err := svc.GetToken(context.Background(), admin, "DEMO")
		require.Error(t, err)
	})
}

// ─── ListTokens ───────────────────────────────────────────────────────────────

func TestSvc_ListTokens(t *testing.T) {
	token := &indexer.Token{InstrumentAdmin: admin, InstrumentID: "DEMO"}

	t.Run("success returns page", func(t *testing.T) {
		svc, store := newSvc(t)
		p := indexer.Pagination{Page: 1, Limit: 10}
		store.EXPECT().ListTokens(mock.Anything, p).Return([]*indexer.Token{token}, int64(1), nil)

		page, err := svc.ListTokens(context.Background(), p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
		assert.Len(t, page.Items, 1)
	})

	t.Run("empty result", func(t *testing.T) {
		svc, store := newSvc(t)
		p := indexer.Pagination{Page: 1, Limit: 50}
		store.EXPECT().ListTokens(mock.Anything, p).Return(nil, int64(0), nil)

		page, err := svc.ListTokens(context.Background(), p)
		require.NoError(t, err)
		assert.Equal(t, int64(0), page.Total)
		assert.Empty(t, page.Items)
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListTokens(mock.Anything, mock.Anything).Return(nil, int64(0), errors.New("db error"))

		_, err := svc.ListTokens(context.Background(), indexer.Pagination{Page: 1, Limit: 50})
		require.Error(t, err)
	})
}

// ─── TotalSupply ──────────────────────────────────────────────────────────────

func TestSvc_TotalSupply(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").
			Return(&indexer.Token{TotalSupply: "500.0"}, nil)

		supply, err := svc.TotalSupply(context.Background(), admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, "500.0", supply)
	})

	t.Run("token not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(nil, nil)

		_, err := svc.TotalSupply(context.Background(), admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})
}

// ─── GetBalance ───────────────────────────────────────────────────────────────

func TestSvc_GetBalance(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "300.0"}

	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(balance, nil)

		got, err := svc.GetBalance(context.Background(), alice, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, balance, got)
	})

	t.Run("not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(nil, nil)

		_, err := svc.GetBalance(context.Background(), alice, admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})
}

// ─── ListBalancesForParty ─────────────────────────────────────────────────────

func TestSvc_ListBalancesForParty(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "100.0"}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListBalancesForParty(mock.Anything, alice, p).
			Return([]*indexer.Balance{balance}, int64(1), nil)

		page, err := svc.ListBalancesForParty(context.Background(), alice, p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})
}

// ─── ListBalancesForToken ─────────────────────────────────────────────────────

func TestSvc_ListBalancesForToken(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "200.0"}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListBalancesForToken(mock.Anything, admin, "DEMO", p).
			Return([]*indexer.Balance{balance}, int64(1), nil)

		page, err := svc.ListBalancesForToken(context.Background(), admin, "DEMO", p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})
}

// ─── GetEvent ─────────────────────────────────────────────────────────────────

func TestSvc_GetEvent(t *testing.T) {
	event := &indexer.ParsedEvent{ContractID: "contract-001", EventType: indexer.EventMint, Amount: "50.0"}

	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetEvent(mock.Anything, "contract-001").Return(event, nil)

		got, err := svc.GetEvent(context.Background(), "contract-001")
		require.NoError(t, err)
		assert.Equal(t, event, got)
	})

	t.Run("not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetEvent(mock.Anything, "unknown").Return(nil, nil)

		_, err := svc.GetEvent(context.Background(), "unknown")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetEvent(mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		_, err := svc.GetEvent(context.Background(), "contract-001")
		require.Error(t, err)
	})
}

// ─── ListTokenEvents ──────────────────────────────────────────────────────────

func TestSvc_ListTokenEvents(t *testing.T) {
	event := &indexer.ParsedEvent{ContractID: "contract-001", EventType: indexer.EventTransfer}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("success applies admin+id filter", func(t *testing.T) {
		svc, store := newSvc(t)
		expectedFilter := indexer.EventFilter{InstrumentAdmin: admin, InstrumentID: "DEMO", EventType: indexer.EventMint}
		store.EXPECT().ListEvents(mock.Anything, expectedFilter, p).
			Return([]*indexer.ParsedEvent{event}, int64(1), nil)

		page, err := svc.ListTokenEvents(context.Background(), admin, "DEMO", indexer.EventFilter{EventType: indexer.EventMint}, p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListEvents(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, int64(0), errors.New("db error"))

		_, err := svc.ListTokenEvents(context.Background(), admin, "DEMO", indexer.EventFilter{}, p)
		require.Error(t, err)
	})
}

// ─── ListPartyEvents ──────────────────────────────────────────────────────────

func TestSvc_ListPartyEvents(t *testing.T) {
	event := &indexer.ParsedEvent{ContractID: "contract-002", EventType: indexer.EventBurn}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("success applies partyID filter", func(t *testing.T) {
		svc, store := newSvc(t)
		expectedFilter := indexer.EventFilter{PartyID: alice}
		store.EXPECT().ListEvents(mock.Anything, expectedFilter, p).
			Return([]*indexer.ParsedEvent{event}, int64(1), nil)

		page, err := svc.ListPartyEvents(context.Background(), alice, indexer.EventFilter{}, p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListEvents(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, int64(0), errors.New("db error"))

		_, err := svc.ListPartyEvents(context.Background(), alice, indexer.EventFilter{}, p)
		require.Error(t, err)
	})
}
