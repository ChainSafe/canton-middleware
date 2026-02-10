//go:build ignore

// Archive old DAML contracts to allow migration to new package versions
//
// This script archives contracts from old packages (CIP56, bridge-wayfinder,
// bridge-core, common) so new contracts can be created with updated packages.
//
// Usage:
//   go run scripts/archive-cip56.go -config config.devnet.yaml [-dry-run]
//
// Flags:
//   -config       Path to config file (for Canton connection and auth)
//   -dry-run      List contracts without archiving (default: true)
//   -archive      Actually archive the contracts (sets dry-run to false)

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
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

// Colors for output
const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
	colorCyan   = "\033[0;36m"
	colorReset  = "\033[0m"
)

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
	jwtSubject  string
)

type ContractInfo struct {
	ContractID string
	TemplateID string
	PackageID  string
	ModuleName string
	EntityName string
}

// Templates to archive per package type
// Package IDs are loaded from config at runtime
var templatesByPackageType = map[string][]TemplateInfo{
	// bridge-wayfinder (config: bridge_package_id)
	"bridge": {
		{ModuleName: "Wayfinder.Bridge", EntityName: "WayfinderBridgeConfig"},
	},
	// bridge-core (config: core_package_id)
	"core": {
		{ModuleName: "Bridge.Contracts", EntityName: "MintCommand"},
		{ModuleName: "Bridge.Contracts", EntityName: "WithdrawalRequest"},
		{ModuleName: "Bridge.Contracts", EntityName: "WithdrawalEvent"},
		// Note: MintEvent/BurnEvent are now in CIP56.Events (cip56 package)
		// common module templates are compiled into bridge-core
		{ModuleName: "Common.FingerprintAuth", EntityName: "FingerprintMapping"},
		{ModuleName: "Common.FingerprintAuth", EntityName: "PendingDeposit"},
		{ModuleName: "Common.FingerprintAuth", EntityName: "DepositReceipt"},
	},
	// cip56-token (config: cip56_package_id)
	"cip56": {
		{ModuleName: "CIP56.Token", EntityName: "CIP56Manager"},
		{ModuleName: "CIP56.Token", EntityName: "CIP56Holding"},
		{ModuleName: "CIP56.Token", EntityName: "LockedAsset"},
		{ModuleName: "CIP56.Compliance", EntityName: "ComplianceRules"},
		{ModuleName: "CIP56.Compliance", EntityName: "ComplianceProof"},
		{ModuleName: "CIP56.Config", EntityName: "TokenConfig"},
		{ModuleName: "CIP56.Events", EntityName: "MintEvent"},
		{ModuleName: "CIP56.Events", EntityName: "BurnEvent"},
	},
}

// buildPackagesFromConfig creates the oldPackages map from config package IDs
func buildPackagesFromConfig(cfg *config.Config) map[string][]TemplateInfo {
	packages := make(map[string][]TemplateInfo)

	// Bridge package (bridge-wayfinder)
	if cfg.Canton.BridgePackageID != "" {
		packages[cfg.Canton.BridgePackageID] = templatesByPackageType["bridge"]
	}

	// Core package (bridge-core)
	if cfg.Canton.CorePackageID != "" {
		packages[cfg.Canton.CorePackageID] = templatesByPackageType["core"]
	}

	// CIP56 package
	if cfg.Canton.CIP56PackageID != "" {
		packages[cfg.Canton.CIP56PackageID] = templatesByPackageType["cip56"]
	}

	return packages
}

type TemplateInfo struct {
	ModuleName string
	EntityName string
}

func main() {
	configPath := flag.String("config", "config.devnet.yaml", "Path to config file")
	dryRun := flag.Bool("dry-run", true, "List contracts without archiving")
	doArchive := flag.Bool("archive", false, "Actually archive the contracts")
	flag.Parse()

	if *doArchive {
		*dryRun = false
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Connect to Canton
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	// Get OAuth2 token
	ctx, err = getAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		log.Fatalf("Failed to get auth context: %v", err)
	}

	issuerParty := cfg.Canton.RelayerParty
	domainID := cfg.Canton.DomainID

	// Build package map from config
	oldPackages := buildPackagesFromConfig(cfg)

	printHeader("Contract Archive Tool")
	fmt.Printf("Canton RPC:     %s\n", cfg.Canton.RPCURL)
	fmt.Printf("Issuer Party:   %s\n", issuerParty)
	fmt.Printf("Domain ID:      %s\n", domainID)
	fmt.Printf("Mode:           %s\n", modeString(*dryRun))
	fmt.Println()
	fmt.Println("Packages to archive (from config):")
	if cfg.Canton.BridgePackageID != "" {
		fmt.Printf("  - bridge:       %s\n", cfg.Canton.BridgePackageID[:16]+"...")
	}
	if cfg.Canton.CorePackageID != "" {
		fmt.Printf("  - core:         %s\n", cfg.Canton.CorePackageID[:16]+"...")
	}
	if cfg.Canton.CIP56PackageID != "" {
		fmt.Printf("  - cip56:        %s\n", cfg.Canton.CIP56PackageID[:16]+"...")
	}
	if len(oldPackages) == 0 {
		printWarning("No package IDs configured - nothing to archive")
		return
	}
	fmt.Println()

	stateService := lapiv2.NewStateServiceClient(conn)
	commandService := lapiv2.NewCommandServiceClient(conn)

	// Query all contracts from all packages
	var allContracts []ContractInfo
	for pkgID, templates := range oldPackages {
		for _, tmpl := range templates {
			contracts, err := findContracts(ctx, stateService, issuerParty, pkgID, tmpl.ModuleName, tmpl.EntityName)
			if err != nil {
				printWarning("Failed to query %s:%s: %v", tmpl.ModuleName, tmpl.EntityName, err)
				continue
			}
			if len(contracts) > 0 {
				printStep("%s:%s (%s...): %d contracts", tmpl.ModuleName, tmpl.EntityName, pkgID[:8], len(contracts))
				for _, c := range contracts {
					printInfo("  %s", truncate(c.ContractID, 60))
				}
			}
			allContracts = append(allContracts, contracts...)
		}
	}

	fmt.Println()
	printStep("Total contracts to archive: %d", len(allContracts))

	if *dryRun {
		fmt.Println()
		printWarning("DRY RUN - no changes made")
		fmt.Println()
		fmt.Println("To archive these contracts, run:")
		fmt.Printf("  go run scripts/archive-cip56.go -config %s -archive\n", *configPath)
		return
	}

	if len(allContracts) == 0 {
		printSuccess("No contracts to archive")
		return
	}

	// Confirm before archiving
	fmt.Println()
	printWarning("About to archive %d contracts. This cannot be undone!", len(allContracts))
	fmt.Print("Continue? (yes/no): ")
	var response string
	fmt.Scanln(&response)
	if response != "yes" {
		printWarning("Aborted")
		return
	}

	// Archive all contracts
	var archived, failed int

	// Archive in reverse dependency order (holdings/events before managers/configs)
	archiveOrder := []string{
		// Audit events first (CIP56.Events)
		"MintEvent", "BurnEvent",
		// Holdings and locked assets (before managers)
		"CIP56Holding", "LockedAsset",
		// Compliance
		"ComplianceProof", "ComplianceRules",
		// Withdrawal/deposit related
		"WithdrawalEvent", "WithdrawalRequest", "MintCommand",
		"DepositReceipt", "PendingDeposit",
		// Fingerprint mappings
		"FingerprintMapping",
		// Managers/configs last (they are referenced by other contracts)
		"CIP56Manager",
		"TokenConfig",
		"WayfinderBridgeConfig",
	}

	for _, entityName := range archiveOrder {
		for _, c := range allContracts {
			if c.EntityName != entityName {
				continue
			}
			if err := archiveContract(ctx, commandService, issuerParty, domainID, c.PackageID, c.ModuleName, c.EntityName, c.ContractID); err != nil {
				printError("Failed to archive %s %s: %v", c.EntityName, truncate(c.ContractID, 20), err)
				failed++
			} else {
				printSuccess("Archived %s: %s", c.EntityName, truncate(c.ContractID, 40))
				archived++
			}
		}
	}

	// Archive any remaining that weren't in the order list
	for _, c := range allContracts {
		found := false
		for _, name := range archiveOrder {
			if c.EntityName == name {
				found = true
				break
			}
		}
		if !found {
			if err := archiveContract(ctx, commandService, issuerParty, domainID, c.PackageID, c.ModuleName, c.EntityName, c.ContractID); err != nil {
				printError("Failed to archive %s %s: %v", c.EntityName, truncate(c.ContractID, 20), err)
				failed++
			} else {
				printSuccess("Archived %s: %s", c.EntityName, truncate(c.ContractID, 40))
				archived++
			}
		}
	}

	fmt.Println()
	printHeader("Archive Complete")
	fmt.Printf("Archived: %d, Failed: %d\n", archived, failed)

	fmt.Println()
	printStep("Next steps:")
	fmt.Println("  1. Update config with new package IDs")
	fmt.Println("  2. Re-bootstrap the bridge:")
	fmt.Printf("     go run scripts/bootstrap-bridge.go -config %s\n", *configPath)
}

func findContracts(ctx context.Context, client lapiv2.StateServiceClient, party, packageID, moduleName, entityName string) ([]ContractInfo, error) {
	ledgerEndResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return nil, nil // Empty ledger
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
										ModuleName: moduleName,
										EntityName: entityName,
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
		return nil, fmt.Errorf("get active contracts failed: %w", err)
	}

	var contracts []ContractInfo
	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			contracts = append(contracts, ContractInfo{
				ContractID: contract.CreatedEvent.ContractId,
				TemplateID: fmt.Sprintf("%s:%s:%s", packageID, moduleName, entityName),
				PackageID:  packageID,
				ModuleName: moduleName,
				EntityName: entityName,
			})
		}
	}

	return contracts, nil
}

func archiveContract(ctx context.Context, client lapiv2.CommandServiceClient, actAs, domainID, packageID, moduleName, entityName, contractID string) error {
	cmdID := fmt.Sprintf("archive-%s-%d", entityName, time.Now().UnixNano())

	// The Archive choice expects a Record of type DA.Internal.Template:Archive with no fields
	archiveArg := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				RecordId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "DA.Internal.Template",
					EntityName: "Archive",
				},
				Fields: []*lapiv2.RecordField{}, // Empty record
			},
		},
	}

	// Use the Archive choice (built-in for all templates)
	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: moduleName,
					EntityName: entityName,
				},
				ContractId:     contractID,
				Choice:         "Archive",
				ChoiceArgument: archiveArg,
			},
		},
	}

	commands := &lapiv2.Commands{
		SynchronizerId: domainID,
		CommandId:      cmdID,
		UserId:         jwtSubject,
		ActAs:          []string{actAs},
		Commands:       []*lapiv2.Command{cmd},
	}

	_, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: commands,
	})
	if err != nil {
		return err
	}

	return nil
}

func getAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, nil // No auth configured
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

	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OAuth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OAuth request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OAuth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OAuth failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse OAuth response: %w", err)
	}

	expiry := now.Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	cachedToken = tokenResp.AccessToken
	tokenExpiry = expiry

	if subject, err := extractJWTSubject(tokenResp.AccessToken); err == nil {
		jwtSubject = subject
	}

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

func modeString(dryRun bool) string {
	if dryRun {
		return colorYellow + "DRY RUN" + colorReset
	}
	return colorRed + "ARCHIVE (destructive)" + colorReset
}

// Output helpers
func printHeader(msg string) {
	fmt.Printf("\n%s══════════════════════════════════════════════════════════════════════%s\n", colorBlue, colorReset)
	fmt.Printf("%s  %s%s\n", colorBlue, msg, colorReset)
	fmt.Printf("%s══════════════════════════════════════════════════════════════════════%s\n", colorBlue, colorReset)
}

func printStep(format string, args ...interface{}) {
	fmt.Printf("%s>>> %s%s\n", colorCyan, fmt.Sprintf(format, args...), colorReset)
}

func printSuccess(format string, args ...interface{}) {
	fmt.Printf("%s✓ %s%s\n", colorGreen, fmt.Sprintf(format, args...), colorReset)
}

func printWarning(format string, args ...interface{}) {
	fmt.Printf("%s⚠ %s%s\n", colorYellow, fmt.Sprintf(format, args...), colorReset)
}

func printError(format string, args ...interface{}) {
	fmt.Printf("%s✗ %s%s\n", colorRed, fmt.Sprintf(format, args...), colorReset)
}

func printInfo(format string, args ...interface{}) {
	fmt.Printf("    %s\n", fmt.Sprintf(format, args...))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
