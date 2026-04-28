package token

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// ListService is the narrow interface the HTTP layer depends on.
//
//go:generate mockery --name ListService --output mocks --outpkg mocks --filename mock_list_service.go --with-expecter
type ListService interface {
	GetSupportedTokens(ctx context.Context, cursor string, limit int) (*TokensPage, error)
}

// HTTP wraps ListService to provide token HTTP endpoints.
type HTTP struct {
	svc    ListService
	logger *zap.Logger
}

// RegisterRoutes registers token endpoints on the given router.
func RegisterRoutes(r chi.Router, svc ListService, logger *zap.Logger) {
	h := &HTTP{svc: svc, logger: logger}
	r.Get("/tokens", apphttp.HandleError(h.listTokens))
}

func (h *HTTP) listTokens(w http.ResponseWriter, r *http.Request) error {
	cursor, limit, err := parsePagination(r)
	if err != nil {
		return err
	}
	resp, err := h.svc.GetSupportedTokens(r.Context(), cursor, limit)
	if err != nil {
		return err
	}
	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

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

func (h *HTTP) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", zap.Error(err))
	}
}
