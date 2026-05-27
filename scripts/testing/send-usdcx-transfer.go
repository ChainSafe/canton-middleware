//go:build ignore

// send-usdcx-transfer.go — Send USDCx from a standalone external party.
//
// Loads party credentials (the same file format as accept-usdcx-transfer.go),
// wires a Canton SDK Client with a custom KeyResolver that returns the
// credentials' secp256k1 key, then exercises TransferFactory_Transfer for the
// configured external token via the api-server's TransferByPartyID path.
//
// Usage:
//   go run scripts/testing/send-usdcx-transfer.go \
//     -config config.api-server.devnet-test.yaml \
//     -creds ./party-credentials.txt \
//     -to "user_xxx::1220..." \
//     -amount "1" \
//     [-symbol USDCx] [-dry-run]
//
// The config must have token.external_tokens populated for the USDCx instrument
// admin → registry URL (the registry is consulted to discover TransferFactory).

package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config file")
	credsPath  = flag.String("creds", "./party-credentials.txt", "Party credentials file")
	toParty    = flag.String("to", "", "Receiver party ID (required)")
	amount     = flag.String("amount", "1", "Amount to send (decimal string)")
	symbol     = flag.String("symbol", "USDCx", "Token symbol")
	dryRun     = flag.Bool("dry-run", false, "Print plan, list holdings, don't submit")
)

func main() {
	flag.Parse()
	if *toParty == "" {
		fatalf("-to is required")
	}

	creds, err := loadCredentials(*credsPath)
	if err != nil {
		fatalf("load credentials: %v", err)
	}
	fromParty := creds["party_id"]
	if fromParty == "" {
		fatalf("party_id missing from credentials")
	}

	privBytes, err := hex.DecodeString(creds["private_key_hex"])
	if err != nil {
		fatalf("decode private key: %v", err)
	}
	kp, err := keys.CantonKeyPairFromPrivateKey(privBytes)
	if err != nil {
		fatalf("reconstruct keypair: %v", err)
	}
	fp, _ := kp.Fingerprint()
	fmt.Printf(">>> sender party       : %s\n", fromParty)
	fmt.Printf(">>> sender fingerprint : %s\n", fp)
	fmt.Printf(">>> receiver party     : %s\n", *toParty)
	fmt.Printf(">>> amount             : %s %s\n", *amount, *symbol)

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// KeyResolver returns our keypair whenever the SDK asks for the sender's signer.
	keyResolver := func(partyID string) (token.Signer, error) {
		if partyID != fromParty {
			return nil, fmt.Errorf("no key for party %s (only %s available)", partyID, fromParty)
		}
		return kp, nil
	}

	cantonClient, err := canton.New(ctx, cfg.Canton,
		canton.WithLogger(logger),
		canton.WithKeyResolver(keyResolver),
	)
	if err != nil {
		fatalf("canton client: %v", err)
	}
	defer func() { _ = cantonClient.Close() }()

	// Diagnostic: list current holdings so we can confirm the sender has at least `amount`.
	holdings, err := cantonClient.Token.GetHoldingsByParty(ctx, fromParty, *symbol)
	if err != nil {
		fatalf("get holdings: %v", err)
	}
	fmt.Printf("\n>>> sender holdings for %s: %d contract(s)\n", *symbol, len(holdings))
	for i, h := range holdings {
		fmt.Printf("    #%d  amount=%s  cid=%s  admin=%s\n", i+1, h.Amount, h.ContractID, h.InstrumentAdmin)
	}
	if len(holdings) == 0 {
		fatalf("no %s holdings visible for party %s — cannot send", *symbol, fromParty)
	}

	if *dryRun {
		fmt.Println("\n[dry-run] would call TransferByPartyID — skipping submission")
		return
	}

	idempotencyKey := fmt.Sprintf("send-usdcx-%s", uuid.NewString())
	fmt.Printf("\n>>> submitting TransferFactory_Transfer (commandId=%s)…\n", idempotencyKey)
	t0 := time.Now()
	if err := cantonClient.Token.TransferByPartyID(ctx, idempotencyKey, fromParty, *toParty, *amount, *symbol); err != nil {
		fatalf("TransferByPartyID: %v", err)
	}
	fmt.Printf(">>> SUCCESS in %s\n", time.Since(t0).Round(time.Millisecond))
}

// loadCredentials reads the key=value text file produced by allocate-standalone-party.go.
func loadCredentials(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		out[k] = v
	}
	return out, scanner.Err()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
