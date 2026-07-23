// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const (
	defaultLimitForListTransfer = 100
	maxRequestBodyBytes         = 1 << 20 // 1MB
)

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
		// Internal registration endpoint for observer-mechanism transfers
		// (called by the api-server at initiation time, not by end users).
		r.Post("/transfers", apphttp.HandleError(h.registerTransfer))
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

func (h *HTTP) registerTransfer(w http.ResponseWriter, r *http.Request) error {
	var req relayer.RegisterTransferRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes)).Decode(&req); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}

	resp, err := h.service.RegisterTransfer(r.Context(), &req)
	if err != nil {
		return err
	}

	status := http.StatusOK
	if resp.Created {
		status = http.StatusCreated
	}
	h.writeJSON(w, status, resp)
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
