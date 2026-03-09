package service

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
)

const defaultLimitForListTransfer = 100

// Engine is the interface for checking relayer readiness.
type Engine interface {
	IsReady() bool
}

// HTTP wraps Service and Engine to provide HTTP endpoints.
type HTTP struct {
	service Service
	engine  Engine
	logger  *zap.Logger
}

// RegisterRoutes registers relayer HTTP endpoints on the given chi router.
func RegisterRoutes(r chi.Router, svc Service, engine Engine, logger *zap.Logger) {
	h := &HTTP{service: svc, engine: engine, logger: logger}

	r.Get("/ready", h.ready)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/transfers", apphttp.HandleError(h.listTransfers))
		r.Get("/transfers/{id}", apphttp.HandleError(h.getTransfer))
		r.Get("/status", apphttp.HandleError(h.getStatus))
	})
}

func (h *HTTP) ready(w http.ResponseWriter, _ *http.Request) {
	if !h.engine.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("NOT_READY"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("READY"))
}

func (h *HTTP) listTransfers(w http.ResponseWriter, r *http.Request) error {
	transfers, err := h.service.ListTransfers(r.Context(), defaultLimitForListTransfer)
	if err != nil {
		h.logger.Error("Failed to list transfers", zap.Error(err))
		return apperrors.GeneralError(err)
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"transfers": transfers})
	return nil
}

func (h *HTTP) getTransfer(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	transfer, err := h.service.GetTransfer(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get transfer", zap.Error(err), zap.String("id", id))
		return apperrors.GeneralError(err)
	}
	if transfer == nil {
		return apperrors.ResourceNotFoundError(nil, "transfer not found")
	}

	h.writeJSON(w, http.StatusOK, transfer)
	return nil
}

func (h *HTTP) getStatus(w http.ResponseWriter, _ *http.Request) error {
	h.writeJSON(w, http.StatusOK, map[string]any{"status": "running"})
	return nil
}

func (h *HTTP) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", zap.Error(err))
	}
}
