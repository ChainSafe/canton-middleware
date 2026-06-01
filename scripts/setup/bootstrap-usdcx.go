//go:build ignore

// bootstrap-usdcx.go — Bootstrap USDCx CIP-56 token on participant2.
//
// Participant2 acts as the "external" USDCx issuer node, separate from the
// middleware's participant1. The script creates the CIP56Manager, TokenConfig,
// AllocationFactory, TransferRule, and InstrumentConfiguration for USDCx under
// the given USDCxIssuer party.
//
// The AllocationFactory (from utility_registry_app_v0) replaces the old
// CIP56TransferFactory. When TransferFactory_Transfer is called on it, it
// creates a TransferOffer contract (pending state), matching devnet behaviour.
// The receiver must exercise TransferInstruction_Accept to complete the transfer.
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
	// Package IDs from deployments/usdcx-dars/manifest.json
	registryAppPkgID = flag.String("registry-app-package-id", "7a75ef6e69f69395a4e60919e228528bb8f3881150ccfde3f31bcc73864b18ab", "utility_registry_app_v0 package ID")
	registryPkgID    = flag.String("registry-package-id", "a236e8e22a3b5f199e37d5554e82bafd2df688f901de02b00be3964bdfa8c1ab", "utility_registry_v0 package ID")
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

	managerCID, err := findContractByPkg(ctx, p2, *issuerParty, *cip56PkgID, "CIP56.Token", "CIP56Manager")
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

	// ── AllocationFactory ─────────────────────────────────────────────────────
	// Replaces CIP56TransferFactory. TransferFactory_Transfer creates a
	// TransferOffer (pending), matching devnet behaviour.

	allocFactoryCID, err := findContractByPkg(ctx, p2, *issuerParty, *registryAppPkgID,
		"Utility.Registry.App.V0.Service.AllocationFactory", "AllocationFactory")
	if err == nil {
		fmt.Printf("    AllocationFactory already exists: %s\n", allocFactoryCID)
	} else {
		fmt.Println(">>> Creating AllocationFactory for USDCx...")
		allocFactoryCID, err = createAllocationFactory(ctx, p2, *issuerParty, *domainIDFlag)
		if err != nil {
			log.Fatalf("create AllocationFactory: %v", err)
		}
		fmt.Printf("    Created: %s\n", allocFactoryCID)
	}

	// ── TransferRule ──────────────────────────────────────────────────────────
	// Required for TransferInstruction_Accept. Must be disclosed to the receiver
	// via choiceContextData at accept time.

	transferRuleCID, err := findContractByPkg(ctx, p2, *issuerParty, *registryPkgID,
		"Utility.Registry.V0.Rule.Transfer", "TransferRule")
	if err == nil {
		fmt.Printf("    TransferRule already exists: %s\n", transferRuleCID)
	} else {
		fmt.Println(">>> Creating TransferRule for USDCx...")
		transferRuleCID, err = createTransferRule(ctx, p2, *issuerParty, *domainIDFlag)
		if err != nil {
			log.Fatalf("create TransferRule: %v", err)
		}
		fmt.Printf("    Created: %s\n", transferRuleCID)
	}

	// ── InstrumentConfiguration ───────────────────────────────────────────────
	// Required for validateTransfer inside TransferRule_TwoStepTransfer.
	// holderRequirements = [] so no Credential contracts are needed locally.

	instrumentConfigCID, err := findContractByPkg(ctx, p2, *issuerParty, *registryPkgID,
		"Utility.Registry.V0.Configuration.Instrument", "InstrumentConfiguration")
	if err == nil {
		fmt.Printf("    InstrumentConfiguration already exists: %s\n", instrumentConfigCID)
	} else {
		fmt.Println(">>> Creating InstrumentConfiguration for USDCx...")
		instrumentConfigCID, err = createInstrumentConfiguration(ctx, p2, *issuerParty, *domainIDFlag)
		if err != nil {
			log.Fatalf("create InstrumentConfiguration: %v", err)
		}
		fmt.Printf("    Created: %s\n", instrumentConfigCID)
	}

	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("USDCx Bootstrap Complete")
	fmt.Println("======================================================================")
	fmt.Printf("Issuer:                %s\n", *issuerParty)
	fmt.Printf("Manager:               %s\n", managerCID)
	fmt.Printf("Config:                %s\n", configCID)
	fmt.Printf("AllocationFactory:     %s\n", allocFactoryCID)
	fmt.Printf("TransferRule:          %s\n", transferRuleCID)
	fmt.Printf("InstrumentConfig:      %s\n", instrumentConfigCID)
}

// ─── Canton helpers ───────────────────────────────────────────────────────────

func cip56ID(module, entity string) *lapiv2.Identifier {
	return &lapiv2.Identifier{
		PackageId:  *cip56PkgID,
		ModuleName: module,
		EntityName: entity,
	}
}

func registryAppID(module, entity string) *lapiv2.Identifier {
	return &lapiv2.Identifier{
		PackageId:  *registryAppPkgID,
		ModuleName: module,
		EntityName: entity,
	}
}

func registryID(module, entity string) *lapiv2.Identifier {
	return &lapiv2.Identifier{
		PackageId:  *registryPkgID,
		ModuleName: module,
		EntityName: entity,
	}
}

// findContractByPkg returns the first active contract of the given template for issuer.
func findContractByPkg(ctx context.Context, c *ledger.Client, issuer, pkgID, module, entity string) (string, error) {
	authCtx := c.AuthContext(ctx)
	offset, err := c.GetLedgerEnd(authCtx)
	if err != nil {
		return "", err
	}
	tid := &lapiv2.Identifier{PackageId: pkgID, ModuleName: module, EntityName: entity}
	events, err := c.GetActiveContractsByTemplate(authCtx, offset, []string{issuer}, tid)
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

// createAllocationFactory creates an AllocationFactory on P2.
// This is DA's template from utility_registry_app_v0 that creates TransferOffer
// contracts (pending state) when TransferFactory_Transfer is called, matching
// devnet behaviour.
func createAllocationFactory(ctx context.Context, c *ledger.Client, issuer, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-alloc-factory-%d", time.Now().UnixNano()),
			UserId:         sub(ctx, c),
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: registryAppID("Utility.Registry.App.V0.Service.AllocationFactory", "AllocationFactory"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "provider", Value: values.PartyValue(issuer)},
						{Label: "registrar", Value: values.PartyValue(issuer)},
						{Label: "operator", Value: values.PartyValue(issuer)},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findInTx(resp.Transaction, "AllocationFactory")
}

// createTransferRule creates a TransferRule on P2.
// Required at accept time — must be disclosed to the receiver's participant via
// choiceContextData["utility.digitalasset.com/transfer-rule"].
func createTransferRule(ctx context.Context, c *ledger.Client, issuer, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-transfer-rule-%d", time.Now().UnixNano()),
			UserId:         sub(ctx, c),
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: registryID("Utility.Registry.V0.Rule.Transfer", "TransferRule"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "operator", Value: values.PartyValue(issuer)},
						{Label: "provider", Value: values.PartyValue(issuer)},
						{Label: "registrar", Value: values.PartyValue(issuer)},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findInTx(resp.Transaction, "TransferRule")
}

// createInstrumentConfiguration creates an InstrumentConfiguration on P2.
// holderRequirements = [] so no Credential contracts are needed locally.
// Required for validateTransfer inside TransferRule_TwoStepTransfer.
func createInstrumentConfiguration(ctx context.Context, c *ledger.Client, issuer, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-instrument-config-%d", time.Now().UnixNano()),
			UserId:         sub(ctx, c),
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: registryID("Utility.Registry.V0.Configuration.Instrument", "InstrumentConfiguration"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "operator", Value: values.PartyValue(issuer)},
						{Label: "provider", Value: values.PartyValue(issuer)},
						{Label: "registrar", Value: values.PartyValue(issuer)},
						// InstrumentIdentifier{source=registrar, id="USDCx", scheme="RegistrarInternalScheme"}
						// scheme must be "RegistrarInternalScheme" per toInstrumentIdentifier in holding package
						{Label: "defaultIdentifier", Value: encodeInstrumentIdentifier(issuer, "USDCx")},
						{Label: "additionalIdentifiers", Value: values.ListValue(nil)},
						{Label: "issuerRequirements", Value: values.ListValue(nil)},
						{Label: "holderRequirements", Value: values.ListValue(nil)},
						{Label: "providerAppRewardBeneficiaries", Value: values.None()},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findInTx(resp.Transaction, "InstrumentConfiguration")
}

// encodeInstrumentIdentifier encodes a Utility.Registry.Holding.V0.Types.InstrumentIdentifier.
// The scheme must be "RegistrarInternalScheme" for registrar-issued identifiers.
func encodeInstrumentIdentifier(source, id string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{Fields: []*lapiv2.RecordField{
				{Label: "source", Value: values.PartyValue(source)},
				{Label: "id", Value: values.TextValue(id)},
				{Label: "scheme", Value: values.TextValue("RegistrarInternalScheme")},
			}},
		},
	}
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
