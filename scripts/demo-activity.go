//go:build ignore

// demo-activity.go - Display token holdings on Canton (DEMO and PROMPT)
//
// Usage:
//   go run scripts/demo-activity.go -config config.devnet.yaml           # DevNet
//   go run scripts/demo-activity.go -config .test-config.yaml            # Local Docker
//   go run scripts/demo-activity.go -config config.devnet.yaml -debug    # Show raw contract data
//
// This script shows:
//   - Active CIP56Holdings grouped by token (DEMO, PROMPT) and user
//   - MintEvents (token minting history)
//   - BurnEvents (token burning history)
//   - TransferEvents (token transfer history)
//   - NativeTokenConfig (token configuration)

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
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const maxMessageSize = 50 * 1024 * 1024 // 50MB

var (
	configPath = flag.String("config", "config.devnet.yaml", "Path to config file")
	debug      = flag.Bool("debug", false, "Show debug info about all contracts found")
	showAll    = flag.Bool("all", false, "Show all events (not just recent)")
)

// Token caching
var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
)

// Holding represents a DEMO token holding
type Holding struct {
	ContractID  string
	Owner       string
	Issuer      string
	Amount      string
	TokenName   string
	TokenSymbol string
	Decimals    string
	CreatedAt   time.Time
	Offset      int64
}

// Event represents a token event
type Event struct {
	Type        string // MintEvent, BurnEvent, TransferEvent
	ContractID  string
	Owner       string
	From        string
	To          string
	Amount      string
	Fingerprint string // User fingerprint (for tracking user vs issuer holdings)
	CreatedAt   time.Time
	Offset      int64
}

// TokenConfig represents NativeTokenConfig
type TokenConfig struct {
	ContractID string
	Issuer     string
	Name       string
	Symbol     string
	Decimals   string
	ManagerCID string
	CreatedAt  time.Time
}

func main() {
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	partyID := cfg.Canton.RelayerParty
	if partyID == "" {
		fmt.Println("Error: canton.relayer_party is required in config")
		os.Exit(1)
	}

	cip56Pkg := cfg.Canton.CIP56PackageID
	nativePkg := cfg.Canton.NativeTokenPackageID
	corePkg := cfg.Canton.CorePackageID // Bridge events are in core package

	if cip56Pkg == "" || nativePkg == "" {
		fmt.Println("Error: cip56_package_id and native_token_package_id are required in config")
		os.Exit(1)
	}

	if corePkg == "" {
		fmt.Println("Warning: core_package_id not set, PROMPT events won't be shown")
	}

	// Connect to Canton
	conn, err := connectToCanton(cfg)
	if err != nil {
		fmt.Printf("Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()
	if cfg.Canton.Auth.TokenURL != "" {
		token, err := getOAuthToken(cfg)
		if err != nil {
			fmt.Printf("Failed to get OAuth token: %v\n", err)
			os.Exit(1)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}

	stateClient := lapiv2.NewStateServiceClient(conn)

	// Get ledger end
	endResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	ledgerEnd := endResp.Offset

	// Print header
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Canton Token Activity (DEMO + PROMPT)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Canton RPC:     %s\n", cfg.Canton.RPCURL)
	fmt.Printf("  Party:          %s...\n", partyID[:50])
	fmt.Printf("  Ledger Offset:  %d\n", ledgerEnd)
	fmt.Printf("  CIP56 Package:  %s...\n", cip56Pkg[:16])
	fmt.Printf("  Native Package: %s...\n", nativePkg[:16])
	if corePkg != "" {
		fmt.Printf("  Core Package:   %s...\n", corePkg[:16])
	}
	fmt.Println()

	// Query holdings
	holdings := queryHoldings(ctx, stateClient, partyID, cip56Pkg, ledgerEnd)

	// Query token config
	tokenConfigs := queryTokenConfigs(ctx, stateClient, partyID, nativePkg, ledgerEnd)

	// Query DEMO events (native token)
	mintEvents := queryEvents(ctx, stateClient, partyID, nativePkg, "Native.Events", "MintEvent", ledgerEnd)
	burnEvents := queryEvents(ctx, stateClient, partyID, nativePkg, "Native.Events", "BurnEvent", ledgerEnd)
	transferEvents := queryEvents(ctx, stateClient, partyID, nativePkg, "Native.Events", "TransferEvent", ledgerEnd)

	// Query PROMPT events (bridged token) - bridge events are in core package
	var bridgeMintEvents, bridgeBurnEvents []Event
	if corePkg != "" {
		bridgeMintEvents = queryEvents(ctx, stateClient, partyID, corePkg, "Bridge.Events", "BridgeMintEvent", ledgerEnd)
		bridgeBurnEvents = queryEvents(ctx, stateClient, partyID, corePkg, "Bridge.Events", "BridgeBurnEvent", ledgerEnd)
	}

	// Print Token Config
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Native Token Configuration")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	if len(tokenConfigs) == 0 {
		fmt.Println("  No NativeTokenConfig found")
	} else {
		for _, tc := range tokenConfigs {
			fmt.Printf("  Token:      %s (%s)\n", tc.Name, tc.Symbol)
			fmt.Printf("  Decimals:   %s\n", tc.Decimals)
			fmt.Printf("  Contract:   %s...\n", tc.ContractID[:50])
			fmt.Printf("  Created:    %s\n", tc.CreatedAt.Format(time.RFC3339))
			fmt.Println()
		}
	}

	// Print Active Holdings (on-chain state)
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Active CIP56Holdings (On-Chain State)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Group holdings by owner
	holdingsByOwner := make(map[string][]Holding)
	for _, h := range holdings {
		holdingsByOwner[h.Owner] = append(holdingsByOwner[h.Owner], h)
	}

	if len(holdings) > 0 {
		fmt.Printf("  Total Holdings: %d contracts\n\n", len(holdings))
		for owner, ownerHoldings := range holdingsByOwner {
			ownerHint := extractPartyHint(owner)
			var totalAmount float64
			for _, h := range ownerHoldings {
				totalAmount += parseAmount(h.Amount)
			}
			fmt.Printf("  %s (%s...):\n", ownerHint, truncateParty(owner))
			// Group holdings by token symbol
			tokenAmounts := make(map[string]float64)
			tokenHoldings := make(map[string][]Holding)
			for _, h := range ownerHoldings {
				symbol := h.TokenSymbol
				if symbol == "" {
					symbol = "UNKNOWN"
				}
				tokenAmounts[symbol] += parseAmount(h.Amount)
				tokenHoldings[symbol] = append(tokenHoldings[symbol], h)
			}
			fmt.Printf("    Holdings: %d contract(s)\n", len(ownerHoldings))
			for symbol, total := range tokenAmounts {
				fmt.Printf("      %s: %.2f\n", symbol, total)
			}
			// Show first 3 holdings per owner
			shown := 0
			for symbol, hList := range tokenHoldings {
				for _, h := range hList {
					if shown < 3 {
						fmt.Printf("      - %.2f %s (cid: %s...)\n", parseAmount(h.Amount), symbol, h.ContractID[:20])
						shown++
					}
				}
			}
			if len(ownerHoldings) > 3 {
				fmt.Printf("      ... and %d more\n", len(ownerHoldings)-3)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("  No active holdings found")
		fmt.Println()
	}

	// Print DEMO Events Summary
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  DEMO Token Events (Native)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Mint Events:     %d\n", len(mintEvents))
	fmt.Printf("  Burn Events:     %d\n", len(burnEvents))
	fmt.Printf("  Transfer Events: %d\n", len(transferEvents))
	fmt.Println()

	// Print all DEMO events
	if len(mintEvents) > 0 {
		fmt.Println("  Mint Events:")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		for _, e := range mintEvents {
			fmt.Printf("    %s: %.2f DEMO minted\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount))
		}
		fmt.Println()
	}

	if len(transferEvents) > 0 {
		fmt.Println("  Transfer Events (Custodial Key Transfers):")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		for _, e := range transferEvents {
			// Extract user-friendly party hints from the full party IDs
			fromHint := extractPartyHint(e.From)
			toHint := extractPartyHint(e.To)
			fmt.Printf("    %s: %.2f DEMO\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount))
			fmt.Printf("      From: %s (Canton party: %s...)\n", fromHint, truncateParty(e.From))
			fmt.Printf("      To:   %s (Canton party: %s...)\n", toHint, truncateParty(e.To))
			fmt.Printf("      Flow: EVM sig → API Server → Custodial Key → CIP56Holding.Transfer\n")
		}
		fmt.Println()

		// Show custodial key flow explanation
		fmt.Println("  Custodial Transfer Flow:")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		fmt.Println("    1. User signs ERC-20 transfer with EVM private key (MetaMask)")
		fmt.Println("    2. API Server receives eth_sendRawTransaction")
		fmt.Println("    3. Server verifies EVM signature → identifies user")
		fmt.Println("    4. Server retrieves user's encrypted Canton private key from DB")
		fmt.Println("    5. Server decrypts key with master key (AES-256-GCM)")
		fmt.Println("    6. Server signs Canton command as user's party")
		fmt.Println("    7. CIP56Holding.Transfer choice exercised (owner-controlled)")
		fmt.Println("    8. New holding created for recipient, sender's holding updated")
		fmt.Println()
	}

	// Print PROMPT Events Summary (bridged token)
	if corePkg != "" {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  PROMPT Token Events (Bridged)")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		fmt.Printf("  Bridge Mint Events: %d\n", len(bridgeMintEvents))
		fmt.Printf("  Bridge Burn Events: %d\n", len(bridgeBurnEvents))
		fmt.Println()

		// Print recent bridge mint events
		if len(bridgeMintEvents) > 0 {
			fmt.Println("  Recent Bridge Deposits (Ethereum → Canton):")
			fmt.Println("  ─────────────────────────────────────────────────────────────────")
			limit := 5
			if len(bridgeMintEvents) < limit {
				limit = len(bridgeMintEvents)
			}
			for i := 0; i < limit; i++ {
				e := bridgeMintEvents[i]
				fmt.Printf("    %s: %.4f PROMPT bridged in\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount))
			}
			fmt.Println()
		}

		// Print recent bridge burn events
		if len(bridgeBurnEvents) > 0 {
			fmt.Println("  Recent Bridge Withdrawals (Canton → Ethereum):")
			fmt.Println("  ─────────────────────────────────────────────────────────────────")
			limit := 5
			if len(bridgeBurnEvents) < limit {
				limit = len(bridgeBurnEvents)
			}
			for i := 0; i < limit; i++ {
				e := bridgeBurnEvents[i]
				fmt.Printf("    %s: %.4f PROMPT withdrawn\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount))
			}
			fmt.Println()
		}

		// Print PROMPT transfers from database (MetaMask transactions)
		printPromptTransfers(cfg)
	}

	// Print summary
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Summary")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Calculate totals by token
	tokenTotals := make(map[string]float64)
	for _, h := range holdings {
		symbol := h.TokenSymbol
		if symbol == "" {
			symbol = "UNKNOWN"
		}
		tokenTotals[symbol] += parseAmount(h.Amount)
	}

	var totalMinted float64
	for _, e := range mintEvents {
		totalMinted += parseAmount(e.Amount)
	}

	var totalBurned float64
	for _, e := range burnEvents {
		totalBurned += parseAmount(e.Amount)
	}

	// Calculate PROMPT totals from bridge events
	var totalBridged float64
	var totalWithdrawn float64
	if corePkg != "" {
		for _, e := range bridgeMintEvents {
			totalBridged += parseAmount(e.Amount)
		}
		for _, e := range bridgeBurnEvents {
			totalWithdrawn += parseAmount(e.Amount)
		}
	}

	// Display reconciled totals based on events (source of truth)
	demoSupply := totalMinted - totalBurned
	promptSupply := totalBridged - totalWithdrawn

	fmt.Println("  Token Supply (from events):")
	fmt.Printf("    DEMO:       %12.2f  (minted: %.0f, burned: %.0f)\n", demoSupply, totalMinted, totalBurned)
	fmt.Printf("    PROMPT:     %12.2f  (bridged: %.0f, withdrawn: %.0f)\n", promptSupply, totalBridged, totalWithdrawn)
	fmt.Println()

	// Get user fingerprints from database to distinguish user vs issuer holdings
	userFingerprints := getUserFingerprints(cfg)

	// Calculate DEMO breakdown by user vs issuer
	var demoUserMinted, demoIssuerMinted float64
	for _, e := range mintEvents {
		amt := parseAmount(e.Amount)
		if _, isUser := userFingerprints[e.Fingerprint]; isUser {
			demoUserMinted += amt
		} else {
			demoIssuerMinted += amt
		}
	}

	// Calculate PROMPT breakdown (all should be user since bridge requires EVM address)
	var promptUserBridged float64
	for _, e := range bridgeMintEvents {
		promptUserBridged += parseAmount(e.Amount)
	}

	fmt.Println("  Supply Breakdown:")
	fmt.Println("    DEMO:")
	fmt.Printf("      User Holdings:    %8.2f  (minted to registered EVM wallets)\n", demoUserMinted)
	fmt.Printf("      Issuer Reserve:   %8.2f  (treasury, not mapped to EVM wallets)\n", demoIssuerMinted)
	fmt.Println("    PROMPT:")
	fmt.Printf("      User Holdings:    %8.2f  (bridged from Ethereum)\n", promptUserBridged)
	fmt.Println()

	// Query database for user-facing balances (what MetaMask shows)
	printUserBalances(cfg)
}

func printUserBalances(cfg *config.Config) {
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  User Balances (MetaMask View)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Use erc20_api database (where user balances are stored)
	// Override database name since relayer config uses 'relayer' db
	dbName := cfg.Database.Database
	if dbName == "relayer" {
		dbName = "erc20_api"
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, dbName, cfg.Database.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf("  Database connection failed: %v\n", err)
		fmt.Println("  (Run with database access to see user balances)")
		fmt.Println()
		return
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT evm_address, balance::text, demo_balance::text 
		FROM users 
		ORDER BY evm_address
	`)
	if err != nil {
		fmt.Printf("  Database query failed: %v\n", err)
		fmt.Println()
		return
	}
	defer rows.Close()

	fmt.Printf("  %-44s  %12s  %12s\n", "EVM ADDRESS", "PROMPT", "DEMO")
	fmt.Println("  ────────────────────────────────────────────────────────────────────")

	var totalPrompt, totalDemo float64
	userCount := 0

	for rows.Next() {
		var addr string
		var promptStr, demoStr string
		if err := rows.Scan(&addr, &promptStr, &demoStr); err != nil {
			fmt.Printf("  Scan error: %v\n", err)
			continue
		}

		prompt := parseAmount(promptStr)
		demo := parseAmount(demoStr)

		userCount++
		totalPrompt += prompt
		totalDemo += demo

		// Format address for display
		displayAddr := addr
		if len(addr) > 20 {
			displayAddr = addr[:6] + "..." + addr[len(addr)-4:]
		}

		fmt.Printf("  %-44s  %12.2f  %12.2f\n", displayAddr, prompt, demo)
	}

	fmt.Println("  ────────────────────────────────────────────────────────────────────")
	fmt.Printf("  %-44s  %12.2f  %12.2f\n", fmt.Sprintf("TOTAL (%d users)", userCount), totalPrompt, totalDemo)
	fmt.Println()
}

// printPromptTransfers queries the database for PROMPT transfers via MetaMask
func printPromptTransfers(cfg *config.Config) {
	dbName := cfg.Database.Database
	if dbName == "relayer" {
		dbName = "erc20_api"
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, dbName, cfg.Database.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return
	}
	defer db.Close()

	// PROMPT token address (lowercase for comparison)
	promptToken := "0x90cb4f9ef6d682f4338f0e360b9c079fbb32048e"

	// Query transfers to the PROMPT token contract
	rows, err := db.Query(`
		SELECT from_address, input, created_at 
		FROM evm_transactions 
		WHERE LOWER(to_address) = $1 AND status = 1
		ORDER BY created_at DESC
	`, promptToken)
	if err != nil {
		return
	}
	defer rows.Close()

	type Transfer struct {
		From      string
		Amount    float64
		CreatedAt time.Time
	}

	var transfers []Transfer
	for rows.Next() {
		var from string
		var data []byte
		var createdAt time.Time
		if err := rows.Scan(&from, &data, &createdAt); err != nil {
			continue
		}

		// Parse transfer amount from ERC-20 transfer call data
		// transfer(address,uint256) = 0xa9059cbb + address (32 bytes) + amount (32 bytes)
		// We need bytes 36-68 for the amount (after 4 byte selector + 32 byte address)
		if len(data) >= 68 {
			// Amount is in the last 32 bytes of standard transfer call
			amountBytes := data[36:68]
			// Convert to float (amount is in wei, divide by 10^18)
			var amount float64
			for _, b := range amountBytes {
				amount = amount*256 + float64(b)
			}
			amount = amount / 1e18

			transfers = append(transfers, Transfer{
				From:      from,
				Amount:    amount,
				CreatedAt: createdAt,
			})
		}
	}

	if len(transfers) > 0 {
		fmt.Println("  Transfers (from database):")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		for _, t := range transfers {
			fromDisplay := t.From
			if len(fromDisplay) > 12 {
				fromDisplay = t.From[:6] + "..." + t.From[len(t.From)-4:]
			}
			fmt.Printf("    %s: %.2f PROMPT from %s\n", t.CreatedAt.Format("2006-01-02 15:04:05"), t.Amount, fromDisplay)
		}
		fmt.Println()
	}
}

// getUserFingerprints returns a map of registered user fingerprints
func getUserFingerprints(cfg *config.Config) map[string]string {
	result := make(map[string]string)

	dbName := cfg.Database.Database
	if dbName == "relayer" {
		dbName = "erc20_api"
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, dbName, cfg.Database.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return result
	}
	defer db.Close()

	rows, err := db.Query("SELECT fingerprint, evm_address FROM users")
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var fp, addr string
		if err := rows.Scan(&fp, &addr); err == nil {
			result[fp] = addr
		}
	}

	return result
}

func parseAmount(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// extractPartyHint extracts the hint prefix from a Canton party ID (e.g., "user_FCAd0B19" from "user_FCAd0B19::1220...")
func extractPartyHint(partyID string) string {
	if partyID == "" {
		return "unknown"
	}
	parts := strings.Split(partyID, "::")
	if len(parts) > 0 {
		return parts[0]
	}
	return partyID
}

// truncateParty returns a truncated version of the party ID for display
func truncateParty(partyID string) string {
	if len(partyID) > 20 {
		return partyID[:20]
	}
	return partyID
}

func connectToCanton(cfg *config.Config) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	opts = append(opts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxMessageSize),
	))

	return grpc.Dial(cfg.Canton.RPCURL, opts...)
}

func getOAuthToken(cfg *config.Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	payload := map[string]string{
		"client_id":     cfg.Canton.Auth.ClientID,
		"client_secret": cfg.Canton.Auth.ClientSecret,
		"audience":      cfg.Canton.Auth.Audience,
		"grant_type":    "client_credentials",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(cfg.Canton.Auth.TokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OAuth error: %s", string(data))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}

	cachedToken = result.AccessToken
	tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)

	// Extract subject from JWT for debugging
	token, _, _ := new(jwt.Parser).ParseUnverified(cachedToken, jwt.MapClaims{})
	if token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if sub, ok := claims["sub"].(string); ok {
				fmt.Printf("  JWT Subject:    %s\n", sub)
			}
		}
	}

	return cachedToken, nil
}

func queryHoldings(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) []Holding {
	filter := &lapiv2.EventFormat{
		FiltersByParty: map[string]*lapiv2.Filters{
			party: {
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  packageID,
									ModuleName: "CIP56.Token",
									EntityName: "CIP56Holding",
								},
							},
						},
					},
				},
			},
		},
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat:    filter,
	})
	if err != nil {
		fmt.Printf("  Error querying holdings: %v\n", err)
		return nil
	}

	var holdings []Holding
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("  Error receiving holdings: %v\n", err)
			break
		}

		if resp.GetActiveContract() != nil {
			event := resp.GetActiveContract().GetCreatedEvent()
			if event != nil {
				h := Holding{
					ContractID: event.ContractId,
					Offset:     event.Offset,
				}

				// Parse create arguments
				fields := event.GetCreateArguments().GetFields()
				if len(fields) >= 4 {
					h.Issuer = fields[0].GetValue().GetParty()
					h.Owner = fields[1].GetValue().GetParty()
					h.Amount = fields[2].GetValue().GetNumeric()

					// Token info is in field 3
					tokenFields := fields[3].GetValue().GetRecord().GetFields()
					if len(tokenFields) >= 3 {
						h.TokenName = tokenFields[0].GetValue().GetText()
						h.TokenSymbol = tokenFields[1].GetValue().GetText()
						h.Decimals = fmt.Sprintf("%d", tokenFields[2].GetValue().GetInt64())
					}
				}

				if event.CreatedAt != nil {
					h.CreatedAt = event.CreatedAt.AsTime()
				}

				holdings = append(holdings, h)
			}
		}
	}

	// Sort by created time descending
	sort.Slice(holdings, func(i, j int) bool {
		return holdings[i].CreatedAt.After(holdings[j].CreatedAt)
	})

	return holdings
}

func queryTokenConfigs(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) []TokenConfig {
	filter := &lapiv2.EventFormat{
		FiltersByParty: map[string]*lapiv2.Filters{
			party: {
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  packageID,
									ModuleName: "Native.Token",
									EntityName: "NativeTokenConfig",
								},
							},
						},
					},
				},
			},
		},
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat:    filter,
	})
	if err != nil {
		return nil
	}

	var configs []TokenConfig
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		if resp.GetActiveContract() != nil {
			event := resp.GetActiveContract().GetCreatedEvent()
			if event != nil {
				tc := TokenConfig{
					ContractID: event.ContractId,
				}

				fields := event.GetCreateArguments().GetFields()
				if len(fields) >= 4 {
					tc.Issuer = fields[0].GetValue().GetParty()

					tokenFields := fields[1].GetValue().GetRecord().GetFields()
					if len(tokenFields) >= 3 {
						tc.Name = tokenFields[0].GetValue().GetText()
						tc.Symbol = tokenFields[1].GetValue().GetText()
						tc.Decimals = fmt.Sprintf("%d", tokenFields[2].GetValue().GetInt64())
					}

					tc.ManagerCID = fields[2].GetValue().GetContractId()
				}

				if event.CreatedAt != nil {
					tc.CreatedAt = event.CreatedAt.AsTime()
				}

				configs = append(configs, tc)
			}
		}
	}

	return configs
}

func queryEvents(ctx context.Context, client lapiv2.StateServiceClient, party, packageID, moduleName, eventType string, offset int64) []Event {
	filter := &lapiv2.EventFormat{
		FiltersByParty: map[string]*lapiv2.Filters{
			party: {
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  packageID,
									ModuleName: moduleName,
									EntityName: eventType,
								},
							},
						},
					},
				},
			},
		},
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat:    filter,
	})
	if err != nil {
		return nil
	}

	var events []Event
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		if resp.GetActiveContract() != nil {
			created := resp.GetActiveContract().GetCreatedEvent()
			if created != nil {
				e := Event{
					Type:       eventType,
					ContractID: created.ContractId,
					Offset:     created.Offset,
				}

				fields := created.GetCreateArguments().GetFields()

				// Parse based on event type
				// Native.Events:
				//   MintEvent: issuer, recipient, amount, holdingCid, tokenSymbol, timestamp, auditObservers, userFingerprint
				//   BurnEvent: issuer, owner, amount, holdingCid, tokenSymbol, timestamp, auditObservers, userFingerprint
				//   TransferEvent: issuer, sender, recipient, amount, senderRemainderCid, recipientHoldingCid, tokenSymbol, timestamp, auditObservers, senderFingerprint, recipientFingerprint
				// Bridge.Events:
				//   BridgeMintEvent: issuer, recipient, amount, holdingCid, tokenSymbol, evmTxHash, fingerprint, timestamp, auditObservers
				//   BridgeBurnEvent: issuer, burnedFrom, amount, remainderCid, evmDestination, tokenSymbol, fingerprint, timestamp, auditObservers
				switch eventType {
				case "MintEvent":
					if len(fields) >= 3 {
						e.Owner = fields[1].GetValue().GetParty() // recipient
						e.Amount = fields[2].GetValue().GetNumeric()
					}
					if len(fields) >= 8 {
						e.Fingerprint = fields[7].GetValue().GetText() // userFingerprint
					}
				case "BurnEvent":
					if len(fields) >= 3 {
						e.Owner = fields[1].GetValue().GetParty() // owner
						e.Amount = fields[2].GetValue().GetNumeric()
					}
					if len(fields) >= 8 {
						e.Fingerprint = fields[7].GetValue().GetText() // userFingerprint
					}
				case "TransferEvent":
					// TransferEvent: issuer(0), sender(1), recipient(2), amount(3), ...
					if len(fields) >= 4 {
						e.From = fields[1].GetValue().GetParty()     // sender
						e.To = fields[2].GetValue().GetParty()       // recipient
						e.Amount = fields[3].GetValue().GetNumeric() // amount
					}
				case "BridgeMintEvent":
					if len(fields) >= 3 {
						e.Owner = fields[1].GetValue().GetParty() // recipient
						e.Amount = fields[2].GetValue().GetNumeric()
					}
					if len(fields) >= 7 {
						e.Fingerprint = fields[6].GetValue().GetText() // fingerprint
					}
				case "BridgeBurnEvent":
					if len(fields) >= 3 {
						e.Owner = fields[1].GetValue().GetParty() // burnedFrom
						e.Amount = fields[2].GetValue().GetNumeric()
					}
					if len(fields) >= 7 {
						e.Fingerprint = fields[6].GetValue().GetText() // fingerprint
					}
				}

				if created.CreatedAt != nil {
					e.CreatedAt = created.CreatedAt.AsTime()
				}

				events = append(events, e)
			}
		}
	}

	// Sort by created time descending
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})

	return events
}
