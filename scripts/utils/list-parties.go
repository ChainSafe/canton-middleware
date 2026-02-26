//go:build ignore
// +build ignore

// List Known Parties Script
//
// This script shows parties known to our setup - from the config (relayer)
// and from the database (registered users), plus queries Canton for any
// parties matching specific prefixes.
//
// Usage:
//   go run scripts/utils/list-parties.go -config config.api-server.mainnet.local.yaml
//   go run scripts/utils/list-parties.go -config config.api-server.mainnet.local.yaml -search native

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.mainnet.local.yaml", "Path to config file")
	search     = flag.String("search", "", "Search Canton for parties matching this prefix (slow on mainnet)")
)

func main() {
	flag.Parse()

	// Load config
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Determine network
	networkName := detectNetwork(cfg.Canton.RPCURL)

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Known Parties - %s\n", networkName)
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Show relayer party from config
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  System Party (from config)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  [daml-autopilot] (Relayer/Issuer)\n")
	fmt.Printf("    %s\n", cfg.Canton.RelayerParty)
	fmt.Println()

	// Connect to database
	fmt.Println(">>> Connecting to database...")
	bunDB, err := pgutil.ConnectDB(&cfg.Database)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer bunDB.Close()
	uStore := userstore.NewStore(bunDB)
	fmt.Println("    ✓ Connected")
	fmt.Println()

	// Get registered users
	users, err := uStore.ListUsers(context.Background())
	if err != nil {
		fmt.Printf("ERROR: Failed to get users: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Registered User Parties (from database) - %d\n", len(users))
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	for _, u := range users {
		hint := extractHint(u.CantonPartyID)
		fmt.Printf("  [%s]\n", hint)
		fmt.Printf("    EVM:   %s\n", u.EVMAddress)
		fmt.Printf("    Party: %s\n", u.CantonPartyID)
		fmt.Println()
	}

	// Known native interop parties (allocated for demo)
	knownNativeParties := []struct {
		Name    string
		PartyID string
	}{
		{"native_interop_1", "native_interop_1::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b"},
		{"native_interop_2", "native_interop_2::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b"},
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Native Demo Parties (pre-allocated) - %d\n", len(knownNativeParties))
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	for _, p := range knownNativeParties {
		fmt.Printf("  [%s]\n", p.Name)
		fmt.Printf("    Party: %s\n", p.PartyID)
		fmt.Println()
	}

	// Search Canton for specific parties if requested
	if *search != "" {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Printf("  Searching Canton for '%s' parties...\n", *search)
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()

		// Create logger (quiet mode)
		logConfig := zap.NewProductionConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
		logger, _ := logConfig.Build()

		// Connect to Canton
		fmt.Println(">>> Connecting to Canton (this may take a while on mainnet)...")
		cantonClient, err := canton.NewFromAppConfig(context.Background(), &cfg.Canton, canton.WithLogger(logger))
		if err != nil {
			fmt.Printf("ERROR: Failed to connect to Canton: %v\n", err)
			os.Exit(1)
		}
		defer cantonClient.Close()

		ctx := context.Background()
		parties, err := cantonClient.Identity.ListParties(ctx)
		if err != nil {
			fmt.Printf("ERROR: Failed to list parties: %v\n", err)
			os.Exit(1)
		}

		var matched []*identity.Party
		for _, p := range parties {
			if strings.Contains(strings.ToLower(p.PartyID), strings.ToLower(*search)) {
				matched = append(matched, p)
			}
		}

		fmt.Printf("    Found %d parties matching '%s':\n\n", len(matched), *search)
		for _, p := range matched {
			hint := extractHint(p.PartyID)
			localStr := ""
			if p.IsLocal {
				localStr = " (local)"
			}
			fmt.Printf("  [%s]%s\n", hint, localStr)
			fmt.Printf("    %s\n", p.PartyID)
			fmt.Println()
		}
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Done")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	if *search == "" {
		fmt.Println("  Tip: Use -search to find parties on Canton:")
		fmt.Println("    -search native      # Find native_interop parties")
		fmt.Println("    -search user_       # Find all user parties")
		fmt.Println()
	}
}

func extractHint(partyID string) string {
	if idx := strings.Index(partyID, "::"); idx != -1 {
		return partyID[:idx]
	}
	return partyID
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
