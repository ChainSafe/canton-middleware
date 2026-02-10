//go:build ignore
// +build ignore

// Interoperability Demo Script
//
// This script demonstrates bidirectional token transfers between MetaMask-registered
// users and native Canton users (who interact directly via Ledger API).
//
// Demo Flow:
//   1. Allocate 2 native Canton parties (not registered with API server)
//   2. User 1 (MetaMask) sends 100 DEMO to Native User 1
//   3. Native User 1 sends 100 DEMO to Native User 2 (via Ledger API)
//   4. Native User 2 sends 100 DEMO back to User 1 (MetaMask)
//   5. Register Native User 1 with the API server
//   6. User 1 sends 100 DEMO to Native User 1 (now visible in MetaMask)
//
// This proves true interoperability between MetaMask users and native Canton users.
//
// Usage:
//   go run scripts/testing/interop-demo.go -config config.api-server.devnet.yaml
//
// Prerequisites:
//   - Bootstrap completed with MetaMask users having DEMO tokens
//   - API server running

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var (
	configPath   = flag.String("config", "config.api-server.devnet.yaml", "Path to config file")
	amount       = flag.String("amount", "100", "Amount to transfer in each step")
	skipAllocate = flag.Bool("skip-allocate", false, "Skip party allocation (use existing native parties)")
	nativeParty1 = flag.String("native1", "", "Existing native party 1 (with -skip-allocate)")
	nativeParty2 = flag.String("native2", "", "Existing native party 2 (with -skip-allocate)")
	skipRegister = flag.Bool("skip-register", false, "Skip native user registration step")
	apiURL       = flag.String("api-url", "http://localhost:8081", "API server URL")
)

func main() {
	flag.Parse()

	printHeader("Canton Interoperability Demo")
	fmt.Println("This demo proves bidirectional token transfers between MetaMask")
	fmt.Println("users and native Canton users who use the Ledger API directly.")
	fmt.Println()

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
	cantonClient, err := canton.NewClient(&cfg.Canton, logger)
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer cantonClient.Close()
	fmt.Println("    Connected to Canton Ledger API")
	fmt.Println()

	ctx := context.Background()

	// Get MetaMask users from database
	printHeader("Step 0: Verify MetaMask Users")
	users, err := db.GetAllUsers()
	if err != nil {
		fatalf("Failed to get users: %v", err)
	}
	if len(users) < 1 {
		fatalf("No MetaMask users found. Run bootstrap first.")
	}

	metamaskUser1 := users[0]
	fmt.Printf("    MetaMask User 1: %s\n", metamaskUser1.EVMAddress)
	fmt.Printf("    Canton Party:    %s\n", truncateParty(metamaskUser1.CantonPartyID))
	fmt.Printf("    DEMO Balance:    %s\n", formatBalance(metamaskUser1.DemoBalance))
	fmt.Println()

	// Show initial holdings
	printHeader("Initial Canton Holdings")
	showHoldings(ctx, cantonClient)
	fmt.Println()

	// Step 1: Allocate native Canton parties
	var native1Party, native2Party string

	if *skipAllocate {
		if *nativeParty1 == "" || *nativeParty2 == "" {
			fatalf("With -skip-allocate, you must provide -native1 and -native2 party IDs")
		}
		native1Party = *nativeParty1
		native2Party = *nativeParty2
		printHeader("Step 1: Using Existing Native Parties")
		fmt.Printf("    Native User 1: %s\n", truncateParty(native1Party))
		fmt.Printf("    Native User 2: %s\n", truncateParty(native2Party))
	} else {
		printHeader("Step 1: Allocate Native Canton Parties")
		fmt.Println("Creating 2 native Canton users (NOT registered with API server)...")
		fmt.Println()

		native1Party, err = allocateNativeParty(ctx, cantonClient, "native_interop_1")
		if err != nil {
			fatalf("Failed to allocate native party 1: %v", err)
		}
		fmt.Printf("    Native User 1: %s\n", truncateParty(native1Party))

		native2Party, err = allocateNativeParty(ctx, cantonClient, "native_interop_2")
		if err != nil {
			fatalf("Failed to allocate native party 2: %v", err)
		}
		fmt.Printf("    Native User 2: %s\n", truncateParty(native2Party))
	}
	fmt.Println()

	// Step 2: MetaMask User 1 -> Native User 1
	printHeader("Step 2: MetaMask User -> Native User")
	fmt.Printf("    Transfer: %s DEMO\n", *amount)
	fmt.Printf("    From:     %s (MetaMask)\n", metamaskUser1.EVMAddress)
	fmt.Printf("    To:       %s (Native)\n", truncateParty(native1Party))
	fmt.Println()

	fmt.Println(">>> Executing transfer via Canton Ledger API...")
	err = cantonClient.TransferByPartyID(ctx, metamaskUser1.CantonPartyID, native1Party, *amount, "DEMO")
	if err != nil {
		if strings.Contains(err.Error(), "PermissionDenied") {
			fmt.Println()
			fmt.Println("    ERROR: PermissionDenied - OAuth token cannot ActAs user parties")
			fmt.Println()
			fmt.Println("    This happens on remote Canton (DevNet/Mainnet) where the OAuth client")
			fmt.Println("    doesn't have ActAs rights for user parties.")
			fmt.Println()
			fmt.Println("    Solutions:")
			fmt.Println("    1. Run demo against local Canton: docker-compose up -d")
			fmt.Println("    2. Contact ChainSafe to grant ActAs rights for user parties")
			fmt.Println("    3. Use operator-controlled transfers (requires contract changes)")
			fmt.Println()
			os.Exit(1)
		}
		fatalf("Transfer failed: %v", err)
	}
	fmt.Println("    Transfer successful!")
	fmt.Println()

	fmt.Println(">>> Holdings after transfer:")
	showHoldings(ctx, cantonClient)
	fmt.Println()

	// Step 3: Native User 1 -> Native User 2 (pure Ledger API)
	printHeader("Step 3: Native User -> Native User (Ledger API)")
	fmt.Printf("    Transfer: %s DEMO\n", *amount)
	fmt.Printf("    From:     %s (Native 1)\n", truncateParty(native1Party))
	fmt.Printf("    To:       %s (Native 2)\n", truncateParty(native2Party))
	fmt.Println()
	fmt.Println("This transfer happens entirely via the Canton Ledger API,")
	fmt.Println("simulating a native Canton user's direct interaction.")
	fmt.Println()

	fmt.Println(">>> Executing transfer via Canton Ledger API...")
	err = cantonClient.TransferByPartyID(ctx, native1Party, native2Party, *amount, "DEMO")
	if err != nil {
		fatalf("Transfer failed: %v", err)
	}
	fmt.Println("    Transfer successful!")
	fmt.Println()

	fmt.Println(">>> Holdings after transfer:")
	showHoldings(ctx, cantonClient)
	fmt.Println()

	// Step 4: Native User 2 -> MetaMask User 1
	printHeader("Step 4: Native User -> MetaMask User")
	fmt.Printf("    Transfer: %s DEMO\n", *amount)
	fmt.Printf("    From:     %s (Native 2)\n", truncateParty(native2Party))
	fmt.Printf("    To:       %s (MetaMask)\n", metamaskUser1.EVMAddress)
	fmt.Println()
	fmt.Println("This proves native Canton users can send tokens back to MetaMask users.")
	fmt.Println()

	fmt.Println(">>> Executing transfer via Canton Ledger API...")
	err = cantonClient.TransferByPartyID(ctx, native2Party, metamaskUser1.CantonPartyID, *amount, "DEMO")
	if err != nil {
		fatalf("Transfer failed: %v", err)
	}
	fmt.Println("    Transfer successful!")
	fmt.Println()

	fmt.Println(">>> Holdings after transfer:")
	showHoldings(ctx, cantonClient)
	fmt.Println()

	// Run reconciliation to update MetaMask view
	fmt.Println(">>> Running reconciliation to update MetaMask balances...")
	reconciler := apidb.NewReconciler(db, cantonClient, logger)
	if err := reconciler.ReconcileUserBalancesFromHoldings(ctx); err != nil {
		fmt.Printf("    WARNING: Reconciliation failed: %v\n", err)
	} else {
		fmt.Println("    Reconciliation complete!")
	}
	fmt.Println()

	fmt.Println(">>> MetaMask balances after reconciliation:")
	showDatabaseBalances(db)
	fmt.Println()

	// Step 5: Register Native User 1 with API Server (optional)
	if !*skipRegister {
		printHeader("Step 5: Register Native User with API Server")
		fmt.Println("Registering Native User 1 so they can use MetaMask...")
		fmt.Println()

		evmAddress, privateKey, err := registerNativeUserWithAPI(ctx, *apiURL, native1Party)
		if err != nil {
			fmt.Printf("    WARNING: Registration failed: %v\n", err)
			fmt.Println("    Skipping final transfer step.")
		} else {
			fmt.Printf("    Registered Native User 1!\n")
			fmt.Printf("    EVM Address:  %s\n", evmAddress)
			fmt.Printf("    Private Key:  %s\n", privateKey)
			fmt.Println()
			fmt.Println("    Native User 1 can now import this key to MetaMask")
			fmt.Println("    and see their DEMO balance.")
			fmt.Println()

			// Step 6: MetaMask User 1 -> Registered Native User 1
			printHeader("Step 6: MetaMask User -> Registered Native User")
			fmt.Printf("    Transfer: %s DEMO\n", *amount)
			fmt.Printf("    From:     %s (MetaMask User 1)\n", metamaskUser1.EVMAddress)
			fmt.Printf("    To:       %s (Now MetaMask-enabled)\n", evmAddress)
			fmt.Println()

			fmt.Println(">>> Executing transfer...")
			err = cantonClient.TransferByPartyID(ctx, metamaskUser1.CantonPartyID, native1Party, *amount, "DEMO")
			if err != nil {
				fmt.Printf("    WARNING: Transfer failed: %v\n", err)
			} else {
				fmt.Println("    Transfer successful!")
				fmt.Println()

				// Run reconciliation
				fmt.Println(">>> Running reconciliation...")
				if err := reconciler.ReconcileUserBalancesFromHoldings(ctx); err != nil {
					fmt.Printf("    WARNING: Reconciliation failed: %v\n", err)
				}
				fmt.Println()

				fmt.Println(">>> Final MetaMask balances:")
				showDatabaseBalances(db)
			}
		}
	}

	// Final summary
	printHeader("Demo Complete!")
	fmt.Println("This demo proved:")
	fmt.Println("  1. MetaMask users can send DEMO to native Canton users")
	fmt.Println("  2. Native Canton users can transfer between themselves via Ledger API")
	fmt.Println("  3. Native Canton users can send DEMO back to MetaMask users")
	if !*skipRegister {
		fmt.Println("  4. Native users can be registered to gain MetaMask access")
	}
	fmt.Println()
	fmt.Println("The Canton bridge enables true interoperability between")
	fmt.Println("MetaMask users and native Canton ledger participants.")
	fmt.Println()
}

func allocateNativeParty(ctx context.Context, client *canton.Client, hint string) (string, error) {
	var partyID string

	result, err := client.AllocateParty(ctx, hint)
	if err != nil {
		// Check if party already exists
		if strings.Contains(err.Error(), "already allocated") || strings.Contains(err.Error(), "already exists") {
			// Try to find existing party by listing
			parties, listErr := client.ListParties(ctx)
			if listErr != nil {
				return "", fmt.Errorf("party exists but could not list: %w", listErr)
			}
			for _, p := range parties {
				if strings.HasPrefix(p.PartyID, hint+"::") {
					partyID = p.PartyID
					break
				}
			}
			if partyID == "" {
				return "", fmt.Errorf("party exists but not found in list")
			}
		} else {
			return "", err
		}
	} else {
		partyID = result.PartyID
	}

	// Grant CanActAs rights to the OAuth client for this party
	// This enables the custodial model for interop demo
	if err := client.GrantCanActAs(ctx, partyID); err != nil {
		// Log warning but continue - right might already exist
		fmt.Printf("    Warning: Failed to grant CanActAs for %s: %v\n", truncateParty(partyID), err)
	}

	return partyID, nil
}

func registerNativeUserWithAPI(ctx context.Context, apiURL, cantonParty string) (evmAddress, privateKey string, err error) {
	// Use Canton native registration flow
	// The API server will generate an EVM keypair for this Canton user
	// No whitelist required for Canton native users

	message := fmt.Sprintf("Register Canton party %s for MetaMask access", cantonParty)

	// For Canton native registration, we send canton_party_id
	// The canton_signature can be skipped if SKIP_CANTON_SIG_VERIFY=true on the server
	reqBody := map[string]string{
		"canton_party_id":  cantonParty,
		"canton_signature": "", // Empty - server should have SKIP_CANTON_SIG_VERIFY=true for demo
		"message":          message,
	}
	jsonBody, _ := json.Marshal(reqBody)

	// Register with API server using Canton native flow
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(apiURL+"/register", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", "", fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("registration failed (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response to get the generated EVM address and private key
	var response struct {
		Party       string `json:"party"`
		Fingerprint string `json:"fingerprint"`
		MappingCID  string `json:"mapping_cid"`
		EVMAddress  string `json:"evm_address"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}

	return response.EVMAddress, response.PrivateKey, nil
}

func showHoldings(ctx context.Context, client *canton.Client) {
	holdings, err := client.GetAllCIP56Holdings(ctx)
	if err != nil {
		fmt.Printf("    ERROR: Failed to get holdings: %v\n", err)
		return
	}

	if len(holdings) == 0 {
		fmt.Println("    No holdings found")
		return
	}

	fmt.Println("    Owner                                    | Symbol | Amount")
	fmt.Println("    -----------------------------------------|--------|--------")

	for _, h := range holdings {
		symbol := h.Symbol
		if symbol == "" {
			symbol = "?"
		}
		fmt.Printf("    %-41s | %-6s | %s\n",
			truncateParty(h.Owner), symbol, formatBalance(h.Amount))
	}
}

func showDatabaseBalances(db *apidb.Store) {
	users, err := db.GetAllUsers()
	if err != nil {
		fmt.Printf("    ERROR: Failed to get users: %v\n", err)
		return
	}

	fmt.Println("    EVM Address                              | DEMO Balance")
	fmt.Println("    -----------------------------------------|-------------")

	for _, user := range users {
		fmt.Printf("    %s | %s\n", user.EVMAddress, formatBalance(user.DemoBalance))
	}
}

func printHeader(title string) {
	fmt.Println("======================================================================")
	fmt.Printf("  %s\n", title)
	fmt.Println("======================================================================")
	fmt.Println()
}

func formatBalance(bal string) string {
	if bal == "" {
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

func truncateParty(party string) string {
	if party == "" {
		return "(none)"
	}
	if len(party) > 40 {
		return party[:30] + "..."
	}
	return party
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
