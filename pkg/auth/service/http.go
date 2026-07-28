// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/auth"
)

const maxRequestBodyBytes = 1 << 20 // 1MB

type httpHandler struct {
	svc    Service
	logger *zap.Logger
}

// RegisterRoutes mounts the login and JWKS endpoints on r.
func RegisterRoutes(r chi.Router, svc Service, logger *zap.Logger) {
	h := &httpHandler{svc: svc, logger: logger}

	r.Get("/auth/nonce", apphttp.HandleError(h.nonce))
	r.Post("/auth/login", apphttp.HandleError(h.login))
	r.Get("/.well-known/jwks.json", apphttp.HandleError(h.jwks))

	logger.Info("SIWE login enabled", zap.String("path", "/auth/login"))
}

// nonce issues a login nonce for the caller's address (GET /auth/nonce?address=).
// The nonce is keyed by address so repeat requests reuse the same live value, which
// stops an unauthenticated caller from churning the store; the address is the one
// the client will put in its SIWE message and is not a secret.
func (h *httpHandler) nonce(w http.ResponseWriter, r *http.Request) error {
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	if !auth.ValidateEVMAddress(address) {
		return apperrors.BadRequestError(nil, "address query parameter must be a 0x-prefixed 40-hex-char EVM address")
	}

	nonce, err := h.svc.Nonce(auth.NormalizeAddress(address))
	if err != nil {
		h.logger.Warn("nonce issuance rejected", zap.Error(err))
		return apperrors.GeneralError(err)
	}

	writeJSON(w, http.StatusOK, &auth.NonceResponse{Nonce: nonce})
	return nil
}

func (h *httpHandler) login(w http.ResponseWriter, r *http.Request) error {
	var req auth.LoginRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Message == "" || req.Signature == "" {
		return apperrors.BadRequestError(nil, "message and signature are required")
	}

	res, err := h.svc.Login(r.Context(), req.Message, req.Signature)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, res)
	return nil
}

func (h *httpHandler) jwks(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, h.svc.JWKS())
	return nil
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
