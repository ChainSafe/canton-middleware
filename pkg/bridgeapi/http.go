// SPDX-License-Identifier: Apache-2.0

package bridgeapi

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
	svc    Service
	logger *zap.Logger
}

// RegisterRoutes registers the bridge endpoints.
func RegisterRoutes(r chi.Router, svc Service, logger *zap.Logger) {
	h := &httpHandler{svc: svc, logger: logger}

	r.Get("/api/v2/bridge/tokens", apphttp.HandleError(h.tokens))
	r.Post("/api/v2/bridge/deposit/quote", apphttp.HandleError(h.depositQuote))
	r.Post("/api/v2/bridge/deposits", apphttp.HandleError(h.registerDeposit))
	r.Get("/api/v2/bridge/transfers/{id}", apphttp.HandleError(h.getTransfer))
	r.Post("/api/v2/bridge/withdraw/prepare", apphttp.HandleError(h.withdrawPrepare))
	r.Post("/api/v2/bridge/withdraw/execute", apphttp.HandleError(h.withdrawExecute))
	r.Post("/api/v2/bridge/withdraw/custodial", apphttp.HandleError(h.withdrawCustodial))
}

func (h *httpHandler) tokens(w http.ResponseWriter, r *http.Request) error {
	h.writeJSON(w, http.StatusOK, map[string]any{"tokens": h.svc.Tokens(r.Context())})
	return nil
}

func (h *httpHandler) depositQuote(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req QuoteRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.Token == "" || req.Amount == "" {
		return apperrors.BadRequestError(nil, "token and amount are required")
	}

	quote, err := h.svc.DepositQuote(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, quote)
	return nil
}

func (h *httpHandler) registerDeposit(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req RegisterDepositRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.Token == "" || req.Amount == "" || req.TxHash == "" {
		return apperrors.BadRequestError(nil, "token, amount, and tx_hash are required")
	}

	resp, err := h.svc.RegisterDeposit(r.Context(), evmAddr, &req)
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

func (h *httpHandler) getTransfer(w http.ResponseWriter, r *http.Request) error {
	// Ids are public tx hashes, so require a session to avoid leaking other
	// users' sender/recipient/amount.
	if _, err := authenticateEVM(r); err != nil {
		return err
	}

	transfer, err := h.svc.GetTransfer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, transfer)
	return nil
}

func (h *httpHandler) withdrawPrepare(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req WithdrawRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.Token == "" || req.Amount == "" || req.Recipient == "" {
		return apperrors.BadRequestError(nil, "token, amount, and recipient are required")
	}

	resp, err := h.svc.WithdrawPrepare(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *httpHandler) withdrawExecute(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req WithdrawExecuteRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.TransferID == "" || req.Signature == "" || req.SignedBy == "" {
		return apperrors.BadRequestError(nil, "transfer_id, signature, and signed_by are required")
	}

	resp, err := h.svc.WithdrawExecute(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusCreated, resp)
	return nil
}

func (h *httpHandler) withdrawCustodial(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req WithdrawRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.Token == "" || req.Amount == "" || req.Recipient == "" {
		return apperrors.BadRequestError(nil, "token, amount, and recipient are required")
	}

	resp, err := h.svc.WithdrawCustodial(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, http.StatusCreated, resp)
	return nil
}

// authenticateEVM recovers the sender EVM address from X-Signature / X-Message
// headers, mirroring the transfer package's per-handler auth.
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
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}
	return nil
}

func (h *httpHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	buf, err := json.Marshal(data)
	if err != nil {
		h.logger.Error("failed to marshal JSON response", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}
