//go:build ignore

// Fund Address Script
//
// Interactively mints DEMO and PROMPT tokens to a registered EVM address.
// The address must already exist in the database (user must be registered).
// Reads all config values automatically from the running Docker stack.
//
// Usage:
//   go run scripts/utils/fund-wallet.go

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	cantontoken "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	"github.com/chainsafe/canton-middleware/scripts/utils/dockerconfig"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Fund Address — Mint DEMO & PROMPT tokens")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	cfg, err := dockerconfig.Load()
	if err != nil {
		fatalf("failed to load config from Docker: %v", err)
	}

	logger, _ := zap.NewDevelopment()

	fmt.Println(">>> Connecting to services...")
	bunDB, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		fatalf("failed to connect to database: %v", err)
	}
	defer bunDB.Close()

	uStore := userstore.NewStore(bunDB)
	fmt.Println("    Connected to PostgreSQL")

	ctx := context.Background()

	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("failed to connect to Canton: %v", err)
	}
	defer cantonClient.Close()
	fmt.Println("    Connected to Canton Ledger API")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Prompt for EVM address
	fmt.Print("Enter EVM address: ")
	evmAddress, _ := reader.ReadString('\n')
	evmAddress = auth.NormalizeAddress(strings.TrimSpace(evmAddress))
	if evmAddress == "" {
		fatalf("EVM address is required")
	}

	// Resolve user from database
	usr, err := uStore.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		fatalf("user not found for address %s: %v\n(make sure the user is registered via the API first)", evmAddress, err)
	}

	partyID := usr.CantonPartyID
	fmt.Printf("\nResolved Canton party: %s\n\n", truncate(partyID, 60))

	// Fetch current balances and total supply for both tokens
	tokens := []string{"DEMO", "PROMPT"}
	balances := make(map[string]string)
	supplies := make(map[string]string)

	fmt.Println(">>> Fetching current balances and total supply...")
	for _, sym := range tokens {
		bal, err := cantonClient.Token.GetBalanceByPartyID(ctx, partyID, sym)
		if err != nil {
			fmt.Printf("    WARN: could not fetch %s balance: %v\n", sym, err)
			bal = "0"
		}
		balances[sym] = bal

		sup, err := cantonClient.Token.GetTotalSupply(ctx, sym)
		if err != nil {
			fmt.Printf("    WARN: could not fetch %s total supply: %v\n", sym, err)
			sup = "unknown"
		}
		supplies[sym] = sup
	}

	fmt.Println()
	fmt.Println("  Current state:")
	for _, sym := range tokens {
		fmt.Printf("    %s — balance: %s  |  total supply: %s\n", sym, balances[sym], supplies[sym])
	}
	fmt.Println()

	// Prompt for amount
	fmt.Print("Enter amount to mint (e.g. 500): ")
	amountStr, _ := reader.ReadString('\n')
	amountStr = strings.TrimSpace(amountStr)
	if amountStr == "" {
		fatalf("amount is required")
	}

	// Mint both tokens
	fmt.Println()
	fmt.Println(">>> Minting tokens...")
	for _, sym := range tokens {
		fmt.Printf("    Minting %s %s to %s...\n", amountStr, sym, evmAddress)
		_, err := cantonClient.Token.Mint(ctx, &cantontoken.MintRequest{
			RecipientParty: partyID,
			Amount:         amountStr,
			TokenSymbol:    sym,
		})
		if err != nil {
			fmt.Printf("    ERROR: failed to mint %s: %v\n", sym, err)
		} else {
			fmt.Printf("    Minted %s %s\n", amountStr, sym)
		}
	}

	// Show updated balances
	fmt.Println()
	fmt.Println(">>> Updated balances:")
	for _, sym := range tokens {
		bal, err := cantonClient.Token.GetBalanceByPartyID(ctx, partyID, sym)
		if err != nil {
			fmt.Printf("    %s: (could not fetch: %v)\n", sym, err)
			continue
		}
		fmt.Printf("    %s: %s\n", sym, bal)
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Done")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func fatalf(format string, args ...any) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
