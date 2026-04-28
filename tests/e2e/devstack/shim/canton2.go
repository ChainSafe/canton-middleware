//go:build e2e

package shim

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// oauthAudienceP2 is the JWT audience for Participant 2's Canton node.
// It differs from P1's "http://canton:5011".
const oauthAudienceP2 = "http://canton:5021"

// errP2NotSupported is returned by Canton2Shim methods that only apply to the
// P1 bridge participant (fingerprint mappings, withdrawal operations).
var errP2NotSupported = errors.New("operation not supported on Participant 2")

var _ stack.Canton = (*Canton2Shim)(nil)

// Canton2Shim implements stack.Canton for Participant 2 — the USDCx issuer
// participant. It shares the same synchronizer as P1 but connects on a
// separate gRPC port (5021) with its own OAuth audience.
//
// Bridge-specific methods (GetFingerprintMapping, InitiateWithdrawal,
// ProcessWithdrawal) return errP2NotSupported; all other Canton interface
// methods are fully implemented.
type Canton2Shim struct {
	grpcEndpoint string
	httpEndpoint string
	client       *http.Client
	ledgerClient ledger.Ledger
	tokenCli     token.Token
}

// NewCanton2 returns a Canton2Shim wired to the Participant 2 endpoints in the
// manifest. The returned shim acts as USDCxInstrumentAdmin for all token
// operations. The gRPC connection is lazy. Call Close() to release it.
func NewCanton2(manifest *stack.ServiceManifest) (*Canton2Shim, error) {
	l, err := ledger.New(&ledger.Config{
		RPCURL: manifest.Canton2GRPC,
		TLS:    &ledger.TLSConfig{Enabled: false},
		Auth: &ledger.AuthConfig{
			ClientID:     oauthClientID,
			ClientSecret: oauthClientSecret,
			Audience:     oauthAudienceP2,
			TokenURL:     manifest.OAuthHTTP + "/oauth/token",
			ExpiryLeeway: 60 * time.Second,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ledger.New P2: %w", err)
	}

	id, err := identity.New(&identity.Config{
		DomainID:    manifest.CantonDomainID,
		IssuerParty: manifest.USDCxInstrumentAdmin,
		UserID:      cantonUserID,
		PackageID:   identityPackageID,
	}, l)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("identity.New P2: %w", err)
	}

	tk, err := token.New(&token.Config{
		DomainID:                manifest.CantonDomainID,
		IssuerParty:             manifest.USDCxInstrumentAdmin,
		UserID:                  cantonUserID,
		CIP56PackageID:          cip56PackageID,
		SpliceTransferPackageID: spliceTransferPackageID,
		SpliceHoldingPackageID:  spliceHoldingPackageID,
	}, l, id)
	if err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("token.New P2: %w", err)
	}

	return &Canton2Shim{
		grpcEndpoint: manifest.Canton2GRPC,
		httpEndpoint: manifest.Canton2HTTP,
		client:       &http.Client{Timeout: 10 * time.Second},
		ledgerClient: l,
		tokenCli:     tk,
	}, nil
}

func (c *Canton2Shim) GRPCEndpoint() string { return c.grpcEndpoint }
func (c *Canton2Shim) HTTPEndpoint() string { return c.httpEndpoint }

// IsHealthy returns true when Participant 2's HTTP JSON API responds with 200.
// The docker-compose canton healthcheck verifies P2 package readiness before
// marking the service healthy, so this probe is a lightweight liveness check.
func (c *Canton2Shim) IsHealthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/version", c.httpEndpoint), nil)
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

// Close releases the P2 gRPC connection.
func (c *Canton2Shim) Close() {
	if c.ledgerClient != nil {
		_ = c.ledgerClient.Close()
	}
}

// AllocateParty allocates a fresh internal Canton party on Participant 2 and
// returns its fully-qualified party ID.
func (c *Canton2Shim) AllocateParty(ctx context.Context, hint string) (string, error) {
	authCtx := c.ledgerClient.AuthContext(ctx)
	resp, err := c.ledgerClient.PartyAdmin().AllocateParty(authCtx, &adminv2.AllocatePartyRequest{
		PartyIdHint: hint,
	})
	if err != nil {
		return "", fmt.Errorf("allocate party %q on P2: %w", hint, err)
	}
	return resp.PartyDetails.Party, nil
}

// MintToken mints amount of tokenSymbol to recipientParty via IssuerMint on P2.
func (c *Canton2Shim) MintToken(ctx context.Context, recipientParty, tokenSymbol, amount string) error {
	_, err := c.tokenCli.Mint(ctx, &token.MintRequest{
		RecipientParty: recipientParty,
		Amount:         amount,
		TokenSymbol:    tokenSymbol,
	})
	return err
}

// GetCantonBalance returns the total balance of tokenSymbol held by partyID.
func (c *Canton2Shim) GetCantonBalance(ctx context.Context, partyID, tokenSymbol string) (string, error) {
	bal, err := c.tokenCli.GetBalanceByPartyID(ctx, partyID, tokenSymbol)
	if err != nil {
		return "0", err
	}
	return bal, nil
}

// GetHoldings returns the CIP56Holding contracts owned by ownerParty for tokenSymbol.
func (c *Canton2Shim) GetHoldings(ctx context.Context, ownerParty, tokenSymbol string) ([]*stack.CantonHolding, error) {
	holdings, err := c.tokenCli.GetHoldings(ctx, ownerParty, tokenSymbol)
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

// TransferToken finds a CIP56TransferFactory and a suitable CIP56Holding for
// senderParty on P2, then exercises TransferFactory_Transfer to move amount of
// tokenSymbol to recipientParty (which may be on a different participant).
func (c *Canton2Shim) TransferToken(ctx context.Context, senderParty, recipientParty, tokenSymbol, amount string) error {
	return c.tokenCli.TransferInternalByPartyID(ctx, uuid.NewString(), senderParty, recipientParty, amount, tokenSymbol)
}

// GetFingerprintMapping is not supported on Participant 2.
func (*Canton2Shim) GetFingerprintMapping(_ context.Context, _ string) (string, error) {
	return "", errP2NotSupported
}

// InitiateWithdrawal is not supported on Participant 2.
func (*Canton2Shim) InitiateWithdrawal(_ context.Context, _, _, _, _ string) (string, error) {
	return "", errP2NotSupported
}

// ProcessWithdrawal is not supported on Participant 2.
func (*Canton2Shim) ProcessWithdrawal(_ context.Context, _ string) (string, error) {
	return "", errP2NotSupported
}
