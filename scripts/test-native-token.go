//go:build ignore

// test-native-token.go - Integration test script for native token operations
// Emulates API server calls to Canton for native token mint/burn/transfer operations
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v3"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

// Config represents the minimal config needed for tests
type Config struct {
	Canton struct {
		RPCURL       string `yaml:"rpc_url"`
		RelayerParty string `yaml:"relayer_party"`
		DomainID     string `yaml:"domain_id"`
		TLS          struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"tls"`
		Auth struct {
			ClientID     string `yaml:"client_id"`
			ClientSecret string `yaml:"client_secret"`
			Audience     string `yaml:"audience"`
			TokenURL     string `yaml:"token_url"`
		} `yaml:"auth"`
	} `yaml:"canton"`
}

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
	jwtSubject  string
)

func main() {
	configPath := flag.String("config", ".test-config.yaml", "Config file path")
	action := flag.String("action", "mint", "Action: setup, mint, burn, transfer, balance, events")
	amount := flag.String("amount", "100.0", "Token amount")
	recipient := flag.String("recipient", "", "Recipient party (for mint/transfer)")
	holdingCid := flag.String("holding-cid", "", "Holding contract ID (for burn/transfer)")
	configCid := flag.String("config-cid", "", "NativeTokenConfig contract ID")
	packageID := flag.String("package-id", "", "Native token package ID (required)")
	cip56PackageID := flag.String("cip56-package-id", "", "CIP56 token package ID (required for setup)")
	userFingerprint := flag.String("fingerprint", "0xtest-fingerprint", "User fingerprint for mint/burn")
	senderFingerprint := flag.String("sender-fingerprint", "0xsender-fp", "Sender fingerprint for transfer")
	recipientFingerprint := flag.String("recipient-fingerprint", "0xrecipient-fp", "Recipient fingerprint for transfer")
	flag.Parse()

	if *packageID == "" {
		*packageID = os.Getenv("NATIVE_TOKEN_PACKAGE_ID")
	}
	if *cip56PackageID == "" {
		*cip56PackageID = os.Getenv("CIP56_PACKAGE_ID")
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Get OAuth token first
	ctx := context.Background()
	token, err := getAccessToken(cfg)
	if err != nil {
		log.Fatalf("Failed to get OAuth token: %v", err)
	}

	// Build gRPC dial options
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	opts = append(opts, grpc.WithUnaryInterceptor(authInterceptor(token)))
	opts = append(opts, grpc.WithStreamInterceptor(streamAuthInterceptor(token)))

	// Connect to Canton
	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	commandService := lapiv2.NewCommandServiceClient(conn)
	stateService := lapiv2.NewStateServiceClient(conn)

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("NATIVE TOKEN INTEGRATION TEST")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("Canton RPC:      %s\n", cfg.Canton.RPCURL)
	fmt.Printf("Issuer Party:    %s\n", cfg.Canton.RelayerParty)
	fmt.Printf("Domain ID:       %s\n", cfg.Canton.DomainID)
	fmt.Printf("JWT Subject:     %s\n", jwtSubject)
	fmt.Printf("Package ID:      %s\n", *packageID)
	fmt.Printf("Action:          %s\n", *action)
	fmt.Println()

	switch *action {
	case "setup":
		if *cip56PackageID == "" {
			log.Fatal("--cip56-package-id required for setup")
		}
		setupNativeToken(ctx, cfg, commandService, *packageID, *cip56PackageID)
	case "mint":
		if *configCid == "" || *packageID == "" {
			log.Fatal("--config-cid and --package-id required for mint")
		}
		mintTokens(ctx, cfg, commandService, *packageID, *configCid, *recipient, *amount, *userFingerprint)
	case "burn":
		if *configCid == "" || *holdingCid == "" || *packageID == "" {
			log.Fatal("--config-cid, --holding-cid, and --package-id required for burn")
		}
		burnTokens(ctx, cfg, commandService, *packageID, *configCid, *holdingCid, *amount, *userFingerprint)
	case "transfer":
		if *configCid == "" || *holdingCid == "" || *recipient == "" || *packageID == "" {
			log.Fatal("--config-cid, --holding-cid, --recipient, and --package-id required for transfer")
		}
		transferTokens(ctx, cfg, commandService, *packageID, *configCid, *holdingCid, *recipient, *amount, *senderFingerprint, *recipientFingerprint)
	case "balance":
		checkBalance(ctx, cfg, stateService, *cip56PackageID)
	case "events":
		listEvents(ctx, cfg, stateService, *packageID)
	default:
		log.Fatalf("Unknown action: %s", *action)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// authInterceptor creates a unary interceptor that adds authorization header
func authInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// streamAuthInterceptor creates a stream interceptor that adds authorization header
func streamAuthInterceptor(token string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func getAccessToken(cfg *Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", cfg.Canton.Auth.ClientID)
	data.Set("client_secret", cfg.Canton.Auth.ClientSecret)
	data.Set("audience", cfg.Canton.Auth.Audience)

	resp, err := http.PostForm(cfg.Canton.Auth.TokenURL, data)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)
	cachedToken = tokenResp.AccessToken
	tokenExpiry = expiry

	// Extract JWT subject
	if subject, err := extractJWTSubject(tokenResp.AccessToken); err == nil {
		jwtSubject = subject
	}

	return tokenResp.AccessToken, nil
}

func extractJWTSubject(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}

	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}

	return claims.Sub, nil
}

func setupNativeToken(ctx context.Context, cfg *Config, commandService lapiv2.CommandServiceClient, packageID, cip56PackageID string) {
	fmt.Println(">>> Setting up NativeTokenConfig...")
	issuer := cfg.Canton.RelayerParty
	domainID := cfg.Canton.DomainID

	// Step 1: Create CIP56Manager
	fmt.Println("    1. Creating CIP56Manager...")
	managerCid, err := createCIP56Manager(ctx, cfg, commandService, cip56PackageID, issuer, domainID)
	if err != nil {
		log.Fatalf("Failed to create CIP56Manager: %v", err)
	}
	fmt.Printf("    ✓ CIP56Manager created: %s\n", managerCid)

	// Step 2: Create NativeTokenConfig
	fmt.Println("    2. Creating NativeTokenConfig...")
	configCid, err := createNativeTokenConfig(ctx, cfg, commandService, packageID, issuer, domainID, managerCid)
	if err != nil {
		log.Fatalf("Failed to create NativeTokenConfig: %v", err)
	}
	fmt.Printf("    ✓ NativeTokenConfig created: %s\n", configCid)

	fmt.Println()
	fmt.Println("Setup complete! Use these contract IDs for subsequent operations:")
	fmt.Printf("  --config-cid %s\n", configCid)
}

func createCIP56Manager(ctx context.Context, cfg *Config, client lapiv2.CommandServiceClient, packageID, issuer, domainID string) (string, error) {
	cmdID := fmt.Sprintf("create-cip56-manager-%d", time.Now().UnixNano())

	metaFields := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "name", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "Native Test Token"}}},
			{Label: "symbol", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "NTT"}}},
			{Label: "decimals", Value: &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: 18}}},
			{Label: "isin", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
			{Label: "dtiCode", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
			{Label: "regulatoryInfo", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
		},
	}

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "meta", Value: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: metaFields}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
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

	return "", fmt.Errorf("CIP56Manager contract ID not found in response")
}

func createNativeTokenConfig(ctx context.Context, cfg *Config, client lapiv2.CommandServiceClient, packageID, issuer, domainID, managerCid string) (string, error) {
	cmdID := fmt.Sprintf("create-native-token-config-%d", time.Now().UnixNano())

	metaFields := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "name", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "Native Test Token"}}},
			{Label: "symbol", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: "NTT"}}},
			{Label: "decimals", Value: &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: 18}}},
			{Label: "isin", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
			{Label: "dtiCode", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
			{Label: "regulatoryInfo", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
		},
	}

	createArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
			{Label: "tokenManagerCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: managerCid}}},
			{Label: "meta", Value: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: metaFields}}},
			{Label: "auditObservers", Value: &lapiv2.Value{Sum: &lapiv2.Value_List{List: &lapiv2.List{}}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Native.Token",
					EntityName: "NativeTokenConfig",
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
				if created.TemplateId.EntityName == "NativeTokenConfig" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("NativeTokenConfig contract ID not found in response")
}

func mintTokens(ctx context.Context, cfg *Config, commandService lapiv2.CommandServiceClient, packageID, configCid, recipient, amount, userFingerprint string) {
	fmt.Printf(">>> Minting %s tokens (fingerprint: %s)...\n", amount, userFingerprint)
	issuer := cfg.Canton.RelayerParty
	domainID := cfg.Canton.DomainID

	if recipient == "" {
		recipient = issuer
	}

	cmdID := fmt.Sprintf("mint-%d", time.Now().UnixNano())

	choiceArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: recipient}}},
			{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amount}}},
			{Label: "eventTime", Value: &lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: time.Now().UnixMicro()}}},
			{Label: "userFingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: userFingerprint}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Native.Token",
					EntityName: "NativeTokenConfig",
				},
				ContractId:     configCid,
				Choice:         "IssuerMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: choiceArgs}},
			},
		},
	}

	resp, err := commandService.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		log.Fatalf("Mint failed: %v", err)
	}

	fmt.Println("✓ Mint successful!")
	if resp.Transaction != nil {
		fmt.Printf("  Transaction ID: %s\n", resp.Transaction.UpdateId)
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				fmt.Printf("  Created: %s (%s)\n", created.TemplateId.EntityName, created.ContractId[:40]+"...")
			}
		}
	}
}

func burnTokens(ctx context.Context, cfg *Config, commandService lapiv2.CommandServiceClient, packageID, configCid, holdingCid, amount, userFingerprint string) {
	fmt.Printf(">>> Burning %s tokens (fingerprint: %s)...\n", amount, userFingerprint)
	issuer := cfg.Canton.RelayerParty
	domainID := cfg.Canton.DomainID

	cmdID := fmt.Sprintf("burn-%d", time.Now().UnixNano())

	choiceArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "holdingCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: holdingCid}}},
			{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amount}}},
			{Label: "eventTime", Value: &lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: time.Now().UnixMicro()}}},
			{Label: "userFingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: userFingerprint}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Native.Token",
					EntityName: "NativeTokenConfig",
				},
				ContractId:     configCid,
				Choice:         "IssuerBurn",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: choiceArgs}},
			},
		},
	}

	resp, err := commandService.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		log.Fatalf("Burn failed: %v", err)
	}

	fmt.Println("✓ Burn successful!")
	if resp.Transaction != nil {
		fmt.Printf("  Transaction ID: %s\n", resp.Transaction.UpdateId)
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				fmt.Printf("  Created: %s (%s)\n", created.TemplateId.EntityName, created.ContractId[:40]+"...")
			}
		}
	}
}

func transferTokens(ctx context.Context, cfg *Config, commandService lapiv2.CommandServiceClient, packageID, configCid, holdingCid, recipient, amount, senderFp, recipientFp string) {
	fmt.Printf(">>> Transferring %s tokens (sender: %s → recipient: %s)...\n", amount, senderFp, recipientFp)
	issuer := cfg.Canton.RelayerParty
	domainID := cfg.Canton.DomainID

	cmdID := fmt.Sprintf("transfer-%d", time.Now().UnixNano())

	choiceArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "senderHoldingCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: holdingCid}}},
			{Label: "recipient", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: recipient}}},
			{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amount}}},
			{Label: "eventTime", Value: &lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: time.Now().UnixMicro()}}},
			{Label: "senderFingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: senderFp}}},
			{Label: "recipientFingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: recipientFp}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Native.Token",
					EntityName: "NativeTokenConfig",
				},
				ContractId:     configCid,
				Choice:         "IssuerTransfer",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: choiceArgs}},
			},
		},
	}

	resp, err := commandService.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		log.Fatalf("Transfer failed: %v", err)
	}

	fmt.Println("✓ Transfer successful!")
	if resp.Transaction != nil {
		fmt.Printf("  Transaction ID: %s\n", resp.Transaction.UpdateId)
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				fmt.Printf("  Created: %s (%s)\n", created.TemplateId.EntityName, created.ContractId[:40]+"...")
			}
		}
	}
}

func checkBalance(ctx context.Context, cfg *Config, stateService lapiv2.StateServiceClient, cip56PackageID string) {
	fmt.Println(">>> Checking CIP56Holding balances...")
	issuer := cfg.Canton.RelayerParty

	// Get ledger end offset
	ledgerEnd, err := stateService.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		log.Fatalf("Failed to get ledger end: %v", err)
	}

	stream, err := stateService.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEnd.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				issuer: {
					Cumulative: []*lapiv2.CumulativeFilter{{
						IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
							WildcardFilter: &lapiv2.WildcardFilter{},
						},
					}},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to query contracts: %v", err)
	}

	holdings := []map[string]interface{}{}

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if c := msg.GetActiveContract(); c != nil {
			tid := c.CreatedEvent.TemplateId
			if tid.ModuleName == "CIP56.Token" && tid.EntityName == "CIP56Holding" {
				holding := map[string]interface{}{
					"contractId": c.CreatedEvent.ContractId,
					"owner":      getFieldValue(c.CreatedEvent.CreateArguments, "owner"),
					"amount":     getFieldValue(c.CreatedEvent.CreateArguments, "amount"),
				}
				holdings = append(holdings, holding)
			}
		}
	}

	fmt.Printf("\nFound %d holdings:\n", len(holdings))
	for _, h := range holdings {
		fmt.Printf("  - Contract: %s\n", h["contractId"].(string))
		fmt.Printf("    Owner:    %v\n", h["owner"])
		fmt.Printf("    Amount:   %v\n", h["amount"])
	}
}

func listEvents(ctx context.Context, cfg *Config, stateService lapiv2.StateServiceClient, packageID string) {
	fmt.Println(">>> Listing audit events (MintEvent, BurnEvent, TransferEvent)...")
	issuer := cfg.Canton.RelayerParty

	// Get ledger end offset
	ledgerEnd, err := stateService.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		log.Fatalf("Failed to get ledger end: %v", err)
	}

	stream, err := stateService.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEnd.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				issuer: {
					Cumulative: []*lapiv2.CumulativeFilter{{
						IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
							WildcardFilter: &lapiv2.WildcardFilter{},
						},
					}},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}

	events := []map[string]interface{}{}

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if c := msg.GetActiveContract(); c != nil {
			tid := c.CreatedEvent.TemplateId
			templateName := tid.EntityName

			if templateName == "MintEvent" || templateName == "BurnEvent" || templateName == "TransferEvent" {
				event := map[string]interface{}{
					"type":       templateName,
					"contractId": c.CreatedEvent.ContractId,
					"amount":     getFieldValue(c.CreatedEvent.CreateArguments, "amount"),
					"timestamp":  getFieldValue(c.CreatedEvent.CreateArguments, "timestamp"),
				}

				switch templateName {
				case "MintEvent":
					event["recipient"] = getFieldValue(c.CreatedEvent.CreateArguments, "recipient")
					event["userFingerprint"] = getFieldValue(c.CreatedEvent.CreateArguments, "userFingerprint")
				case "BurnEvent":
					event["burnedFrom"] = getFieldValue(c.CreatedEvent.CreateArguments, "burnedFrom")
					event["userFingerprint"] = getFieldValue(c.CreatedEvent.CreateArguments, "userFingerprint")
				case "TransferEvent":
					event["sender"] = getFieldValue(c.CreatedEvent.CreateArguments, "sender")
					event["recipient"] = getFieldValue(c.CreatedEvent.CreateArguments, "recipient")
					event["senderFingerprint"] = getFieldValue(c.CreatedEvent.CreateArguments, "senderFingerprint")
					event["recipientFingerprint"] = getFieldValue(c.CreatedEvent.CreateArguments, "recipientFingerprint")
				}

				events = append(events, event)
			}
		}
	}

	fmt.Printf("\nFound %d audit events:\n", len(events))
	for _, e := range events {
		data, _ := json.MarshalIndent(e, "  ", "  ")
		fmt.Printf("%s\n", data)
	}
}

func getFieldValue(record *lapiv2.Record, fieldName string) interface{} {
	if record == nil {
		return nil
	}
	for _, field := range record.GetFields() {
		if field.GetLabel() == fieldName {
			switch v := field.GetValue().GetSum().(type) {
			case *lapiv2.Value_Party:
				return v.Party
			case *lapiv2.Value_Numeric:
				return v.Numeric
			case *lapiv2.Value_Text:
				return v.Text
			case *lapiv2.Value_Timestamp:
				return time.UnixMicro(v.Timestamp).Format(time.RFC3339)
			case *lapiv2.Value_ContractId:
				return v.ContractId
			}
		}
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
