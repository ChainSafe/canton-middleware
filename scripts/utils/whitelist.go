//go:build ignore

// whitelist.go — Interactive EVM address whitelisting utility.
//
// Reads all config values automatically from the running Docker stack.
//
// Usage (interactive):
//
//	go run scripts/utils/whitelist.go
//
// Usage (non-interactive):
//
//	go run scripts/utils/whitelist.go -address 0xABCD... -note "operator"

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	"github.com/chainsafe/canton-middleware/scripts/utils/dockerconfig"
	_ "github.com/lib/pq"
)

var (
	addrFlag = flag.String("address", "", "EVM address to whitelist (skips interactive prompt)")
	noteFlag = flag.String("note", "", "Optional note to attach to the whitelist entry")
)

func main() {
	flag.Parse()

	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("  Whitelist Manager")
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println()

	cfg, err := dockerconfig.Load()
	if err != nil {
		fatalf("failed to load config from Docker: %v", err)
	}

	fmt.Println(">>> Connecting to database...")
	bunDB, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		fatalf("failed to connect to database: %v", err)
	}
	defer bunDB.Close()
	store := userstore.NewStore(bunDB)
	fmt.Println("    Connected to PostgreSQL")
	fmt.Println()

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	rawAddr := strings.TrimSpace(*addrFlag)
	rawNote := strings.TrimSpace(*noteFlag)

	if rawAddr == "" {
		fmt.Print("Enter EVM address to whitelist: ")
		line, _ := reader.ReadString('\n')
		rawAddr = strings.TrimSpace(line)
	}
	if rawAddr == "" {
		fatalf("address is required")
	}

	normalized := auth.NormalizeAddress(rawAddr)
	if normalized == "0x0000000000000000000000000000000000000000" {
		fatalf("invalid EVM address: %q", rawAddr)
	}
	if normalized != rawAddr {
		fmt.Printf("  Normalized: %s\n", normalized)
	}

	already, err := store.IsWhitelisted(ctx, normalized)
	if err != nil {
		fatalf("failed to check whitelist: %v", err)
	}

	if err := store.AddToWhitelist(ctx, normalized, rawNote); err != nil {
		fatalf("failed to add to whitelist: %v", err)
	}

	fmt.Println()
	if already {
		fmt.Printf("  Updated:  %s\n", normalized)
	} else {
		fmt.Printf("  Added:    %s\n", normalized)
	}
	if rawNote != "" {
		fmt.Printf("  Note:     %s\n", rawNote)
	}
	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("  Done")
	fmt.Println("════════════════════════════════════════════════════════════════════")
}

func fatalf(format string, args ...any) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
