//go:build e2e

package shim

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
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
	domainID     string
	issuerParty  string
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
		DomainID:                 manifest.CantonDomainID,
		IssuerParty:              manifest.USDCxInstrumentAdmin,
		UserID:                   cantonUserID,
		CIP56PackageID:           cip56PackageID,
		SpliceTransferPackageID:  spliceTransferPackageID,
		SpliceHoldingPackageID:   spliceHoldingPackageID,
		UtilityRegistryPackageID: utilityRegistryPackageID,
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
		domainID:     manifest.CantonDomainID,
		issuerParty:  manifest.USDCxInstrumentAdmin,
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

// MintToken creates a Utility.Registry.Holding.V0.Holding owned by recipientParty
// for amount of tokenSymbol on P2. Required for AllocationFactory-based transfers
// (e.g., USDCx) which fetch input holdings as Utility.Registry.Holding rather than
// CIP56Holding.
//
// Only self-mint (recipientParty == issuer) is supported because the Holding
// template signatories are {registrar, owner}; for cross-party mints the recipient
// would also have to authorize, which the devstack issuer cannot do unilaterally.
// In practice, tests seed the issuer with a balance via self-mint and then use
// TransferToken to deliver tokens to other parties through the offer/accept flow.
func (c *Canton2Shim) MintToken(ctx context.Context, recipientParty, tokenSymbol, amount string) error {
	if recipientParty != c.issuerParty {
		return fmt.Errorf("MintToken on P2 only supports self-mint to issuer (got recipient %q, issuer %q)",
			recipientParty, c.issuerParty)
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
			TemplateId: &lapiv2.Identifier{
				PackageId:  utilityRegistryHoldingPackageID,
				ModuleName: "Utility.Registry.Holding.V0.Holding",
				EntityName: "Holding",
			},
			CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
				{Label: "operator", Value: values.PartyValue(c.issuerParty)},
				{Label: "provider", Value: values.PartyValue(c.issuerParty)},
				{Label: "registrar", Value: values.PartyValue(c.issuerParty)},
				{Label: "owner", Value: values.PartyValue(recipientParty)},
				{Label: "instrument", Value: encodeRegistrarInstrumentID(c.issuerParty, tokenSymbol)},
				{Label: "label", Value: values.TextValue("")},
				{Label: "amount", Value: values.NumericValue(amount)},
				{Label: "lock", Value: values.None()},
			}},
		}},
	}

	authCtx := c.ledgerClient.AuthContext(ctx)
	_, err := c.ledgerClient.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.domainID,
			CommandId:      "p2-mint-" + uuid.NewString(),
			UserId:         cantonUserID,
			ActAs:          []string{c.issuerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("create Utility.Registry.Holding for %s: %w", tokenSymbol, err)
	}
	return nil
}

// encodeRegistrarInstrumentID builds a Utility.Registry.Holding.V0.Types.InstrumentIdentifier
// with the registrar-internal scheme. The Holding template's `ensure` clause requires
// `instrument == toInstrumentIdentifier registrar (InstrumentId{admin=registrar, id=tokenSymbol})`
// which produces this exact shape — any deviation rejects the create.
func encodeRegistrarInstrumentID(registrar, id string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{Fields: []*lapiv2.RecordField{
				{Label: "source", Value: values.PartyValue(registrar)},
				{Label: "id", Value: values.TextValue(id)},
				{Label: "scheme", Value: values.TextValue("RegistrarInternalScheme")},
			}},
		},
	}
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
//
// Multiple e2e tests share the same USDCx issuer party, so concurrent TransferToken
// calls can pick the same input Holding and lose the race with Canton's contract
// lock manager (LOCAL_VERDICT_LOCKED_CONTRACTS). Retry a few times with a small
// delay — the lock is held only for the duration of a single transfer and the
// retry will pick a different Holding from GetHoldings.
func (c *Canton2Shim) TransferToken(ctx context.Context, senderParty, recipientParty, tokenSymbol, amount string) error {
	const maxAttempts = 5
	const baseDelay = 100 * time.Millisecond
	var lastErr error
	for attempt := range maxAttempts {
		err := c.tokenCli.TransferInternalByPartyID(ctx, uuid.NewString(), senderParty, recipientParty, amount, tokenSymbol)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isContentionError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(baseDelay * time.Duration(1<<attempt)):
		}
	}
	return fmt.Errorf("TransferToken: exhausted %d retries: %w", maxAttempts, lastErr)
}

// isContentionError reports whether err is a transient Canton concurrency
// failure that should be retried — locked contracts (one of our own holdings
// is held by a concurrent transfer) or generic Aborted status.
func isContentionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "LOCAL_VERDICT_LOCKED_CONTRACTS") ||
		strings.Contains(msg, "CONTENTION") ||
		strings.Contains(msg, "code = Aborted")
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
