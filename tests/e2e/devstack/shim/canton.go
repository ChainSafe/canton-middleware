//go:build e2e

package shim

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
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
	bridgePackageID         = "6fac182df4943e7e2f70360b413b6e3ab10e65289ba0d971978b6d861a860d72"
	bridgeModule            = "Wayfinder.Bridge"

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
	grpcEndpoint      string
	httpEndpoint      string
	client            *http.Client
	ledgerClient      ledger.Ledger
	tokenClient       token.Token       // acts as DemoInstrumentAdmin — used for DEMO ops
	promptTokenClient token.Token       // acts as PromptInstrumentAdmin — used for PROMPT holdings
	identityClient    identity.Identity
	bridgeClient      bridge.Bridge
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

	// Identity client for token operations — acts as the DEMO instrument admin.
	demoIdCfg := &identity.Config{
		DomainID:    manifest.CantonDomainID,
		IssuerParty: manifest.DemoInstrumentAdmin,
		UserID:      cantonUserID,
		PackageID:   identityPackageID,
	}
	demoID, err := identity.New(demoIdCfg, l)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("identity.New (demo): %w", err)
	}

	tokenCfg := &token.Config{
		DomainID:                manifest.CantonDomainID,
		IssuerParty:             manifest.DemoInstrumentAdmin,
		UserID:                  cantonUserID,
		CIP56PackageID:          cip56PackageID,
		SpliceTransferPackageID: spliceTransferPackageID,
	}
	tk, err := token.New(tokenCfg, l, demoID)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("token.New: %w", err)
	}

	// Identity client for bridge operations — acts as the PROMPT instrument admin
	// (bridge operator). FingerprintMappings created during user registration are
	// visible to this party, matching the production relayer configuration.
	bridgeIdCfg := &identity.Config{
		DomainID:    manifest.CantonDomainID,
		IssuerParty: manifest.PromptInstrumentAdmin,
		UserID:      cantonUserID,
		PackageID:   identityPackageID,
	}
	bridgeID, err := identity.New(bridgeIdCfg, l)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("identity.New (bridge): %w", err)
	}

	// Token client for PROMPT holdings — acts as PromptInstrumentAdmin so that
	// GetHoldings returns results visible to the bridge operator, matching how
	// the production relayer queries holdings before initiating withdrawals.
	promptTokenCfg := &token.Config{
		DomainID:                manifest.CantonDomainID,
		IssuerParty:             manifest.PromptInstrumentAdmin,
		UserID:                  cantonUserID,
		CIP56PackageID:          cip56PackageID,
		SpliceTransferPackageID: spliceTransferPackageID,
	}
	promptTk, err := token.New(promptTokenCfg, l, bridgeID)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("token.New (prompt): %w", err)
	}

	bridgeCfg := &bridge.Config{
		DomainID:      manifest.CantonDomainID,
		UserID:        cantonUserID,
		OperatorParty: manifest.PromptInstrumentAdmin,
		PackageID:     bridgePackageID,
		Module:        bridgeModule,
	}
	br, err := bridge.New(bridgeCfg, l, bridgeID)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("bridge.New: %w", err)
	}

	return &CantonShim{
		grpcEndpoint:      manifest.CantonGRPC,
		httpEndpoint:      manifest.CantonHTTP,
		client:            &http.Client{Timeout: 10 * time.Second},
		ledgerClient:      l,
		tokenClient:       tk,
		promptTokenClient: promptTk,
		identityClient:    bridgeID,
		bridgeClient:      br,
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

// tokenClientFor returns the appropriate token client for the given symbol.
// PROMPT holdings are only visible to PromptInstrumentAdmin (bridge operator);
// all other tokens use the default client acting as DemoInstrumentAdmin.
func (c *CantonShim) tokenClientFor(tokenSymbol string) token.Token {
	if tokenSymbol == "PROMPT" {
		return c.promptTokenClient
	}
	return c.tokenClient
}

// GetCantonBalance returns the total balance of tokenSymbol held by partyID
// as a decimal string. Returns "0" when the party has no holdings.
func (c *CantonShim) GetCantonBalance(ctx context.Context, partyID, tokenSymbol string) (string, error) {
	bal, err := c.tokenClientFor(tokenSymbol).GetBalanceByPartyID(ctx, partyID, tokenSymbol)
	if err != nil {
		return "0", err
	}
	return bal, nil
}

// GetHoldings returns the CIP56Holding contracts owned by ownerParty for tokenSymbol.
func (c *CantonShim) GetHoldings(ctx context.Context, ownerParty, tokenSymbol string) ([]*stack.CantonHolding, error) {
	holdings, err := c.tokenClientFor(tokenSymbol).GetHoldings(ctx, ownerParty, tokenSymbol)
	if err != nil {
		return nil, err
	}
	result := make([]*stack.CantonHolding, len(holdings))
	for i, h := range holdings {
		result[i] = &stack.CantonHolding{
			ContractID: h.ContractID,
			Amount:     h.Amount,
			Symbol:     h.Symbol,
		}
	}
	return result, nil
}

// GetFingerprintMapping returns the FingerprintMapping contract ID for the
// given fingerprint (as returned in the RegisterResponse.Fingerprint field).
func (c *CantonShim) GetFingerprintMapping(ctx context.Context, fingerprint string) (string, error) {
	m, err := c.identityClient.GetFingerprintMapping(ctx, fingerprint)
	if err != nil {
		return "", err
	}
	return m.ContractID, nil
}

// InitiateWithdrawal calls the WayfinderBridgeConfig.InitiateWithdrawal DAML
// choice and returns the resulting WithdrawalRequest contract ID.
func (c *CantonShim) InitiateWithdrawal(ctx context.Context, mappingCID, holdingCID, amount, evmDest string) (string, error) {
	return c.bridgeClient.InitiateWithdrawal(ctx, bridge.InitiateWithdrawalRequest{
		MappingCID:     mappingCID,
		HoldingCID:     holdingCID,
		Amount:         amount,
		EvmDestination: evmDest,
	})
}
