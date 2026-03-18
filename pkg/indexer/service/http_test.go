package service_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
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

// ─── test helpers ─────────────────────────────────────────────────────────────

// testEnv holds shared test infrastructure for a single test.
type testEnv struct {
	srv        *httptest.Server
	svc        *mocks.Service
	readiness  *mocks.ReadinessChecker
	privateKey *rsa.PrivateKey
	partyID    string
}

// newTestEnv spins up a real httptest.Server with RegisterRoutes and a JWKS
// mock server so the JWT middleware can validate tokens signed by the test key.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Serve a minimal JWKS containing the test public key.
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pub := &privateKey.PublicKey
		nBytes := pub.N.Bytes()
		eBytes := big.NewInt(int64(pub.E)).Bytes()

		jwks := map[string]any{
			"keys": []any{
				map[string]any{
					"kty": "RSA",
					"kid": "test-key",
					"alg": "RS256",
					"use": "sig",
					"n":   base64.RawURLEncoding.EncodeToString(nBytes),
					"e":   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(jwksServer.Close)

	validator := auth.NewJWTValidator(jwksServer.URL, "")

	svcMock := mocks.NewService(t)
	readinessMock := mocks.NewReadinessChecker(t)

	r := chi.NewRouter()
	service.RegisterRoutes(r, svcMock, readinessMock, validator, zap.NewNop())

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testEnv{
		srv:        srv,
		svc:        svcMock,
		readiness:  readinessMock,
		privateKey: privateKey,
		partyID:    "alice::122059f6ef3a88b2da18a1c8a7462836543c4c7c5dbfc8c76db5db67a8e53b13e5b7",
	}
}

// signedToken creates a signed JWT with the given partyID claim.
func (e *testEnv) signedToken(t *testing.T, partyID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"canton_party_id": partyID,
		"exp":             time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(e.privateKey)
	require.NoError(t, err)
	return signed
}

// get performs a GET request to path with an Authorization: Bearer header for e.partyID.
func (e *testEnv) get(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+e.signedToken(t, e.partyID))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// getNoAuth performs a GET without an Authorization header.
func (e *testEnv) getNoAuth(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

// ─── readiness middleware ──────────────────────────────────────────────────────

func TestReadinessMiddleware(t *testing.T) {
	t.Run("returns 503 when not ready", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(false)

		resp := e.getNoAuth(t, "/indexer/v1/tokens")
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	})

	t.Run("passes through when ready", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		// JWT check will fire next; missing header → 401.
		resp := e.getNoAuth(t, "/indexer/v1/tokens")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListTokens(mock.Anything, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.Token]{Items: []*indexer.Token{token}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.Token]](t, resp)
		assert.Equal(t, int64(1), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "DEMO", page.Items[0].InstrumentID)
	})

	t.Run("pagination params forwarded", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListTokens(mock.Anything, indexer.Pagination{Page: 2, Limit: 10}).
			Return(&indexer.Page[*indexer.Token]{Items: nil, Total: 0, Page: 2, Limit: 10}, nil)

		resp := e.get(t, "/indexer/v1/tokens?page=2&limit=10")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invalid page returns 400", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)

		resp := e.get(t, "/indexer/v1/tokens?page=0")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("invalid limit returns 400", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)

		resp := e.get(t, "/indexer/v1/tokens?limit=999")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("service error propagates", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListTokens(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/tokens")
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetToken(mock.Anything, "admin-party", "DEMO").Return(token, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := decodeJSON[indexer.Token](t, resp)
		assert.Equal(t, "DEMO", got.InstrumentID)
		assert.Equal(t, "500.0", got.TotalSupply)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetToken(mock.Anything, "admin-party", "NOPE").
			Return(nil, apperr.ResourceNotFoundError(nil, "token not found"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/NOPE")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetToken(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO")
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ─── GET /indexer/v1/tokens/{admin}/{id}/supply ───────────────────────────────

func TestHTTP_GetTokenSupply(t *testing.T) {
	t.Run("success returns total_supply", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().TotalSupply(mock.Anything, "admin-party", "DEMO").Return("1000.0", nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/supply")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeJSON[map[string]string](t, resp)
		assert.Equal(t, "1000.0", body["total_supply"])
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().TotalSupply(mock.Anything, mock.Anything, mock.Anything).
			Return("", apperr.ResourceNotFoundError(nil, "token not found"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/NOPE/supply")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListBalancesForToken(mock.Anything, "admin-party", "DEMO", indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.Balance]{Items: []*indexer.Balance{balance}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/balances")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.Balance]](t, resp)
		assert.Equal(t, int64(1), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "250.0", page.Items[0].Amount)
	})

	t.Run("service error propagates", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListBalancesForToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/balances")
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListTokenEvents(mock.Anything, "admin-party", "DEMO", indexer.EventFilter{}, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: []*indexer.ParsedEvent{event}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/events")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.ParsedEvent]](t, resp)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("event_type filter forwarded", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListTokenEvents(mock.Anything, "admin-party", "DEMO", indexer.EventFilter{EventType: indexer.EventMint}, mock.Anything).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: nil, Total: 0, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/events?event_type=MINT")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invalid event_type returns 400", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)

		resp := e.get(t, "/indexer/v1/tokens/admin-party/DEMO/events?event_type=INVALID")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListBalancesForParty(mock.Anything, e.partyID, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.Balance]{Items: []*indexer.Balance{balance}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/balances")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.Balance]](t, resp)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("forbidden when accessing another party", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListBalancesForParty(mock.Anything, "other-party", mock.Anything).
			Return(nil, apperr.ForbiddenError(nil, "access denied"))

		resp := e.get(t, "/indexer/v1/parties/other-party/balances")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetBalance(mock.Anything, e.partyID, "admin-party", "DEMO").Return(balance, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/balances/admin-party/DEMO")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := decodeJSON[indexer.Balance](t, resp)
		assert.Equal(t, "750.0", got.Amount)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetBalance(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, apperr.ResourceNotFoundError(nil, "balance not found"))

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/balances/admin-party/DEMO")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListPartyEvents(mock.Anything, e.partyID, indexer.EventFilter{}, indexer.Pagination{Page: 1, Limit: 50}).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: []*indexer.ParsedEvent{event}, Total: 1, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/events")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		page := decodeJSON[indexer.Page[*indexer.ParsedEvent]](t, resp)
		assert.Equal(t, int64(1), page.Total)
	})

	t.Run("BURN event_type filter forwarded", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListPartyEvents(mock.Anything, e.partyID, indexer.EventFilter{EventType: indexer.EventBurn}, mock.Anything).
			Return(&indexer.Page[*indexer.ParsedEvent]{Items: nil, Total: 0, Page: 1, Limit: 50}, nil)

		resp := e.get(t, "/indexer/v1/parties/"+e.partyID+"/events?event_type=BURN")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("forbidden accessing another party events", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().ListPartyEvents(mock.Anything, "other-party", mock.Anything, mock.Anything).
			Return(nil, apperr.ForbiddenError(nil, "access denied"))

		resp := e.get(t, "/indexer/v1/parties/other-party/events")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
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
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetEvent(mock.Anything, "contract-abc").Return(event, nil)

		resp := e.get(t, "/indexer/v1/events/contract-abc")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		got := decodeJSON[indexer.ParsedEvent](t, resp)
		assert.Equal(t, "200.0", got.Amount)
		assert.Equal(t, indexer.EventMint, got.EventType)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetEvent(mock.Anything, "unknown").
			Return(nil, apperr.ResourceNotFoundError(nil, "event not found"))

		resp := e.get(t, "/indexer/v1/events/unknown")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)
		e.svc.EXPECT().GetEvent(mock.Anything, mock.Anything).
			Return(nil, errors.New("db error"))

		resp := e.get(t, "/indexer/v1/events/contract-abc")
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ─── JWT middleware ───────────────────────────────────────────────────────────

func TestHTTP_JWTMiddleware(t *testing.T) {
	t.Run("missing header returns 401", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)

		resp := e.getNoAuth(t, "/indexer/v1/tokens")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("malformed header returns 401", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)

		req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/indexer/v1/tokens", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "NotBearer token")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		e := newTestEnv(t)
		e.readiness.EXPECT().Ready().Return(true)

		req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/indexer/v1/tokens", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
