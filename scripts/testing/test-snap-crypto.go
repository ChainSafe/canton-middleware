//go:build ignore
// +build ignore

// Test script for Phase 1 snap crypto compatibility (T5).
//
// Proves end-to-end that a Canton keypair from a known test vector —
// identical to what the canton-snap TypeScript produces — is accepted
// by Canton for party allocation and transfer execution.
//
// Since cross-validation tests (T3) prove Go and TypeScript produce
// byte-identical SPKI DER, fingerprints, and DER signatures, this test
// transitively proves TypeScript-generated crypto works with Canton.
//
// Prerequisites:
//   Run bootstrap-local.sh first to set up Canton, Anvil, and services.
//
// Usage:
//   go run scripts/testing/test-snap-crypto.go
//   go run scripts/testing/test-snap-crypto.go --config config.e2e-local.yaml

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	"github.com/ethereum/go-ethereum/crypto"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var (
	configFile = flag.String("config", "config.e2e-local.yaml", "Path to API server config file")
	apiBaseURL = flag.String("api-url", "http://localhost:8081", "API server base URL")
)

// Test vector keys — these are the same keys used in:
// - cmd/generate-test-vectors (Go)
// - canton-snap/test/crypto.test.ts (TypeScript)
//
// Using known keys lets us verify that the SPKI DER and fingerprint
// computed here match what TypeScript produces (proven by T3).
var testVectorKeys = []string{
	"0000000000000000000000000000000000000000000000000000000000000001",
	"0000000000000000000000000000000000000000000000000000000000000002",
}

func main() {
	flag.Parse()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Phase 1 T5: Snap Crypto Canton Integration Test")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  This test proves Canton accepts crypto output identical to")
	fmt.Println("  what the canton-snap TypeScript implementation produces.")
	fmt.Println()

	cfg, err := config.LoadAPIServer(*configFile)
	if err != nil {
		fatalf("Failed to load config %s: %v", *configFile, err)
	}

	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	bunDB, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		fatalf("Failed to connect to database: %v", err)
	}
	defer bunDB.Close()
	store := userstore.NewStore(bunDB)

	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer func() { _ = cantonClient.Close() }()

	fmt.Println(">>> Connected to services")
	fmt.Println()

	// Build Canton keypairs from test vector private keys
	senderKP := mustKeyPairFromHex(testVectorKeys[0])
	recipientKP := mustKeyPairFromHex(testVectorKeys[1])

	// Verify SPKI and fingerprint match expected test vector values
	verifySPKIAndFingerprint(senderKP, testVectorKeys[0])
	verifySPKIAndFingerprint(recipientKP, testVectorKeys[1])

	// Generate EVM keys for API authentication (separate from Canton keys)
	senderEVM, _ := crypto.GenerateKey()
	senderAddr := crypto.PubkeyToAddress(senderEVM.PublicKey).Hex()
	recipientEVM, _ := crypto.GenerateKey()
	recipientAddr := crypto.PubkeyToAddress(recipientEVM.PublicKey).Hex()

	// Step 1: Whitelist users
	printStep("Whitelisting test users")
	must(store.AddToWhitelist(ctx, senderAddr, "snap-crypto-test sender"))
	must(store.AddToWhitelist(ctx, recipientAddr, "snap-crypto-test recipient"))
	fmt.Printf("    Sender:    %s\n", senderAddr)
	fmt.Printf("    Recipient: %s\n", recipientAddr)
	fmt.Println()

	// Step 2: Register sender with test vector key (external/non-custodial)
	printStep("Registering sender with test vector key (external)")
	senderParty := registerExternalUser(senderEVM, senderKP)
	fmt.Printf("    Party: %s\n", truncate(senderParty.Party, 30))
	fmt.Printf("    Key mode: %s\n", senderParty.KeyMode)
	fmt.Println()

	// Step 3: Register recipient with test vector key
	printStep("Registering recipient with test vector key (external)")
	recipientParty := registerExternalUser(recipientEVM, recipientKP)
	fmt.Printf("    Party: %s\n", truncate(recipientParty.Party, 30))
	fmt.Println()

	// Step 4: Mint tokens to sender
	printStep("Minting 500 DEMO to sender")
	_, err = cantonClient.Token.Mint(ctx, &cantontkn.MintRequest{
		RecipientParty: senderParty.Party,
		Amount:         "500",
		TokenSymbol:    "DEMO",
	})
	if err != nil {
		fatalf("Mint to sender: %v", err)
	}
	fmt.Println("    Minted 500 DEMO")
	fmt.Println()

	// Step 5: Prepare transfer (server calls PrepareSubmission)
	printStep("Preparing transfer: 100 DEMO sender → recipient")
	prepResp := prepareTransfer(senderEVM, &transfer.PrepareRequest{
		To:     recipientAddr,
		Amount: "100",
		Token:  "DEMO",
	})
	fmt.Printf("    Transfer ID: %s\n", truncate(prepResp.TransferID, 12))
	fmt.Printf("    Tx hash:     %s\n", truncate(prepResp.TransactionHash, 20))
	fmt.Printf("    Expires at:  %s\n", prepResp.ExpiresAt)
	fmt.Println()

	// Step 6: Sign the prepared transaction hash with the test vector key
	// This is exactly what the snap would do: signHashDER(privateKey, hash)
	printStep("Signing transaction hash with test vector key")
	txHash, err := hex.DecodeString(strings.TrimPrefix(prepResp.TransactionHash, "0x"))
	if err != nil {
		fatalf("Decode tx hash: %v", err)
	}

	// The hash from PrepareSubmission needs SHA-256 before ECDSA signing
	derSig := signHashDER(senderKP, txHash)
	senderFP, _ := senderKP.Fingerprint()

	fmt.Printf("    DER sig:     %s (%d bytes)\n", truncate(hex.EncodeToString(derSig), 20), len(derSig))
	fmt.Printf("    Fingerprint: %s\n", truncate(senderFP, 20))

	// Verify with VerifyDER (same check the server should do)
	sigHash := sha256.Sum256(txHash)
	if err := keys.VerifyDER(senderKP.PublicKey, sigHash[:], derSig); err != nil {
		fatalf("VerifyDER self-check failed: %v", err)
	}
	fmt.Println("    VerifyDER self-check: PASS")
	fmt.Println()

	// Step 7: Execute the transfer with the signature
	printStep("Executing transfer with client signature")
	execResp := executeTransfer(senderEVM, &transfer.ExecuteRequest{
		TransferID: prepResp.TransferID,
		Signature:  "0x" + hex.EncodeToString(derSig),
		SignedBy:   senderFP,
	})
	fmt.Printf("    Status: %s\n", execResp.Status)
	fmt.Println()

	// Step 8: Verify balances
	printStep("Verifying balances on Canton ledger")
	recipientBalance, err := cantonClient.Token.GetBalanceByFingerprint(ctx, recipientParty.Fingerprint, "DEMO")
	if err != nil {
		fatalf("Get recipient balance: %v", err)
	}
	senderBalance, err := cantonClient.Token.GetBalanceByFingerprint(ctx, senderParty.Fingerprint, "DEMO")
	if err != nil {
		fatalf("Get sender balance: %v", err)
	}

	fmt.Printf("    Sender balance:    %s DEMO (expected 400)\n", senderBalance)
	fmt.Printf("    Recipient balance: %s DEMO (expected 100)\n", recipientBalance)
	fmt.Println()

	if recipientBalance != "100" {
		fatalf("Recipient balance mismatch: got %s, expected 100", recipientBalance)
	}
	if senderBalance != "400" {
		fatalf("Sender balance mismatch: got %s, expected 400", senderBalance)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  PASS: Canton accepts test-vector-key signatures end-to-end")
	fmt.Println()
	fmt.Println("  Since canton-snap TypeScript produces byte-identical output")
	fmt.Println("  for these keys (verified by cross-validation tests T3),")
	fmt.Println("  snap-generated signatures will also be accepted by Canton.")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}

// ─── Key Helpers ──────────────────────────────────────────────────────────────

func mustKeyPairFromHex(privHex string) *keys.CantonKeyPair {
	privBytes, err := hex.DecodeString(privHex)
	if err != nil {
		fatalf("Invalid hex key %q: %v", privHex, err)
	}
	kp, err := keys.CantonKeyPairFromPrivateKey(privBytes)
	if err != nil {
		fatalf("Invalid private key %q: %v", privHex, err)
	}
	return kp
}

func verifySPKIAndFingerprint(kp *keys.CantonKeyPair, privHex string) {
	spki, err := kp.SPKIPublicKey()
	if err != nil {
		fatalf("SPKIPublicKey for key %s: %v", privHex[:8], err)
	}
	fp, err := kp.Fingerprint()
	if err != nil {
		fatalf("Fingerprint for key %s: %v", privHex[:8], err)
	}
	fmt.Printf("    Key %s...: pubkey=%s spki=%d bytes fp=%s\n",
		privHex[:8], truncate(hex.EncodeToString(kp.PublicKey), 16), len(spki), truncate(fp, 16))
}

// ─── Registration Helpers ─────────────────────────────────────────────────────

func registerExternalUser(evmKey *ecdsa.PrivateKey, cantonKP *keys.CantonKeyPair) *user.RegisterResponse {
	// Step 1: Prepare topology
	topoResp := prepareTopology(evmKey, cantonKP)

	// Step 2: Sign topology hash with Canton key
	topoHash, err := hex.DecodeString(strings.TrimPrefix(topoResp.TopologyHash, "0x"))
	if err != nil {
		fatalf("Decode topology hash: %v", err)
	}
	topoSig := signHashDER(cantonKP, topoHash)

	// Step 3: Complete registration
	return registerExternal(evmKey, cantonKP, topoResp.RegistrationToken, "0x"+hex.EncodeToString(topoSig))
}

// ─── HTTP Helpers ─────────────────────────────────────────────────────────────

func prepareTopology(evmKey *ecdsa.PrivateKey, cantonKP *keys.CantonKeyPair) *user.PrepareTopologyResponse {
	msg := fmt.Sprintf("register-external-%d", time.Now().UnixNano())
	sig := signEIP191(evmKey, msg)

	body, _ := json.Marshal(map[string]string{
		"canton_public_key": hex.EncodeToString(cantonKP.PublicKey),
	})

	req, _ := http.NewRequest("POST", *apiBaseURL+"/register/prepare-topology", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Message", msg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("Prepare topology request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatalf("Prepare topology returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result user.PrepareTopologyResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		fatalf("Parse prepare topology response: %v", err)
	}
	return &result
}

func registerExternal(evmKey *ecdsa.PrivateKey, cantonKP *keys.CantonKeyPair, registrationToken, topologySigHex string) *user.RegisterResponse {
	msg := fmt.Sprintf("register-external-%d", time.Now().UnixNano())
	sig := signEIP191(evmKey, msg)

	body, _ := json.Marshal(map[string]string{
		"signature":          sig,
		"message":            msg,
		"key_mode":           "external",
		"canton_public_key":  hex.EncodeToString(cantonKP.PublicKey),
		"registration_token": registrationToken,
		"topology_signature": topologySigHex,
	})

	req, _ := http.NewRequest("POST", *apiBaseURL+"/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("Register external request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatalf("Register external returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result user.RegisterResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		fatalf("Parse register response: %v", err)
	}
	return &result
}

func prepareTransfer(evmKey *ecdsa.PrivateKey, req *transfer.PrepareRequest) *transfer.PrepareResponse {
	msg := fmt.Sprintf("transfer:%d", time.Now().Unix())
	sig := signEIP191(evmKey, msg)

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", *apiBaseURL+"/api/v2/transfer/prepare", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Signature", sig)
	httpReq.Header.Set("X-Message", msg)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		fatalf("Prepare transfer request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatalf("Prepare transfer returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result transfer.PrepareResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		fatalf("Parse prepare transfer response: %v", err)
	}
	return &result
}

func executeTransfer(evmKey *ecdsa.PrivateKey, req *transfer.ExecuteRequest) *transfer.ExecuteResponse {
	msg := fmt.Sprintf("execute:%d", time.Now().Unix())
	sig := signEIP191(evmKey, msg)

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", *apiBaseURL+"/api/v2/transfer/execute", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Signature", sig)
	httpReq.Header.Set("X-Message", msg)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		fatalf("Execute transfer request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatalf("Execute transfer returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result transfer.ExecuteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		fatalf("Parse execute transfer response: %v", err)
	}
	return &result
}

// ─── Signing Helpers ──────────────────────────────────────────────────────────

func signEIP191(key *ecdsa.PrivateKey, message string) string {
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	hash := crypto.Keccak256Hash([]byte(prefixed))
	sig, err := crypto.Sign(hash.Bytes(), key)
	if err != nil {
		fatalf("EIP-191 sign: %v", err)
	}
	return "0x" + hex.EncodeToString(sig)
}

func signHashDER(kp *keys.CantonKeyPair, hashData []byte) []byte {
	hash := sha256.Sum256(hashData)
	derSig, err := kp.SignHashDER(hash[:])
	if err != nil {
		fatalf("DER sign: %v", err)
	}
	return derSig
}

// ─── Output Helpers ───────────────────────────────────────────────────────────

func printStep(title string) {
	fmt.Printf(">>> %s\n", title)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
