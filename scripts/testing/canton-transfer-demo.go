//go:build ignore
// +build ignore

// Canton Transfer Demo Script
//
// This script demonstrates the balance reconciliation feature by:
// 1. Showing current balances from database
// 2. Performing a transfer directly via Canton Ledger API (simulating native Canton user)
// 3. Triggering reconciliation
// 4. Showing updated balances
//
// This proves that the API server can detect and sync balance changes made
// directly on the Canton ledger, enabling interoperability between MetaMask
// and native Canton users.
//
// Usage:
//   go run scripts/canton-transfer-demo.go -config config.e2e-local.yaml
//
// Prerequisites:
//   - Bootstrap completed (DEMO tokens distributed)
//   - Native user registered (run register-native-user.go first)

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	canton "github.com/chainsafe/canton-middleware/pkg/canton-sdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var (
	configPath    = flag.String("config", "config.e2e-local.yaml", "Path to config file")
	fromParty     = flag.String("from", "", "Sender's Canton party ID (required)")
	toParty       = flag.String("to", "", "Recipient's Canton party ID (required)")
	amount        = flag.String("amount", "5", "Amount to transfer")
	token         = flag.String("token", "DEMO", "Token to transfer (DEMO or PROMPT)")
	skipTransfer  = flag.Bool("skip-transfer", false, "Skip transfer, only show balances")
	noReconcile   = flag.Bool("no-reconcile", false, "Skip reconciliation (use with -skip-transfer to only show balances)")
	reconcileOnly = flag.Bool("reconcile-only", false, "Only run reconciliation")
	showHoldings  = flag.Bool("show-holdings", false, "Show all Canton holdings")
)

func main() {
	flag.Parse()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Canton Transfer & Reconciliation Demo")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load config
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	logger, _ := zap.NewDevelopment()

	// Connect to database
	fmt.Println(">>> Connecting to database...")
	db, err := apidb.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	fmt.Println("    Connected to PostgreSQL")
	fmt.Println()

	// Connect to Canton
	fmt.Println(">>> Connecting to Canton...")
	cantonClient, err := canton.NewFromAppConfig(context.Background(), &cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer cantonClient.Close()
	fmt.Println("    Connected to Canton Ledger API")
	fmt.Println()

	ctx := context.Background()

	// Show holdings if requested
	if *showHoldings {
		showAllHoldings(ctx, cantonClient)
		fmt.Println()
	}

	// Step 1: Show current balances from database
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 1: Current Balances (from database cache)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	showDatabaseBalances(db)
	fmt.Println()

	// Create reconciler
	reconciler := apidb.NewReconciler(db, cantonClient.Token, logger)

	if *reconcileOnly {
		// Just run reconciliation and show results
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Running Reconciliation Only")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()

		fmt.Println(">>> Running balance reconciliation from Canton holdings...")
		if err := reconciler.ReconcileUserBalancesFromHoldings(ctx); err != nil {
			fmt.Printf("ERROR: Reconciliation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("    Reconciliation complete!")
		fmt.Println()

		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Updated Balances (after reconciliation)")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		showDatabaseBalances(db)
		return
	}

	// If -skip-transfer and -no-reconcile, just show balances and exit
	if *skipTransfer && *noReconcile {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Done (balances only, no reconciliation)")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		return
	}

	// Step 2: Perform transfer directly via Canton (if not skipped)
	if !*skipTransfer {
		if *fromParty == "" || *toParty == "" {
			fmt.Println("ERROR: --from and --to party IDs are required for transfer")
			fmt.Println()
			fmt.Println("Usage examples:")
			fmt.Println("  Show balances only:       go run scripts/canton-transfer-demo.go -skip-transfer -no-reconcile")
			fmt.Println("  Show balances + reconcile: go run scripts/canton-transfer-demo.go -skip-transfer")
			fmt.Println("  Run reconciliation only:  go run scripts/canton-transfer-demo.go -reconcile-only")
			fmt.Println("  Transfer tokens:          go run scripts/canton-transfer-demo.go -from <party> -to <party>")
			os.Exit(1)
		}

		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Step 2: Transfer via Canton Ledger API (Direct)")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		fmt.Printf("    From Party: %s\n", truncateParty(*fromParty))
		fmt.Printf("    To Party:   %s\n", truncateParty(*toParty))
		fmt.Printf("    Amount:     %s %s\n", *amount, *token)
		fmt.Println()

		// Get fingerprints for the transfer
		fromFingerprint := getFingerprint(db, *fromParty)
		toFingerprint := getFingerprint(db, *toParty)

		if fromFingerprint == "" {
			fmt.Println("ERROR: Could not find sender's fingerprint. Is the user registered?")
			os.Exit(1)
		}
		if toFingerprint == "" {
			fmt.Println("ERROR: Could not find recipient's fingerprint. Is the user registered?")
			os.Exit(1)
		}

		fmt.Println(">>> Executing transfer via Canton Client...")
		err = cantonClient.Token.TransferByFingerprint(ctx, fromFingerprint, toFingerprint, *amount, *token)
		if err != nil {
			fmt.Printf("ERROR: Transfer failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("    Transfer successful!")
		fmt.Println()
	}

	// Step 3: Run reconciliation (unless -no-reconcile)
	if *noReconcile {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Skipping reconciliation (-no-reconcile flag)")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		fmt.Println("Run reconciliation separately with: -reconcile-only")
		fmt.Println()
		return
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 3: Reconcile Balances from Canton Holdings")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Println(">>> Running balance reconciliation...")
	start := time.Now()
	if err := reconciler.ReconcileUserBalancesFromHoldings(ctx); err != nil {
		fmt.Printf("ERROR: Reconciliation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    Reconciliation complete in %v\n", time.Since(start).Round(time.Millisecond))
	fmt.Println()

	// Step 4: Show updated balances
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 4: Updated Balances (after reconciliation)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	showDatabaseBalances(db)
	fmt.Println()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Demo Complete!")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("The reconciler successfully detected and synced balance changes from")
	fmt.Println("the Canton ledger. MetaMask users can now see updated balances that")
	fmt.Println("reflect transfers made by native Canton users.")
	fmt.Println()
}

func showDatabaseBalances(db *apidb.Store) {
	users, err := db.GetAllUsers()
	if err != nil {
		fmt.Printf("    ERROR: Failed to get users: %v\n", err)
		return
	}

	fmt.Println("    Address                                     | PROMPT    | DEMO      | Canton Party")
	fmt.Println("    --------------------------------------------|-----------|-----------|---------------------------")

	for _, user := range users {
		prompt := formatBalance(user.PromptBalance)
		demo := formatBalance(user.DemoBalance)
		party := truncateParty(user.CantonPartyID)
		if party == "" {
			party = "(not allocated)"
		}

		fmt.Printf("    %s | %9s | %9s | %s\n",
			user.EVMAddress, prompt, demo, party)
	}
}

func showAllHoldings(ctx context.Context, client *canton.Client) {
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Canton CIP56 Holdings (from ledger)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	holdings, err := client.Token.GetAllHoldings(ctx)
	if err != nil {
		fmt.Printf("    ERROR: Failed to get holdings: %v\n", err)
		return
	}

	if len(holdings) == 0 {
		fmt.Println("    No holdings found on Canton ledger")
		return
	}

	fmt.Println("    Owner                            | Symbol  | Amount")
	fmt.Println("    ---------------------------------|---------|------------------")

	for _, h := range holdings {
		symbol := h.Symbol
		if symbol == "" {
			symbol = "?"
		}
		fmt.Printf("    %s | %-7s | %s\n",
			truncateParty(h.Owner), symbol, formatBalance(h.Amount))
	}
}

func getFingerprint(db *apidb.Store, partyID string) string {
	user, err := db.GetUserByCantonPartyID(partyID)
	if err != nil || user == nil {
		return ""
	}
	return user.Fingerprint
}

func formatBalance(bal string) string {
	if bal == "" {
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

func truncateParty(party string) string {
	if party == "" {
		return ""
	}
	if len(party) > 30 {
		return party[:20] + "..."
	}
	return party
}
