//go:build ignore

// test-reconcile.go - Test event-based reconciliation using bridge audit events
//
// This script tests the reconciliation functionality that uses MintEvent
// and BurnEvent contracts (CIP56.Events) from Canton to reconcile user balances in PostgreSQL.
// Note: Transfers are internal Canton operations and don't create bridge events.
//
// Prerequisites:
//   1. Docker services running (canton, postgres, mock-oauth2)
//   2. Bridge events created (run e2e-local.go first to create deposits/transfers)
//
// Usage:
//   go run scripts/test-reconcile.go                    # Uses default config
//   go run scripts/test-reconcile.go -verbose          # Show detailed output
//   go run scripts/test-reconcile.go -full-reconcile   # Reset and rebuild all balances
//
// Example workflow:
//   go run scripts/e2e-local.go -skip-docker           # Create bridge events
//   go run scripts/test-reconcile.go -verbose          # Test reconciliation

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
	configPath    = flag.String("config", "config.docker.yaml", "Path to config file")
	verbose       = flag.Bool("verbose", false, "Enable verbose output")
	fullReconcile = flag.Bool("full-reconcile", false, "Reset all balances and rebuild from events")
)

func main() {
	flag.Parse()

	printHeader("Event-Based Reconciliation Test")

	// Load configuration
	printStep("Loading configuration from %s...", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		printError("Failed to load config: %v", err)
		os.Exit(1)
	}
	printSuccess("Configuration loaded")

	// Create logger
	logLevel := zapcore.InfoLevel
	if *verbose {
		logLevel = zapcore.DebugLevel
	}
	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level = zap.NewAtomicLevelAt(logLevel)
	logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger, _ := logConfig.Build()
	defer logger.Sync()

	// Override Canton URL for local access (Docker uses internal hostnames)
	localCantonCfg := cfg.Canton
	localCantonCfg.RPCURL = "localhost:5011"
	localCantonCfg.Auth.TokenURL = "http://localhost:8088/oauth/token"
	localCantonCfg.Auth.Audience = "http://localhost:5011"

	// Try to get relayer_party from .test-config.yaml if not set
	if localCantonCfg.RelayerParty == "" {
		testCfg, err := config.Load(".test-config.yaml")
		if err != nil {
			if *verbose {
				printInfo("Could not load .test-config.yaml: %v", err)
			}
			// Try reading party directly from bootstrap output
			partyBytes, err := os.ReadFile(".test-config.yaml")
			if err == nil {
				// Parse YAML manually for just the relayer_party field
				lines := strings.Split(string(partyBytes), "\n")
				for _, line := range lines {
					if strings.Contains(line, "relayer_party:") {
						parts := strings.SplitN(line, ":", 2)
						if len(parts) == 2 {
							party := strings.Trim(strings.TrimSpace(parts[1]), `"`)
							if party != "" {
								localCantonCfg.RelayerParty = party
								printInfo("Using relayer_party from .test-config.yaml: %s", truncate(party, 40))
							}
						}
					}
					if strings.Contains(line, "domain_id:") {
						parts := strings.SplitN(line, ":", 2)
						if len(parts) == 2 {
							domain := strings.Trim(strings.TrimSpace(parts[1]), `"`)
							if domain != "" {
								localCantonCfg.DomainID = domain
							}
						}
					}
				}
			}
		} else if testCfg.Canton.RelayerParty != "" {
			localCantonCfg.RelayerParty = testCfg.Canton.RelayerParty
			localCantonCfg.DomainID = testCfg.Canton.DomainID
			printInfo("Using relayer_party from .test-config.yaml: %s", truncate(localCantonCfg.RelayerParty, 40))
		}
		
		if localCantonCfg.RelayerParty == "" {
			printWarning("relayer_party not set - run e2e-local.go first to bootstrap Canton")
		}
	}

	// Connect to Canton
	printStep("Connecting to Canton at %s...", localCantonCfg.RPCURL)
	cantonClient, err := canton.NewClient(&localCantonCfg, logger)
	if err != nil {
		printError("Failed to connect to Canton: %v", err)
		os.Exit(1)
	}
	defer cantonClient.Close()
	printSuccess("Connected to Canton")

	// Connect to PostgreSQL (use local host for testing)
	dsn := fmt.Sprintf("host=localhost port=%d user=%s password=%s dbname=erc20_api sslmode=disable",
		cfg.Database.Port, cfg.Database.User, cfg.Database.Password)
	printStep("Connecting to PostgreSQL...")
	db, err := apidb.NewStore(dsn)
	if err != nil {
		printError("Failed to connect to PostgreSQL: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	printSuccess("Connected to PostgreSQL")

	ctx := context.Background()

	// Show current state before reconciliation
	printHeader("Current State (Before Reconciliation)")
	showBridgeEventsFromCanton(ctx, cantonClient)
	showStoredEvents(db)
	showUserBalances(db)

	// Create reconciler and run
	printHeader("Running Reconciliation")
	reconciler := apidb.NewReconciler(db, cantonClient, logger)

	if *fullReconcile {
		printStep("Performing FULL balance reconciliation (resetting all balances)...")
		if err := reconciler.FullBalanceReconciliation(ctx); err != nil {
			printError("Full reconciliation failed: %v", err)
			os.Exit(1)
		}
		printSuccess("Full reconciliation completed")
	} else {
		printStep("Performing incremental reconciliation...")
		if err := reconciler.ReconcileFromBridgeEvents(ctx); err != nil {
			printError("Reconciliation failed: %v", err)
			os.Exit(1)
		}
		printSuccess("Incremental reconciliation completed")
	}

	// Show state after reconciliation
	printHeader("State After Reconciliation")
	showStoredEvents(db)
	showUserBalances(db)
	showReconciliationState(db)

	printHeader("Reconciliation Test Completed Successfully!")
}

func showBridgeEventsFromCanton(ctx context.Context, client *canton.Client) {
	printStep("Fetching bridge events from Canton...")

	// Get mint events
	mintEvents, err := client.GetMintEvents(ctx)
	if err != nil {
		printWarning("Failed to get mint events: %v", err)
	} else {
		printInfo("Found %d MintEvent contracts:", len(mintEvents))
		for i, e := range mintEvents {
			if *verbose {
				printInfo("  [%d] Amount: %s, Fingerprint: %s, EvmTx: %s",
					i+1, e.Amount, truncate(e.UserFingerprint, 20), truncate(e.EvmTxHash, 20))
			}
		}
	}

	// Get burn events
	burnEvents, err := client.GetBurnEvents(ctx)
	if err != nil {
		printWarning("Failed to get burn events: %v", err)
	} else {
		printInfo("Found %d BurnEvent contracts:", len(burnEvents))
		for i, e := range burnEvents {
			if *verbose {
				printInfo("  [%d] Amount: %s, Fingerprint: %s, Destination: %s",
					i+1, e.Amount, truncate(e.UserFingerprint, 20), truncate(e.EvmDestination, 20))
			}
		}
	}
}

func showStoredEvents(store *apidb.Store) {
	printStep("Checking stored events in PostgreSQL...")

	events, err := store.GetRecentBridgeEvents(20)
	if err != nil {
		printWarning("Failed to get stored events: %v", err)
		return
	}

	if len(events) == 0 {
		printInfo("No bridge events stored in database yet")
		return
	}

	printInfo("Found %d stored bridge events:", len(events))
	for _, e := range events {
		if *verbose {
			printInfo("  [%s] %s - Amount: %s, Fingerprint: %s",
				e.EventType, truncate(e.ContractID, 16), e.Amount, truncate(e.Fingerprint, 16))
		}
	}

	// Show counts by type
	counts, err := store.GetEventCountByType()
	if err == nil {
		printInfo("Event counts: mint=%d, burn=%d",
			counts["mint"], counts["burn"])
	}
}

func showUserBalances(store *apidb.Store) {
	printStep("Checking user balances in PostgreSQL...")

	users, err := store.GetAllUsers()
	if err != nil {
		printWarning("Failed to get users: %v", err)
		return
	}

	if len(users) == 0 {
		printInfo("No users registered yet")
		return
	}

	printInfo("User balances:")
	for _, u := range users {
		printInfo("  %s: %s (fingerprint: %s)",
			truncate(u.EVMAddress, 12), u.Balance, truncate(u.Fingerprint, 16))
	}
}

func showReconciliationState(store *apidb.Store) {
	printStep("Checking reconciliation state...")

	state, err := store.GetReconciliationState()
	if err != nil {
		printWarning("Failed to get reconciliation state: %v", err)
		return
	}

	printInfo("Reconciliation state:")
	printInfo("  Events processed: %d", state.EventsProcessed)
	printInfo("  Last updated: %s", state.UpdatedAt.Format(time.RFC3339))
	if state.LastFullReconcileAt != nil {
		printInfo("  Last full reconcile: %s", state.LastFullReconcileAt.Format(time.RFC3339))
	}
}

// =============================================================================
// Direct Database Queries (for verification)
// =============================================================================

func queryBridgeEventsTable(dsn string) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		printWarning("Failed to open DB: %v", err)
		return
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, event_type, contract_id, fingerprint, amount, processed_at 
		FROM bridge_events 
		ORDER BY id DESC 
		LIMIT 10
	`)
	if err != nil {
		printWarning("Failed to query bridge_events: %v", err)
		return
	}
	defer rows.Close()

	printInfo("bridge_events table contents:")
	for rows.Next() {
		var id int64
		var eventType, contractID, fingerprint, amount string
		var processedAt time.Time
		if err := rows.Scan(&id, &eventType, &contractID, &fingerprint, &amount, &processedAt); err != nil {
			continue
		}
		printInfo("  [%d] %s: %s (%s)", id, eventType, amount, truncate(fingerprint, 16))
	}
}

// =============================================================================
// Output Helpers
// =============================================================================

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

