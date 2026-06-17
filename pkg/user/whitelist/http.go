// SPDX-License-Identifier: Apache-2.0

package whitelist

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
)

const maxRequestBodyBytes = 1 << 20 // 1MB

// addRequest is the body for POST /admin/whitelist.
type addRequest struct {
	EVMAddress string `json:"evm_address"`
	Note       string `json:"note,omitempty"`
}

type httpHandler struct {
	mgr    Manager
	logger *zap.Logger
}

// RegisterAdminRoutes mounts the privileged whitelist-management endpoints under
// /admin on r, gated by a static bearer token.
func RegisterAdminRoutes(r chi.Router, mgr Manager, token string, logger *zap.Logger) {
	h := &httpHandler{mgr: mgr, logger: logger}

	r.Route("/admin", func(ar chi.Router) {
		ar.Use(bearerAuth(token))
		ar.Post("/whitelist", apphttp.HandleError(h.add))
		ar.Delete("/whitelist/{address}", apphttp.HandleError(h.remove))
		ar.Get("/whitelist", apphttp.HandleError(h.list))
	})

	logger.Info("Admin API enabled", zap.String("path", "/admin/whitelist"))
}

// add handles POST /admin/whitelist — adds (or updates the note of) an address.
func (h *httpHandler) add(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		return apperrors.BadRequestError(err, "failed to read request")
	}

	var req addRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return apperrors.BadRequestError(jsonErr, "invalid JSON")
	}

	if err := h.mgr.Add(r.Context(), req.EVMAddress, req.Note); err != nil {
		return err
	}

	h.writeJSON(w, map[string]string{"status": "whitelisted"})
	return nil
}

// remove handles DELETE /admin/whitelist/{address} — 404 when not whitelisted.
func (h *httpHandler) remove(w http.ResponseWriter, r *http.Request) error {
	if err := h.mgr.Remove(r.Context(), chi.URLParam(r, "address")); err != nil {
		return err
	}

	h.writeJSON(w, map[string]string{"status": "removed"})
	return nil
}

// list handles GET /admin/whitelist?cursor=&limit= — cursor-paginated listing.
func (h *httpHandler) list(w http.ResponseWriter, r *http.Request) error {
	cursor, limit, err := parsePagination(r)
	if err != nil {
		return err
	}

	page, err := h.mgr.List(r.Context(), cursor, limit)
	if err != nil {
		return err
	}

	h.writeJSON(w, page)
	return nil
}

// parsePagination reads ?cursor=&limit= from the request, defaulting limit to
// DefaultLimit and rejecting values outside [1, MaxLimit].
func parsePagination(r *http.Request) (cursor string, limit int, err error) {
	cursor = r.URL.Query().Get("cursor")
	limit = DefaultLimit

	if s := r.URL.Query().Get("limit"); s != "" {
		v, parseErr := strconv.Atoi(s)
		if parseErr != nil || v < 1 || v > MaxLimit {
			return "", 0, apperrors.BadRequestError(nil, "limit must be an integer between 1 and 200")
		}
		limit = v
	}

	return cursor, limit, nil
}

func (h *httpHandler) writeJSON(w http.ResponseWriter, data any) {
	// Marshal before writing the status line so a serialization failure yields a
	// 500 rather than a 200 with a truncated body.
	buf, err := json.Marshal(data)
	if err != nil {
		h.logger.Error("failed to marshal JSON response", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)
}

// bearerAuth returns a chi middleware that authorizes requests carrying a static
// admin token as "Authorization: Bearer <token>". The comparison is constant-time
// over SHA-256 digests so neither the token value nor its length leaks via timing.
func bearerAuth(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := bearerToken(r)
			got := sha256.Sum256([]byte(provided))
			if provided == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				apphttp.DefaultErrorHandler(w, apperrors.UnAuthorizedError(nil, "invalid or missing admin credentials"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header,
// returning "" when the header is absent or not a bearer credential.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
