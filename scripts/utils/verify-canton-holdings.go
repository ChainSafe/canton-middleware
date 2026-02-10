//go:build ignore
// +build ignore

// Verify Canton Holdings Script
//
// This script queries CIP56Holding contracts directly from Canton via gRPC,
// and compares them with the API server's database cache to verify consistency.
//
// Usage:
//   go run scripts/verify-canton-holdings.go -config config.e2e-local.yaml
//   go run scripts/verify-canton-holdings.go -config config.e2e-local.yaml -party <party_id>
//   go run scripts/verify-canton-holdings.go -config config.e2e-local.yaml -compare
//
// Options:
//   -party    Filter by party ID (partial match)
//   -compare  Compare Canton holdings with database cache
//   -verbose  Show detailed contract information

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	configPath  = flag.String("config", "config.e2e-local.yaml", "Path to config file")
	partyFilter = flag.String("party", "", "Filter by party ID (partial match)")
	compare     = flag.Bool("compare", false, "Compare Canton holdings with database")
	verbose     = flag.Bool("verbose", false, "Show detailed contract information")
)

func main() {
	flag.Parse()

	// Load config first to determine network
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Determine network from config
	networkName := detectNetwork(cfg.Canton.RPCURL)

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton Holdings Verification - %s\n", networkName)
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Config:    %s\n", *configPath)
	fmt.Printf("  Network:   %s\n", networkName)
	fmt.Printf("  Endpoint:  %s\n", cfg.Canton.RPCURL)
	fmt.Printf("  Domain:    %s\n", truncateDomain(cfg.Canton.DomainID))
	fmt.Println()

	// Create logger (quiet mode)
	logConfig := zap.NewProductionConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	logger, _ := logConfig.Build()

	// Connect to Canton
	fmt.Println(">>> Connecting to Canton...")
	cantonClient, err := canton.NewClient(&cfg.Canton, logger)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer cantonClient.Close()
	fmt.Printf("    ✓ Connected to %s\n", networkName)
	fmt.Println()

	ctx := context.Background()

	// Query all holdings from Canton
	fmt.Println(">>> Querying CIP56Holding contracts from Canton...")
	holdings, err := cantonClient.GetAllCIP56Holdings(ctx)
	if err != nil {
		fmt.Printf("ERROR: Failed to query holdings: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    Found %d CIP56Holding contract(s)\n", len(holdings))
	fmt.Println()

	// Filter by party if specified
	if *partyFilter != "" {
		var filtered []*canton.CIP56Holding
		for _, h := range holdings {
			if strings.Contains(h.Owner, *partyFilter) {
				filtered = append(filtered, h)
			}
		}
		holdings = filtered
		fmt.Printf("    Filtered to %d holding(s) matching: %s\n", len(holdings), *partyFilter)
		fmt.Println()
	}

	if len(holdings) == 0 {
		fmt.Println("No holdings found.")
		return
	}

	// Separate DEMO holdings from others
	var demoHoldings []*canton.CIP56Holding
	var otherHoldings []*canton.CIP56Holding
	for _, h := range holdings {
		if h.Symbol == "DEMO" {
			demoHoldings = append(demoHoldings, h)
		} else {
			otherHoldings = append(otherHoldings, h)
		}
	}

	// Show raw DEMO contracts first
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  DEMO Token Holdings (Raw Contracts)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	if len(demoHoldings) == 0 {
		fmt.Println("  No DEMO holdings found.")
	} else {
		fmt.Printf("  Found %d DEMO CIP56Holding contract(s):\n", len(demoHoldings))
		fmt.Println()
		for i, h := range demoHoldings {
			ownerDisplay := h.Owner
			if len(ownerDisplay) > 60 {
				ownerDisplay = ownerDisplay[:57] + "..."
			}
			contractDisplay := h.ContractID
			if len(contractDisplay) > 40 {
				contractDisplay = contractDisplay[:37] + "..."
			}
			fmt.Printf("  [%d] %s DEMO\n", i+1, formatBalance(h.Amount))
			fmt.Printf("      Owner:    %s\n", ownerDisplay)
			fmt.Printf("      Contract: %s\n", contractDisplay)
			fmt.Println()
		}
	}

	// Group holdings by owner and symbol for summary
	type BalanceKey struct {
		Owner  string
		Symbol string
	}
	balances := make(map[BalanceKey]decimal.Decimal)
	holdingCounts := make(map[BalanceKey]int)

	for _, h := range holdings {
		key := BalanceKey{Owner: h.Owner, Symbol: h.Symbol}
		if key.Symbol == "" {
			key.Symbol = "UNKNOWN"
		}
		amount, _ := decimal.NewFromString(h.Amount)
		balances[key] = balances[key].Add(amount)
		holdingCounts[key]++
	}

	// Display summary
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Summary (Aggregated by Owner)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Owner                                              | Symbol  | Balance    | Holdings")
	fmt.Println("---------------------------------------------------|---------|------------|----------")

	// Show DEMO balances first
	for key, balance := range balances {
		if key.Symbol != "DEMO" {
			continue
		}
		ownerDisplay := key.Owner
		if len(ownerDisplay) > 50 {
			ownerDisplay = ownerDisplay[:47] + "..."
		}
		fmt.Printf("%-50s | %-7s | %10s | %d\n", ownerDisplay, key.Symbol, formatBalance(balance.String()), holdingCounts[key])
	}

	// Show other tokens
	for key, balance := range balances {
		if key.Symbol == "DEMO" {
			continue
		}
		ownerDisplay := key.Owner
		if len(ownerDisplay) > 50 {
			ownerDisplay = ownerDisplay[:47] + "..."
		}
		fmt.Printf("%-50s | %-7s | %10s | %d\n", ownerDisplay, key.Symbol, formatBalance(balance.String()), holdingCounts[key])
	}
	fmt.Println()

	// Show verbose details for non-DEMO if requested
	if *verbose && len(otherHoldings) > 0 {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Other Token Contracts (Verbose)")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		for _, h := range otherHoldings {
			fmt.Printf("Contract ID: %s\n", h.ContractID[:40]+"...")
			fmt.Printf("  Owner:  %s\n", h.Owner)
			fmt.Printf("  Symbol: %s\n", h.Symbol)
			fmt.Printf("  Amount: %s\n", h.Amount)
			fmt.Println()
		}
	}

	// Compare with database if requested
	if *compare {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Database Comparison")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()

		// Connect to database
		fmt.Println(">>> Connecting to database...")
		db, err := apidb.NewStore(cfg.Database.GetConnectionString())
		if err != nil {
			fmt.Printf("ERROR: Failed to connect to database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		// Get all users
		users, err := db.GetAllUsers()
		if err != nil {
			fmt.Printf("ERROR: Failed to get users: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("User                                               | Token   | Canton    | Database  | Match")
		fmt.Println("---------------------------------------------------|---------|-----------|-----------|------")

		var mismatches int
		for _, user := range users {
			if user.CantonPartyID == "" {
				continue
			}

			// Get Canton balance for this user
			demoKey := BalanceKey{Owner: user.CantonPartyID, Symbol: "DEMO"}
			promptKey := BalanceKey{Owner: user.CantonPartyID, Symbol: "PROMPT"}

			cantonDemo := balances[demoKey]
			cantonPrompt := balances[promptKey]

			dbDemo, _ := decimal.NewFromString(user.DemoBalance)
			dbPrompt, _ := decimal.NewFromString(user.PromptBalance)

			// Compare DEMO
			demoMatch := cantonDemo.Equal(dbDemo)
			matchIcon := "✓"
			if !demoMatch {
				matchIcon = "✗"
				mismatches++
			}

			userDisplay := user.EVMAddress
			if len(userDisplay) > 50 {
				userDisplay = userDisplay[:47] + "..."
			}

			fmt.Printf("%-50s | %-7s | %9s | %9s | %s\n",
				userDisplay, "DEMO",
				formatBalance(cantonDemo.String()),
				formatBalance(dbDemo.String()),
				matchIcon)

			// Compare PROMPT
			promptMatch := cantonPrompt.Equal(dbPrompt)
			matchIcon = "✓"
			if !promptMatch {
				matchIcon = "✗"
				mismatches++
			}

			fmt.Printf("%-50s | %-7s | %9s | %9s | %s\n",
				"", "PROMPT",
				formatBalance(cantonPrompt.String()),
				formatBalance(dbPrompt.String()),
				matchIcon)
		}

		fmt.Println()
		if mismatches == 0 {
			fmt.Println("✓ All balances match! Canton ledger and database are consistent.")
		} else {
			fmt.Printf("✗ Found %d mismatches. Run reconciliation to sync.\n", mismatches)
		}
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Verification Complete")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
}

func formatBalance(bal string) string {
	if bal == "" || bal == "0" {
		return "0"
	}
	// Truncate to 2 decimal places for display
	if idx := strings.Index(bal, "."); idx != -1 {
		end := idx + 3
		if end > len(bal) {
			end = len(bal)
		}
		return bal[:end]
	}
	return bal
}

func detectNetwork(rpcURL string) string {
	switch {
	case strings.Contains(rpcURL, "prod1"):
		return "MAINNET (ChainSafe Production)"
	case strings.Contains(rpcURL, "staging"):
		return "DEVNET (ChainSafe Staging)"
	case strings.Contains(rpcURL, "localhost") || strings.Contains(rpcURL, "127.0.0.1"):
		return "LOCAL (Docker)"
	default:
		return "UNKNOWN"
	}
}

func truncateDomain(domain string) string {
	if len(domain) > 50 {
		return domain[:47] + "..."
	}
	return domain
}
