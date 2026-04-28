package token_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/token/mocks"
)

func newTokenTestServer(svc token.ListService) http.Handler {
	r := chi.NewRouter()
	token.RegisterRoutes(r, svc, zap.NewNop())
	return r
}

func TestListTokensHTTP_DefaultPagination_ReturnsOK(t *testing.T) {
	svc := mocks.NewListService(t)
	svc.EXPECT().
		GetSupportedTokens(mock.Anything, "", token.DefaultLimit).
		Return(&token.TokensPage{
			Items: []token.TokenItem{
				{Address: "0xabc", Name: "Demo", Symbol: "DEMO", Decimals: 18},
			},
			HasMore: false,
		}, nil)

	req := httptest.NewRequest(http.MethodGet, "/tokens", nil)
	rec := httptest.NewRecorder()
	newTokenTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", ct)
	}

	var got token.TokensPage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Symbol != "DEMO" {
		t.Fatalf("unexpected items: %+v", got.Items)
	}
	if got.HasMore {
		t.Fatal("expected has_more false")
	}
}

func TestListTokensHTTP_WithCursor_CallsServiceWithParams(t *testing.T) {
	svc := mocks.NewListService(t)
	svc.EXPECT().
		GetSupportedTokens(mock.Anything, "0x2abc", 10).
		Return(&token.TokensPage{Items: []token.TokenItem{}, HasMore: false}, nil)

	req := httptest.NewRequest(http.MethodGet, "/tokens?cursor=0x2abc&limit=10", nil)
	rec := httptest.NewRecorder()
	newTokenTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestListTokensHTTP_HasMore_ReturnsNextCursor(t *testing.T) {
	svc := mocks.NewListService(t)
	svc.EXPECT().
		GetSupportedTokens(mock.Anything, "", 1).
		Return(&token.TokensPage{
			Items:      []token.TokenItem{{Address: "0x1aaa", Name: "Demo", Symbol: "DEMO", Decimals: 18}},
			NextCursor: "0x1aaa",
			HasMore:    true,
		}, nil)

	req := httptest.NewRequest(http.MethodGet, "/tokens?limit=1", nil)
	rec := httptest.NewRecorder()
	newTokenTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var got token.TokensPage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !got.HasMore {
		t.Fatal("expected has_more true")
	}
	if got.NextCursor != "0x1aaa" {
		t.Fatalf("expected next_cursor %q, got %q", "0x1aaa", got.NextCursor)
	}
}

func TestListTokensHTTP_InvalidLimit_ReturnsBadRequest(t *testing.T) {
	svc := mocks.NewListService(t)
	handler := newTokenTestServer(svc)

	for _, tc := range []string{"0", "201", "abc"} {
		req := httptest.NewRequest(http.MethodGet, "/tokens?limit="+tc, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("limit=%q: expected status %d, got %d", tc, http.StatusBadRequest, rec.Code)
		}
	}
}

func TestListTokensHTTP_ServiceError_Returns500(t *testing.T) {
	svc := mocks.NewListService(t)
	svc.EXPECT().
		GetSupportedTokens(mock.Anything, "", token.DefaultLimit).
		Return(nil, errors.New("unexpected store error"))

	req := httptest.NewRequest(http.MethodGet, "/tokens", nil)
	rec := httptest.NewRecorder()
	newTokenTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestListTokensHTTP_EmptyList_ReturnsOKWithEmptyItems(t *testing.T) {
	svc := mocks.NewListService(t)
	svc.EXPECT().
		GetSupportedTokens(mock.Anything, "", token.DefaultLimit).
		Return(&token.TokensPage{Items: []token.TokenItem{}, HasMore: false}, nil)

	req := httptest.NewRequest(http.MethodGet, "/tokens", nil)
	rec := httptest.NewRecorder()
	newTokenTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var got token.TokensPage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("expected empty items slice, got %+v", got.Items)
	}
}
