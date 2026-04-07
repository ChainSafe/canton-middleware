//go:build e2e

package shim

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// IndexerShim implements stack.Indexer via the admin HTTP API.
type IndexerShim struct {
	httpClient
}

// NewIndexer returns an IndexerShim for the indexer endpoint in the manifest.
func NewIndexer(manifest *stack.ServiceManifest) *IndexerShim {
	return &IndexerShim{httpClient{
		endpoint: manifest.IndexerHTTP,
		client:   &http.Client{Timeout: 10 * time.Second},
	}}
}

func (s *IndexerShim) Endpoint() string { return s.endpoint }

// Health returns nil when GET /health responds with 200.
func (s *IndexerShim) Health(ctx context.Context) error {
	return s.getOK(ctx, "/health")
}

func (s *IndexerShim) GetToken(ctx context.Context, admin, id string) (*indexer.Token, error) {
	var out indexer.Token
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s", admin, id), nil, &out)
}

func (s *IndexerShim) TotalSupply(ctx context.Context, admin, id string) (string, error) {
	var out struct {
		TotalSupply string `json:"total_supply"`
	}
	return out.TotalSupply, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s/supply", admin, id), nil, &out)
}

func (s *IndexerShim) ListTokens(ctx context.Context, page, limit int) (*indexer.Page[*indexer.Token], error) {
	var out indexer.Page[*indexer.Token]
	return &out, s.get(ctx, "/indexer/v1/admin/tokens", pageQuery(page, limit, ""), &out)
}

func (s *IndexerShim) GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error) {
	var out indexer.Balance
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/parties/%s/balances/%s/%s", partyID, admin, id), nil, &out)
}

func (s *IndexerShim) ListBalancesForParty(ctx context.Context, partyID string, page, limit int) (*indexer.Page[*indexer.Balance], error) {
	var out indexer.Page[*indexer.Balance]
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/parties/%s/balances", partyID), pageQuery(page, limit, ""), &out)
}

func (s *IndexerShim) GetBalanceForToken(ctx context.Context, admin, id string, page, limit int) (*indexer.Page[*indexer.Balance], error) {
	var out indexer.Page[*indexer.Balance]
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s/balances", admin, id), pageQuery(page, limit, ""), &out)
}

func (s *IndexerShim) GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error) {
	var out indexer.ParsedEvent
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/events/%s", contractID), nil, &out)
}

func (s *IndexerShim) ListPartyEvents(ctx context.Context, partyID string, eventType indexer.EventType, page, limit int) (*indexer.Page[*indexer.ParsedEvent], error) {
	var out indexer.Page[*indexer.ParsedEvent]
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/parties/%s/events", partyID), pageQuery(page, limit, string(eventType)), &out)
}

func (s *IndexerShim) ListTokenEvents(ctx context.Context, admin, id string, eventType indexer.EventType, page, limit int) (*indexer.Page[*indexer.ParsedEvent], error) {
	var out indexer.Page[*indexer.ParsedEvent]
	return &out, s.get(ctx, fmt.Sprintf("/indexer/v1/admin/tokens/%s/%s/events", admin, id), pageQuery(page, limit, string(eventType)), &out)
}

// pageQuery builds pagination + optional event_type query params.
func pageQuery(page, limit int, eventType string) url.Values {
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("limit", fmt.Sprintf("%d", limit))
	if eventType != "" {
		q.Set("event_type", eventType)
	}
	return q
}
