package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/service"
	"github.com/chainsafe/canton-middleware/pkg/indexer/service/mocks"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

type testEnv struct {
	srv     *httptest.Server
	svc     *mocks.Service
	partyID string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	svcMock := mocks.NewService(t)

	r := chi.NewRouter()
	service.RegisterRoutes(r, svcMock, zap.NewNop())

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testEnv{
		srv:     srv,
		svc:     svcMock,
		partyID: "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7",
	}
}

func (e *testEnv) get(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.srv.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	assert.Equal(t, want, resp.StatusCode)
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

// ─── GET /indexer/v1/tokens ───────────────────────────────────────────────────

func TestHTTP_ListTokens(t *testing.T) {
	token := &indexer.Token{
		InstrumentAdmin: "admin-party",
		InstrumentID:    "DEMO",
		Issuer:          "issuer-party",
		TotalSupply:     "1000.0",
		HolderCount:     3,
	}

	t.Run("success returns page", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListTokens(mock.Anything, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.Token]{Items: []*indexer.Token{token}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.Token]](t, resp)
		assert.Equal(t, int64(1), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "DEMO", page.Items[0].InstrumentID)
	})

	t.Run("pagination params forwarded", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListTokens(mock.Anything, indexer.Pagination{Page: 2, Limit: 10}).
			Return(&indexer.Page[*indexer.Token]{Items: nil, Total: 0, Page: 2, Limit: 10}, nil)

		resp := e.get(t, "/indexer/v1/tokens?page=2&limit=10")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("invalid page returns 400", func(t *testing.T) {
		e := newTestEnv(t)

		resp := e.get(t, "/indexer/v1/tokens?page=0")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("invalid limit returns 400", func(t *testing.T) {
		e := newTestEnv(t)

		resp := e.get(t, "/indexer/v1/tokens?limit=999")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("service error propagates", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListTokens(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/tokens")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusInternalServerError)
	})
}

// ─── GET /indexer/v1/tokens/{admin}/{id} ──────────────────────────────────────

func TestHTTP_GetToken(t *testing.T) {
	token := &indexer.Token{
		InstrumentAdmin: "admin-party",
		InstrumentID:    "DEMO",
		TotalSupply:     "500.0",
	}

	t.Run("found returns token", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetToken(mock.Anything, "admin-party", "DEMO").Return(token, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := decodeJSON[indexer.Token](t, resp)
		assert.Equal(t, "DEMO", got.InstrumentID)
		assert.Equal(t, "500.0", got.TotalSupply)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetToken(mock.Anything, "admin-party", "NOPE").
			Return(nil, apperr.ResourceNotFoundError(nil, "token not found"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/NOPE")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusNotFound)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetToken(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusInternalServerError)
	})
}

// ─── GET /indexer/v1/tokens/{admin}/{id}/supply ───────────────────────────────

func TestHTTP_GetTokenSupply(t *testing.T) {
	t.Run("success returns total_supply", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().TotalSupply(mock.Anything, "admin-party", "DEMO").Return("1000.0", nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/supply")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeJSON[map[string]string](t, resp)
		assert.Equal(t, "1000.0", body["total_supply"])
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().TotalSupply(mock.Anything, mock.Anything, mock.Anything).
			Return("", apperr.ResourceNotFoundError(nil, "token not found"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/NOPE/supply")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusNotFound)
	})
}

// ─── GET /indexer/v1/tokens/{admin}/{id}/balances ─────────────────────────────

func TestHTTP_ListTokenBalances(t *testing.T) {
	balance := &indexer.Balance{
		PartyID:         "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7",
		InstrumentAdmin: "admin-party",
		InstrumentID:    "DEMO",
		Amount:          "250.0",
	}

	t.Run("success returns page", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListBalancesForToken(mock.Anything, "admin-party", "DEMO", indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.Balance]{Items: []*indexer.Balance{balance}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/balances")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.Balance]](t, resp)
		assert.Equal(t, int64(1), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "250.0", page.Items[0].Amount)
	})

	t.Run("service error propagates", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListBalancesForToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/balances")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusInternalServerError)
	})
}

// ─── GET /indexer/v1/tokens/{admin}/{id}/events ───────────────────────────────

func TestHTTP_ListTokenEvents(t *testing.T) {
	event := &indexer.ParsedEvent{
		InstrumentAdmin: "admin-party",
		InstrumentID:    "DEMO",
		EventType:       indexer.EventMint,
		Amount:          "100.0",
		ContractID:      "contract-001",
	}

	t.Run("success returns page", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListTokenEvents(mock.Anything, "admin-party", "DEMO", indexer.EventFilter{}, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: []*indexer.ParsedEvent{event}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/events")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.ParsedEvent]](t, resp)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("event_type filter forwarded", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListTokenEvents(mock.Anything, "admin-party", "DEMO", indexer.EventFilter{EventType: indexer.EventMint}, mock.Anything).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: nil, Total: 0, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/events?event_type=MINT")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("invalid event_type returns 400", func(t *testing.T) {
		e := newTestEnv(t)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/events?event_type=INVALID")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

// ─── GET /indexer/v1/parties/{partyID}/balances ───────────────────────────────

func TestHTTP_ListPartyBalances(t *testing.T) {
	balance := &indexer.Balance{
		PartyID:         "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7",
		InstrumentAdmin: "admin-party",
		InstrumentID:    "DEMO",
		Amount:          "100.0",
	}

	t.Run("success returns page", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListBalancesForParty(mock.Anything, e.partyID, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.Balance]{Items: []*indexer.Balance{balance}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/balances")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.Balance]](t, resp)
		assert.Equal(t, int64(1), page.Total)
	})
}

// ─── GET /indexer/v1/parties/{partyID}/balances/{admin}/{id} ─────────────────

func TestHTTP_GetPartyBalance(t *testing.T) {
	balance := &indexer.Balance{
		PartyID:         "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7",
		InstrumentAdmin: "admin-party",
		InstrumentID:    "DEMO",
		Amount:          "750.0",
	}

	t.Run("success returns balance", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetBalance(mock.Anything, e.partyID, "admin-party", "DEMO").Return(balance, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/balances/admin-party/DEMO")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := decodeJSON[indexer.Balance](t, resp)
		assert.Equal(t, "750.0", got.Amount)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetBalance(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, apperr.ResourceNotFoundError(nil, "balance not found"))

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/balances/admin-party/DEMO")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusNotFound)
	})
}

// ─── GET /indexer/v1/parties/{partyID}/events ────────────────────────────────

func TestHTTP_ListPartyEvents(t *testing.T) {
	event := &indexer.ParsedEvent{
		EventType:  indexer.EventTransfer,
		Amount:     "50.0",
		ContractID: "contract-002",
	}

	t.Run("success returns page", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListPartyEvents(mock.Anything, e.partyID, indexer.EventFilter{}, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: []*indexer.ParsedEvent{event}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/events")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.ParsedEvent]](t, resp)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("BURN event_type filter forwarded", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().ListPartyEvents(mock.Anything, e.partyID, indexer.EventFilter{EventType: indexer.EventBurn}, mock.Anything).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: nil, Total: 0, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/events?event_type=BURN")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
	})
}

// ─── GET /indexer/v1/events/{contractID} ─────────────────────────────────────

func TestHTTP_GetEvent(t *testing.T) {
	event := &indexer.ParsedEvent{
		EventType:  indexer.EventMint,
		Amount:     "200.0",
		ContractID: "contract-abc",
	}

	t.Run("found returns event", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetEvent(mock.Anything, "contract-abc").Return(event, nil)

		resp := e.get(t, "/indexer/v1/events/contract-abc")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := decodeJSON[indexer.ParsedEvent](t, resp)
		assert.Equal(t, "200.0", got.Amount)
		assert.Equal(t, indexer.EventMint, got.EventType)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetEvent(mock.Anything, "unknown").
			Return(nil, apperr.ResourceNotFoundError(nil, "event not found"))

		resp := e.get(t, "/indexer/v1/events/unknown")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusNotFound)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		e := newTestEnv(t)
		e.svc.EXPECT().GetEvent(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/events/contract-abc")
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusInternalServerError)
	})
}
