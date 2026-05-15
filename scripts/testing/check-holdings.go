//go:build ignore

// check-holdings.go — Query Splice HoldingV1 holdings directly for any party ID.
//
// Usage:
//   go run scripts/testing/check-holdings.go \
//     -config config.api-server.devnet-test.yaml \
//     -party "user_f39Fd6e5::1220d7dca32461837f5507effa024b31e5cd2119c23e7581f465c55fb7257761beb5"

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"

	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Path to API server config file")
	partyID    = flag.String("party", "", "Canton party ID to query (required)")
	instrument = flag.String("instrument", "", "Instrument ID filter (empty = all tokens)")
)

func main() {
	flag.Parse()

	if *partyID == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -party flag is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Query Holdings by Party (Splice HoldingV1)")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:     %s\n", cfg.Canton.Ledger.RPCURL)
	fmt.Printf("  Party:      %s\n", *partyID)
	fmt.Printf("  Instrument: %s\n", valueOrAll(*instrument))
	fmt.Println()

	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer func() { _ = cantonClient.Close() }()

	holdings, err := cantonClient.Token.GetHoldingsByParty(ctx, *partyID, *instrument)
	if err != nil {
		fatalf("GetHoldingsByParty failed: %v", err)
	}

	fmt.Printf(">>> Found %d holding(s)\n\n", len(holdings))

	if len(holdings) == 0 {
		fmt.Println("No holdings found for this party.")
		return
	}

	for i, h := range holdings {
		fmt.Printf("Holding #%d:\n", i+1)
		fmt.Printf("  ContractID:      %s\n", h.ContractID)
		fmt.Printf("  Owner:           %s\n", h.Owner)
		fmt.Printf("  Issuer:          %s\n", h.Issuer)
		fmt.Printf("  InstrumentAdmin: %s\n", h.InstrumentAdmin)
		fmt.Printf("  InstrumentID:    %s\n", h.InstrumentID)
		fmt.Printf("  Amount:          %s\n", h.Amount)
		fmt.Printf("  Symbol:          %s\n", h.Symbol)
		fmt.Printf("  Locked:          %v\n", h.Locked)
		fmt.Println()
	}
}

func valueOrAll(v string) string {
	if v == "" {
		return "(all)"
	}
	return v
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
