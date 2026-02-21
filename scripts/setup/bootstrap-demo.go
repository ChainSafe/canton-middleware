//go:build ignore

// bootstrap-demo.go - Bootstrap DEMO token (native Canton token) for testing
//
// This script creates a TokenConfig (CIP56.Config) with its own CIP56Manager (DEMO metadata)
// and mints initial tokens to test users.
//
// Prerequisites:
// 1. Canton is running with DARs uploaded (cip56-token)
// 2. Users are registered (have FingerprintMapping contracts)
//
// Usage:
//   go run scripts/bootstrap-demo.go -config config.yaml \
//     -user1-fingerprint "0x..." \
//     -user2-fingerprint "0x..."
//
// After running, Users 1 and 2 will each have 500 DEMO tokens.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
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
)

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
	jwtSubject  string
)

// DEMO token metadata
var demoMetadata = map[string]interface{}{
	"name":           "Demo Token",
	"symbol":         "DEMO",
	"decimals":       int64(18),
	"isin":           nil,
	"dtiCode":        nil,
	"regulatoryInfo": nil,
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	cip56PackageID := flag.String("cip56-package-id", "", "CIP56 package ID (uses config.canton.cip56_package_id if not set)")
	issuerFlag := flag.String("issuer", "", "Issuer party ID (optional, uses config if not specified)")
	domainFlag := flag.String("domain", "", "Domain/synchronizer ID (optional, uses config if not specified)")
	user1Fingerprint := flag.String("user1-fingerprint", "", "User 1 EVM fingerprint (required)")
	user2Fingerprint := flag.String("user2-fingerprint", "", "User 2 EVM fingerprint (required)")
	user1PartyFlag := flag.String("user1-party", "", "User 1 Canton party ID (optional, skips FingerprintMapping lookup for DEMO-only mode)")
	user2PartyFlag := flag.String("user2-party", "", "User 2 Canton party ID (optional, skips FingerprintMapping lookup for DEMO-only mode)")
	mintAmount := flag.String("mint-amount", "500.0", "Amount to mint to each user")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Get package IDs - TokenConfig is now in cip56-token package
	if *cip56PackageID == "" {
		*cip56PackageID = cfg.Canton.CIP56PackageID
	}
	if *cip56PackageID == "" {
		log.Fatal("cip56_package_id is required in config or via -cip56-package-id flag")
	}

	// Get issuer party (prefer flag over config)
	issuer := *issuerFlag
	if issuer == "" {
		issuer = cfg.Canton.RelayerParty
	}
	if issuer == "" {
		log.Fatal("issuer is required (set via -issuer flag or config.canton.relayer_party)")
	}

	// Get domain ID (prefer flag over config)
	domainID := *domainFlag
	if domainID == "" {
		domainID = cfg.Canton.DomainID
	}
	if domainID == "" {
		log.Fatal("domain is required (set via -domain flag or config.canton.domain_id)")
	}

	if *user1Fingerprint == "" || *user2Fingerprint == "" {
		log.Fatal("Both -user1-fingerprint and -user2-fingerprint are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

	// Get auth context
	ctx, err = getAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		log.Fatalf("Failed to get auth context: %v", err)
	}

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("DEMO TOKEN BOOTSTRAP")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Printf("Canton RPC: %s\n", cfg.Canton.RPCURL)
	fmt.Printf("Issuer:     %s\n", issuer)
	fmt.Printf("CIP56 Package:  %s\n", *cip56PackageID)
	fmt.Printf("Mint Amount: %s DEMO per user\n", *mintAmount)
	fmt.Println()

	stateService := lapiv2.NewStateServiceClient(conn)
	commandService := lapiv2.NewCommandServiceClient(conn)

	// Step 1: Check if TokenConfig (DEMO) already exists
	fmt.Println(">>> Step 1: Checking for existing TokenConfig (DEMO)...")
	existingConfig, err := findTokenConfig(ctx, stateService, issuer, *cip56PackageID)
	if err == nil && existingConfig != "" {
		fmt.Printf("    [EXISTS] TokenConfig: %s\n", existingConfig)
		fmt.Println()
		fmt.Println("    Skipping DEMO CIP56Manager creation...")

		// Still mint to users if they don't have tokens
		mintToUsers(ctx, stateService, commandService, cfg, *cip56PackageID, existingConfig,
			*user1Fingerprint, *user2Fingerprint, *user1PartyFlag, *user2PartyFlag, *mintAmount, issuer, domainID)
		return
	}
	fmt.Println("    No existing TokenConfig (DEMO) found, creating new one...")
	fmt.Println()

	// Step 2: Create DEMO CIP56Manager
	fmt.Println(">>> Step 2: Creating DEMO CIP56Manager...")
	demoManagerCid, err := createDemoTokenManager(ctx, commandService, issuer, *cip56PackageID, domainID)
	if err != nil {
		log.Fatalf("Failed to create DEMO CIP56Manager: %v", err)
	}
	fmt.Printf("    DEMO CIP56Manager: %s\n", demoManagerCid)
	fmt.Println()

	// Step 3: Create TokenConfig (DEMO)
	fmt.Println(">>> Step 3: Creating TokenConfig (DEMO)...")
	nativeConfigCid, err := createTokenConfig(ctx, commandService, issuer, *cip56PackageID, domainID, demoManagerCid)
	if err != nil {
		log.Fatalf("Failed to create TokenConfig: %v", err)
	}
	fmt.Printf("    TokenConfig (DEMO): %s\n", nativeConfigCid)
	fmt.Println()

	// Step 4-5: Mint to users
	mintToUsers(ctx, stateService, commandService, cfg, *cip56PackageID, nativeConfigCid,
		*user1Fingerprint, *user2Fingerprint, *user1PartyFlag, *user2PartyFlag, *mintAmount, issuer, domainID)

	// Step 6: Update database with DEMO balances
	fmt.Println(">>> Step 6: Updating database with DEMO balances...")
	if err := updateDemoBalancesInDB(cfg, *user1Fingerprint, *user2Fingerprint, *mintAmount); err != nil {
		log.Printf("Warning: Failed to update database: %v", err)
		fmt.Println("    [WARN] Database update failed - balances won't show in MetaMask until manually updated")
	} else {
		fmt.Println("    Database updated successfully")
	}
	fmt.Println()

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("DEMO TOKEN BOOTSTRAP COMPLETE")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()
	fmt.Printf("DEMO CIP56Manager:    %s\n", demoManagerCid)
	fmt.Printf("TokenConfig (DEMO):   %s\n", nativeConfigCid)
	fmt.Printf("User 1 (%s): %s DEMO\n", *user1Fingerprint, *mintAmount)
	fmt.Printf("User 2 (%s): %s DEMO\n", *user2Fingerprint, *mintAmount)
}

func mintToUsers(ctx context.Context, stateService lapiv2.StateServiceClient, commandService lapiv2.CommandServiceClient, cfg *config.Config,
	cip56PackageID, nativeConfigCid, user1Fingerprint, user2Fingerprint, user1PartyOpt, user2PartyOpt, mintAmount, issuer, domainID string) {

	var user1Party, user2Party string
	var err error

	// Use provided party IDs or look up via FingerprintMapping
	if user1PartyOpt != "" {
		user1Party = user1PartyOpt
		fmt.Println("    Using provided User 1 party ID (DEMO-only mode)")
	} else {
		user1Party, err = getUserParty(ctx, stateService,
			cfg.Canton.BridgePackageID, issuer, user1Fingerprint)
		if err != nil {
			log.Fatalf("Failed to get User 1 party: %v (make sure user is registered or provide -user1-party flag for DEMO-only mode)", err)
		}
	}

	if user2PartyOpt != "" {
		user2Party = user2PartyOpt
		fmt.Println("    Using provided User 2 party ID (DEMO-only mode)")
	} else {
		user2Party, err = getUserParty(ctx, stateService,
			cfg.Canton.BridgePackageID, issuer, user2Fingerprint)
		if err != nil {
			log.Fatalf("Failed to get User 2 party: %v (make sure user is registered or provide -user2-party flag for DEMO-only mode)", err)
		}
	}

	// Step 4: Mint to User 1
	fmt.Println(">>> Step 4: Minting DEMO to User 1...")
	fmt.Printf("    Party: %s\n", user1Party)
	holding1, err := mintDemoTokens(ctx, commandService, cip56PackageID, nativeConfigCid,
		user1Party, mintAmount, user1Fingerprint, issuer, domainID)
	if err != nil {
		log.Fatalf("Failed to mint to User 1: %v", err)
	}
	fmt.Printf("    Holding: %s\n", holding1)
	fmt.Println()

	// Step 5: Mint to User 2
	fmt.Println(">>> Step 5: Minting DEMO to User 2...")
	fmt.Printf("    Party: %s\n", user2Party)
	holding2, err := mintDemoTokens(ctx, commandService, cip56PackageID, nativeConfigCid,
		user2Party, mintAmount, user2Fingerprint, issuer, domainID)
	if err != nil {
		log.Fatalf("Failed to mint to User 2: %v", err)
	}
	fmt.Printf("    Holding: %s\n", holding2)
	fmt.Println()
}

func getAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		// No auth configured, return context as-is (local Canton)
		return ctx, nil
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
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OAuth token response: %w", err)
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
	}

	return tokenResp.AccessToken, nil
}

func extractJWTSubject(tokenString string) (string, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", err
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

func findTokenConfig(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string) (string, error) {
	ledgerEndResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", err
	}
	if ledgerEndResp.Offset == 0 {
		return "", fmt.Errorf("ledger is empty")
	}

	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEndResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  packageID,
										ModuleName: "CIP56.Config",
										EntityName: "TokenConfig",
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
		return "", err
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			event := contract.CreatedEvent
			// Check that this is the DEMO TokenConfig by inspecting the meta.symbol field
			// TokenConfig fields: issuer(0), tokenManagerCid(1), meta(2), auditObservers(3)
			fields := event.GetCreateArguments().GetFields()
			if len(fields) >= 3 {
				metaFields := fields[2].GetValue().GetRecord().GetFields()
				if len(metaFields) >= 2 {
					symbol := metaFields[1].GetValue().GetText()
					if symbol == "DEMO" {
						return event.ContractId, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("not found")
}

func createDemoTokenManager(ctx context.Context, client lapiv2.CommandServiceClient, issuer, cip56PackageID, domainID string) (string, error) {
	cmdID := fmt.Sprintf("create-demo-manager-%d", time.Now().UnixNano())

	metaRecord := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "name", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "Demo Token"}}},
			{Label: "symbol", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "DEMO"}}},
			{Label: "decimals", Value: &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: 18}}},
			{Label: "isin", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
			{Label: "dtiCode", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
			{Label: "regulatoryInfo", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
		},
	}

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "meta", Value: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: metaRecord}}},
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
				if created.TemplateId.EntityName == "CIP56Manager" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("CIP56Manager not found in response")
}

func createTokenConfig(ctx context.Context, client lapiv2.CommandServiceClient, issuer, cip56PackageID, domainID, tokenManagerCid string) (string, error) {
	cmdID := fmt.Sprintf("create-native-config-%d", time.Now().UnixNano())

	metaRecord := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "name", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "Demo Token"}}},
			{Label: "symbol", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "DEMO"}}},
			{Label: "decimals", Value: &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: 18}}},
			{Label: "isin", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
			{Label: "dtiCode", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
			{Label: "regulatoryInfo", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
		},
	}

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "tokenManagerCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: tokenManagerCid}}},
			{Label: "meta", Value: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: metaRecord}}},
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
		return "", fmt.Errorf("submit failed: %w", err)
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

func mintDemoTokens(ctx context.Context, client lapiv2.CommandServiceClient, cip56PackageID, nativeConfigCid, recipientParty, amount, fingerprint, issuer, domainID string) (string, error) {
	cmdID := fmt.Sprintf("mint-demo-%d", time.Now().UnixNano())

	mintArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: recipientParty}}},
			{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amount}}},
			{Label: "eventTime", Value: &lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: time.Now().UnixMicro()}}},
			{Label: "userFingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: fingerprint}}},
			{Label: "evmTxHash", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  cip56PackageID,
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
				},
				ContractId:     nativeConfigCid,
				Choice:         "IssuerMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: mintArgs}},
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
				if created.TemplateId.EntityName == "CIP56Holding" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("CIP56Holding not found in response")
}

func getUserParty(ctx context.Context, client lapiv2.StateServiceClient, bridgePackageID, issuer, fingerprint string) (string, error) {
	ledgerEndResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", err
	}
	if ledgerEndResp.Offset == 0 {
		return "", fmt.Errorf("ledger is empty")
	}

	// Query FingerprintMapping contracts using wildcard filter
	// FingerprintMapping is in the 'common' package which has a different package ID
	// than bridge-wayfinder, so we use wildcard and filter by entity name
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEndResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				issuer: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
								WildcardFilter: &lapiv2.WildcardFilter{},
							},
						},
					},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return "", err
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			// Filter by module and entity name since we're using wildcard
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "Common.FingerprintAuth" || templateId.EntityName != "FingerprintMapping" {
				continue
			}

			args := contract.CreatedEvent.CreateArguments
			if args == nil {
				continue
			}

			var foundFingerprint, foundParty string
			for _, field := range args.Fields {
				switch field.Label {
				case "fingerprint":
					if t := field.Value.GetText(); t != "" {
						foundFingerprint = t
					}
				case "userParty":
					if p := field.Value.GetParty(); p != "" {
						foundParty = p
					}
				}
			}

			// Normalize fingerprints for comparison (case-insensitive)
			if strings.EqualFold(foundFingerprint, fingerprint) && foundParty != "" {
				return foundParty, nil
			}
		}
	}

	return "", fmt.Errorf("no FingerprintMapping found for fingerprint %s", fingerprint)
}

// updateDemoBalancesInDB connects to the database and updates DEMO balances for users
func updateDemoBalancesInDB(cfg *config.Config, user1Fingerprint, user2Fingerprint, amount string) error {
	// Build database connection string
	// Use localhost since we're running from host machine
	dbHost := cfg.Database.Host
	if dbHost == "postgres" {
		dbHost = "localhost" // Convert docker service name to localhost
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		dbHost,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Database,
		cfg.Database.SSLMode,
	)
	if cfg.Database.SSLMode == "" {
		connStr = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			dbHost,
			cfg.Database.Port,
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Database,
		)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Verify connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Ensure the demo_balance column exists (migration)
	_, err = db.Exec(`
		DO $$ 
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
						   WHERE table_name='users' AND column_name='demo_balance') THEN
				ALTER TABLE users ADD COLUMN demo_balance DECIMAL(38,18) DEFAULT 0;
			END IF;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("failed to ensure demo_balance column exists: %w", err)
	}

	// Update User 1's DEMO balance
	result, err := db.Exec(`
		UPDATE users 
		SET demo_balance = COALESCE(demo_balance, 0) + $1::DECIMAL,
			balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, amount, user1Fingerprint, strings.TrimPrefix(user1Fingerprint, "0x"))
	if err != nil {
		return fmt.Errorf("failed to update User 1 DEMO balance: %w", err)
	}
	rows, _ := result.RowsAffected()
	fmt.Printf("    User 1 (%s): %s DEMO (rows affected: %d)\n", user1Fingerprint[:20]+"...", amount, rows)

	// Update User 2's DEMO balance
	result, err = db.Exec(`
		UPDATE users 
		SET demo_balance = COALESCE(demo_balance, 0) + $1::DECIMAL,
			balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, amount, user2Fingerprint, strings.TrimPrefix(user2Fingerprint, "0x"))
	if err != nil {
		return fmt.Errorf("failed to update User 2 DEMO balance: %w", err)
	}
	rows, _ = result.RowsAffected()
	fmt.Printf("    User 2 (%s): %s DEMO (rows affected: %d)\n", user2Fingerprint[:20]+"...", amount, rows)

	return nil
}
