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
//   - MintEvents (token minting history, unified CIP56.Events)
//   - BurnEvents (token burning history, unified CIP56.Events)
//   - TokenConfig (unified token configuration from CIP56.Config)

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

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
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

// Event represents a token event (unified CIP56.Events)
type Event struct {
	Type           string // MintEvent, BurnEvent
	ContractID     string
	Owner          string
	From           string
	To             string
	Amount         string
	Fingerprint    string // User fingerprint (for tracking user vs issuer holdings)
	EvmTxHash      string // Optional: set for bridge deposits
	EvmDestination string // Optional: set for bridge withdrawals
	CreatedAt      time.Time
	Offset         int64
}

// TokenConfig represents CIP56.Config.TokenConfig
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

	if cip56Pkg == "" {
		fmt.Println("Error: cip56_package_id is required in config")
		os.Exit(1)
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
	fmt.Println()

	// Query holdings
	holdings := queryHoldings(ctx, stateClient, partyID, cip56Pkg, ledgerEnd)

	// Query token configs (unified CIP56.Config.TokenConfig)
	tokenConfigs := queryTokenConfigs(ctx, stateClient, partyID, cip56Pkg, ledgerEnd)

	// Query unified events (all tokens use CIP56.Events)
	mintEvents := queryEvents(ctx, stateClient, partyID, cip56Pkg, "CIP56.Events", "MintEvent", ledgerEnd)
	burnEvents := queryEvents(ctx, stateClient, partyID, cip56Pkg, "CIP56.Events", "BurnEvent", ledgerEnd)

	// Print Token Config
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Token Configuration (CIP56.Config.TokenConfig)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	if len(tokenConfigs) == 0 {
		fmt.Println("  No TokenConfig found")
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

	// Print Unified Events Summary
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Token Events (Unified CIP56.Events)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Mint Events:     %d\n", len(mintEvents))
	fmt.Printf("  Burn Events:     %d\n", len(burnEvents))
	fmt.Println()

	// Print all mint events (both native and bridged use the same MintEvent)
	if len(mintEvents) > 0 {
		fmt.Println("  Mint Events:")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		for _, e := range mintEvents {
			evmNote := ""
			if e.EvmTxHash != "" {
				evmNote = fmt.Sprintf(" (bridge deposit, evmTx: %s...)", e.EvmTxHash[:16])
			}
			fmt.Printf("    %s: %.4f minted to %s%s\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount), extractPartyHint(e.Owner), evmNote)
		}
		fmt.Println()
	}

	// Print all burn events (both native and bridged use the same BurnEvent)
	if len(burnEvents) > 0 {
		fmt.Println("  Burn Events:")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		for _, e := range burnEvents {
			evmNote := ""
			if e.EvmDestination != "" {
				evmNote = fmt.Sprintf(" (bridge withdrawal to %s)", e.EvmDestination)
			}
			fmt.Printf("    %s: %.4f burned from %s%s\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount), extractPartyHint(e.Owner), evmNote)
		}
		fmt.Println()
	}

	// Print PROMPT transfers from database (MetaMask transactions)
	printPromptTransfers(cfg)

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

	// Group events by whether they have evmTxHash (bridge) or not (native)
	var totalNativeMinted, totalBridgeMinted float64
	var totalNativeBurned, totalBridgeBurned float64
	for _, e := range mintEvents {
		amt := parseAmount(e.Amount)
		if e.EvmTxHash != "" {
			totalBridgeMinted += amt
		} else {
			totalNativeMinted += amt
		}
	}
	for _, e := range burnEvents {
		amt := parseAmount(e.Amount)
		if e.EvmDestination != "" {
			totalBridgeBurned += amt
		} else {
			totalNativeBurned += amt
		}
	}

	demoSupply := totalNativeMinted - totalNativeBurned
	promptSupply := totalBridgeMinted - totalBridgeBurned

	fmt.Println("  Token Supply (from unified events):")
	fmt.Printf("    DEMO:       %12.2f  (minted: %.0f, burned: %.0f)\n", demoSupply, totalNativeMinted, totalNativeBurned)
	fmt.Printf("    PROMPT:     %12.2f  (bridged: %.0f, withdrawn: %.0f)\n", promptSupply, totalBridgeMinted, totalBridgeBurned)
	fmt.Println()

	// Get user fingerprints from database to distinguish user vs issuer holdings
	userFingerprints := getUserFingerprints(cfg)

	// Calculate breakdown by user vs issuer
	var demoUserMinted, demoIssuerMinted float64
	for _, e := range mintEvents {
		if e.EvmTxHash != "" {
			continue // skip bridge events for DEMO breakdown
		}
		amt := parseAmount(e.Amount)
		if _, isUser := userFingerprints[e.Fingerprint]; isUser {
			demoUserMinted += amt
		} else {
			demoIssuerMinted += amt
		}
	}

	fmt.Println("  Supply Breakdown:")
	fmt.Println("    DEMO:")
	fmt.Printf("      User Holdings:    %8.2f  (minted to registered EVM wallets)\n", demoUserMinted)
	fmt.Printf("      Issuer Reserve:   %8.2f  (treasury, not mapped to EVM wallets)\n", demoIssuerMinted)
	fmt.Println("    PROMPT:")
	fmt.Printf("      User Holdings:    %8.2f  (bridged from Ethereum)\n", totalBridgeMinted)
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
		SELECT evm_address, prompt_balance::text, demo_balance::text 
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
		Verbose: true,
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

				fm := values.RecordToMap(event.GetCreateArguments())
				h.Issuer = values.Party(fm["issuer"])
				h.Owner = values.Party(fm["owner"])
				h.Amount = values.Numeric(fm["amount"])

				meta := values.DecodeMetadata(fm["meta"])
				h.TokenName = meta["splice.chainsafe.io/name"]
				h.TokenSymbol = meta["splice.chainsafe.io/symbol"]
				h.Decimals = meta["splice.chainsafe.io/decimals"]

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

				fm := values.RecordToMap(event.GetCreateArguments())
				tc.Issuer = values.Party(fm["issuer"])
				tc.ManagerCID = values.ContractID(fm["tokenManagerCid"])

				meta := values.DecodeMetadata(fm["meta"])
				tc.Name = meta["splice.chainsafe.io/name"]
				tc.Symbol = meta["splice.chainsafe.io/symbol"]
				tc.Decimals = meta["splice.chainsafe.io/decimals"]

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
		Verbose: true,
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

				fm := values.RecordToMap(created.GetCreateArguments())

				switch eventType {
				case "MintEvent":
					e.Owner = values.Party(fm["recipient"])
					e.Amount = values.Numeric(fm["amount"])
					e.Fingerprint = values.Text(fm["userFingerprint"])
					if !values.IsNone(fm["evmTxHash"]) {
						if opt, ok := fm["evmTxHash"].Sum.(*lapiv2.Value_Optional); ok && opt.Optional.Value != nil {
							e.EvmTxHash = values.Text(opt.Optional.Value)
						}
					}
				case "BurnEvent":
					e.Owner = values.Party(fm["burnedFrom"])
					e.Amount = values.Numeric(fm["amount"])
					e.Fingerprint = values.Text(fm["userFingerprint"])
					if !values.IsNone(fm["evmDestination"]) {
						if opt, ok := fm["evmDestination"].Sum.(*lapiv2.Value_Optional); ok && opt.Optional.Value != nil {
							e.EvmDestination = values.Text(opt.Optional.Value)
						}
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
