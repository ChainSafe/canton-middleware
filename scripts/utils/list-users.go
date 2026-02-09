//go:build ignore
// +build ignore

// List Registered Users Script
//
// This script lists all users registered with the API server from the database.
//
// Usage:
//   go run scripts/utils/list-users.go -config config.api-server.mainnet.local.yaml

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
)

var configPath = flag.String("config", "config.api-server.mainnet.local.yaml", "Path to config file")

func main() {
	flag.Parse()

	// Load config
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Determine network from config
	networkName := detectNetwork(cfg.Canton.RPCURL)

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Registered Users - %s\n", networkName)
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Config:   %s\n", *configPath)
	fmt.Printf("  Database: %s\n", cfg.Database.Database)
	fmt.Println()

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

	// Get all users
	users, err := db.GetAllUsers()
	if err != nil {
		fmt.Printf("ERROR: Failed to get users: %v\n", err)
		os.Exit(1)
	}

	if len(users) == 0 {
		fmt.Println("No registered users found.")
		return
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Registered Users (%d total)\n", len(users))
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	for i, user := range users {
		fmt.Printf("[%d] EVM Address: %s\n", i+1, user.EVMAddress)

		partyDisplay := user.CantonPartyID
		if len(partyDisplay) > 70 {
			partyDisplay = partyDisplay[:67] + "..."
		}
		fmt.Printf("    Canton Party: %s\n", partyDisplay)

		fpDisplay := user.Fingerprint
		if len(fpDisplay) > 70 {
			fpDisplay = fpDisplay[:67] + "..."
		}
		fmt.Printf("    Fingerprint:  %s\n", fpDisplay)

		fmt.Printf("    DEMO Balance: %s\n", formatBalance(user.DemoBalance))
		promptBal := formatBalance(user.PromptBalance)
		if promptBal != "0" && promptBal != "0.00" {
			fmt.Printf("    PROMPT Balance: %s\n", promptBal)
		}
		fmt.Println()
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Done")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
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
