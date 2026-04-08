package provider

import (
	"context"
	"fmt"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	indexerclient "github.com/chainsafe/canton-middleware/pkg/indexer/client"
)

// Indexer implements token.Provider using the indexer's HTTP client.
//
// Unlike the Canton provider — which issues live gRPC ACS scans for every
// balanceOf/totalSupply call — the Indexer provider reads from pre-materialized
// PostgreSQL tables kept current by the indexer's streaming processor.
//
// It is a thin adapter: symbol → admin lookup is handled here; all HTTP
// concerns live in the indexer client.
type Indexer struct {
	client      indexerclient.Client
	// instruments maps tokenSymbol (InstrumentID) → InstrumentAdmin party.
	// Required because the indexer keys by (admin, id) but the Provider
	// interface only receives the token symbol.
	instruments map[string]string
}

// NewIndexer creates an Indexer-backed token provider.
//
// instruments maps each supported token symbol to its Canton instrument admin
// party string, e.g. map[string]string{"DEMO": "admin::abc123@domain"}.
func NewIndexer(client indexerclient.Client, instruments map[string]string) *Indexer {
	return &Indexer{client: client, instruments: instruments}
}

// GetTotalSupply returns total supply for tokenSymbol via the indexer client.
func (p *Indexer) GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error) {
	admin, err := p.instrumentAdmin(tokenSymbol)
	if err != nil {
		return "0", err
	}
	return p.client.TotalSupply(ctx, admin, tokenSymbol)
}

// GetBalance returns the token balance for the given Canton party ID.
// The partyID is resolved upstream in the service layer (from the user record),
// so no additional lookup is needed here.
// A not-found response from the indexer means the party holds zero of this token.
func (p *Indexer) GetBalance(ctx context.Context, tokenSymbol, partyID string) (string, error) {
	admin, err := p.instrumentAdmin(tokenSymbol)
	if err != nil {
		return "0", err
	}
	b, err := p.client.GetBalance(ctx, partyID, admin, tokenSymbol)
	if err != nil {
		if apperrors.Is(err, apperrors.CategoryResourceNotFound) {
			return "0", nil
		}
		return "0", err
	}
	return b.Amount, nil
}

// instrumentAdmin returns the InstrumentAdmin party for the given token symbol,
// or an error if the symbol is not in the configured instruments map.
func (p *Indexer) instrumentAdmin(tokenSymbol string) (string, error) {
	admin, ok := p.instruments[tokenSymbol]
	if !ok {
		return "", fmt.Errorf("unknown token symbol: %s", tokenSymbol)
	}
	return admin, nil
}
