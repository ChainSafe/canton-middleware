package service

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

// HTTP wraps the Service to provide HTTP endpoints.
type HTTP struct {
	service Service
	logger  *zap.Logger
}

// RegisterPrivateRoutes registers the indexer admin API on the given chi router.
// All routes are mounted under /indexer/v1/admin.
//
// These routes are unauthenticated and intended for internal/trusted callers only
// (e.g. backend services, ops tooling). A public, JWT-protected read API will be
// added in a future iteration — at that point these routes will remain separate.
// Callers are responsible for restricting network access to this port.
func RegisterPrivateRoutes(r chi.Router, svc Service, logger *zap.Logger) {
	h := &HTTP{service: svc, logger: logger}

	r.Route("/indexer/v1/admin", func(r chi.Router) {
		r.Get("/tokens", apphttp.HandleError(h.listTokens))
		r.Get("/tokens/{admin}/{id}", apphttp.HandleError(h.getToken))
		r.Get("/tokens/{admin}/{id}/supply", apphttp.HandleError(h.getTokenSupply))
		r.Get("/tokens/{admin}/{id}/balances", apphttp.HandleError(h.listTokenBalances))
		r.Get("/tokens/{admin}/{id}/events", apphttp.HandleError(h.listTokenEvents))

		r.Get("/parties/{partyID}/balances", apphttp.HandleError(h.listPartyBalances))
		r.Get("/parties/{partyID}/balances/{admin}/{id}", apphttp.HandleError(h.getPartyBalance))
		r.Get("/parties/{partyID}/events", apphttp.HandleError(h.listPartyEvents))
		r.Get("/parties/{partyID}/pending-offers", apphttp.HandleError(h.listPendingOffers))
		r.Get("/pending-offers", apphttp.HandleError(h.listAllPendingOffers))

		r.Get("/events/{contractID}", apphttp.HandleError(h.getEvent))
	})
}

func (h *HTTP) listTokens(w http.ResponseWriter, r *http.Request) error {
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	page, err := h.service.ListTokens(r.Context(), p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func (h *HTTP) getToken(w http.ResponseWriter, r *http.Request) error {
	admin := chi.URLParam(r, "admin")
	id := chi.URLParam(r, "id")
	t, err := h.service.GetToken(r.Context(), admin, id)
	if err != nil {
		return err
	}
	h.writeJSON(w, t)
	return nil
}

func (h *HTTP) getTokenSupply(w http.ResponseWriter, r *http.Request) error {
	admin := chi.URLParam(r, "admin")
	id := chi.URLParam(r, "id")
	supply, err := h.service.TotalSupply(r.Context(), admin, id)
	if err != nil {
		return err
	}
	h.writeJSON(w, map[string]string{"total_supply": supply})
	return nil
}

func (h *HTTP) listTokenBalances(w http.ResponseWriter, r *http.Request) error {
	admin := chi.URLParam(r, "admin")
	id := chi.URLParam(r, "id")
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	page, err := h.service.ListBalancesForToken(r.Context(), admin, id, p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func (h *HTTP) listTokenEvents(w http.ResponseWriter, r *http.Request) error {
	admin := chi.URLParam(r, "admin")
	id := chi.URLParam(r, "id")
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	et, err := parseEventType(r)
	if err != nil {
		return err
	}
	page, err := h.service.ListTokenEvents(r.Context(), admin, id, indexer.EventFilter{EventType: et}, p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func (h *HTTP) listPartyBalances(w http.ResponseWriter, r *http.Request) error {
	partyID := chi.URLParam(r, "partyID")
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	page, err := h.service.ListBalancesForParty(r.Context(), partyID, p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func (h *HTTP) getPartyBalance(w http.ResponseWriter, r *http.Request) error {
	partyID := chi.URLParam(r, "partyID")
	admin := chi.URLParam(r, "admin")
	id := chi.URLParam(r, "id")
	b, err := h.service.GetBalance(r.Context(), partyID, admin, id)
	if err != nil {
		return err
	}
	h.writeJSON(w, b)
	return nil
}

func (h *HTTP) listPartyEvents(w http.ResponseWriter, r *http.Request) error {
	partyID := chi.URLParam(r, "partyID")
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	et, err := parseEventType(r)
	if err != nil {
		return err
	}
	page, err := h.service.ListPartyEvents(r.Context(), partyID, indexer.EventFilter{EventType: et}, p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func (h *HTTP) getEvent(w http.ResponseWriter, r *http.Request) error {
	contractID := chi.URLParam(r, "contractID")
	e, err := h.service.GetEvent(r.Context(), contractID)
	if err != nil {
		return err
	}
	h.writeJSON(w, e)
	return nil
}

func (h *HTTP) listPendingOffers(w http.ResponseWriter, r *http.Request) error {
	partyID := chi.URLParam(r, "partyID")
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	page, err := h.service.GetPendingOffersForParty(r.Context(), partyID, p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func (h *HTTP) listAllPendingOffers(w http.ResponseWriter, r *http.Request) error {
	p, err := parsePagination(r)
	if err != nil {
		return err
	}
	page, err := h.service.GetAllPendingOffers(r.Context(), p)
	if err != nil {
		return err
	}
	h.writeJSON(w, page)
	return nil
}

func parsePagination(r *http.Request) (indexer.Pagination, error) {
	p := indexer.Pagination{Page: 1, Limit: DefaultLimit}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		v, err := strconv.Atoi(pageStr)
		if err != nil || v < 1 {
			return p, apperrors.BadRequestError(nil, "page must be an integer >= 1")
		}
		p.Page = v
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v < 1 || v > MaxLimit {
			return p, apperrors.BadRequestError(nil, "limit must be an integer between 1 and 200")
		}
		p.Limit = v
	}
	return p, nil
}

func parseEventType(r *http.Request) (indexer.EventType, error) {
	et := r.URL.Query().Get("event_type")
	if et == "" {
		return "", nil
	}
	switch indexer.EventType(et) {
	case indexer.EventMint, indexer.EventBurn, indexer.EventTransfer:
		return indexer.EventType(et), nil
	default:
		return "", apperrors.BadRequestError(nil, "event_type must be MINT, BURN, or TRANSFER")
	}
}

func (h *HTTP) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", zap.Error(err))
	}
}
