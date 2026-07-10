// SPDX-License-Identifier: Apache-2.0

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

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/user/service/mocks"
)

func newRegisterTestServer(svc Service) http.Handler {
	return newRegisterTestServerWithAuth(svc, passthroughAuth)
}

func newRegisterTestServerWithAuth(svc Service, readAuth func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	RegisterRoutes(r, svc, readAuth, zap.NewNop())
	return r
}

// passthroughAuth forwards the request untouched, simulating a request that
// reached the handler without an authenticated identity in context.
func passthroughAuth(next http.Handler) http.Handler { return next }

// authAs returns middleware that injects a fixed authenticated EVM address,
// simulating a validated bearer token.
func authAs(evmAddress string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithAuthInfo(r.Context(), &auth.AuthInfo{
				EVMAddress:  evmAddress,
				CantonParty: "party::test",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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

func TestGetUserHTTP_AuthDisabled_MissingAddress_Returns400(t *testing.T) {
	svc := mocks.NewService(t)
	// passthroughAuth = auth disabled: no context identity and no ?address=, so
	// there is no caller to resolve — a 400, and GetUser is never called.
	handler := newRegisterTestServerWithAuth(svc, passthroughAuth)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetUserHTTP_AuthDisabled_UsesQueryAddress(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().GetUser(mock.Anything, "0x0000000000000000000000000000000000000001").
		Return(&user.User{EVMAddress: "0x0000000000000000000000000000000000000001"}, nil)
	handler := newRegisterTestServerWithAuth(svc, passthroughAuth)

	req := httptest.NewRequest(http.MethodGet, "/profile?address=0x0000000000000000000000000000000000000001", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestGetUserHTTP_Authenticated_ReturnsUser(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().
		GetUser(mock.Anything, "0xabc").
		Return(&user.User{EVMAddress: "0xabc", CantonParty: "party::xyz"}, nil)
	handler := newRegisterTestServerWithAuth(svc, authAs("0xabc"))

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var got user.User
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if got.EVMAddress != "0xabc" {
		t.Fatalf("expected evm_address %q, got %q", "0xabc", got.EVMAddress)
	}
}

func TestGetUserHTTP_ServiceReturnsNotFound_Returns404(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().
		GetUser(mock.Anything, "0xabc").
		Return(nil, apperrors.ResourceNotFoundError(nil, "user not found"))
	handler := newRegisterTestServerWithAuth(svc, authAs("0xabc"))

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if got.Error != "user not found" {
		t.Fatalf("expected error %q, got %q", "user not found", got.Error)
	}
}
