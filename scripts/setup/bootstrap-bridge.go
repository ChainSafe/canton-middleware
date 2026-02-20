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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
	jwtSubject  string
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	issuerParty := flag.String("issuer", "", "Full issuer party ID (uses config relayer_party if not specified)")
	packageID := flag.String("package", "", "Optional: bridge-wayfinder package ID (uses config if not specified)")
	domainIDFlag := flag.String("domain", "", "Optional: Domain/synchronizer ID (uses config if not specified)")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Use config values as defaults
	if *issuerParty == "" {
		*issuerParty = cfg.Canton.RelayerParty
	}
	if *packageID == "" {
		*packageID = cfg.Canton.BridgePackageID
	}
	if *domainIDFlag == "" {
		*domainIDFlag = cfg.Canton.DomainID
	}

	if *issuerParty == "" {
		fmt.Println("ERROR: -issuer flag is required (or set canton.relayer_party in config)")
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect to Canton with TLS if enabled
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
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

	// Get OAuth2 token and auth context
	ctx, err = getAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		log.Fatalf("Failed to get auth context: %v", err)
	}

	// Extract fingerprint from party ID
	parts := strings.Split(*issuerParty, "::")
	fingerprint := ""
	if len(parts) > 1 {
		fingerprint = parts[1]
	}

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("WAYFINDER BRIDGE BOOTSTRAP")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Printf("Canton RPC: %s\n", cfg.Canton.RPCURL)
	fmt.Printf("Issuer:     %s\n", *issuerParty)
	fmt.Printf("JWT Subject: %s\n", jwtSubject)
	fmt.Println()

	// Initialize service clients
	packageService := lapiv2.NewPackageServiceClient(conn)
	commandService := lapiv2.NewCommandServiceClient(conn)
	stateService := lapiv2.NewStateServiceClient(conn)

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
	cip56PackageID := cfg.Canton.CIP56PackageID
	if cip56PackageID == "" {
		log.Fatalf("cip56_package_id not set in config - run test-bridge.sh or set it manually")
	}
	fmt.Printf("    Using cip56-token package: %s\n", cip56PackageID)
	tokenManagerCid, err := createTokenManager(ctx, commandService, *issuerParty, cip56PackageID, domainID, cfg.Canton.ApplicationID)
	if err != nil {
		log.Fatalf("Failed to create CIP56Manager: %v", err)
	}
	fmt.Printf("    CIP56Manager Contract ID: %s\n", tokenManagerCid)
	fmt.Println()

	// Step 5: Create TokenConfig for PROMPT
	fmt.Println(">>> Step 5: Creating TokenConfig for PROMPT token...")
	tokenConfigCid, err := createPromptTokenConfig(ctx, commandService, *issuerParty, cip56PackageID, domainID, tokenManagerCid)
	if err != nil {
		log.Fatalf("Failed to create TokenConfig for PROMPT: %v", err)
	}
	fmt.Printf("    TokenConfig Contract ID: %s\n", tokenConfigCid)
	fmt.Println()

	// Step 5.5: Create CIP56TransferFactory
	fmt.Println(">>> Step 5.5: Creating CIP56TransferFactory...")
	factoryCid, err := createTransferFactory(ctx, commandService, *issuerParty, cip56PackageID, domainID)
	if err != nil {
		log.Fatalf("Failed to create CIP56TransferFactory: %v", err)
	}
	fmt.Printf("    CIP56TransferFactory: %s\n", factoryCid)
	fmt.Println()

	// Step 6: Create WayfinderBridgeConfig
	fmt.Println(">>> Step 6: Creating WayfinderBridgeConfig...")
	configCid, err := createBridgeConfig(ctx, commandService, *issuerParty, pkgID, domainID, cfg.Canton.ApplicationID, tokenConfigCid)
	if err != nil {
		log.Fatalf("Failed to create WayfinderBridgeConfig: %v", err)
	}
	fmt.Printf("    WayfinderBridgeConfig Contract ID: %s\n", configCid)
	fmt.Println()

	// Output config values
	printConfig(*issuerParty, pkgID, domainID, fingerprint, tokenManagerCid, configCid)
}

func getAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, fmt.Errorf("OAuth2 client credentials not configured")
	}

	token, err := getOAuthToken(auth)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func getOAuthToken(auth *config.AuthConfig) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	now := time.Now()
	if cachedToken != "" && now.Before(tokenExpiry) {
		return cachedToken, nil
	}

	payload := map[string]string{
		"client_id":     auth.ClientID,
		"client_secret": auth.ClientSecret,
		"audience":      auth.Audience,
		"grant_type":    "client_credentials",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OAuth token request: %w", err)
	}

	fmt.Printf("Fetching OAuth2 access token from %s...\n", auth.TokenURL)

	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OAuth token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("OAuth token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OAuth token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth token response missing access_token")
	}

	expiry := now.Add(5 * time.Minute)
	if tokenResp.ExpiresIn > 0 {
		leeway := 60
		if tokenResp.ExpiresIn <= leeway {
			leeway = tokenResp.ExpiresIn / 2
		}
		expiry = now.Add(time.Duration(tokenResp.ExpiresIn-leeway) * time.Second)
	}

	cachedToken = tokenResp.AccessToken
	tokenExpiry = expiry

	if subject, err := extractJWTSubject(tokenResp.AccessToken); err == nil {
		jwtSubject = subject
		fmt.Printf("JWT subject: %s\n", subject)
	}

	fmt.Printf("OAuth2 token obtained (expires in %d seconds)\n", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

func extractJWTSubject(tokenString string) (string, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse JWT: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid JWT claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("JWT missing 'sub' claim")
	}
	return sub, nil
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

	fmt.Printf("    Found %d packages on ledger\n", len(resp.PackageIds))

	// Package ID should be specified in config (auto-detected by test-bridge.sh)
	// Return the last one as a fallback heuristic
	return resp.PackageIds[len(resp.PackageIds)-1], nil
}

func getDomainID(ctx context.Context, client lapiv2.StateServiceClient, party string) (string, error) {
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

func createTokenManager(ctx context.Context, client lapiv2.CommandServiceClient, issuer, cip56PackageID, domainID, _ string) (string, error) {
	cmdID := fmt.Sprintf("bootstrap-token-manager-%d", time.Now().UnixNano())

	fmt.Printf("    Debug: issuer=%s, cip56PackageID=%s, domainID=%s\n", issuer, cip56PackageID, domainID)

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "PROMPT")},
			{Label: "meta", Value: values.EncodeMetadata(map[string]string{
				"splice.chainsafe.io/name":     "Wayfinder PROMPT",
				"splice.chainsafe.io/symbol":   "PROMPT",
				"splice.chainsafe.io/decimals": "18",
				"splice.chainsafe.io/erc20":    "0x28d38df637db75533bd3f71426f3410a82041544",
			})},
		},
	}

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

	commands := &lapiv2.Commands{
		SynchronizerId: domainID,
		CommandId:      cmdID,
		UserId:         jwtSubject,
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

func createPromptTokenConfig(ctx context.Context, client lapiv2.CommandServiceClient, issuer, cip56PackageID, domainID, tokenManagerCid string) (string, error) {
	cmdID := fmt.Sprintf("create-prompt-config-%d", time.Now().UnixNano())

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "tokenManagerCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: tokenManagerCid}}},
			{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "PROMPT")},
			{Label: "meta", Value: values.EncodeMetadata(map[string]string{
				"splice.chainsafe.io/name":     "Wayfinder PROMPT",
				"splice.chainsafe.io/symbol":   "PROMPT",
				"splice.chainsafe.io/decimals": "18",
				"splice.chainsafe.io/erc20":    "0x28d38df637db75533bd3f71426f3410a82041544",
			})},
			{Label: "auditObservers", Value: &lapiv2.Value{Sum: &lapiv2.Value_List{List: &lapiv2.List{Elements: []*lapiv2.Value{}}}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  cip56PackageID,
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
				},
				CreateArguments: createArgs,
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("submit create TokenConfig failed: %w", err)
	}

	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				if created.TemplateId.EntityName == "TokenConfig" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("TokenConfig not found in response")
}

func createBridgeConfig(ctx context.Context, client lapiv2.CommandServiceClient, issuer, packageID, domainID, _, tokenConfigCid string) (string, error) {
	cmdID := fmt.Sprintf("bootstrap-bridge-config-%d", time.Now().UnixNano())

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "tokenConfigCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: tokenConfigCid}}},
			{Label: "auditObservers", Value: &lapiv2.Value{Sum: &lapiv2.Value_List{List: &lapiv2.List{Elements: []*lapiv2.Value{}}}}},
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

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("submit create WayfinderBridgeConfig failed: %w", err)
	}

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

func createTransferFactory(ctx context.Context, client lapiv2.CommandServiceClient, issuer, cip56PackageID, domainID string) (string, error) {
	cmdID := fmt.Sprintf("bootstrap-factory-%d", time.Now().UnixNano())

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "admin", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  cip56PackageID,
					ModuleName: "CIP56.TransferFactory",
					EntityName: "CIP56TransferFactory",
				},
				CreateArguments: createArgs,
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("submit failed: %w", err)
	}

	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				if created.TemplateId.EntityName == "CIP56TransferFactory" {
					return created.ContractId, nil
				}
			}
		}
	}
	return "", fmt.Errorf("CIP56TransferFactory not found in response")
}

