//go:build ignore

// bootstrap-usdcx.go — Bootstrap USDCx CIP-56 token on participant2.
//
// Participant2 acts as the "external" USDCx issuer node, separate from the
// middleware's participant1. The script creates the CIP56Manager, TokenConfig,
// and CIP56TransferFactory for USDCx under the given USDCxIssuer party.
//
// The USDCxIssuer party must be allocated before calling this script (the
// docker-bootstrap.sh allocates it via the Canton HTTP API on port 5023).
//
// When the indexer (running on P1) uses FiltersForAnyParty, it sees all
// TokenTransferEvents routed through the shared synchronizer, including USDCx
// events whose issuer lives on P2.
//
// Usage (inside docker bootstrap):
//
//	/app/bootstrap-usdcx \
//	  -p2              canton:5021 \
//	  -p2-audience     http://canton:5021 \
//	  -issuer          "USDCxIssuer::1220..." \
//	  -domain          "local::1220..." \
//	  -token-url       http://mock-oauth2:8088/oauth/token \
//	  -client-id       local-test-client \
//	  -client-secret   local-test-secret

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

var (
	p2Addr       = flag.String("p2", "canton:5021", "Participant2 gRPC address")
	p2Audience   = flag.String("p2-audience", "http://canton:5021", "JWT audience for participant2")
	issuerParty  = flag.String("issuer", "", "USDCxIssuer party ID (required)")
	domainIDFlag = flag.String("domain", "", "Synchronizer/domain ID (required)")
	tokenURLFlag = flag.String("token-url", "http://mock-oauth2:8088/oauth/token", "OAuth2 token endpoint")
	clientIDFlag = flag.String("client-id", "local-test-client", "OAuth2 client ID")
	clientSecFlag = flag.String("client-secret", "local-test-secret", "OAuth2 client secret")
	cip56PkgID   = flag.String("cip56-package-id", "c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2", "CIP56 package ID")
)

func main() {
	flag.Parse()

	if *issuerParty == "" {
		log.Fatal("-issuer is required (USDCxIssuer party ID)")
	}
	if *domainIDFlag == "" {
		log.Fatal("-domain is required (synchronizer ID)")
	}

	fmt.Println(">>> Bootstrap USDCx on participant2")
	fmt.Printf("    P2:     %s\n", *p2Addr)
	fmt.Printf("    Issuer: %s\n", *issuerParty)
	fmt.Printf("    Domain: %s\n", *domainIDFlag)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	p2, err := ledger.New(&ledger.Config{
		RPCURL:         *p2Addr,
		MaxMessageSize: 52428800,
		TLS:            &ledger.TLSConfig{Enabled: false},
		Auth: &ledger.AuthConfig{
			ClientID:     *clientIDFlag,
			ClientSecret: *clientSecFlag,
			Audience:     *p2Audience,
			TokenURL:     *tokenURLFlag,
			ExpiryLeeway: 60 * time.Second,
		},
	})
	if err != nil {
		log.Fatalf("connect to P2 (%s): %v", *p2Addr, err)
	}
	defer p2.Close()

	fmt.Println(">>> Connected to participant2")

	// ── CIP56Manager ──────────────────────────────────────────────────────────

	managerCID, err := findContract(ctx, p2, *issuerParty, "CIP56.Token", "CIP56Manager")
	if err == nil {
		fmt.Printf("    CIP56Manager already exists: %s\n", managerCID)
	} else {
		fmt.Println(">>> Creating CIP56Manager for USDCx...")
		managerCID, err = createManager(ctx, p2, *issuerParty, *domainIDFlag)
		if err != nil {
			log.Fatalf("create CIP56Manager: %v", err)
		}
		fmt.Printf("    Created: %s\n", managerCID)
	}

	// ── TokenConfig ───────────────────────────────────────────────────────────

	configCID, err := findTokenConfig(ctx, p2, *issuerParty)
	if err == nil {
		fmt.Printf("    TokenConfig already exists: %s\n", configCID)
	} else {
		fmt.Println(">>> Creating TokenConfig for USDCx...")
		configCID, err = createTokenConfig(ctx, p2, *issuerParty, *domainIDFlag, managerCID)
		if err != nil {
			log.Fatalf("create TokenConfig: %v", err)
		}
		fmt.Printf("    Created: %s\n", configCID)
	}

	// ── CIP56TransferFactory ──────────────────────────────────────────────────

	factoryCID, err := findContract(ctx, p2, *issuerParty, "CIP56.TransferFactory", "CIP56TransferFactory")
	if err == nil {
		fmt.Printf("    CIP56TransferFactory already exists: %s\n", factoryCID)
	} else {
		fmt.Println(">>> Creating CIP56TransferFactory for USDCx...")
		factoryCID, err = createTransferFactory(ctx, p2, *issuerParty, *domainIDFlag)
		if err != nil {
			fmt.Printf("    [WARN] CIP56TransferFactory: %v (may already exist)\n", err)
		} else {
			fmt.Printf("    Created: %s\n", factoryCID)
		}
	}

	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("USDCx Bootstrap Complete")
	fmt.Println("======================================================================")
	fmt.Printf("Issuer:  %s\n", *issuerParty)
	fmt.Printf("Manager: %s\n", managerCID)
	fmt.Printf("Config:  %s\n", configCID)
}

// ─── Canton helpers ───────────────────────────────────────────────────────────

func cip56ID(module, entity string) *lapiv2.Identifier {
	return &lapiv2.Identifier{
		PackageId:  *cip56PkgID,
		ModuleName: module,
		EntityName: entity,
	}
}

// findContract returns the first active contract of the given template for the issuer.
func findContract(ctx context.Context, c *ledger.Client, issuer, module, entity string) (string, error) {
	authCtx := c.AuthContext(ctx)
	offset, err := c.GetLedgerEnd(authCtx)
	if err != nil {
		return "", err
	}
	events, err := c.GetActiveContractsByTemplate(authCtx, offset, []string{issuer}, cip56ID(module, entity))
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "", fmt.Errorf("not found")
	}
	return events[0].ContractId, nil
}

// findTokenConfig looks for an existing USDCx TokenConfig on P2.
func findTokenConfig(ctx context.Context, c *ledger.Client, issuer string) (string, error) {
	authCtx := c.AuthContext(ctx)
	offset, err := c.GetLedgerEnd(authCtx)
	if err != nil {
		return "", err
	}
	events, err := c.GetActiveContractsByTemplate(authCtx, offset, []string{issuer}, cip56ID("CIP56.Config", "TokenConfig"))
	if err != nil {
		return "", err
	}
	for _, e := range events {
		if values.MetaSymbolFromRecord(e.GetCreateArguments()) == "USDCx" {
			return e.ContractId, nil
		}
	}
	return "", fmt.Errorf("USDCx TokenConfig not found")
}

func sub(ctx context.Context, c *ledger.Client) string {
	authCtx := c.AuthContext(ctx)
	s, _ := c.JWTSubject(authCtx)
	if s == "" {
		return "test-user"
	}
	return s
}

func createManager(ctx context.Context, c *ledger.Client, issuer, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-mgr-%d", time.Now().UnixNano()),
			UserId:         sub(ctx, c),
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56ID("CIP56.Token", "CIP56Manager"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "issuer", Value: values.PartyValue(issuer)},
						{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "USDCx")},
						{Label: "meta", Value: values.EncodeMetadata(map[string]string{
							"splice.chainsafe.io/name":     "USD Coin",
							"splice.chainsafe.io/symbol":   "USDCx",
							"splice.chainsafe.io/decimals": "6",
						})},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findInTx(resp.Transaction, "CIP56Manager")
}

func createTokenConfig(ctx context.Context, c *ledger.Client, issuer, syncID, managerCID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-cfg-%d", time.Now().UnixNano()),
			UserId:         sub(ctx, c),
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56ID("CIP56.Config", "TokenConfig"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "issuer", Value: values.PartyValue(issuer)},
						{Label: "tokenManagerCid", Value: values.ContractIDValue(managerCID)},
						{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "USDCx")},
						{Label: "meta", Value: values.EncodeMetadata(map[string]string{
							"splice.chainsafe.io/name":     "USD Coin",
							"splice.chainsafe.io/symbol":   "USDCx",
							"splice.chainsafe.io/decimals": "6",
						})},
						{Label: "auditObservers", Value: values.ListValue(nil)},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findInTx(resp.Transaction, "TokenConfig")
}

func createTransferFactory(ctx context.Context, c *ledger.Client, admin, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-factory-%d", time.Now().UnixNano()),
			UserId:         sub(ctx, c),
			ActAs:          []string{admin},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56ID("CIP56.TransferFactory", "CIP56TransferFactory"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "admin", Value: values.PartyValue(admin)},
						{Label: "auditObservers", Value: values.ListValue(nil)},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findInTx(resp.Transaction, "CIP56TransferFactory")
}

func findInTx(tx *lapiv2.Transaction, entity string) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("nil transaction")
	}
	for _, ev := range tx.Events {
		if c := ev.GetCreated(); c != nil && c.TemplateId != nil && c.TemplateId.EntityName == entity {
			return c.ContractId, nil
		}
	}
	return "", fmt.Errorf("%s not found in transaction", entity)
}
