// SPDX-License-Identifier: Apache-2.0

package whitelist_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist/mocks"
)

const (
	adminToken = "s3cr3t-admin-token"
	adminAddr  = "0xAbC1230000000000000000000000000000000001"
)

func newAdminServer(t *testing.T, mgr whitelist.Manager) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	whitelist.RegisterAdminRoutes(r, mgr, adminToken, zap.NewNop())
	return r
}

func adminReq(method, target, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, http.NoBody)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	r.Header.Set("Authorization", "Bearer "+adminToken)
	return r
}

func TestAdminAdd_Success(t *testing.T) {
	mgr := mocks.NewManager(t)
	mgr.EXPECT().Add(mock.Anything, adminAddr, "vip").Return(nil)

	rec := httptest.NewRecorder()
	newAdminServer(t, mgr).ServeHTTP(rec, adminReq(http.MethodPost, "/admin/whitelist",
		`{"evm_address":"`+adminAddr+`","note":"vip"}`))

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAdminAdd_InvalidJSON(t *testing.T) {
	mgr := mocks.NewManager(t) // no call expected
	rec := httptest.NewRecorder()
	newAdminServer(t, mgr).ServeHTTP(rec, adminReq(http.MethodPost, "/admin/whitelist", `{not json`))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminRemove_NotFoundPropagates(t *testing.T) {
	mgr := mocks.NewManager(t)
	mgr.EXPECT().Remove(mock.Anything, adminAddr).
		Return(apperrors.ResourceNotFoundError(nil, "address not whitelisted"))

	rec := httptest.NewRecorder()
	newAdminServer(t, mgr).ServeHTTP(rec, adminReq(http.MethodDelete, "/admin/whitelist/"+adminAddr, ""))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestAdminList_PassesPaginationParams verifies the handler parses ?cursor=&limit=
// and forwards them to the service, returning the page envelope.
func TestAdminList_PassesPaginationParams(t *testing.T) {
	mgr := mocks.NewManager(t)
	mgr.EXPECT().List(mock.Anything, "0xStart", 5).Return(&whitelist.Page{
		Items:      []user.WhitelistEntry{{EVMAddress: adminAddr}},
		NextCursor: adminAddr,
		HasMore:    true,
	}, nil)

	rec := httptest.NewRecorder()
	newAdminServer(t, mgr).ServeHTTP(rec, adminReq(http.MethodGet, "/admin/whitelist?cursor=0xStart&limit=5", ""))

	require.Equal(t, http.StatusOK, rec.Code)
	var page whitelist.Page
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.True(t, page.HasMore)
	require.Equal(t, adminAddr, page.NextCursor)
	require.Len(t, page.Items, 1)
}

func TestAdminList_DefaultLimit(t *testing.T) {
	mgr := mocks.NewManager(t)
	// No limit query param → DefaultLimit, empty cursor.
	mgr.EXPECT().List(mock.Anything, "", whitelist.DefaultLimit).Return(&whitelist.Page{}, nil)

	rec := httptest.NewRecorder()
	newAdminServer(t, mgr).ServeHTTP(rec, adminReq(http.MethodGet, "/admin/whitelist", ""))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAdminList_InvalidLimit(t *testing.T) {
	mgr := mocks.NewManager(t) // no service call expected
	rec := httptest.NewRecorder()
	newAdminServer(t, mgr).ServeHTTP(rec, adminReq(http.MethodGet, "/admin/whitelist?limit=99999", ""))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminAuth_Rejected(t *testing.T) {
	mgr := mocks.NewManager(t) // no calls on rejected requests
	srv := newAdminServer(t, mgr)

	cases := []struct{ name, header string }{
		{"missing", ""},
		{"wrong token", "Bearer wrong-token"},
		{"not bearer", "Basic " + adminToken},
		{"empty bearer", "Bearer "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/admin/whitelist", http.NoBody)
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}
			srv.ServeHTTP(rec, r)
			require.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}
