package transfer

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/auth"
)

const (
	maxRequestBodyBytes = 1 << 20 // 1MB
	messageMaxAge       = 5 * time.Minute
)

type httpHandler struct {
	svc    *TransferService
	logger *zap.Logger
}

// RegisterRoutes registers the non-custodial prepare/execute transfer endpoints.
func RegisterRoutes(r chi.Router, svc *TransferService, logger *zap.Logger) {
	h := &httpHandler{svc: svc, logger: logger}

	r.Post("/api/v2/transfer/prepare", apphttp.HandleError(h.prepare))
	r.Post("/api/v2/transfer/execute", apphttp.HandleError(h.execute))
}

func (h *httpHandler) prepare(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req PrepareRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}

	resp, err := h.svc.Prepare(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *httpHandler) execute(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req ExecuteRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}

	resp, err := h.svc.Execute(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

// authenticateEVM recovers the sender EVM address from X-Signature / X-Message headers.
// The message must contain a colon-separated Unix timestamp (e.g. "transfer:1710000000")
// that is within messageMaxAge of the current server time.
func authenticateEVM(r *http.Request) (string, error) {
	sig := r.Header.Get("X-Signature")
	msg := r.Header.Get("X-Message")
	if sig == "" || msg == "" {
		return "", apperrors.UnAuthorizedError(nil, "authentication required")
	}

	if err := auth.ValidateTimedMessage(msg, messageMaxAge); err != nil {
		return "", apperrors.UnAuthorizedError(err, "message expired or invalid format")
	}

	recovered, err := auth.VerifyEIP191Signature(msg, sig)
	if err != nil {
		return "", apperrors.UnAuthorizedError(err, "invalid signature")
	}

	return auth.NormalizeAddress(recovered.Hex()), nil
}

func readJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		return apperrors.BadRequestError(err, "failed to read request")
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}
	return nil
}

func (h *httpHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", zap.Error(err))
	}
}
