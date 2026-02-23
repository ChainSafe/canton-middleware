package service

import (
	"encoding/json"
	"io"
	"net/http"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/registration"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// HTTP wraps the Service to provide HTTP endpoints
type HTTP struct {
	service Service
	logger  *zap.Logger
}

// RegisterRoutes registers HTTP endpoints for registration service on the given chi router
func RegisterRoutes(r chi.Router, service Service, logger *zap.Logger) {
	h := &HTTP{
		service: service,
		logger:  logger,
	}

	r.Post("/register", apphttp.HandleError(h.register))
}

// register handles HTTP requests
func (h *HTTP) register(w http.ResponseWriter, r *http.Request) error {
	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		return apperrors.BadRequestError(err, "failed to read request")
	}

	// Parse request
	var req registration.RegisterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}

	var resp *registration.RegisterResponse
	var regErr error

	// Determine registration type and route accordingly
	if req.CantonPartyID != "" {
		// Canton native user registration
		resp, regErr = h.service.RegisterCantonNativeUser(r.Context(), &req)
	} else {
		// Web3 user registration
		// Try headers if not in body
		if req.Signature == "" {
			req.Signature = r.Header.Get("X-Signature")
			req.Message = r.Header.Get("X-Message")
		}

		if req.Signature == "" || req.Message == "" {
			return apperrors.UnAuthorizedError(nil, "signature and message required")
		}
		resp, regErr = h.service.RegisterWeb3User(r.Context(), &req)
	}

	if regErr != nil {
		return regErr
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *HTTP) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
