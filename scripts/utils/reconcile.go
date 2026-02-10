//go:build ignore
// +build ignore

// Reconcile Script
//
// Synchronize the API server database with Canton ledger holdings.
// This updates user balances to match their actual Canton holdings.
//
// Usage:
//   go run scripts/utils/reconcile.go -config config.api-server.mainnet.local.yaml

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
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.mainnet.local.yaml", "Path to config file")
	verbose    = flag.Bool("verbose", false, "Show detailed output")
)

func main() {
	flag.Parse()

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	networkName := detectNetwork(cfg.Canton.RPCURL)

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Balance Reconciliation - %s\n", networkName)
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Config:   %s\n", *configPath)
	fmt.Printf("  Network:  %s\n", networkName)
	fmt.Printf("  Database: %s\n", cfg.Database.Database)
	fmt.Println()

	// Create logger
	var logger *zap.Logger
	if *verbose {
		logger, _ = zap.NewDevelopment()
	} else {
		logConfig := zap.NewProductionConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
		logger, _ = logConfig.Build()
	}

	// Connect to Canton
	fmt.Println(">>> Connecting to Canton...")
	cantonClient, err := canton.NewClient(&cfg.Canton, logger)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer cantonClient.Close()
	fmt.Printf("    ✓ Connected to %s\n", networkName)

	// Connect to database
	fmt.Println(">>> Connecting to database...")
	db, err := apidb.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	fmt.Println("    ✓ Connected")
	fmt.Println()

	ctx := context.Background()

	// Show balances before reconciliation
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Balances Before Reconciliation")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	showBalances(db)

	// Run reconciliation
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Running Reconciliation")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println(">>> Querying Canton for current holdings...")
	fmt.Println(">>> Updating database balances...")

	reconciler := apidb.NewReconciler(db, cantonClient, logger)
	if err := reconciler.FullBalanceReconciliation(ctx); err != nil {
		fmt.Printf("ERROR: Reconciliation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("    ✓ Reconciliation complete")
	fmt.Println()

	// Show balances after reconciliation
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Balances After Reconciliation")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	showBalances(db)

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  ✓ Reconciliation Successful!")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  Database balances now match Canton ledger holdings.")
	fmt.Println("  MetaMask will show updated balances on next refresh.")
	fmt.Println()
}

func showBalances(db *apidb.Store) {
	users, err := db.GetAllUsers()
	if err != nil {
		fmt.Printf("    (Failed to get users: %v)\n", err)
		return
	}

	if len(users) == 0 {
		fmt.Println("    No registered users found.")
		return
	}

	for _, u := range users {
		hint := extractHint(u.CantonPartyID)
		fmt.Printf("  [%s]\n", hint)
		fmt.Printf("    EVM:     %s\n", u.EVMAddress)
		fmt.Printf("    Balance: %s DEMO\n", formatBalance(u.DemoBalance))
		fmt.Println()
	}
}

func extractHint(partyID string) string {
	if idx := strings.Index(partyID, "::"); idx != -1 {
		return partyID[:idx]
	}
	return partyID
}

func formatBalance(bal string) string {
	if bal == "" || bal == "0" {
		return "0"
	}
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
