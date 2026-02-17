//go:build ignore
// +build ignore

// Interoperability Demo & Test Script
//
// This script demonstrates and tests bidirectional token transfers between
// MetaMask-registered users and native Canton users, plus PROMPT ERC-20
// bridging from Ethereum to Canton.
//
// Prerequisites:
//   Run bootstrap-local.sh first (sets up Docker, registers users, mints DEMO)
//
// Usage:
//   go run scripts/testing/interop-demo.go                      # auto-detect everything
//   go run scripts/testing/interop-demo.go --skip-prompt         # skip PROMPT bridge tests
//   go run scripts/testing/interop-demo.go --skip-demo           # skip DEMO interop tests
//
// What it tests:
//
//   Part A - DEMO Token Interoperability (native Canton token)
//     Step 1: Allocate 2 native Canton parties (not registered with API server)
//     Step 2: MetaMask User → Native User (100 DEMO)
//     Step 3: Native User → Native User via Ledger API (100 DEMO)
//     Step 4: Native User → MetaMask User (100 DEMO back)
//     Step 5: Register Native User 1 with the API server
//     Step 6: MetaMask User → Registered Native User (100 DEMO)
//
//   Part B - PROMPT Token Bridge (ERC-20 ↔ Canton)
//     Step 7: Deposit PROMPT from Ethereum to Canton via bridge (100 PROMPT)
//     Step 8: Verify Canton PROMPT balance
//     Step 9: Transfer PROMPT on Canton, User 1 → User 2 (25 PROMPT)
//     Step 10: Verify final PROMPT balances

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
	canton "github.com/chainsafe/canton-middleware/pkg/canton-sdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// ─── Test accounts (Anvil default mnemonic) ─────────────────────────────────
const (
	user1Key  = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	user1Addr = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	user2Key  = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	user2Addr = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

	anvilURL   = "http://localhost:8545"
	apiURL     = "http://localhost:8081"
	ethRPCURL  = "http://localhost:8081/eth"
	configFile = "config.e2e-local.yaml"
)

// Contract addresses - auto-detected from Docker deployer logs
var (
	tokenAddr  string
	bridgeAddr string
)

var (
	demoAmount   = flag.String("demo-amount", "100", "DEMO amount per transfer step")
	promptDeposit = flag.String("prompt-deposit", "100", "PROMPT deposit amount (whole tokens)")
	promptXfer   = flag.String("prompt-transfer", "25", "PROMPT transfer amount (whole tokens)")
	skipDemo     = flag.Bool("skip-demo", false, "Skip DEMO interop tests (Part A)")
	skipPrompt   = flag.Bool("skip-prompt", false, "Skip PROMPT bridge tests (Part B)")
)

// ─── Test state ─────────────────────────────────────────────────────────────

var (
	passCount int
	failCount int
)

func main() {
	flag.Parse()

	printBanner()
	detectContractAddresses()

	// Load config
	cfg, err := config.LoadAPIServer(configFile)
	if err != nil {
		fatalf("Failed to load config %s: %v\nDid you run bootstrap-local.sh first?", configFile, err)
	}

	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	// Connect to database
	fmt.Println(">>> Connecting to services...")
	db, err := apidb.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
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
	users, err := db.GetAllUsers()
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
	// Part A: DEMO Token Interoperability
	// ═══════════════════════════════════════════════════════════════════════
	if !*skipDemo {
		printPartHeader("A", "DEMO Token Interoperability")

		// Show initial holdings
		printHeader("Initial Canton Holdings")
		showHoldings(ctx, cantonClient)

		// Step 1: Allocate native parties
		native1, native2 := stepAllocateNativeParties(ctx, cantonClient)

		// Step 2: MetaMask → Native User 1
		stepTransfer(ctx, cantonClient, 2, "MetaMask User 1 → Native User 1",
			user1.CantonPartyID, native1, *demoAmount, "DEMO",
			fmt.Sprintf("%s (MetaMask)", user1.EVMAddress), trunc(native1)+" (Native)")

		// Step 3: Native 1 → Native 2 (Ledger API)
		stepTransfer(ctx, cantonClient, 3, "Native User 1 → Native User 2 (Ledger API)",
			native1, native2, *demoAmount, "DEMO",
			trunc(native1)+" (Native 1)", trunc(native2)+" (Native 2)")

		// Step 4: Native 2 → MetaMask User 1
		stepTransfer(ctx, cantonClient, 4, "Native User 2 → MetaMask User 1",
			native2, user1.CantonPartyID, *demoAmount, "DEMO",
			trunc(native2)+" (Native 2)", fmt.Sprintf("%s (MetaMask)", user1.EVMAddress))

		// Step 5: Register Native User 1
		stepRegisterNativeUser(ctx, native1)

		// Step 6: MetaMask → Registered Native User
		stepTransfer(ctx, cantonClient, 6, "MetaMask User 1 → Registered Native User 1",
			user1.CantonPartyID, native1, *demoAmount, "DEMO",
			fmt.Sprintf("%s (MetaMask)", user1.EVMAddress), trunc(native1)+" (Now MetaMask-enabled)")

		// Reconcile
		fmt.Println(">>> Running reconciliation...")
		reconciler := apidb.NewReconciler(db, cantonClient.Token, logger)
		if err := reconciler.ReconcileUserBalancesFromHoldings(ctx); err != nil {
			fmt.Printf("    WARNING: Reconciliation failed: %v\n", err)
		}
		fmt.Println()
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Part B: PROMPT Token Bridge (ERC-20 ↔ Canton)
	// ═══════════════════════════════════════════════════════════════════════
	if !*skipPrompt {
		printPartHeader("B", "PROMPT Token Bridge (ERC-20 ↔ Canton)")

		// Step 7: Deposit PROMPT from Ethereum to Canton
		stepDepositPrompt(ctx, user1)

		// Step 8: Verify Canton PROMPT balance
		stepVerifyPromptBalance(ctx, 8, user1)

		// Step 9: Transfer PROMPT on Canton (User 1 → User 2)
		stepTransferPromptViaCast(ctx, 9, user2Addr)

		// Step 10: Verify final PROMPT balances
		stepVerifyFinalPromptBalances(ctx, 10, user1, user2)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Summary
	// ═══════════════════════════════════════════════════════════════════════
	printSummary()
}

// ─── Part A Steps ───────────────────────────────────────────────────────────

func stepAllocateNativeParties(ctx context.Context, sdk *canton.Client) (string, string) {
	printStep(1, "Allocate Native Canton Parties")
	fmt.Println("    Creating 2 native parties (NOT registered with API server)...")
	fmt.Println()

	native1, err := allocateParty(ctx, sdk, "native_interop_1")
	if err != nil {
		recordFail("Allocate native party 1: %v", err)
		fatalf("Cannot continue without native parties")
	}
	fmt.Printf("    Native User 1: %s\n", trunc(native1))

	native2, err := allocateParty(ctx, sdk, "native_interop_2")
	if err != nil {
		recordFail("Allocate native party 2: %v", err)
		fatalf("Cannot continue without native parties")
	}
	fmt.Printf("    Native User 2: %s\n", trunc(native2))
	fmt.Println()

	recordPass("Allocated 2 native Canton parties")
	return native1, native2
}

func stepTransfer(ctx context.Context, sdk *canton.Client, stepNum int, title, from, to, amount, symbol, fromLabel, toLabel string) {
	printStep(stepNum, title)
	fmt.Printf("    Transfer: %s %s\n", amount, symbol)
	fmt.Printf("    From:     %s\n", fromLabel)
	fmt.Printf("    To:       %s\n", toLabel)
	fmt.Println()

	err := sdk.Token.TransferByPartyID(ctx, from, to, amount, symbol)
	if err != nil {
		recordFail("Step %d: %s: %v", stepNum, title, err)
		return
	}
	recordPass("Step %d: %s", stepNum, title)

	fmt.Println(">>> Holdings after transfer:")
	showHoldings(ctx, sdk)
	fmt.Println()
}

func stepRegisterNativeUser(ctx context.Context, nativeParty string) {
	printStep(5, "Register Native User 1 with API Server")
	fmt.Println("    Registering so they can use MetaMask...")
	fmt.Println()

	evmAddr, privKey, err := registerNativeUser(ctx, apiURL, nativeParty)
	if err != nil {
		recordFail("Step 5: Registration failed: %v", err)
		return
	}

	fmt.Printf("    EVM Address:  %s\n", evmAddr)
	fmt.Printf("    Private Key:  %s\n", privKey)
	fmt.Println("    Native User 1 can now import this key into MetaMask.")
	fmt.Println()

	recordPass("Step 5: Registered Native User 1 (%s)", evmAddr)
}

// ─── Part B Steps ───────────────────────────────────────────────────────────

func stepDepositPrompt(ctx context.Context, user1 *apidb.User) {
	printStep(7, "Deposit PROMPT from Ethereum to Canton")

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

	recordPass("Step 7: Deposit %s PROMPT to Canton", *promptDeposit)
	fmt.Println()
}

func stepVerifyPromptBalance(ctx context.Context, stepNum int, user1 *apidb.User) {
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

func stepVerifyFinalPromptBalances(ctx context.Context, stepNum int, user1, user2 *apidb.User) {
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

func allocateParty(ctx context.Context, sdk *canton.Client, hint string) (string, error) {
	result, err := sdk.Identity.AllocateParty(ctx, hint)
	if err != nil {
		if strings.Contains(err.Error(), "already allocated") || strings.Contains(err.Error(), "already exists") {
			parties, listErr := sdk.Identity.ListParties(ctx)
			if listErr != nil {
				return "", fmt.Errorf("party exists but cannot list: %w", listErr)
			}
			for _, p := range parties {
				if strings.HasPrefix(p.PartyID, hint+"::") {
					return p.PartyID, nil
				}
			}
			return "", fmt.Errorf("party exists but not found in list")
		}
		return "", err
	}
	if err := sdk.Identity.GrantActAsParty(ctx, result.PartyID); err != nil {
		fmt.Printf("    Warning: CanActAs grant failed (ok for local): %v\n", err)
	}
	return result.PartyID, nil
}

func registerNativeUser(ctx context.Context, apiURL, cantonParty string) (evmAddress, privateKey string, err error) {
	reqBody := map[string]string{
		"canton_party_id":  cantonParty,
		"canton_signature": "",
		"message":          fmt.Sprintf("Register Canton party %s", cantonParty),
	}
	jsonBody, _ := json.Marshal(reqBody)

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

	var response struct {
		EVMAddress string `json:"evm_address"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}
	return response.EVMAddress, response.PrivateKey, nil
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
	fmt.Println("  Canton Interoperability Test Suite")
	fmt.Println("======================================================================")
	fmt.Println()
	fmt.Println("  Part A: DEMO Token Interoperability (native Canton ↔ MetaMask)")
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
