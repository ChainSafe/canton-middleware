// SPDX-License-Identifier: Apache-2.0

package transfer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

// stubService embeds Service so it satisfies the interface; only the method under
// test is implemented. Any other call panics, which is the desired signal in a test.
type stubService struct {
	Service
	gotAddr string
}

func (s *stubService) ListIncoming(_ context.Context, evmAddr string, _ indexer.Pagination) (*IncomingTransfersList, error) {
	s.gotAddr = evmAddr
	return &IncomingTransfersList{}, nil
}

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

// TestListIncoming_UsesTokenIdentity verifies the read endpoint derives the address
// from the authenticated context (no ?address= supplied).
func TestListIncoming_UsesTokenIdentity(t *testing.T) {
	// The address in context is already checksummed at token issuance; the handler
	// passes it through unchanged.
	authed := auth.NormalizeAddress("0x000000000000000000000000000000000000dead")
	svc := &stubService{}

	r := chi.NewRouter()
	RegisterRoutes(r, svc, authAs(authed), zap.NewNop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/transfer/incoming", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if svc.gotAddr != authed {
		t.Fatalf("service received %q, want authenticated address %q", svc.gotAddr, authed)
	}
}

// TestListIncoming_MatchingQueryAddressAllowed verifies that a ?address= equal to the
// token identity (case-insensitively) is accepted and resolves to the token address.
func TestListIncoming_MatchingQueryAddressAllowed(t *testing.T) {
	authed := auth.NormalizeAddress("0x000000000000000000000000000000000000dead")
	svc := &stubService{}

	r := chi.NewRouter()
	RegisterRoutes(r, svc, authAs(authed), zap.NewNop())

	rec := httptest.NewRecorder()
	// Lowercased form of the same address must still be accepted.
	req := httptest.NewRequest(http.MethodGet, "/api/v2/transfer/incoming?address=0x000000000000000000000000000000000000dead", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if svc.gotAddr != authed {
		t.Fatalf("service received %q, want authenticated address %q", svc.gotAddr, authed)
	}
}

// TestListIncoming_MismatchedQueryAddressForbidden verifies that a ?address= that does
// not match the token is rejected with a 403 (instead of silently returning the
// caller's own data), and the service is never reached.
func TestListIncoming_MismatchedQueryAddressForbidden(t *testing.T) {
	authed := auth.NormalizeAddress("0x000000000000000000000000000000000000dead")
	svc := &stubService{}

	r := chi.NewRouter()
	RegisterRoutes(r, svc, authAs(authed), zap.NewNop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/transfer/incoming?address=0x0000000000000000000000000000000000000001", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if svc.gotAddr != "" {
		t.Fatalf("service must not be called on a mismatched address, got %q", svc.gotAddr)
	}
}

// passthrough is the middleware used when read auth is disabled.
func passthrough(next http.Handler) http.Handler { return next }

// TestListIncoming_AuthDisabled_UsesQueryAddress verifies that with auth disabled
// (passthrough middleware, no context identity) the handler falls back to ?address=.
func TestListIncoming_AuthDisabled_UsesQueryAddress(t *testing.T) {
	want := auth.NormalizeAddress("0x0000000000000000000000000000000000000001")
	svc := &stubService{}

	r := chi.NewRouter()
	RegisterRoutes(r, svc, passthrough, zap.NewNop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/transfer/incoming?address="+want, nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if svc.gotAddr != want {
		t.Fatalf("service received %q, want %q", svc.gotAddr, want)
	}
}

// TestListIncoming_AuthDisabled_MissingAddress400 verifies that with auth disabled
// and no ?address= there is no caller to resolve, so it's a 400.
func TestListIncoming_AuthDisabled_MissingAddress400(t *testing.T) {
	svc := &stubService{}

	r := chi.NewRouter()
	RegisterRoutes(r, svc, passthrough, zap.NewNop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/transfer/incoming", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if svc.gotAddr != "" {
		t.Fatal("service must not be called without a resolvable caller")
	}
}
