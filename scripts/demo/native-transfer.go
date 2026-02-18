//go:build ignore
// +build ignore

// Native Transfer Script
//
// Execute a CIP56 token transfer between two Canton parties via the Ledger API.
// This simulates a native Canton user making a transfer without MetaMask.
//
// Usage:
//   go run scripts/demo/native-transfer.go -config config.api-server.mainnet.local.yaml \
//     -from "party1::..." -to "party2::..." -amount "100"
//
// Flags:
//   -config   Path to config file
//   -from     Sender Canton party ID
//   -to       Recipient Canton party ID
//   -amount   Amount to transfer
//   -token    Token symbol (default: DEMO)
//   -verbose  Show detailed output

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.mainnet.local.yaml", "Path to config file")
	fromParty  = flag.String("from", "", "Sender Canton party ID (required)")
	toParty    = flag.String("to", "", "Recipient Canton party ID (required)")
	amount     = flag.String("amount", "", "Amount to transfer (required)")
	token      = flag.String("token", "DEMO", "Token symbol")
	verbose    = flag.Bool("verbose", false, "Show detailed output")
)

func main() {
	flag.Parse()

	// Validate required flags
	if *fromParty == "" || *toParty == "" || *amount == "" {
		fmt.Println("ERROR: -from, -to, and -amount are required")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  go run scripts/demo/native-transfer.go -config config.yaml \\")
		fmt.Println("    -from \"sender_party::...\" -to \"recipient_party::...\" -amount \"100\"")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Determine network
	networkName := detectNetwork(cfg.Canton.RPCURL)

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Native Canton Transfer - %s\n", networkName)
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Network:  %s\n", networkName)
	fmt.Printf("  Endpoint: %s\n", cfg.Canton.RPCURL)
	fmt.Println()
	fmt.Println("  Transfer Details:")
	fmt.Printf("    From:   %s\n", truncateParty(*fromParty))
	fmt.Printf("    To:     %s\n", truncateParty(*toParty))
	fmt.Printf("    Amount: %s %s\n", *amount, *token)
	fmt.Println()

	// Create logger
	var logger *zap.Logger
	if *verbose {
		logger, _ = zap.NewDevelopment()
	} else {
		logConfig := zap.NewProductionConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
		logger, _ = logConfig.Build()
	}

	// Connect to Canton
	fmt.Println(">>> Connecting to Canton...")
	cantonClient, err := canton.NewFromAppConfig(context.Background(), &cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer cantonClient.Close()
	fmt.Printf("    ✓ Connected to %s\n", networkName)
	fmt.Println()

	ctx := context.Background()

	// Show Ledger API details for demo purposes
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Ledger API Command Details")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  gRPC Service:    CommandSubmissionService")
	fmt.Println("  gRPC Method:     SubmitAndWait")
	fmt.Println("  DAML Template:   CIP56:CIP56Holding")
	fmt.Println("  DAML Choice:     Transfer")
	fmt.Println()
	fmt.Println("  Choice Arguments:")
	fmt.Printf("    newOwner:      %s\n", *toParty)
	fmt.Printf("    amount:        %s\n", *amount)
	fmt.Println()
	fmt.Printf("  Act As Party:    %s\n", truncateParty(*fromParty))
	fmt.Println()

	// Execute transfer
	fmt.Println(">>> Submitting command to Canton Ledger API...")
	err = cantonClient.Token.TransferByPartyID(ctx, *fromParty, *toParty, *amount, *token)
	if err != nil {
		if strings.Contains(err.Error(), "PermissionDenied") {
			fmt.Println()
			fmt.Println("    ERROR: PermissionDenied")
			fmt.Println()
			fmt.Println("    The OAuth client does not have ActAs rights for the sender party.")
			fmt.Println("    This can happen if the party was created outside this session.")
			fmt.Println()
			fmt.Println("    Try granting CanActAs rights first:")
			fmt.Println("      go run scripts/remote/grant-any-party-rights.go ...")
			fmt.Println()
			os.Exit(1)
		}
		fmt.Printf("ERROR: Transfer failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("    ✓ Command accepted by Canton sequencer")
	fmt.Println("    ✓ Transaction committed to ledger")
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  ✓ Ledger API Transfer Successful!")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  Result:")
	fmt.Printf("    %s %s transferred via DAML CIP56Holding.Transfer\n", *amount, *token)
	fmt.Printf("    From: %s\n", truncateParty(*fromParty))
	fmt.Printf("    To:   %s\n", truncateParty(*toParty))
	fmt.Println()
	fmt.Println("  Note: This transfer used the Canton Ledger API directly.")
	fmt.Println("        No EVM/MetaMask involvement - pure DAML smart contract execution.")
	fmt.Println()
	fmt.Println("  Verify with:")
	fmt.Printf("    go run scripts/utils/verify-canton-holdings.go -config %s\n", *configPath)
	fmt.Println()
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

func truncateParty(party string) string {
	if party == "" {
		return "(none)"
	}
	if len(party) > 50 {
		return party[:40] + "..."
	}
	return party
}
