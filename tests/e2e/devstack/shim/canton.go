//go:build e2e

package shim

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// Local devnet package IDs — compiled into the DAR files, stable across devnet restarts.
const (
	cip56PackageID          = "c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2"
	spliceTransferPackageID = "55ba4deb0ad4662c4168b39859738a0e91388d252286480c7331b3f71a517281"
	identityPackageID       = "c4d8bc62b74dfb93c0feda15cbceb5db16aef37d0e7ee37c17887faa9cbd33b9"

	// cantonUserID is the JWT subject claim emitted by the mock OAuth2 server
	// when authenticating with client_id "local-test-client". Canton uses
	// wildcard auth in the local devnet so any user_id is accepted.
	cantonUserID = "local-test-client"

	// oauthClientID / oauthClientSecret are the default credentials for the
	// local mock OAuth2 server (see docker-compose.yaml CANTON_AUTH_* env vars).
	oauthClientID = "local-test-client"
	// #nosec G101 -- local devnet-only mock OAuth credential from docker-compose.
	oauthClientSecret = "local-test-secret"

	// oauthAudience matches the audience the api-server requests when obtaining
	// its own Canton JWT. Canton wildcard auth ignores the audience value.
	oauthAudience = "http://canton:5011"
)

var _ stack.Canton = (*CantonShim)(nil)

// CantonShim implements stack.Canton.
type CantonShim struct {
	grpcEndpoint   string
	httpEndpoint   string
	client         *http.Client
	ledgerClient   ledger.Ledger
	identityClient identity.Identity
	tokenClient    token.Token
}

// NewCanton returns a CantonShim wired to the endpoints in the manifest.
// It dials the Canton gRPC endpoint and initializes the token client for
// MintToken and GetCantonBalance. The gRPC connection is lazy — it does not
// actually connect until the first RPC call. Returns an error if the client
// config is invalid.
func NewCanton(manifest *stack.ServiceManifest) (*CantonShim, error) {
	ledgerCfg := &ledger.Config{
		RPCURL: manifest.CantonGRPC,
		TLS:    &ledger.TLSConfig{Enabled: false},
		Auth: &ledger.AuthConfig{
			ClientID:     oauthClientID,
			ClientSecret: oauthClientSecret,
			Audience:     oauthAudience,
			TokenURL:     manifest.OAuthHTTP + "/oauth/token",
			ExpiryLeeway: 60 * time.Second,
		},
	}

	l, err := ledger.New(ledgerCfg)
	if err != nil {
		return nil, fmt.Errorf("ledger.New: %w", err)
	}

	idCfg := &identity.Config{
		DomainID:    manifest.CantonDomainID,
		IssuerParty: manifest.DemoInstrumentAdmin,
		UserID:      cantonUserID,
		PackageID:   identityPackageID,
	}
	id, err := identity.New(idCfg, l)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("identity.New: %w", err)
	}

	tokenCfg := &token.Config{
		DomainID:                manifest.CantonDomainID,
		IssuerParty:             manifest.DemoInstrumentAdmin,
		UserID:                  cantonUserID,
		CIP56PackageID:          cip56PackageID,
		SpliceTransferPackageID: spliceTransferPackageID,
	}
	tk, err := token.New(tokenCfg, l, id)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("token.New: %w", err)
	}

	return &CantonShim{
		grpcEndpoint:   manifest.CantonGRPC,
		httpEndpoint:   manifest.CantonHTTP,
		client:         &http.Client{Timeout: 10 * time.Second},
		ledgerClient:   l,
		identityClient: id,
		tokenClient:    tk,
	}, nil
}

func (c *CantonShim) GRPCEndpoint() string { return c.grpcEndpoint }
func (c *CantonShim) HTTPEndpoint() string { return c.httpEndpoint }

// Close releases the underlying gRPC connection. Call via coreShims.close().
func (c *CantonShim) Close() {
	if c.ledgerClient != nil {
		_ = c.ledgerClient.Close()
	}
}

// IsHealthy returns true when the Canton HTTP JSON API responds with 200. It
// does not block — callers should use WaitForCanton in the DSL for polling.
func (c *CantonShim) IsHealthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/version", c.httpEndpoint), nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// AllocateParty allocates a new internal Canton party with the given hint and
// returns its fully-qualified party ID. Use this in tests to create fresh
// parties without depending on manifest fixtures.
func (c *CantonShim) AllocateParty(ctx context.Context, hint string) (string, error) {
	p, err := c.identityClient.AllocateParty(ctx, hint)
	if err != nil {
		return "", fmt.Errorf("allocate party %q: %w", hint, err)
	}
	return p.PartyID, nil
}

// MintToken mints amount of tokenSymbol to recipientParty via the IssuerMint
// DAML choice on the TokenConfig contract.
func (c *CantonShim) MintToken(ctx context.Context, recipientParty, tokenSymbol, amount string) error {
	_, err := c.tokenClient.Mint(ctx, &token.MintRequest{
		RecipientParty: recipientParty,
		Amount:         amount,
		TokenSymbol:    tokenSymbol,
	})
	return err
}

// GetCantonBalance returns the total balance of tokenSymbol held by partyID
// as a decimal string. Returns "0" when the party has no holdings.
func (c *CantonShim) GetCantonBalance(ctx context.Context, partyID, tokenSymbol string) (string, error) {
	bal, err := c.tokenClient.GetBalanceByPartyID(ctx, partyID, tokenSymbol)
	if err != nil {
		return "0", err
	}
	return bal, nil
}
