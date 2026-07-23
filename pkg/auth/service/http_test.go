// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/auth/jwt"
	"github.com/chainsafe/canton-middleware/pkg/auth/service/mocks"
)

func newLoginTestServer(svc Service) http.Handler {
	r := chi.NewRouter()
	RegisterRoutes(r, svc, zap.NewNop())
	return r
}

func TestNonceHTTP_ReturnsNonce(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().Nonce(mock.Anything).Return("the-nonce", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/nonce?address=0x0000000000000000000000000000000000000001", nil)
	newLoginTestServer(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got auth.NonceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Nonce != "the-nonce" {
		t.Fatalf("nonce = %q, want the-nonce", got.Nonce)
	}
}

func TestNonceHTTP_MissingOrInvalidAddress_Returns400(t *testing.T) {
	svc := mocks.NewService(t) // Nonce must not be called

	for _, q := range []string{"/auth/nonce", "/auth/nonce?address=not-an-address"} {
		rec := httptest.NewRecorder()
		newLoginTestServer(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestLoginHTTP_Success(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().Login(mock.Anything, "msg", "sig").
		Return(&auth.LoginResponse{Token: "tok", ExpiresAt: 12345}, nil)

	body, _ := json.Marshal(auth.LoginRequest{Message: "msg", Signature: "sig"})
	rec := httptest.NewRecorder()
	newLoginTestServer(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(body))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got auth.LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Token != "tok" || got.ExpiresAt != 12345 {
		t.Fatalf("response = %+v, want {tok 12345}", got)
	}
}

func TestLoginHTTP_InvalidJSON_Returns400(t *testing.T) {
	svc := mocks.NewService(t) // Login must not be called
	rec := httptest.NewRecorder()
	newLoginTestServer(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{not json")))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLoginHTTP_MissingFields_Returns400(t *testing.T) {
	svc := mocks.NewService(t)                                 // Login must not be called
	body, _ := json.Marshal(auth.LoginRequest{Message: "msg"}) // signature missing
	rec := httptest.NewRecorder()
	newLoginTestServer(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(body))))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLoginHTTP_ServiceUnauthorized_Returns401(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().Login(mock.Anything, "msg", "sig").
		Return(nil, apperrors.UnAuthorizedError(nil, "invalid sign-in message or signature"))

	body, _ := json.Marshal(auth.LoginRequest{Message: "msg", Signature: "sig"})
	rec := httptest.NewRecorder()
	newLoginTestServer(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(body))))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestJWKSHTTP_ServesKeySet(t *testing.T) {
	svc := mocks.NewService(t)
	svc.EXPECT().JWKS().Return(jwt.JWKS{Keys: []jwt.JWK{{Kid: "kid-1", Kty: "RSA"}}})

	rec := httptest.NewRecorder()
	newLoginTestServer(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got jwt.JWKS
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Keys) != 1 || got.Keys[0].Kid != "kid-1" {
		t.Fatalf("jwks = %+v, want one key kid-1", got)
	}
}
