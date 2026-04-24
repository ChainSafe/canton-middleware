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
		GetSupportedTokens(mock.Anything, 1, token.DefaultLimit).
		Return(&token.TokensPage{
			Items: []token.TokenItem{
				{Address: "0xabc", Name: "Demo", Symbol: "DEMO", Decimals: 18},
			},
			Total: 1,
			Page:  1,
			Limit: token.DefaultLimit,
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
	if got.Total != 1 {
		t.Fatalf("expected total 1, got %d", got.Total)
	}
	if len(got.Items) != 1 || got.Items[0].Symbol != "DEMO" {
		t.Fatalf("unexpected items: %+v", got.Items)
	}
}

func TestListTokensHTTP_ExplicitPagination_CallsServiceWithParams(t *testing.T) {
	svc := mocks.NewListService(t)
	svc.EXPECT().
		GetSupportedTokens(mock.Anything, 2, 10).
		Return(&token.TokensPage{Items: []token.TokenItem{}, Total: 5, Page: 2, Limit: 10}, nil)

	req := httptest.NewRequest(http.MethodGet, "/tokens?page=2&limit=10", nil)
	rec := httptest.NewRecorder()
	newTokenTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var got token.TokensPage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Page != 2 || got.Limit != 10 {
		t.Fatalf("expected page=2 limit=10, got page=%d limit=%d", got.Page, got.Limit)
	}
}

func TestListTokensHTTP_InvalidPage_ReturnsBadRequest(t *testing.T) {
	svc := mocks.NewListService(t)
	handler := newTokenTestServer(svc)

	for _, tc := range []string{"0", "-1", "abc"} {
		req := httptest.NewRequest(http.MethodGet, "/tokens?page="+tc, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("page=%q: expected status %d, got %d", tc, http.StatusBadRequest, rec.Code)
		}
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
		GetSupportedTokens(mock.Anything, 1, token.DefaultLimit).
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
		GetSupportedTokens(mock.Anything, 1, token.DefaultLimit).
		Return(&token.TokensPage{Items: []token.TokenItem{}, Total: 0, Page: 1, Limit: token.DefaultLimit}, nil)

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
