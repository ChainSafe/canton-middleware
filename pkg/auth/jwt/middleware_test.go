// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/auth"
)

func TestRequireAuth_RejectsMissingAndAcceptsValid(t *testing.T) {
	issuer := NewIssuer(newTestKey(t), "kid-1", testIssuer, testAud, time.Hour)
	validator := NewValidatorWithKey(issuer.KeyID(), issuer.PublicKey(), testIssuer)

	var seenAddr string
	protected := RequireAuth(validator, testAud)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAddr, _ = auth.EVMAddressFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// No token -> 401.
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", rec.Code)
	}

	// Valid token -> 200, identity populated.
	token, _, _ := issuer.Issue("0xdead", "party::dead")
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token status = %d, want 200", rec.Code)
	}
	if seenAddr != "0xdead" {
		t.Fatalf("context evm address = %q, want 0xdead", seenAddr)
	}
}

func TestRequireAuth_WrongAudienceRejected(t *testing.T) {
	issuer := NewIssuer(newTestKey(t), "kid-1", testIssuer, "some-other-aud", time.Hour)
	validator := NewValidatorWithKey(issuer.KeyID(), issuer.PublicKey(), testIssuer)

	protected := RequireAuth(validator, testAud)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token, _, _ := issuer.Issue("0xdead", "party::dead")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("audience mismatch status = %d, want 401", rec.Code)
	}
}
