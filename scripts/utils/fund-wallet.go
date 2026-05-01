//go:build ignore
// +build ignore

// Fund Address Script
//
// Interactively mints DEMO and PROMPT tokens to a registered EVM address.
// The address must already exist in the database (user must be registered).
// Reads all config values automatically from the running Docker stack.
//
// Usage:
//   go run scripts/utils/fund-address.go

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	cantontoken "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	containerName = "erc20-api-server"
	resolvedCfg   = "/app/state/api-server-config.yaml"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Fund Address — Mint DEMO & PROMPT tokens")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	cfg, err := loadConfigFromDocker()
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

// loadConfigFromDocker pulls config values from the running Docker stack:
// - env vars (DATABASE_URL, auth credentials) from the api-server container
// - resolved YAML (domain_id, issuer_party) from the container's state volume
// Docker-internal hostnames are rewritten to localhost for host-side access.
func loadConfigFromDocker() (*config.APIServer, error) {
	// 1. Read env vars from the running container and set them locally so
	//    config.LoadAPIServer can expand ${VAR} placeholders via os.ExpandEnv.
	envOut, err := exec.Command("docker", "inspect", containerName,
		"--format", "{{range .Config.Env}}{{println .}}{{end}}").Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s failed: %w\n(is the Docker stack running?)", containerName, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(envOut)), "\n") {
		if idx := strings.IndexByte(line, '='); idx > 0 {
			os.Setenv(line[:idx], line[idx+1:])
		}
	}

	// Fix DATABASE_URL: container uses postgres hostname, host machine uses localhost
	if dbURL := os.Getenv("API_SERVER_DATABASE_URL"); dbURL != "" {
		os.Setenv("API_SERVER_DATABASE_URL", strings.ReplaceAll(dbURL, "@postgres:", "@localhost:"))
	}

	// 2. Read the bootstrap-resolved config YAML from the shared state volume.
	//    This file already has domain_id and issuer_party substituted by the
	//    bootstrap script; the remaining ${VAR} placeholders are expanded by
	//    config.LoadAPIServer via os.ExpandEnv using the env vars set above.
	yamlOut, err := exec.Command("docker", "exec", containerName, "cat", resolvedCfg).Output()
	if err != nil {
		return nil, fmt.Errorf("docker exec cat %s failed: %w", resolvedCfg, err)
	}

	// Rewrite docker-internal service hostnames to localhost for host access.
	resolved := string(yamlOut)
	resolved = strings.ReplaceAll(resolved, "canton:5011", "localhost:5011")
	resolved = strings.ReplaceAll(resolved, "mock-oauth2:8088", "localhost:8088")

	// 3. Write patched YAML to a temp file and load via the standard loader.
	tmp, err := os.CreateTemp("", "fund-addr-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(resolved); err != nil {
		return nil, err
	}
	tmp.Close()

	return config.LoadAPIServer(tmp.Name())
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
