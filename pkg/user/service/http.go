package service

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

const maxRequestBodyBytes = 1 << 20

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
	r.Post("/register/prepare-topology", apphttp.HandleError(h.prepareTopology))
	r.Get("/user", apphttp.HandleError(h.getUser))
}

// register handles HTTP requests
func (h *HTTP) register(w http.ResponseWriter, r *http.Request) error {
	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes)) // 1MB limit
	if err != nil {
		return apperrors.BadRequestError(err, "failed to read request")
	}

	// Parse request
	var req user.RegisterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}

	var resp *user.RegisterResponse
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
		if req.KeyMode == "external" {
			if req.RegistrationToken == "" || req.TopologySignature == "" || req.CantonPublicKey == "" {
				return apperrors.BadRequestError(
					nil, "registration_token, topology_signature, and canton_public_key are required for external registration",
				)
			}
		}
		resp, regErr = h.service.RegisterWeb3User(r.Context(), &req)
	}

	if regErr != nil {
		return regErr
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

// prepareTopology handles step 1 of external user registration.
func (h *HTTP) prepareTopology(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		return apperrors.BadRequestError(err, "failed to read request")
	}

	var req user.RegisterRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return apperrors.BadRequestError(jsonErr, "invalid JSON")
	}

	// Try headers if not in body
	if req.Signature == "" {
		req.Signature = r.Header.Get("X-Signature")
		req.Message = r.Header.Get("X-Message")
	}
	if req.Signature == "" || req.Message == "" {
		return apperrors.UnAuthorizedError(nil, "signature and message required")
	}
	if req.CantonPublicKey == "" {
		return apperrors.BadRequestError(nil, "canton_public_key is required")
	}

	resp, err := h.service.PrepareExternalRegistration(r.Context(), &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

// getUser handles GET /user?address=0x... and returns the registered user profile.
// The caller must provide an EIP-191 signature over the message via X-Signature and
// X-Message headers. Credentials are kept out of query params to avoid leaking them
// into server access logs, CDN logs, and browser history.
// Returns 404 if the address is not registered.
func (h *HTTP) getUser(w http.ResponseWriter, r *http.Request) error {
	address := r.URL.Query().Get("address")
	if address == "" {
		return apperrors.BadRequestError(nil, "address query parameter required")
	}

	signature := r.Header.Get("X-Signature")
	message := r.Header.Get("X-Message")
	if signature == "" || message == "" {
		return apperrors.UnAuthorizedError(nil, "X-Signature and X-Message headers required")
	}

	resp, err := h.service.GetUser(r.Context(), address, message, signature)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *HTTP) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", zap.Error(err))
	}
}
