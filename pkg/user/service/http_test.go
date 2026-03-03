package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/user/service/mocks"
)

func newRegisterTestServer(svc Service) http.Handler {
	r := chi.NewRouter()
	RegisterRoutes(r, svc, zap.NewNop())
	return r
}

func TestRegisterHTTP_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	svc := mocks.NewService(t)
	handler := newRegisterTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var got struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if got.Error != "invalid JSON" {
		t.Fatalf("expected error %q, got %q", "invalid JSON", got.Error)
	}
	if got.Code != http.StatusBadRequest {
		t.Fatalf("expected code %d, got %d", http.StatusBadRequest, got.Code)
	}
}

func TestRegisterHTTP_MissingSignatureAndMessage_ReturnsUnauthorized(t *testing.T) {
	svc := mocks.NewService(t)
	handler := newRegisterTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	var got struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if got.Error != "signature and message required" {
		t.Fatalf("expected error %q, got %q", "signature and message required", got.Error)
	}
	if got.Code != http.StatusUnauthorized {
		t.Fatalf("expected code %d, got %d", http.StatusUnauthorized, got.Code)
	}
}

func TestRegisterHTTP_Web3HeadersFallback_ResponseCheck(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().
		RegisterWeb3User(mock.Anything, mock.Anything).
		Return(&user.RegisterResponse{
			Party:       "party-1",
			Fingerprint: "fp-1",
		}, nil)
	handler := newRegisterTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Signature", "sig")
	req.Header.Set("X-Message", "msg")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type %q, got %q", "application/json", ct)
	}

	var got user.RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if got.Party != "party-1" {
		t.Fatalf("expected party %q, got %q", "party-1", got.Party)
	}
	if got.Fingerprint != "fp-1" {
		t.Fatalf("expected fingerprint %q, got %q", "fp-1", got.Fingerprint)
	}
}

func TestRegisterHTTP_CantonNative_ResponseCheck(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().
		RegisterCantonNativeUser(mock.Anything, mock.Anything).
		Return(&user.RegisterResponse{
			Party:      "party-native",
			EVMAddress: "0xabc",
		}, nil)
	handler := newRegisterTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(`{"canton_party_id":"party::1"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type %q, got %q", "application/json", ct)
	}

	var got user.RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if got.Party != "party-native" {
		t.Fatalf("expected party %q, got %q", "party-native", got.Party)
	}
	if got.EVMAddress != "0xabc" {
		t.Fatalf("expected evm_address %q, got %q", "0xabc", got.EVMAddress)
	}
}
