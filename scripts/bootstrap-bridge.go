//go:build ignore

// Bootstrap script for initializing the Wayfinder Bridge on Canton
//
// PREREQUISITES:
// 1. Canton is running (docker compose up -d)
// 2. DARs are uploaded (run deploy-dars.canton)
// 3. Issuer party is allocated (via Canton console)
//
// Run: go run scripts/bootstrap-bridge.go -config config.yaml -issuer "BridgeIssuer::1220..."
//
// After running, update your config.yaml with the output values.

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	issuerParty := flag.String("issuer", "", "Full issuer party ID (e.g., BridgeIssuer::1220abc...)")
	packageID := flag.String("package", "", "Optional: bridge-wayfinder package ID (auto-detected if not specified)")
	domainIDFlag := flag.String("domain", "", "Optional: Domain/synchronizer ID (e.g., local::1220...)")
	flag.Parse()

	if *issuerParty == "" {
		fmt.Println("ERROR: -issuer flag is required")
		fmt.Println()
		fmt.Println("First allocate a party via HTTP API:")
		fmt.Println("  curl -X POST http://localhost:5013/v2/parties \\")
		fmt.Println("    -H 'Content-Type: application/json' \\")
		fmt.Println("    -d '{\"partyIdHint\": \"BridgeIssuer\"}'")
		fmt.Println()
		fmt.Println("Then run this script with the full party ID:")
		fmt.Println("  go run scripts/bootstrap-bridge.go -config config.yaml \\")
		fmt.Println("    -issuer \"BridgeIssuer::1220...\" \\")
		fmt.Println("    -domain \"local::1220...\"")
		fmt.Println()
		fmt.Println("Find domain ID with: docker logs canton 2>&1 | grep -oE 'local::[a-f0-9]{64}' | head -1")
		os.Exit(1)
	}

	// Validate issuer party format
	if !strings.Contains(*issuerParty, "::") {
		log.Fatalf("Invalid issuer party format. Expected 'Hint::Fingerprint', got: %s", *issuerParty)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect to Canton with TLS if enabled
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,           // Skip cert verification for dev
			NextProtos:         []string{"h2"}, // Force HTTP/2 ALPN
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
		fmt.Println("TLS: enabled (skip verify, HTTP/2)")
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		fmt.Println("TLS: disabled")
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	// Load JWT token if configured
	var authToken string
	if cfg.Canton.Auth.TokenFile != "" {
		tokenBytes, err := os.ReadFile(cfg.Canton.Auth.TokenFile)
		if err != nil {
			log.Fatalf("Failed to read token file %s: %v", cfg.Canton.Auth.TokenFile, err)
		}
		authToken = strings.TrimSpace(string(tokenBytes))
		fmt.Printf("Auth: JWT token loaded from %s\n", cfg.Canton.Auth.TokenFile)
	}

	// Create auth context helper
	getAuthCtx := func(ctx context.Context) context.Context {
		if authToken != "" {
			md := metadata.Pairs("authorization", "Bearer "+authToken)
			return metadata.NewOutgoingContext(ctx, md)
		}
		return ctx
	}
	// Use auth context for all calls
	ctx = getAuthCtx(ctx)

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("WAYFINDER BRIDGE BOOTSTRAP")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Printf("Canton RPC: %s\n", cfg.Canton.RPCURL)
	fmt.Printf("Issuer:     %s\n", *issuerParty)
	fmt.Println()

	// Initialize service clients
	packageService := lapiv2.NewPackageServiceClient(conn)
	commandService := lapiv2.NewCommandServiceClient(conn)
	stateService := lapiv2.NewStateServiceClient(conn)

	// Extract fingerprint from party ID
	parts := strings.Split(*issuerParty, "::")
	fingerprint := ""
	if len(parts) > 1 {
		fingerprint = parts[1]
	}

	// Step 1: Find bridge package ID
	fmt.Println(">>> Step 1: Finding bridge-wayfinder package...")
	pkgID := *packageID
	if pkgID == "" {
		var err error
		pkgID, err = findBridgePackage(ctx, packageService)
		if err != nil {
			log.Fatalf("Failed to find bridge package: %v", err)
		}
	}
	fmt.Printf("    Package ID: %s\n", pkgID)
	fmt.Println()

	// Step 2: Get domain ID
	fmt.Println(">>> Step 2: Getting domain ID...")
	domainID := *domainIDFlag
	if domainID == "" {
		// Try to get from config
		domainID = cfg.Canton.DomainID
	}
	if domainID == "" {
		// Try to auto-detect (may not work with all Canton versions)
		var err error
		domainID, err = getDomainID(ctx, stateService, *issuerParty)
		if err != nil {
			fmt.Println("    [WARN] Could not auto-detect domain ID")
			fmt.Println()
			fmt.Println("Find domain ID with:")
			fmt.Println("  docker logs canton 2>&1 | grep -oE 'local::[a-f0-9]{64}' | head -1")
			fmt.Println()
			fmt.Println("Then re-run with -domain flag:")
			fmt.Printf("  go run scripts/bootstrap-bridge.go -config %s -issuer \"%s\" -domain \"local::...\"\n", *configPath, *issuerParty)
			os.Exit(1)
		}
	}
	fmt.Printf("    Domain ID: %s\n", domainID)
	fmt.Println()

	// Step 3: Check if bridge config already exists
	fmt.Println(">>> Step 3: Checking for existing WayfinderBridgeConfig...")
	existingConfig, err := findExistingBridgeConfig(ctx, stateService, *issuerParty, pkgID)
	if err == nil && existingConfig != "" {
		fmt.Printf("    [EXISTS] WayfinderBridgeConfig: %s\n", existingConfig)
		fmt.Println()
		fmt.Println("Bridge is already bootstrapped! Config values:")
		printConfig(*issuerParty, pkgID, domainID, fingerprint, "", existingConfig)
		return
	}
	fmt.Println("    No existing config found, creating new one...")
	fmt.Println()

	// Step 4: Create CIP56Manager
	fmt.Println(">>> Step 4: Creating CIP56Manager for PROMPT token...")
	tokenManagerCid, err := createTokenManager(ctx, commandService, *issuerParty, pkgID, domainID, cfg.Canton.ApplicationID)
	if err != nil {
		log.Fatalf("Failed to create CIP56Manager: %v", err)
	}
	fmt.Printf("    CIP56Manager Contract ID: %s\n", tokenManagerCid)
	fmt.Println()

	// Step 5: Create WayfinderBridgeConfig
	fmt.Println(">>> Step 5: Creating WayfinderBridgeConfig...")
	configCid, err := createBridgeConfig(ctx, commandService, *issuerParty, pkgID, domainID, cfg.Canton.ApplicationID, tokenManagerCid)
	if err != nil {
		log.Fatalf("Failed to create WayfinderBridgeConfig: %v", err)
	}
	fmt.Printf("    WayfinderBridgeConfig Contract ID: %s\n", configCid)
	fmt.Println()

	// Output config values
	printConfig(*issuerParty, pkgID, domainID, fingerprint, tokenManagerCid, configCid)
}

func printConfig(issuerParty, pkgID, domainID, fingerprint, tokenManagerCid, configCid string) {
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("BOOTSTRAP COMPLETE - Update your config.yaml with these values:")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()
	fmt.Println("canton:")
	fmt.Printf("  relayer_party: \"%s\"\n", issuerParty)
	fmt.Printf("  bridge_package_id: \"%s\"\n", pkgID)
	fmt.Printf("  domain_id: \"%s\"\n", domainID)
	fmt.Println()
	fmt.Println("# Contract IDs (for reference):")
	if tokenManagerCid != "" {
		fmt.Printf("# CIP56Manager: %s\n", tokenManagerCid)
	}
	fmt.Printf("# WayfinderBridgeConfig: %s\n", configCid)
	fmt.Println()
	fmt.Printf("# Issuer fingerprint (for user deposits): %s\n", fingerprint)
}

func findBridgePackage(ctx context.Context, client lapiv2.PackageServiceClient) (string, error) {
	resp, err := client.ListPackages(ctx, &lapiv2.ListPackagesRequest{})
	if err != nil {
		return "", fmt.Errorf("list packages failed: %w", err)
	}

	if len(resp.PackageIds) == 0 {
		return "", fmt.Errorf("no packages found - ensure DARs are uploaded via deploy-dars.canton")
	}

	// For now, return the last package (most recently uploaded)
	// In a more sophisticated implementation, we'd parse the DAR metadata
	// to find the bridge-wayfinder package specifically
	return resp.PackageIds[len(resp.PackageIds)-1], nil
}

func getDomainID(ctx context.Context, client lapiv2.StateServiceClient, party string) (string, error) {
	// In Canton v2 API, domains are called "synchronizers"
	resp, err := client.GetConnectedSynchronizers(ctx, &lapiv2.GetConnectedSynchronizersRequest{
		Party: party,
	})
	if err != nil {
		return "", fmt.Errorf("get connected synchronizers failed: %w", err)
	}

	if len(resp.ConnectedSynchronizers) == 0 {
		return "", fmt.Errorf("no connected synchronizers found for party %s", party)
	}

	return resp.ConnectedSynchronizers[0].SynchronizerId, nil
}

func findExistingBridgeConfig(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string) (string, error) {
	// V2 API requires ActiveAtOffset - get current ledger end
	ledgerEndResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return "", fmt.Errorf("ledger is empty, no contracts exist")
	}

	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  packageID,
										ModuleName: "Wayfinder.Bridge",
										EntityName: "WayfinderBridgeConfig",
									},
								},
							},
						},
					},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("get active contracts failed: %w", err)
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			return contract.CreatedEvent.ContractId, nil
		}
	}

	return "", fmt.Errorf("no active WayfinderBridgeConfig found")
}

func createTokenManager(ctx context.Context, client lapiv2.CommandServiceClient, issuer, packageID, domainID, _ string) (string, error) {
	cmdID := fmt.Sprintf("bootstrap-token-manager-%d", time.Now().UnixNano())

	fmt.Printf("    Debug: issuer=%s, packageID=%s, domainID=%s\n", issuer, packageID, domainID)

	// PROMPT token metadata
	metaRecord := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "name", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "Wayfinder PROMPT"}}},
			{Label: "symbol", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "PROMPT"}}},
			{Label: "decimals", Value: &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: 18}}},
			{Label: "isin", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
			{Label: "dtiCode", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
			{Label: "regulatoryInfo", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{
				Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "ERC20: 0x28d38df637db75533bd3f71426f3410a82041544"}},
			}}}},
		},
	}

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "meta", Value: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: metaRecord}}},
		},
	}

	// CIP56Manager is in cip56-token package, not bridge-wayfinder
	cip56PackageID := "e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe"

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  cip56PackageID,
					ModuleName: "CIP56.Token",
					EntityName: "CIP56Manager",
				},
				CreateArguments: createArgs,
			},
		},
	}

	// UserId must match the JWT subject for authorization
	userID := "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients" // TODO: make configurable
	commands := &lapiv2.Commands{
		SynchronizerId: domainID,
		CommandId:      cmdID,
		UserId:         userID,
		ActAs:          []string{issuer},
		Commands:       []*lapiv2.Command{cmd},
	}

	fmt.Printf("    Submitting: SynchronizerId=%s, CmdId=%s, UserId=%s, ActAs=%v\n",
		commands.SynchronizerId, commands.CommandId, commands.UserId, commands.ActAs)

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: commands,
	})
	if err != nil {
		return "", fmt.Errorf("submit create CIP56Manager failed: %w", err)
	}

	// Extract contract ID from response
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "CIP56.Token" && templateId.EntityName == "CIP56Manager" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("CIP56Manager contract ID not found in response")
}

func createBridgeConfig(ctx context.Context, client lapiv2.CommandServiceClient, issuer, packageID, domainID, _, tokenManagerCid string) (string, error) {
	cmdID := fmt.Sprintf("bootstrap-bridge-config-%d", time.Now().UnixNano())

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "tokenManagerCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: tokenManagerCid}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				CreateArguments: createArgs,
			},
		},
	}

	// UserId must match the JWT subject for authorization
	userID := "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients" // TODO: make configurable
	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         userID,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("submit create WayfinderBridgeConfig failed: %w", err)
	}

	// Extract contract ID from response
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Wayfinder.Bridge" && templateId.EntityName == "WayfinderBridgeConfig" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("WayfinderBridgeConfig contract ID not found in response")
}
