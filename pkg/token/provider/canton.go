package provider

import (
	"context"

	cantontoken "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

// Canton implements token.Provider using the Canton token client.
type Canton struct {
	client cantontoken.Token
}

// NewCanton creates a Canton-backed token provider.
func NewCanton(client cantontoken.Token) *Canton {
	return &Canton{
		client: client,
	}
}

// GetBalance returns token balance by token symbol and user fingerprint.
func (p *Canton) GetBalance(ctx context.Context, tokenSymbol, fingerprint string) (string, error) {
	return p.client.GetBalanceByFingerprint(ctx, fingerprint, tokenSymbol)
}

// GetTotalSupply returns token total supply by token symbol.
func (p *Canton) GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error) {
	return p.client.GetTotalSupply(ctx, tokenSymbol)
}
