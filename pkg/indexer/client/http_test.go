package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/client"
)

// ── fixtures ─────────────────────────────────────────────────────────────────

const (
	admin      = "admin::demo123@domain"
	id         = "DEMO"
	partyID    = "alice::abc123@domain"
	contractID = "contract::abc@node"
)

var (
	testPagination = indexer.Pagination{Page: 1, Limit: 50}
	testToken      = &indexer.Token{
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Issuer:          "issuer::xyz@domain",
		TotalSupply:     "1000.000000000000000000",
		HolderCount:     3,
		FirstSeenOffset: 42,
		FirstSeenAt:     time.Unix(0, 0).UTC(),
	}
	testBalance = &indexer.Balance{
		PartyID:         partyID,
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Amount:          "500.000000000000000000",
	}
	testEvent = &indexer.ParsedEvent{
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Issuer:          "issuer::xyz@domain",
		EventType:       indexer.EventMint,
		Amount:          "100.000000000000000000",
		ContractID:      contractID,
		TxID:            "tx123",
		LedgerOffset:    10,
		Timestamp:       time.Unix(0, 0).UTC(),
		EffectiveTime:   time.Unix(0, 0).UTC(),
	}
)

// ── helpers ───────────────────────────────────────────────────────────────────

// mustNew wraps client.New for use in tests; it fails the test on error.
func mustNew(t *testing.T, baseURL string, httpClient *http.Client) *client.HTTP {
	t.Helper()
	c, err := client.New(baseURL, httpClient)
	require.NoError(t, err)
	return c
}

func jsonResp(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func errResp(status int, message string) http.HandlerFunc {
	return jsonResp(status, map[string]any{"error": message, "code": status})
}

func pageOf[T any](items []T) indexer.Page[T] {
	return indexer.Page[T]{Items: items, Total: int64(len(items)), Page: 1, Limit: 50}
}

// assertPagination checks that page and limit query params are present.
func assertPagination(t *testing.T, r *http.Request) {
	t.Helper()
	assert.Equal(t, "1", r.URL.Query().Get("page"))
	assert.Equal(t, "50", r.URL.Query().Get("limit"))
}

// ── GetToken ─────────────────────────────────────────────────────────────────

func TestHTTP_GetToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s", admin, id), r.URL.Path)
		jsonResp(http.StatusOK, testToken)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.GetToken(context.Background(), admin, id)

	require.NoError(t, err)
	assert.Equal(t, testToken.InstrumentAdmin, got.InstrumentAdmin)
	assert.Equal(t, testToken.TotalSupply, got.TotalSupply)
}

func TestHTTP_GetToken_NotFound(t *testing.T) {
	srv := httptest.NewServer(errResp(http.StatusNotFound, "token not found"))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	_, err := c.GetToken(context.Background(), admin, id)

	require.Error(t, err)
	assert.True(t, apperrors.Is(err, apperrors.CategoryResourceNotFound))
}

// ── ListTokens ────────────────────────────────────────────────────────────────

func TestHTTP_ListTokens_Success(t *testing.T) {
	page := pageOf([]*indexer.Token{testToken})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/indexer/v1/admin/tokens", r.URL.Path)
		assertPagination(t, r)
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListTokens(context.Background(), testPagination)

	require.NoError(t, err)
	assert.Equal(t, int64(1), got.Total)
	assert.Len(t, got.Items, 1)
}

func TestHTTP_ListTokens_Empty(t *testing.T) {
	page := pageOf([]*indexer.Token{})
	srv := httptest.NewServer(jsonResp(http.StatusOK, page))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListTokens(context.Background(), testPagination)

	require.NoError(t, err)
	assert.Empty(t, got.Items)
}

// ── TotalSupply ───────────────────────────────────────────────────────────────

func TestHTTP_TotalSupply_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s/supply", admin, id), r.URL.Path)
		jsonResp(http.StatusOK, map[string]string{"total_supply": "1000.000000000000000000"})(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	supply, err := c.TotalSupply(context.Background(), admin, id)

	require.NoError(t, err)
	assert.Equal(t, "1000.000000000000000000", supply)
}

func TestHTTP_TotalSupply_ServerError(t *testing.T) {
	srv := httptest.NewServer(errResp(http.StatusInternalServerError, "database unavailable"))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	supply, err := c.TotalSupply(context.Background(), admin, id)

	assert.Equal(t, "0", supply)
	require.Error(t, err)
	assert.ErrorContains(t, err, "indexer HTTP 500")
}

func TestHTTP_TotalSupply_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	supply, err := c.TotalSupply(context.Background(), admin, id)

	assert.Equal(t, "0", supply)
	require.Error(t, err)
}

func TestHTTP_TotalSupply_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	supply, err := c.TotalSupply(context.Background(), admin, id)

	assert.Equal(t, "0", supply)
	require.Error(t, err)
	assert.ErrorContains(t, err, "decode response")
}

func TestHTTP_TotalSupply_NonJSONErrorBody(t *testing.T) {
	// Gateway/proxy may return an HTML error page instead of JSON.
	// The client should still return an error with the status code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>Bad Gateway</body></html>"))
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	supply, err := c.TotalSupply(context.Background(), admin, id)

	assert.Equal(t, "0", supply)
	require.Error(t, err)
	assert.ErrorContains(t, err, "indexer HTTP 502")
}

// ── GetBalance ────────────────────────────────────────────────────────────────

func TestHTTP_GetBalance_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			fmt.Sprintf("/indexer/v1/admin/parties/%s/balances/%s/%s", partyID, admin, id),
			r.URL.Path,
		)
		jsonResp(http.StatusOK, testBalance)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.GetBalance(context.Background(), partyID, admin, id)

	require.NoError(t, err)
	assert.Equal(t, "500.000000000000000000", got.Amount)
	assert.Equal(t, partyID, got.PartyID)
}

func TestHTTP_GetBalance_NotFound_ReturnsResourceNotFoundError(t *testing.T) {
	// 404 is surfaced as apperrors.ResourceNotFoundError, not swallowed.
	// The provider layer (not the client) is responsible for converting not-found to "0".
	srv := httptest.NewServer(errResp(http.StatusNotFound, "balance not found"))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.GetBalance(context.Background(), partyID, admin, id)

	assert.Nil(t, got)
	require.Error(t, err)
	assert.True(t, apperrors.Is(err, apperrors.CategoryResourceNotFound))
}

func TestHTTP_GetBalance_ServerError(t *testing.T) {
	srv := httptest.NewServer(errResp(http.StatusBadGateway, "upstream failure"))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.GetBalance(context.Background(), partyID, admin, id)

	assert.Nil(t, got)
	require.Error(t, err)
	assert.ErrorContains(t, err, "indexer HTTP 502")
}

func TestHTTP_GetBalance_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.GetBalance(context.Background(), partyID, admin, id)

	assert.Nil(t, got)
	require.Error(t, err)
}

// ── ListBalancesForParty ──────────────────────────────────────────────────────

func TestHTTP_ListBalancesForParty_Success(t *testing.T) {
	page := pageOf([]*indexer.Balance{testBalance})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprintf("/indexer/v1/admin/parties/%s/balances", partyID), r.URL.Path)
		assertPagination(t, r)
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListBalancesForParty(context.Background(), partyID, testPagination)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
	assert.Equal(t, "500.000000000000000000", got.Items[0].Amount)
}

// ── ListBalancesForToken ──────────────────────────────────────────────────────

func TestHTTP_ListBalancesForToken_Success(t *testing.T) {
	page := pageOf([]*indexer.Balance{testBalance})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s/balances", admin, id),
			r.URL.Path,
		)
		assertPagination(t, r)
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListBalancesForToken(context.Background(), admin, id, testPagination)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
}

// ── GetEvent ──────────────────────────────────────────────────────────────────

func TestHTTP_GetEvent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprintf("/indexer/v1/admin/events/%s", contractID), r.URL.Path)
		jsonResp(http.StatusOK, testEvent)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.GetEvent(context.Background(), contractID)

	require.NoError(t, err)
	assert.Equal(t, contractID, got.ContractID)
	assert.Equal(t, indexer.EventMint, got.EventType)
}

func TestHTTP_GetEvent_NotFound(t *testing.T) {
	srv := httptest.NewServer(errResp(http.StatusNotFound, "event not found"))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	_, err := c.GetEvent(context.Background(), contractID)

	require.Error(t, err)
	assert.True(t, apperrors.Is(err, apperrors.CategoryResourceNotFound))
}

// ── ListTokenEvents ───────────────────────────────────────────────────────────

func TestHTTP_ListTokenEvents_NoFilter(t *testing.T) {
	page := pageOf([]*indexer.ParsedEvent{testEvent})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s/events", admin, id),
			r.URL.Path,
		)
		assertPagination(t, r)
		assert.Empty(t, r.URL.Query().Get("event_type"), "event_type should be absent when filter is empty")
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListTokenEvents(context.Background(), admin, id, indexer.EventFilter{}, testPagination)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
}

func TestHTTP_ListTokenEvents_WithEventTypeFilter(t *testing.T) {
	page := pageOf([]*indexer.ParsedEvent{testEvent})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, string(indexer.EventMint), r.URL.Query().Get("event_type"))
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListTokenEvents(context.Background(), admin, id,
		indexer.EventFilter{EventType: indexer.EventMint}, testPagination)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
}

// ── ListPartyEvents ───────────────────────────────────────────────────────────

func TestHTTP_ListPartyEvents_NoFilter(t *testing.T) {
	page := pageOf([]*indexer.ParsedEvent{testEvent})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprintf("/indexer/v1/admin/parties/%s/events", partyID), r.URL.Path)
		assertPagination(t, r)
		assert.Empty(t, r.URL.Query().Get("event_type"))
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListPartyEvents(context.Background(), partyID, indexer.EventFilter{}, testPagination)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
}

func TestHTTP_ListPartyEvents_WithEventTypeFilter(t *testing.T) {
	page := pageOf([]*indexer.ParsedEvent{testEvent})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, string(indexer.EventBurn), r.URL.Query().Get("event_type"))
		jsonResp(http.StatusOK, page)(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL, srv.Client())
	got, err := c.ListPartyEvents(context.Background(), partyID,
		indexer.EventFilter{EventType: indexer.EventBurn}, testPagination)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
}

// ── Constructor ───────────────────────────────────────────────────────────────

func TestNew_NilHTTPClient_UsesDefaultClient(t *testing.T) {
	c := mustNew(t, "http://localhost:8080", nil)
	require.NotNil(t, c)
}

func TestNew_InvalidURL_ReturnsError(t *testing.T) {
	_, err := client.New("://bad-url", nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid indexer base URL")
}

func TestNew_NonHTTPScheme_ReturnsError(t *testing.T) {
	_, err := client.New("grpc://localhost:8080", nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "scheme must be http or https")
}

func TestNew_TrailingSlashStripped(t *testing.T) {
	// A baseURL supplied with a trailing slash must not produce double-slash paths.
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		jsonResp(http.StatusOK, map[string]string{"total_supply": "1"})(w, r)
	}))
	defer srv.Close()

	c := mustNew(t, srv.URL+"/", srv.Client()) // trailing slash
	_, err := c.TotalSupply(context.Background(), admin, id)

	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(gotPath, "//"), "path should not start with //: %s", gotPath)
}
