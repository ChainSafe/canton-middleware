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
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/service"
	"github.com/chainsafe/canton-middleware/pkg/indexer/service/mocks"
)

const (
	alice = "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7"
	bob   = "bob::abcd1234ef567890abcd1234ef567890abcd1234ef567890abcd1234ef567890ef"
	admin = "admin-party::deadbeef"
)

// ctxAs returns a context with the given party ID injected as the authenticated party.
func ctxAs(party string) context.Context {
	return auth.WithCantonParty(context.Background(), party)
}

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

		got, err := svc.GetToken(ctxAs(alice), admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, token, got)
	})

	t.Run("store returns nil → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(nil, nil)

		_, err := svc.GetToken(ctxAs(alice), admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(nil, errors.New("db error"))

		_, err := svc.GetToken(ctxAs(alice), admin, "DEMO")
		require.Error(t, err)
	})

	t.Run("missing party in context → 401", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.GetToken(context.Background(), admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryUnauthorized))
	})
}

// ─── ListTokens ───────────────────────────────────────────────────────────────

func TestSvc_ListTokens(t *testing.T) {
	token := &indexer.Token{InstrumentAdmin: admin, InstrumentID: "DEMO"}

	t.Run("success returns page", func(t *testing.T) {
		svc, store := newSvc(t)
		p := indexer.Pagination{Page: 1, Limit: 10}
		store.EXPECT().ListTokens(mock.Anything, p).Return([]*indexer.Token{token}, int64(1), nil)

		page, err := svc.ListTokens(ctxAs(alice), p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
		assert.Len(t, page.Items, 1)
	})

	t.Run("empty result", func(t *testing.T) {
		svc, store := newSvc(t)
		p := indexer.Pagination{Page: 1, Limit: 50}
		store.EXPECT().ListTokens(mock.Anything, p).Return(nil, int64(0), nil)

		page, err := svc.ListTokens(ctxAs(alice), p)
		require.NoError(t, err)
		assert.Equal(t, int64(0), page.Total)
		assert.Empty(t, page.Items)
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListTokens(mock.Anything, mock.Anything).Return(nil, int64(0), errors.New("db error"))

		_, err := svc.ListTokens(ctxAs(alice), indexer.Pagination{Page: 1, Limit: 50})
		require.Error(t, err)
	})
}

// ─── TotalSupply ──────────────────────────────────────────────────────────────

func TestSvc_TotalSupply(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").
			Return(&indexer.Token{TotalSupply: "500.0"}, nil)

		supply, err := svc.TotalSupply(ctxAs(alice), admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, "500.0", supply)
	})

	t.Run("token not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetToken(mock.Anything, admin, "DEMO").Return(nil, nil)

		_, err := svc.TotalSupply(ctxAs(alice), admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})
}

// ─── BalanceOf ────────────────────────────────────────────────────────────────

func TestSvc_BalanceOf(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "250.0"}

	t.Run("owner can read own balance", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(balance, nil)

		amount, err := svc.BalanceOf(ctxAs(alice), alice, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, "250.0", amount)
	})

	t.Run("token admin can read any balance", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(balance, nil)

		amount, err := svc.BalanceOf(ctxAs(admin), alice, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, "250.0", amount)
	})

	t.Run("third party is forbidden", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.BalanceOf(ctxAs(bob), alice, admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})

	t.Run("balance not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(nil, nil)

		_, err := svc.BalanceOf(ctxAs(alice), alice, admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})
}

// ─── GetBalance ───────────────────────────────────────────────────────────────

func TestSvc_GetBalance(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "300.0"}

	t.Run("owner can get own balance", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(balance, nil)

		got, err := svc.GetBalance(ctxAs(alice), alice, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, balance, got)
	})

	t.Run("admin can get any balance", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(balance, nil)

		got, err := svc.GetBalance(ctxAs(admin), alice, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, balance, got)
	})

	t.Run("third party is forbidden", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.GetBalance(ctxAs(bob), alice, admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})

	t.Run("not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetBalance(mock.Anything, alice, admin, "DEMO").Return(nil, nil)

		_, err := svc.GetBalance(ctxAs(alice), alice, admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})
}

// ─── ListBalancesForParty ─────────────────────────────────────────────────────

func TestSvc_ListBalancesForParty(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "100.0"}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("party can list own balances", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListBalancesForParty(mock.Anything, alice, p).
			Return([]*indexer.Balance{balance}, int64(1), nil)

		page, err := svc.ListBalancesForParty(ctxAs(alice), alice, p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("other party is forbidden", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.ListBalancesForParty(ctxAs(bob), alice, p)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})
}

// ─── ListBalancesForToken ─────────────────────────────────────────────────────

func TestSvc_ListBalancesForToken(t *testing.T) {
	balance := &indexer.Balance{PartyID: alice, Amount: "200.0"}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("token admin can list all balances", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListBalancesForToken(mock.Anything, admin, "DEMO", p).
			Return([]*indexer.Balance{balance}, int64(1), nil)

		page, err := svc.ListBalancesForToken(ctxAs(admin), admin, "DEMO", p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("non-admin is forbidden", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.ListBalancesForToken(ctxAs(alice), admin, "DEMO", p)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})
}

// ─── GetEvent ─────────────────────────────────────────────────────────────────

func TestSvc_GetEvent(t *testing.T) {
	event := &indexer.ParsedEvent{ContractID: "contract-001", EventType: indexer.EventMint, Amount: "50.0"}

	t.Run("success", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetEvent(mock.Anything, "contract-001").Return(event, nil)

		got, err := svc.GetEvent(ctxAs(alice), "contract-001")
		require.NoError(t, err)
		assert.Equal(t, event, got)
	})

	t.Run("not found → 404", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetEvent(mock.Anything, "unknown").Return(nil, nil)

		_, err := svc.GetEvent(ctxAs(alice), "unknown")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryResourceNotFound))
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetEvent(mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		_, err := svc.GetEvent(ctxAs(alice), "contract-001")
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

		page, err := svc.ListTokenEvents(ctxAs(alice), admin, "DEMO", indexer.EventFilter{EventType: indexer.EventMint}, p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListEvents(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, int64(0), errors.New("db error"))

		_, err := svc.ListTokenEvents(ctxAs(alice), admin, "DEMO", indexer.EventFilter{}, p)
		require.Error(t, err)
	})
}

// ─── ListPartyEvents ──────────────────────────────────────────────────────────

func TestSvc_ListPartyEvents(t *testing.T) {
	event := &indexer.ParsedEvent{ContractID: "contract-002", EventType: indexer.EventBurn}
	p := indexer.Pagination{Page: 1, Limit: 50}

	t.Run("party can list own events", func(t *testing.T) {
		svc, store := newSvc(t)
		expectedFilter := indexer.EventFilter{PartyID: alice}
		store.EXPECT().ListEvents(mock.Anything, expectedFilter, p).
			Return([]*indexer.ParsedEvent{event}, int64(1), nil)

		page, err := svc.ListPartyEvents(ctxAs(alice), alice, indexer.EventFilter{}, p)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("other party is forbidden", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.ListPartyEvents(ctxAs(bob), alice, indexer.EventFilter{}, p)
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().ListEvents(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, int64(0), errors.New("db error"))

		_, err := svc.ListPartyEvents(ctxAs(alice), alice, indexer.EventFilter{}, p)
		require.Error(t, err)
	})
}

// ─── Allowance ────────────────────────────────────────────────────────────────

func TestSvc_Allowance(t *testing.T) {
	t.Run("owner can query own allowance", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetAllowance(mock.Anything, alice, bob, admin, "DEMO").Return("0", nil)

		amount, err := svc.Allowance(ctxAs(alice), alice, bob, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, "0", amount)
	})

	t.Run("admin can query any allowance", func(t *testing.T) {
		svc, store := newSvc(t)
		store.EXPECT().GetAllowance(mock.Anything, alice, bob, admin, "DEMO").Return("100", nil)

		amount, err := svc.Allowance(ctxAs(admin), alice, bob, admin, "DEMO")
		require.NoError(t, err)
		assert.Equal(t, "100", amount)
	})

	t.Run("third party is forbidden", func(t *testing.T) {
		svc, _ := newSvc(t)

		_, err := svc.Allowance(ctxAs(bob), alice, bob, admin, "DEMO")
		require.Error(t, err)
		assert.True(t, apperr.Is(err, apperr.CategoryForbidden))
	})
}
