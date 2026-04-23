package client

import (
	"context"

	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"github.com/prometheus/client_golang/prometheus"
)

// InstrumentedClient wraps a Client and records Prometheus metrics for every
// outbound indexer HTTP call.
type InstrumentedClient struct {
	inner   Client
	metrics *Metrics
}

// Compile-time check that InstrumentedClient implements Client.
var _ Client = (*InstrumentedClient)(nil)

// NewInstrumentedClient returns a metrics-instrumented wrapper around the given Client.
func NewInstrumentedClient(inner Client, metrics *Metrics) *InstrumentedClient {
	return &InstrumentedClient{inner: inner, metrics: metrics}
}

func (c *InstrumentedClient) observe(op ClientOperation) func() {
	timer := prometheus.NewTimer(c.metrics.RequestDuration.WithLabelValues(string(op)))
	return func() { timer.ObserveDuration() }
}

func (c *InstrumentedClient) incErr(op ClientOperation, err error) {
	if err != nil {
		c.metrics.RequestErrors.WithLabelValues(string(op)).Inc()
	}
}

func (c *InstrumentedClient) GetToken(ctx context.Context, admin, id string) (*indexer.Token, error) {
	defer c.observe(OpGetToken)()
	t, err := c.inner.GetToken(ctx, admin, id)
	c.incErr(OpGetToken, err)
	return t, err
}

func (c *InstrumentedClient) ListTokens(ctx context.Context, p indexer.Pagination) (*indexer.Page[*indexer.Token], error) {
	defer c.observe(OpListTokens)()
	page, err := c.inner.ListTokens(ctx, p)
	c.incErr(OpListTokens, err)
	return page, err
}

func (c *InstrumentedClient) TotalSupply(ctx context.Context, admin, id string) (string, error) {
	defer c.observe(OpTotalSupply)()
	s, err := c.inner.TotalSupply(ctx, admin, id)
	c.incErr(OpTotalSupply, err)
	return s, err
}

func (c *InstrumentedClient) GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error) {
	defer c.observe(OpGetBalance)()
	b, err := c.inner.GetBalance(ctx, partyID, admin, id)
	c.incErr(OpGetBalance, err)
	return b, err
}

func (c *InstrumentedClient) ListBalancesForParty(
	ctx context.Context, partyID string, p indexer.Pagination,
) (*indexer.Page[*indexer.Balance], error) {
	defer c.observe(OpListBalancesForParty)()
	page, err := c.inner.ListBalancesForParty(ctx, partyID, p)
	c.incErr(OpListBalancesForParty, err)
	return page, err
}

func (c *InstrumentedClient) ListBalancesForToken(
	ctx context.Context, admin, id string, p indexer.Pagination,
) (*indexer.Page[*indexer.Balance], error) {
	defer c.observe(OpListBalancesForToken)()
	page, err := c.inner.ListBalancesForToken(ctx, admin, id, p)
	c.incErr(OpListBalancesForToken, err)
	return page, err
}

func (c *InstrumentedClient) GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error) {
	defer c.observe(OpGetEvent)()
	e, err := c.inner.GetEvent(ctx, contractID)
	c.incErr(OpGetEvent, err)
	return e, err
}

func (c *InstrumentedClient) ListTokenEvents(
	ctx context.Context,
	admin, id string,
	f indexer.EventFilter,
	p indexer.Pagination,
) (*indexer.Page[*indexer.ParsedEvent], error) {
	defer c.observe(OpListTokenEvents)()
	page, err := c.inner.ListTokenEvents(ctx, admin, id, f, p)
	c.incErr(OpListTokenEvents, err)
	return page, err
}

func (c *InstrumentedClient) ListPartyEvents(
	ctx context.Context,
	partyID string,
	f indexer.EventFilter,
	p indexer.Pagination,
) (*indexer.Page[*indexer.ParsedEvent], error) {
	defer c.observe(OpListPartyEvents)()
	page, err := c.inner.ListPartyEvents(ctx, partyID, f, p)
	c.incErr(OpListPartyEvents, err)
	return page, err
}
