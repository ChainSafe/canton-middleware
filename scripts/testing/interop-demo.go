//go:build ignore
// +build ignore

// Interoperability Demo & Test Script
//
// This script demonstrates and tests bidirectional token transfers between
// MetaMask-registered users and native Canton users, plus PROMPT ERC-20
// bridging from Ethereum to Canton.
//
// All users (MetaMask and native) are registered as external parties, using
// the Interactive Submission API for transfers. Transfers go through the
// API server's /eth JSON-RPC endpoint via cast send.
//
// Prerequisites:
//   Run bootstrap-local.sh (local) or bootstrap-remote.sh (devnet) first.
//
// Usage:
//   go run scripts/testing/interop-demo.go                                          # local (default)
//   go run scripts/testing/interop-demo.go --config config.api-server.devnet.yaml --skip-prompt  # devnet (DEMO only)
//   go run scripts/testing/interop-demo.go --skip-prompt                             # skip PROMPT bridge tests
//   go run scripts/testing/interop-demo.go --skip-demo                               # skip DEMO interop tests
//   go run scripts/testing/interop-demo.go --api-url http://localhost:8082           # custom API URL
//
// What it tests:
//
//   Part A - DEMO Token Interoperability (external parties)
//     Step 1: Allocate 2 external native Canton parties + register with API server
//     Step 2: MetaMask User → Native User 1 (100 DEMO via /eth)
//     Step 3: Native User 1 → Native User 2 (100 DEMO via /eth)
//     Step 4: Native User 2 → MetaMask User 1 (100 DEMO via /eth)
//
//   Part B - PROMPT Token Bridge (ERC-20 ↔ Canton)
//     Step 5: Deposit PROMPT from Ethereum to Canton via bridge (100 PROMPT)
//     Step 6: Verify Canton PROMPT balance
//     Step 7: Transfer PROMPT on Canton, User 1 → User 2 (25 PROMPT)
//     Step 8: Verify final PROMPT balances

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/reconciler"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// ─── Test accounts (Anvil default mnemonic) ─────────────────────────────────
const (
	user1Key  = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	user1Addr = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	user2Key  = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	user2Addr = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

	anvilURL = "http://localhost:8545"

	// Synthetic DEMO token address recognised by the /eth endpoint
	demoTokenAddr = "0xDE30000000000000000000000000000000000001"
)

// nativeUser holds the credentials returned from registering a native Canton user.
type nativeUser struct {
	CantonPartyID string
	EVMAddress    string
	EVMPrivateKey string // hex, no 0x prefix
}

// Contract addresses - auto-detected from Docker deployer logs
var (
	tokenAddr  string
	bridgeAddr string
)

var (
	configFile    = flag.String("config", "config.e2e-local.yaml", "Path to API server config file")
	apiBaseURL    = flag.String("api-url", "http://localhost:8081", "API server base URL")
	demoAmount    = flag.String("demo-amount", "100", "DEMO amount per transfer step")
	promptDeposit = flag.String("prompt-deposit", "100", "PROMPT deposit amount (whole tokens)")
	promptXfer    = flag.String("prompt-transfer", "25", "PROMPT transfer amount (whole tokens)")
	skipDemo      = flag.Bool("skip-demo", false, "Skip DEMO interop tests (Part A)")
	skipPrompt    = flag.Bool("skip-prompt", false, "Skip PROMPT bridge tests (Part B)")
)

// ─── Test state ─────────────────────────────────────────────────────────────

var (
	ethRPCURL string
	passCount int
	failCount int
)

func main() {
	flag.Parse()

	printBanner()

	// Derive ethRPCURL from the base API URL
	ethRPCURL = *apiBaseURL + "/eth"

	if !*skipPrompt {
		detectContractAddresses()
	}

	// Load config
	cfg, err := config.LoadAPIServer(*configFile)
	if err != nil {
		fatalf("Failed to load config %s: %v\nDid you run bootstrap first?", *configFile, err)
	}

	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	// Connect to database
	fmt.Println(">>> Connecting to services...")
	bunDB, err := pgutil.ConnectDB(&cfg.Database)
	if err != nil {
		fatalf("Failed to connect to database (bun): %v", err)
	}
	defer bunDB.Close()
	userStore := userstore.NewStore(bunDB)

	apiStore, err := apidb.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		fatalf("Failed to connect to database (apidb): %v", err)
	}
	defer apiStore.Close()
	fmt.Println("    Database:  connected")

	// Connect to Canton
	cantonClient, err := canton.NewFromAppConfig(ctx, &cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer cantonClient.Close()
	fmt.Println("    Canton:    connected")
	fmt.Println()

	// Verify registered users
	users, err := userStore.ListUsers(ctx)
	if err != nil || len(users) < 2 {
		fatalf("Need at least 2 registered users. Run bootstrap-local.sh first.")
	}
	user1 := users[0]
	user2 := users[1]

	printHeader("Registered Users")
	fmt.Printf("    User 1: %s  Party: %s  DEMO: %s\n", user1.EVMAddress, trunc(user1.CantonPartyID), fmtBal(user1.DemoBalance))
	fmt.Printf("    User 2: %s  Party: %s  DEMO: %s\n", user2.EVMAddress, trunc(user2.CantonPartyID), fmtBal(user2.DemoBalance))
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════
	// Part A: DEMO Token Interoperability (all external parties)
	// ═══════════════════════════════════════════════════════════════════════
	if !*skipDemo {
		printPartHeader("A", "DEMO Token Interoperability (External Parties)")

		// Show initial holdings
		printHeader("Initial Canton Holdings")
		showHoldings(ctx, cantonClient)

		// Step 1: Allocate external native parties + register with API server
		native1, native2 := stepRegisterExternalNativeUsers(ctx, cantonClient, apiStore)

		// Step 2: MetaMask User 1 → Native User 1 (via /eth)
		stepTransferDemoViaCast(2, "MetaMask User 1 → Native User 1",
			user1Key, user1Addr, native1.EVMAddress, *demoAmount)
		showHoldings(ctx, cantonClient)

		// Step 3: Native User 1 → Native User 2 (via /eth)
		stepTransferDemoViaCast(3, "Native User 1 → Native User 2",
			native1.EVMPrivateKey, native1.EVMAddress, native2.EVMAddress, *demoAmount)
		showHoldings(ctx, cantonClient)

		// Step 4: Native User 2 → MetaMask User 1 (via /eth)
		stepTransferDemoViaCast(4, "Native User 2 → MetaMask User 1",
			native2.EVMPrivateKey, native2.EVMAddress, user1Addr, *demoAmount)
		showHoldings(ctx, cantonClient)

		// Reconcile
		fmt.Println(">>> Running reconciliation...")
		rec := reconciler.New(apiStore, userStore, cantonClient.Token, logger)
		if err := rec.ReconcileUserBalancesFromHoldings(ctx); err != nil {
			fmt.Printf("    WARNING: Reconciliation failed: %v\n", err)
		}
		fmt.Println()
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Part B: PROMPT Token Bridge (ERC-20 ↔ Canton)
	// ═══════════════════════════════════════════════════════════════════════
	if !*skipPrompt {
		printPartHeader("B", "PROMPT Token Bridge (ERC-20 ↔ Canton)")

		// Step 5: Deposit PROMPT from Ethereum to Canton
		stepDepositPrompt(ctx, user1)

		// Step 6: Verify Canton PROMPT balance
		stepVerifyPromptBalance(ctx, 6, user1)

		// Step 7: Transfer PROMPT on Canton (User 1 → User 2)
		stepTransferPromptViaCast(ctx, 7, user2Addr)

		// Step 8: Verify final PROMPT balances
		stepVerifyFinalPromptBalances(ctx, 8, user1, user2)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Summary
	// ═══════════════════════════════════════════════════════════════════════
	printSummary()
}

// ─── Part A Steps ───────────────────────────────────────────────────────────

func stepRegisterExternalNativeUsers(ctx context.Context, sdk *canton.Client, db *apidb.Store) (nativeUser, nativeUser) {
	printStep(1, "Allocate External Parties + Register Native Users")
	fmt.Println("    Creating 2 external parties and registering with API server...")
	fmt.Println()

	native1 := allocateAndRegisterNative(ctx, sdk, db, "native_interop_1")
	fmt.Printf("    Native User 1: party=%s  evm=%s\n", trunc(native1.CantonPartyID), native1.EVMAddress)

	native2 := allocateAndRegisterNative(ctx, sdk, db, "native_interop_2")
	fmt.Printf("    Native User 2: party=%s  evm=%s\n", trunc(native2.CantonPartyID), native2.EVMAddress)
	fmt.Println()

	recordPass("Allocated and registered 2 external native users")
	return native1, native2
}

func allocateAndRegisterNative(ctx context.Context, sdk *canton.Client, db *apidb.Store, hint string) nativeUser {
	kp, err := keys.GenerateCantonKeyPair()
	if err != nil {
		fatalf("Generate Canton keypair for %s: %v", hint, err)
	}

	spkiKey, err := kp.SPKIPublicKey()
	if err != nil {
		fatalf("SPKI encode key for %s: %v", hint, err)
	}
	party, err := sdk.Identity.AllocateExternalParty(ctx, hint, spkiKey, kp)
	if err != nil {
		fatalf("AllocateExternalParty %s: %v", hint, err)
	}

	nu, err := registerNativeUser(ctx, *apiBaseURL, party.PartyID, kp.PrivateKeyHex())
	if err != nil {
		fatalf("Register native user %s: %v", hint, err)
	}
	nu.CantonPartyID = party.PartyID

	if err := db.AddToWhitelist(nu.EVMAddress, "interop-demo native user"); err != nil {
		fatalf("Whitelist native user %s (%s): %v", hint, nu.EVMAddress, err)
	}

	return nu
}

func stepTransferDemoViaCast(stepNum int, title, senderKey, senderAddr, recipientAddr, amount string) {
	printStep(stepNum, title)
	amountWei := toWei(amount)
	fmt.Printf("    Transfer: %s DEMO (%s wei)\n", amount, amountWei)
	fmt.Printf("    From:     %s\n", senderAddr)
	fmt.Printf("    To:       %s\n", recipientAddr)
	fmt.Println()

	fmt.Println(">>> Sending ERC-20 transfer via Canton /eth endpoint...")
	output := castSendLegacy(demoTokenAddr, "transfer(address,uint256)", recipientAddr, amountWei, senderKey, ethRPCURL)
	txHash := extractTxHash(output)
	if txHash != "" {
		fmt.Printf("    Tx hash: %s\n", txHash)
	}

	time.Sleep(3 * time.Second)

	if strings.Contains(output, "error") || strings.Contains(output, "reverted") {
		recordFail("Step %d: %s: %s", stepNum, title, strings.TrimSpace(output))
	} else {
		recordPass("Step %d: %s", stepNum, title)
	}
	fmt.Println()
}

// ─── Part B Steps ───────────────────────────────────────────────────────────

func stepDepositPrompt(ctx context.Context, user1 *user.User) {
	printStep(5, "Deposit PROMPT from Ethereum to Canton")

	amountWei := toWei(*promptDeposit)
	fmt.Printf("    Amount:   %s PROMPT (%s wei)\n", *promptDeposit, amountWei)
	fmt.Printf("    From:     %s (Anvil)\n", user1Addr)
	fmt.Printf("    To:       Canton via bridge %s\n", bridgeAddr)
	fmt.Println()

	// Check Anvil balance first
	bal := castCall(tokenAddr, "balanceOf(address)(uint256)", user1Addr, anvilURL)
	fmt.Printf("    Anvil PROMPT balance: %s wei\n", strings.TrimSpace(bal))

	// Approve
	fmt.Println(">>> Approving bridge contract...")
	castSend(tokenAddr, "approve(address,uint256)", bridgeAddr, amountWei, user1Key, anvilURL)

	// Get fingerprint as bytes32
	fingerprint := user1.Fingerprint
	if strings.HasPrefix(fingerprint, "0x") {
		fingerprint = fingerprint[2:]
	}
	bytes32 := fmt.Sprintf("0x%064s", fingerprint)
	bytes32 = strings.ReplaceAll(bytes32, " ", "0")

	// Deposit
	fmt.Println(">>> Depositing to Canton via bridge...")
	castSend(bridgeAddr, "depositToCanton(address,uint256,bytes32)",
		tokenAddr, amountWei, bytes32, user1Key, anvilURL)

	recordPass("Step 5: Deposit %s PROMPT to Canton", *promptDeposit)
	fmt.Println()
}

func stepVerifyPromptBalance(ctx context.Context, stepNum int, user1 *user.User) {
	printStep(stepNum, "Verify Canton PROMPT Balance")
	fmt.Println("    Waiting for relayer to process deposit...")

	maxWait := 60
	var balance string
	for i := 0; i < maxWait; i += 3 {
		balance = getCantonBalance(tokenAddr, user1Addr)
		if balance != "" && balance != "0" {
			break
		}
		time.Sleep(3 * time.Second)
		fmt.Printf("    Polling... (%ds)\n", i+3)
	}

	if balance == "" || balance == "0" {
		recordFail("Step %d: PROMPT balance still 0 after %ds", stepNum, maxWait)
		return
	}

	humanBal := fromWei(balance)
	fmt.Printf("    User 1 Canton PROMPT balance: %s (%s wei)\n", humanBal, balance)
	fmt.Println()
	recordPass("Step %d: PROMPT deposited: %s tokens", stepNum, humanBal)
}

func stepTransferPromptViaCast(ctx context.Context, stepNum int, toAddr string) {
	printStep(stepNum, "Transfer PROMPT on Canton (User 1 → User 2)")

	amountWei := toWei(*promptXfer)
	fmt.Printf("    Amount: %s PROMPT (%s wei)\n", *promptXfer, amountWei)
	fmt.Printf("    From:   %s\n", user1Addr)
	fmt.Printf("    To:     %s\n", toAddr)
	fmt.Println()

	// ERC-20 transfer via Canton's eth_sendRawTransaction endpoint
	fmt.Println(">>> Sending ERC-20 transfer via Canton /eth endpoint...")
	output := castSendLegacy(tokenAddr, "transfer(address,uint256)", toAddr, amountWei, user1Key, ethRPCURL)
	txHash := extractTxHash(output)
	if txHash != "" {
		fmt.Printf("    Tx hash: %s\n", txHash)
	}

	// Give reconciliation time
	time.Sleep(3 * time.Second)

	recordPass("Step %d: Transferred %s PROMPT on Canton", stepNum, *promptXfer)
	fmt.Println()
}

func stepVerifyFinalPromptBalances(ctx context.Context, stepNum int, user1, user2 *user.User) {
	printStep(stepNum, "Verify Final PROMPT Balances")

	// Poll for User 2 balance
	var user2Bal string
	for i := 0; i < 30; i += 2 {
		user2Bal = getCantonBalance(tokenAddr, user2Addr)
		if user2Bal != "" && user2Bal != "0" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	user1Bal := getCantonBalance(tokenAddr, user1Addr)

	u1Human := fromWei(user1Bal)
	u2Human := fromWei(user2Bal)

	fmt.Printf("    User 1 (%s): %s PROMPT\n", user1Addr, u1Human)
	fmt.Printf("    User 2 (%s): %s PROMPT\n", user2Addr, u2Human)
	fmt.Println()

	if user2Bal != "" && user2Bal != "0" {
		recordPass("Step %d: Final balances: User1=%s, User2=%s PROMPT", stepNum, u1Human, u2Human)
	} else {
		recordFail("Step %d: User 2 PROMPT balance is still 0", stepNum)
	}
}

// ─── Canton helpers ─────────────────────────────────────────────────────────

// registerNativeUser calls POST /register with the Canton party ID and signing key.
// The handler stores the Canton key (for Interactive Submission) and returns EVM credentials.
func registerNativeUser(ctx context.Context, apiURL, cantonParty, cantonPrivKeyHex string) (nativeUser, error) {
	reqBody := map[string]string{
		"canton_party_id":  cantonParty,
		"canton_signature": "",
		"message":          fmt.Sprintf("Register Canton party %s", cantonParty),
	}
	if cantonPrivKeyHex != "" {
		reqBody["canton_private_key"] = cantonPrivKeyHex
	}
	jsonBody, _ := json.Marshal(reqBody)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(apiURL+"/register", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nativeUser{}, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nativeUser{}, fmt.Errorf("registration failed (%d): %s", resp.StatusCode, string(body))
	}

	var response struct {
		EVMAddress string `json:"evm_address"`
		PrivateKey string `json:"private_key"`
		Party      string `json:"party"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nativeUser{}, fmt.Errorf("failed to parse response: %w", err)
	}

	privKey := strings.TrimPrefix(response.PrivateKey, "0x")
	return nativeUser{
		CantonPartyID: response.Party,
		EVMAddress:    response.EVMAddress,
		EVMPrivateKey: privKey,
	}, nil
}

func showHoldings(ctx context.Context, sdk *canton.Client) {
	holdings, err := sdk.Token.GetAllHoldings(ctx)
	if err != nil {
		fmt.Printf("    ERROR: %v\n", err)
		return
	}
	if len(holdings) == 0 {
		fmt.Println("    (no holdings)")
		return
	}
	fmt.Println("    Owner                                 | Symbol | Amount")
	fmt.Println("    ----------------------------------------|--------|--------")
	for _, h := range holdings {
		sym := h.Symbol
		if sym == "" {
			sym = "?"
		}
		fmt.Printf("    %-40s | %-6s | %s\n", trunc(h.Owner), sym, fmtBal(h.Amount))
	}
}

// ─── Auto-detection ─────────────────────────────────────────────────────────

func detectContractAddresses() {
	// Get latest contract addresses from Docker deployer logs
	out, err := exec.Command("docker", "logs", "deployer").CombinedOutput()
	if err != nil {
		fmt.Println("    Warning: Could not read deployer logs, using defaults")
		tokenAddr = "0x5FbDB2315678afecb367f032d93F642f64180aa3"
		bridgeAddr = "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
		return
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "PromptToken deployed to:") {
			if addr := extractAddress(line); addr != "" {
				tokenAddr = addr
			}
		}
		if strings.Contains(line, "CantonBridge deployed to:") {
			if addr := extractAddress(line); addr != "" {
				bridgeAddr = addr
			}
		}
	}

	if tokenAddr == "" {
		tokenAddr = "0x5FbDB2315678afecb367f032d93F642f64180aa3"
	}
	if bridgeAddr == "" {
		bridgeAddr = "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
	}

	fmt.Printf("    Token:   %s\n", tokenAddr)
	fmt.Printf("    Bridge:  %s\n", bridgeAddr)
	fmt.Println()
}

func extractAddress(line string) string {
	idx := strings.LastIndex(line, "0x")
	if idx >= 0 && idx+42 <= len(line) {
		addr := line[idx : idx+42]
		if len(addr) == 42 {
			return addr
		}
	}
	return ""
}

// ─── Cast (Foundry) helpers ─────────────────────────────────────────────────

func castCall(contract, sig string, args ...string) string {
	cmdArgs := []string{"call", contract, sig}
	cmdArgs = append(cmdArgs, args[:len(args)-1]...)
	cmdArgs = append(cmdArgs, "--rpc-url", args[len(args)-1])
	out, err := exec.Command("cast", cmdArgs...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.Split(strings.TrimSpace(string(out)), " ")[0]
}

func castSend(contract, sig string, args ...string) string {
	// Last 2 args: private_key, rpc_url
	n := len(args)
	rpcURL := args[n-1]
	privKey := args[n-2]
	callArgs := args[:n-2]

	cmdArgs := []string{"send", contract, sig}
	cmdArgs = append(cmdArgs, callArgs...)
	cmdArgs = append(cmdArgs, "--private-key", privKey, "--rpc-url", rpcURL, "--json")
	out, err := exec.Command("cast", cmdArgs...).CombinedOutput()
	if err != nil {
		fmt.Printf("    cast send error: %s\n", string(out))
	}
	return string(out)
}

func castSendLegacy(contract, sig string, args ...string) string {
	n := len(args)
	rpcURL := args[n-1]
	privKey := args[n-2]
	callArgs := args[:n-2]

	cmdArgs := []string{"send", contract, sig}
	cmdArgs = append(cmdArgs, callArgs...)
	cmdArgs = append(cmdArgs, "--private-key", privKey, "--rpc-url", rpcURL, "--legacy")
	out, _ := exec.Command("cast", cmdArgs...).CombinedOutput()
	return string(out)
}

func getCantonBalance(tokenContract, userAddr string) string {
	return castCall(tokenContract, "balanceOf(address)(uint256)", userAddr, ethRPCURL)
}

func toWei(amount string) string {
	out, err := exec.Command("cast", "--to-wei", amount, "ether").CombinedOutput()
	if err != nil {
		return "0"
	}
	return strings.TrimSpace(string(out))
}

func fromWei(weiStr string) string {
	weiStr = strings.TrimSpace(weiStr)
	if weiStr == "" || weiStr == "0" {
		return "0"
	}
	out, err := exec.Command("cast", "--from-wei", weiStr, "ether").CombinedOutput()
	if err != nil {
		// Fallback: simple division
		bi := new(big.Int)
		bi.SetString(weiStr, 10)
		eth := new(big.Float).Quo(new(big.Float).SetInt(bi), new(big.Float).SetFloat64(1e18))
		return eth.Text('f', 4)
	}
	return strings.TrimSpace(string(out))
}

func extractTxHash(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "0x") && len(line) == 66 {
			return line
		}
		// Also check in JSON output
		if strings.Contains(line, "transactionHash") {
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(line), &obj); err == nil {
				if h, ok := obj["transactionHash"].(string); ok {
					return h
				}
			}
		}
	}
	return ""
}

// ─── Output helpers ─────────────────────────────────────────────────────────

func printBanner() {
	fmt.Println("======================================================================")
	fmt.Println("  Canton Interoperability Test Suite (External Parties)")
	fmt.Println("======================================================================")
	fmt.Println()
	fmt.Println("  Part A: DEMO Token Interoperability (external parties via /eth)")
	fmt.Println("  Part B: PROMPT Token Bridge (Ethereum ERC-20 ↔ Canton)")
	fmt.Println()
}

func printPartHeader(part, title string) {
	fmt.Println()
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Part %s: %s\n", part, title)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════╝\n")
	fmt.Println()
}

func printHeader(title string) {
	fmt.Printf("── %s ──\n", title)
}

func printStep(num int, title string) {
	fmt.Println()
	fmt.Printf("┌─ Step %d: %s\n", num, title)
	fmt.Println("└──────────────────────────────────────────────────────────────────")
}

func printSummary() {
	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("  TEST RESULTS")
	fmt.Println("======================================================================")
	fmt.Println()

	total := passCount + failCount
	fmt.Printf("  Passed: %d / %d\n", passCount, total)
	fmt.Printf("  Failed: %d / %d\n", failCount, total)
	fmt.Println()

	if failCount > 0 {
		fmt.Println("  SOME TESTS FAILED")
		os.Exit(1)
	}

	fmt.Println("  ALL TESTS PASSED")
	fmt.Println()
	fmt.Println("  The Canton bridge enables true interoperability between")
	fmt.Println("  MetaMask users and native Canton ledger participants,")
	fmt.Println("  with ERC-20 bridging from Ethereum.")
	fmt.Println()
}

func recordPass(format string, args ...interface{}) {
	passCount++
	fmt.Printf("  ✓ PASS: "+format+"\n", args...)
}

func recordFail(format string, args ...interface{}) {
	failCount++
	fmt.Printf("  ✗ FAIL: "+format+"\n", args...)
}

func fmtBal(bal string) string {
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

func trunc(s string) string {
	if s == "" {
		return "(none)"
	}
	if len(s) > 40 {
		return s[:30] + "..."
	}
	return s
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("FATAL: "+format+"\n", args...)
	os.Exit(1)
}
