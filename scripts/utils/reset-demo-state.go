//go:build ignore
// +build ignore

// Reset Demo State
//
// This script resets the demo state back to initial bootstrap configuration:
// - User 1: 500 DEMO (single holding)
// - User 2: 500 DEMO (single holding)
// - Native users: 0 DEMO (holdings archived)
// - Database cleaned up
//
// Usage:
//   go run scripts/utils/reset-demo-state.go -config config.api-server.devnet.local.yaml

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
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.devnet.local.yaml", "Path to config file")
	dryRun     = flag.Bool("dry-run", false, "Show what would be done without making changes")
	mintAmount = flag.String("amount", "500", "Amount to mint to each user")
)

func main() {
	flag.Parse()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Reset Demo State (Archive & Remint)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	if *dryRun {
		fmt.Println(">>> DRY RUN MODE - No changes will be made")
		fmt.Println()
	}

	// Load config
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	// Create logger
	logger, _ := zap.NewDevelopment()

	// Connect to database
	fmt.Println(">>> Connecting to services...")
	db, err := apidb.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	fmt.Println("    Connected to PostgreSQL")

	// Connect to Canton
	cantonClient, err := canton.NewFromAppConfig(context.Background(), &cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer cantonClient.Close()
	fmt.Println("    Connected to Canton Ledger API")
	fmt.Println()

	ctx := context.Background()

	// Get current holdings
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Current Holdings")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	holdings, err := cantonClient.Token.GetAllHoldings(ctx)
	if err != nil {
		fatalf("Failed to get holdings: %v", err)
	}

	// Identify user parties and collect all holdings
	var user1Party, user2Party string
	var allDemoHoldings []*token.Holding
	var nativeParties []string

	for _, h := range holdings {
		if h.Symbol != "DEMO" {
			continue
		}
		allDemoHoldings = append(allDemoHoldings, h)

		if strings.HasPrefix(h.Owner, "user_f39Fd6e5::") {
			user1Party = h.Owner
		} else if strings.HasPrefix(h.Owner, "user_70997970::") {
			user2Party = h.Owner
		} else if strings.HasPrefix(h.Owner, "native_") {
			// Track unique native parties
			found := false
			for _, np := range nativeParties {
				if np == h.Owner {
					found = true
					break
				}
			}
			if !found {
				nativeParties = append(nativeParties, h.Owner)
			}
		}
	}

	fmt.Println("    Identified parties:")
	fmt.Printf("    User 1:      %s\n", truncateParty(user1Party))
	fmt.Printf("    User 2:      %s\n", truncateParty(user2Party))
	for _, np := range nativeParties {
		fmt.Printf("    Native:      %s\n", truncateParty(np))
	}
	fmt.Printf("\n    Total DEMO holdings to archive: %d\n\n", len(allDemoHoldings))

	// Show current holdings summary
	holdingsByOwner := make(map[string]float64)
	for _, h := range allDemoHoldings {
		holdingsByOwner[h.Owner] += parseAmount(h.Amount)
	}
	for owner, total := range holdingsByOwner {
		fmt.Printf("    %s: %.2f DEMO\n", truncateParty(owner), total)
	}
	fmt.Println()

	// Step 1: Archive all DEMO holdings using CIP56Manager.Burn
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 1: Archive All DEMO Holdings")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	if !*dryRun {
		for i, h := range allDemoHoldings {
			fmt.Printf("    [%d/%d] Archiving %s DEMO from %s...\n", i+1, len(allDemoHoldings), h.Amount, truncateParty(h.Owner))
			err = cantonClient.Token.Burn(ctx, token.BurnRequest{
				HoldingCID:      h.ContractID,
				Amount:          h.Amount,
				UserFingerprint: "",
				TokenSymbol:     "DEMO",
				EvmDestination:  "",
			})
			if err != nil {
				fmt.Printf("    ERROR: %v\n", err)
			} else {
				fmt.Println("    Archived!")
			}
			time.Sleep(300 * time.Millisecond) // Rate limit
		}
	} else {
		for i, h := range allDemoHoldings {
			fmt.Printf("    [%d/%d] Would archive %s DEMO from %s\n", i+1, len(allDemoHoldings), h.Amount, truncateParty(h.Owner))
		}
	}
	fmt.Println()

	// Step 2: Mint fresh holdings to users
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 2: Mint Fresh DEMO Holdings")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Get user fingerprints from database
	users, err := db.GetAllUsers()
	if err != nil {
		fatalf("Failed to get users: %v", err)
	}

	// Find fingerprints for each user party
	user1Fingerprint := ""
	user2Fingerprint := ""
	for _, u := range users {
		if u.CantonPartyID == user1Party || u.CantonParty == user1Party {
			user1Fingerprint = u.Fingerprint
		} else if u.CantonPartyID == user2Party || u.CantonParty == user2Party {
			user2Fingerprint = u.Fingerprint
		}
	}

	fmt.Printf("    Minting %s DEMO to User 1 (%s)...\n", *mintAmount, truncateParty(user1Party))
	if !*dryRun && user1Party != "" {
		_, err := cantonClient.Token.Mint(ctx, token.MintRequest{
			RecipientParty:  user1Party,
			Amount:          *mintAmount,
			UserFingerprint: user1Fingerprint,
			TokenSymbol:     "DEMO",
		})
		if err != nil {
			fmt.Printf("    ERROR: %v\n", err)
		} else {
			fmt.Println("    Minted!")
		}
	} else if *dryRun {
		fmt.Println("    [DRY RUN - skipped]")
	}

	fmt.Printf("    Minting %s DEMO to User 2 (%s)...\n", *mintAmount, truncateParty(user2Party))
	if !*dryRun && user2Party != "" {
		_, err := cantonClient.Token.Mint(ctx, token.MintRequest{
			RecipientParty:  user2Party,
			Amount:          *mintAmount,
			UserFingerprint: user2Fingerprint,
			TokenSymbol:     "DEMO",
		})
		if err != nil {
			fmt.Printf("    ERROR: %v\n", err)
		} else {
			fmt.Println("    Minted!")
		}
	} else if *dryRun {
		fmt.Println("    [DRY RUN - skipped]")
	}
	fmt.Println()

	// Step 2b: Ensure FingerprintMappings exist for all registered users
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 2b: Ensure FingerprintMappings Exist")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	for _, u := range users {
		if u.MappingCID != "" {
			fmt.Printf("    %s: Already has FingerprintMapping\n", u.EVMAddress)
			continue
		}
		if u.CantonPartyID == "" || u.Fingerprint == "" {
			fmt.Printf("    %s: Missing party ID or fingerprint, skipping\n", u.EVMAddress)
			continue
		}

		fmt.Printf("    Creating FingerprintMapping for %s...\n", u.EVMAddress)
		if !*dryRun {
			m, err := cantonClient.Identity.CreateFingerprintMapping(ctx, identity.CreateFingerprintMappingRequest{
				UserParty:   u.CantonPartyID,
				Fingerprint: u.Fingerprint,
				EvmAddress:  u.EVMAddress,
			})
			if err != nil {
				fmt.Printf("    ERROR: %v\n", err)
			} else {
				fmt.Printf("    Created! CID: %s...\n", m.ContractID[:40])
				// Update database with mapping CID
				if err := db.UpdateUserMappingCID(u.EVMAddress, m.ContractID); err != nil {
					fmt.Printf("    WARNING: Failed to update database: %v\n", err)
				}
			}
		} else {
			fmt.Println("    [DRY RUN - skipped]")
		}
	}
	fmt.Println()

	// Step 3: Clean up database - remove native users
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 3: Clean Up Database")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	for _, nativeParty := range nativeParties {
		fmt.Printf("    Removing %s from users table...\n", truncateParty(nativeParty))
		if !*dryRun {
			result, err := db.DB().Exec("DELETE FROM users WHERE canton_party = $1 OR canton_party_id = $1", nativeParty)
			if err != nil {
				fmt.Printf("    ERROR: %v\n", err)
			} else {
				rows, _ := result.RowsAffected()
				fmt.Printf("    Success! (%d rows deleted)\n", rows)
			}
		} else {
			fmt.Println("    [DRY RUN - skipped]")
		}
	}
	fmt.Println()

	// Step 4: Reconcile balances
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Step 4: Reconcile Balances")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	if !*dryRun {
		fmt.Println("    Running reconciliation...")
		reconciler := apidb.NewReconciler(db, cantonClient.Token, logger)
		err = reconciler.ReconcileUserBalancesFromHoldings(ctx)
		if err != nil {
			fmt.Printf("    ERROR: %v\n", err)
		} else {
			fmt.Println("    Success!")
		}
	} else {
		fmt.Println("    [DRY RUN - skipped]")
	}
	fmt.Println()

	// Show final state
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Final State")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	if !*dryRun {
		// Show Canton holdings
		fmt.Println(">>> Canton Holdings:")
		holdings, err = cantonClient.Token.GetAllHoldings(ctx)
		if err != nil {
			fmt.Printf("    ERROR: %v\n", err)
		} else {
			countByOwner := make(map[string]int)
			balanceByOwner := make(map[string]float64)
			for _, h := range holdings {
				if h.Symbol == "DEMO" {
					countByOwner[h.Owner]++
					balanceByOwner[h.Owner] += parseAmount(h.Amount)
				}
			}
			for owner, count := range countByOwner {
				fmt.Printf("    %s: %.2f DEMO (%d holding(s))\n", truncateParty(owner), balanceByOwner[owner], count)
			}
		}
		fmt.Println()

		// Show database users
		fmt.Println(">>> Database Users:")
		users, err = db.GetAllUsers()
		if err != nil {
			fmt.Printf("    ERROR: %v\n", err)
		} else {
			for _, u := range users {
				fmt.Printf("    %s: %.2f DEMO\n", u.EVMAddress, parseAmount(u.DemoBalance))
			}
		}
	} else {
		fmt.Println("    [DRY RUN - no changes made]")
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Reset Complete!")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
}

func truncateParty(party string) string {
	if len(party) > 40 {
		return party[:30] + "..."
	}
	return party
}

func parseAmount(amount string) float64 {
	var f float64
	fmt.Sscanf(amount, "%f", &f)
	return f
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
