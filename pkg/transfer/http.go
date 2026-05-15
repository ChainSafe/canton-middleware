package transfer

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

const (
	maxRequestBodyBytes = 1 << 20 // 1MB
	messageMaxAge       = 5 * time.Minute

	// listIncomingDefaultLimit / listIncomingMaxLimit cap how many pending
	// offers a single GET /api/v2/transfer/incoming response returns. Match
	// the indexer admin API's caps so we never ask the indexer for more than
	// it would serve in one round-trip.
	listIncomingDefaultLimit = 50
	listIncomingMaxLimit     = 200
)

type httpHandler struct {
	svc    Service
	logger *zap.Logger
}

// RegisterRoutes registers the non-custodial prepare/execute transfer endpoints.
func RegisterRoutes(r chi.Router, svc Service, logger *zap.Logger) {
	h := &httpHandler{svc: svc, logger: logger}

	r.Post("/api/v2/transfer/prepare", apphttp.HandleError(h.prepare))
	r.Post("/api/v2/transfer/execute", apphttp.HandleError(h.execute))

	r.Get("/api/v2/transfer/incoming", apphttp.HandleError(h.listIncoming))
	r.Post("/api/v2/transfer/incoming/{contractID}/prepare", apphttp.HandleError(h.prepareAccept))
	r.Post("/api/v2/transfer/incoming/{contractID}/execute", apphttp.HandleError(h.executeAccept))
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

	if req.To == "" || req.Amount == "" || req.Token == "" {
		return apperrors.BadRequestError(nil, "to, amount, and token are required")
	}
	if !auth.ValidateEVMAddress(req.To) {
		return apperrors.BadRequestError(nil, "invalid recipient address: must be a 0x-prefixed 40-hex-char EVM address")
	}
	amt, parseErr := decimal.NewFromString(req.Amount)
	if parseErr != nil || !amt.IsPositive() {
		return apperrors.BadRequestError(nil, "invalid amount: must be a positive decimal number")
	}

	resp, err := h.svc.Prepare(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, resp)
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

	if req.TransferID == "" || req.Signature == "" || req.SignedBy == "" {
		return apperrors.BadRequestError(nil, "transfer_id, signature, and signed_by are required")
	}

	resp, err := h.svc.Execute(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, resp)
	return nil
}

// listIncoming is intentionally unauthenticated for now: callers pass the EVM
// address as a query parameter and receive that user's pending offers. The
// endpoint is read-only and exposes only data already visible to the receiver
// party on-ledger, so dropping the signature requirement does not leak anything
// new — it just lets clients (and tests) poll incoming offers without prior
// signing-key access. Sensitive fields (party IDs) are truncated server-side.
//
// Pagination is page/limit based to match the indexer envelope so each request
// translates to exactly one indexer round-trip — no in-process buffering of all
// offers for a receiver.
func (h *httpHandler) listIncoming(w http.ResponseWriter, r *http.Request) error {
	evmAddr := strings.TrimSpace(r.URL.Query().Get("address"))
	if evmAddr == "" {
		return apperrors.BadRequestError(nil, "address query parameter is required")
	}
	if !auth.ValidateEVMAddress(evmAddr) {
		return apperrors.BadRequestError(nil, "invalid address: must be a 0x-prefixed 40-hex-char EVM address")
	}

	p, err := parseListPagination(r)
	if err != nil {
		return err
	}

	resp, err := h.svc.ListIncoming(r.Context(), auth.NormalizeAddress(evmAddr), p)
	if err != nil {
		return err
	}

	h.writeJSON(w, resp)
	return nil
}

// parseListPagination reads ?page=N&limit=L from the request, defaulting to
// {Page:1, Limit:listIncomingDefaultLimit} when either is omitted. Mirrors the
// indexer admin API's bounds (max 200) so a client can't ask for more than the
// underlying source is willing to serve.
func parseListPagination(r *http.Request) (indexer.Pagination, error) {
	p := indexer.Pagination{Page: 1, Limit: listIncomingDefaultLimit}
	if s := r.URL.Query().Get("page"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			return p, apperrors.BadRequestError(nil, "page must be an integer >= 1")
		}
		p.Page = v
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > listIncomingMaxLimit {
			return p, apperrors.BadRequestError(nil, "limit must be an integer between 1 and 200")
		}
		p.Limit = v
	}
	return p, nil
}

func (h *httpHandler) prepareAccept(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	contractID := chi.URLParam(r, "contractID")
	if contractID == "" {
		return apperrors.BadRequestError(nil, "contractID path parameter is required")
	}

	var req PrepareAcceptRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.InstrumentAdmin == "" {
		return apperrors.BadRequestError(nil, "instrument_admin is required")
	}

	resp, err := h.svc.PrepareAccept(r.Context(), evmAddr, contractID, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, resp)
	return nil
}

func (h *httpHandler) executeAccept(w http.ResponseWriter, r *http.Request) error {
	evmAddr, err := authenticateEVM(r)
	if err != nil {
		return err
	}

	var req ExecuteRequest
	if jsonErr := readJSON(r, &req); jsonErr != nil {
		return jsonErr
	}
	if req.TransferID == "" || req.Signature == "" || req.SignedBy == "" {
		return apperrors.BadRequestError(nil, "transfer_id, signature, and signed_by are required")
	}

	resp, err := h.svc.ExecuteAccept(r.Context(), evmAddr, &req)
	if err != nil {
		return err
	}

	h.writeJSON(w, resp)
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
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperrors.BadRequestError(err, "invalid JSON")
	}
	return nil
}

func (h *httpHandler) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", zap.Error(err))
	}
}
