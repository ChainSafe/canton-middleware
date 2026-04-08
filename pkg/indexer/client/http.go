// Package client provides an HTTP client for the indexer's admin API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

// Client is the read interface over the indexer's HTTP admin API.
// Its method set mirrors indexer/service.Service so that callers are agnostic
// to whether they are talking to an in-process service or a remote indexer.
//
//go:generate mockery --name Client --output mocks --outpkg mocks --filename mock_client.go --with-expecter
type Client interface {
	// Token queries
	GetToken(ctx context.Context, admin, id string) (*indexer.Token, error)
	ListTokens(ctx context.Context, p indexer.Pagination) (*indexer.Page[*indexer.Token], error)

	// ERC-20 analogs
	TotalSupply(ctx context.Context, admin, id string) (string, error)

	// Balance queries
	GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error)
	ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error)
	ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error)

	// Audit trail
	GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error)
	ListTokenEvents(ctx context.Context, admin, id string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error)
	ListPartyEvents(ctx context.Context, partyID string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error)
}

// HTTP implements Client by calling the indexer's unauthenticated admin HTTP API.
// All paths are under /indexer/v1/admin.
type HTTP struct {
	baseURL    string
	httpClient *http.Client
}

// New creates an HTTP-backed indexer client.
// baseURL is the indexer's base URL without a trailing slash (e.g. "http://localhost:8080").
// httpClient may be nil; http.DefaultClient is used in that case.
func New(baseURL string, httpClient *http.Client) *HTTP {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTP{baseURL: baseURL, httpClient: httpClient}
}

// GetToken calls GET /indexer/v1/admin/tokens/{admin}/{id}.
func (c *HTTP) GetToken(ctx context.Context, admin, id string) (*indexer.Token, error) {
	u := c.tokenBase(admin, id)
	var t indexer.Token
	if err := c.getJSON(ctx, u, &t); err != nil {
		return nil, fmt.Errorf("get token %s/%s: %w", admin, id, err)
	}
	return &t, nil
}

// ListTokens calls GET /indexer/v1/admin/tokens.
func (c *HTTP) ListTokens(ctx context.Context, p indexer.Pagination) (*indexer.Page[*indexer.Token], error) {
	u := c.baseURL + "/indexer/v1/admin/tokens?" + pageQuery(p).Encode()
	var page indexer.Page[*indexer.Token]
	if err := c.getJSON(ctx, u, &page); err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return &page, nil
}

// TotalSupply calls GET /indexer/v1/admin/tokens/{admin}/{id}/supply.
func (c *HTTP) TotalSupply(ctx context.Context, admin, id string) (string, error) {
	u := c.tokenBase(admin, id) + "/supply"
	var resp struct {
		TotalSupply string `json:"total_supply"`
	}
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return "0", fmt.Errorf("total supply for %s/%s: %w", admin, id, err)
	}
	return resp.TotalSupply, nil
}

// GetBalance calls GET /indexer/v1/admin/parties/{partyID}/balances/{admin}/{id}.
// Returns apperrors.ResourceNotFoundError when the party has no balance record.
func (c *HTTP) GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error) {
	u := c.partyBase(partyID) + "/balances/" + url.PathEscape(admin) + "/" + url.PathEscape(id)
	var b indexer.Balance
	if err := c.getJSON(ctx, u, &b); err != nil {
		return nil, fmt.Errorf("balance for party %s token %s/%s: %w", partyID, admin, id, err)
	}
	return &b, nil
}

// ListBalancesForParty calls GET /indexer/v1/admin/parties/{partyID}/balances.
func (c *HTTP) ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error) {
	u := c.partyBase(partyID) + "/balances?" + pageQuery(p).Encode()
	var page indexer.Page[*indexer.Balance]
	if err := c.getJSON(ctx, u, &page); err != nil {
		return nil, fmt.Errorf("list balances for party %s: %w", partyID, err)
	}
	return &page, nil
}

// ListBalancesForToken calls GET /indexer/v1/admin/tokens/{admin}/{id}/balances.
func (c *HTTP) ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error) {
	u := c.tokenBase(admin, id) + "/balances?" + pageQuery(p).Encode()
	var page indexer.Page[*indexer.Balance]
	if err := c.getJSON(ctx, u, &page); err != nil {
		return nil, fmt.Errorf("list balances for token %s/%s: %w", admin, id, err)
	}
	return &page, nil
}

// GetEvent calls GET /indexer/v1/admin/events/{contractID}.
func (c *HTTP) GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error) {
	u := c.baseURL + "/indexer/v1/admin/events/" + url.PathEscape(contractID)
	var e indexer.ParsedEvent
	if err := c.getJSON(ctx, u, &e); err != nil {
		return nil, fmt.Errorf("get event %s: %w", contractID, err)
	}
	return &e, nil
}

// ListTokenEvents calls GET /indexer/v1/admin/tokens/{admin}/{id}/events.
func (c *HTTP) ListTokenEvents(ctx context.Context, admin, id string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error) {
	q := pageQuery(p)
	if f.EventType != "" {
		q.Set("event_type", string(f.EventType))
	}
	u := c.tokenBase(admin, id) + "/events?" + q.Encode()
	var page indexer.Page[*indexer.ParsedEvent]
	if err := c.getJSON(ctx, u, &page); err != nil {
		return nil, fmt.Errorf("list events for token %s/%s: %w", admin, id, err)
	}
	return &page, nil
}

// ListPartyEvents calls GET /indexer/v1/admin/parties/{partyID}/events.
func (c *HTTP) ListPartyEvents(ctx context.Context, partyID string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error) {
	q := pageQuery(p)
	if f.EventType != "" {
		q.Set("event_type", string(f.EventType))
	}
	u := c.partyBase(partyID) + "/events?" + q.Encode()
	var page indexer.Page[*indexer.ParsedEvent]
	if err := c.getJSON(ctx, u, &page); err != nil {
		return nil, fmt.Errorf("list events for party %s: %w", partyID, err)
	}
	return &page, nil
}

func (c *HTTP) tokenBase(admin, id string) string {
	return fmt.Sprintf("%s/indexer/v1/admin/tokens/%s/%s",
		c.baseURL, url.PathEscape(admin), url.PathEscape(id))
}

func (c *HTTP) partyBase(partyID string) string {
	return fmt.Sprintf("%s/indexer/v1/admin/parties/%s",
		c.baseURL, url.PathEscape(partyID))
}

func pageQuery(p indexer.Pagination) url.Values {
	q := url.Values{}
	q.Set("page", strconv.Itoa(p.Page))
	q.Set("limit", strconv.Itoa(p.Limit))
	return q
}

// getJSON performs a GET request and JSON-decodes a successful response into dest.
// Non-2xx responses are translated to typed app errors:
//   - 404 → apperrors.ResourceNotFoundError
//   - other → plain fmt.Errorf with status and body message
func (c *HTTP) getJSON(ctx context.Context, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if resp.StatusCode == http.StatusNotFound {
			return apperrors.ResourceNotFoundError(nil, body.Error)
		}
		return fmt.Errorf("indexer HTTP %d: %s", resp.StatusCode, body.Error)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
