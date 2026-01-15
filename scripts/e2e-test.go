//go:build ignore

// e2e-test.go - End-to-end test for Sepolia → Canton bridge flow
//
// This script tests the complete token transfer flow:
// 1. Register users on API server
// 2. Approve and deposit tokens from Ethereum to Canton
// 3. Verify balances on Canton side
// 4. Transfer tokens between users on Canton
// 5. Verify final balances
// 6. Test ERC20 metadata endpoints
//
// Usage:
//   go run scripts/e2e-test.go \
//     -config config.devnet.yaml \
//     -test-config config.e2e-test.yaml \
//     [-local] [-skip-docker]
//
// Flags:
//   -config       Path to main service config (e.g., config.devnet.yaml)
//   -test-config  Path to test config with user keys and amounts
//   -local        Use local docker compose services
//   -skip-docker  Skip docker compose start (assume services are running)
//   -no-relayer   Don't start local relayer (use remote DevNet relayer instead)

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
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

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	_ "github.com/lib/pq" // PostgreSQL driver
	"gopkg.in/yaml.v3"
)

// Colors for output
const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
	colorCyan   = "\033[0;36m"
	colorReset  = "\033[0m"
)

// TestConfig holds test-specific configuration
type TestConfig struct {
	Users struct {
		User1 UserConfig `yaml:"user1"`
		User2 UserConfig `yaml:"user2"`
	} `yaml:"users"`

	Services struct {
		RelayerURL   string `yaml:"relayer_url"`
		APIServerURL string `yaml:"api_server_url"`
	} `yaml:"services"`

	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Database string `yaml:"database"`
	} `yaml:"database"`

	Amounts struct {
		TotalDeposit   string `yaml:"total_deposit"`
		TransferAmount string `yaml:"transfer_amount"`
	} `yaml:"amounts"`

	Contracts struct {
		TokenAddress  string `yaml:"token_address"`
		BridgeAddress string `yaml:"bridge_address"`
	} `yaml:"contracts"`

	Timeouts struct {
		DepositConfirmation string `yaml:"deposit_confirmation"`
		BalanceUpdate       string `yaml:"balance_update"`
		RPCCall             string `yaml:"rpc_call"`
	} `yaml:"timeouts"`
}

type UserConfig struct {
	PrivateKey  string `yaml:"private_key"`
	Address     string `yaml:"address"`
	Fingerprint string `yaml:"fingerprint"`
}

// RPCRequest represents a JSON-RPC 2.0 request
type RPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

// RPCResponse represents a JSON-RPC 2.0 response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RegisterResult from user_register
type RegisterResult struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mappingCid,omitempty"`
}

// BalanceResult from erc20_balanceOf
type BalanceResult struct {
	Balance string `json:"balance"`
	Address string `json:"address"`
}

// TransferResult from erc20_transfer
type TransferResult struct {
	Success bool   `json:"success"`
	TxID    string `json:"txId,omitempty"`
}

var (
	configPath     = flag.String("config", "config.devnet.yaml", "Path to main service config")
	testConfigPath = flag.String("test-config", "config.e2e-test.yaml", "Path to test config")
	localMode      = flag.Bool("local", false, "Use local docker compose services")
	skipDocker     = flag.Bool("skip-docker", true, "Skip docker compose start")
	noRelayer      = flag.Bool("no-relayer", true, "Don't start local relayer (use remote DevNet relayer)")
)

func main() {
	flag.Parse()

	printHeader("Canton-Ethereum Bridge E2E Test")

	// Load configs
	printStep("Loading configurations...")
	cfg, err := config.Load(*configPath)
	if err != nil {
		printError("Failed to load main config: %v", err)
		os.Exit(1)
	}

	testCfg, err := loadTestConfig(*testConfigPath)
	if err != nil {
		printError("Failed to load test config: %v", err)
		os.Exit(1)
	}

	// Validate test config
	if err := validateTestConfig(testCfg); err != nil {
		printError("Invalid test config: %v", err)
		os.Exit(1)
	}
	printSuccess("Configurations loaded")

	// Start docker compose if needed
	if *localMode && !*skipDocker {
		if err := startDockerCompose(*noRelayer); err != nil {
			printError("Failed to start docker compose: %v", err)
			os.Exit(1)
		}
	}

	// Wait for services to be ready
	printStep("Waiting for services...")
	if err := waitForServices(testCfg, *noRelayer); err != nil {
		printError("Services not ready: %v", err)
		os.Exit(1)
	}
	printSuccess("Services are ready")

	// Run the E2E test
	ctx := context.Background()
	if err := runE2ETest(ctx, cfg, testCfg); err != nil {
		printError("E2E test failed: %v", err)
		os.Exit(1)
	}

	printHeader("E2E Test Completed Successfully!")
}

func loadTestConfig(path string) (*TestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read test config: %w", err)
	}

	var cfg TestConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse test config: %w", err)
	}

	// Set defaults
	if cfg.Services.RelayerURL == "" {
		cfg.Services.RelayerURL = "http://localhost:8080"
	}
	if cfg.Services.APIServerURL == "" {
		cfg.Services.APIServerURL = "http://localhost:8081/rpc"
	}
	if cfg.Timeouts.DepositConfirmation == "" {
		cfg.Timeouts.DepositConfirmation = "120s"
	}
	if cfg.Timeouts.BalanceUpdate == "" {
		cfg.Timeouts.BalanceUpdate = "30s"
	}
	if cfg.Timeouts.RPCCall == "" {
		cfg.Timeouts.RPCCall = "30s"
	}

	return &cfg, nil
}

func validateTestConfig(cfg *TestConfig) error {
	if cfg.Users.User1.PrivateKey == "" || cfg.Users.User1.PrivateKey == "YOUR_USER1_PRIVATE_KEY_HERE" {
		return fmt.Errorf("user1 private key not configured")
	}
	if cfg.Users.User2.PrivateKey == "" || cfg.Users.User2.PrivateKey == "YOUR_USER2_PRIVATE_KEY_HERE" {
		return fmt.Errorf("user2 private key not configured")
	}
	if cfg.Amounts.TotalDeposit == "" {
		return fmt.Errorf("total_deposit amount not configured")
	}
	if cfg.Amounts.TransferAmount == "" {
		return fmt.Errorf("transfer_amount not configured")
	}
	return nil
}

func startDockerCompose(skipRelayer bool) error {
	printStep("Starting docker compose services...")

	args := []string{"compose", "-f", "docker-compose.yaml", "-f", "docker-compose.devnet.yaml", "up", "-d"}
	if skipRelayer {
		// Only start postgres and api-server, skip relayer (use remote DevNet relayer)
		args = append(args, "postgres", "api-server")
		printInfo("Skipping local relayer (using remote DevNet relayer)")
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose failed: %w", err)
	}
	printSuccess("Docker compose started")
	return nil
}

func waitForServices(cfg *TestConfig, skipRelayer bool) error {
	// Wait for API server health
	apiHealthURL := strings.TrimSuffix(cfg.Services.APIServerURL, "/rpc") + "/health"
	if err := waitForHealth(apiHealthURL, 60); err != nil {
		return fmt.Errorf("API server not healthy: %w", err)
	}

	// Wait for relayer health (skip if using remote relayer)
	if !skipRelayer {
		relayerHealthURL := cfg.Services.RelayerURL + "/health"
		if err := waitForHealth(relayerHealthURL, 60); err != nil {
			return fmt.Errorf("relayer not healthy: %w", err)
		}
	}

	return nil
}

func waitForHealth(url string, maxAttempts int) error {
	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func runE2ETest(ctx context.Context, cfg *config.Config, testCfg *TestConfig) error {
	// Parse timeouts
	depositTimeout, _ := time.ParseDuration(testCfg.Timeouts.DepositConfirmation)
	balanceTimeout, _ := time.ParseDuration(testCfg.Timeouts.BalanceUpdate)

	// Get contract addresses
	tokenAddr := testCfg.Contracts.TokenAddress
	if tokenAddr == "" {
		tokenAddr = cfg.Ethereum.TokenContract
	}
	bridgeAddr := testCfg.Contracts.BridgeAddress
	if bridgeAddr == "" {
		bridgeAddr = cfg.Ethereum.BridgeContract
	}

	// Parse private keys
	user1Key, user1Addr, err := parsePrivateKey(testCfg.Users.User1.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse user1 private key: %w", err)
	}
	user2Key, user2Addr, err := parsePrivateKey(testCfg.Users.User2.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse user2 private key: %w", err)
	}

	printInfo("User1 Address: %s", user1Addr.Hex())
	printInfo("User2 Address: %s", user2Addr.Hex())
	printInfo("Token Contract: %s", tokenAddr)
	printInfo("Bridge Contract: %s", bridgeAddr)

	// =========================================================================
	// Step 0 (local mode only): Whitelist users in PostgreSQL
	// =========================================================================
	if *localMode && testCfg.Database.Host != "" {
		printHeader("Step 0: Whitelist Users (Local Mode)")
		if err := whitelistUsers(testCfg, user1Addr, user2Addr); err != nil {
			return fmt.Errorf("failed to whitelist users: %w", err)
		}
	}

	// =========================================================================
	// Step 1: Register users on API server
	// =========================================================================
	printHeader("Step 1: Register Users")

	user1Fingerprint, err := registerUser(testCfg.Services.APIServerURL, user1Key, "User1")
	if err != nil {
		return fmt.Errorf("failed to register user1: %w", err)
	}
	printSuccess("User1 registered, fingerprint: %s", truncate(user1Fingerprint, 20))

	user2Fingerprint, err := registerUser(testCfg.Services.APIServerURL, user2Key, "User2")
	if err != nil {
		return fmt.Errorf("failed to register user2: %w", err)
	}
	printSuccess("User2 registered, fingerprint: %s", truncate(user2Fingerprint, 20))

	// =========================================================================
	// Step 2: Approve and deposit tokens from Ethereum
	// =========================================================================
	printHeader("Step 2: Approve and Deposit Tokens")

	// Connect to Ethereum
	ethClient, err := ethclient.Dial(cfg.Ethereum.RPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to Ethereum: %w", err)
	}
	defer ethClient.Close()

	// Parse deposit amount
	depositAmount, err := parseTokenAmount(testCfg.Amounts.TotalDeposit, 18)
	if err != nil {
		return fmt.Errorf("failed to parse deposit amount: %w", err)
	}
	printInfo("Deposit amount: %s tokens (%s wei)", testCfg.Amounts.TotalDeposit, depositAmount.String())

	// Create token and bridge bindings
	token, err := contracts.NewPromptToken(common.HexToAddress(tokenAddr), ethClient)
	if err != nil {
		return fmt.Errorf("failed to bind token contract: %w", err)
	}
	bridge, err := contracts.NewCantonBridge(common.HexToAddress(bridgeAddr), ethClient)
	if err != nil {
		return fmt.Errorf("failed to bind bridge contract: %w", err)
	}

	// Check User1 token balance
	balance, err := token.BalanceOf(&bind.CallOpts{}, user1Addr)
	if err != nil {
		return fmt.Errorf("failed to get user1 token balance: %w", err)
	}
	printInfo("User1 token balance: %s wei", balance.String())

	if balance.Cmp(depositAmount) < 0 {
		return fmt.Errorf("user1 has insufficient tokens: has %s, needs %s", balance.String(), depositAmount.String())
	}

	// Approve bridge to spend tokens
	printStep("Approving bridge contract...")
	auth, err := getTransactor(ctx, ethClient, user1Key, cfg.Ethereum.ChainID)
	if err != nil {
		return fmt.Errorf("failed to create transactor: %w", err)
	}

	tx, err := token.Approve(auth, common.HexToAddress(bridgeAddr), depositAmount)
	if err != nil {
		return fmt.Errorf("failed to approve tokens: %w", err)
	}
	printSuccess("Approval tx: %s", tx.Hash().Hex())

	// Wait for approval confirmation (90s timeout for Sepolia's ~12s block times)
	if err := waitForTx(ctx, ethClient, tx.Hash(), 90*time.Second); err != nil {
		return fmt.Errorf("approval tx failed: %w", err)
	}

	// Deposit to Canton
	printStep("Depositing tokens to Canton...")
	cantonRecipient := fingerprintToBytes32(user1Fingerprint)
	printInfo("Canton recipient (bytes32): 0x%s", hex.EncodeToString(cantonRecipient[:]))

	auth, err = getTransactor(ctx, ethClient, user1Key, cfg.Ethereum.ChainID)
	if err != nil {
		return fmt.Errorf("failed to create transactor: %w", err)
	}

	tx, err = bridge.DepositToCanton(auth, common.HexToAddress(tokenAddr), depositAmount, cantonRecipient)
	if err != nil {
		return fmt.Errorf("failed to deposit to Canton: %w", err)
	}
	printSuccess("Deposit tx: %s", tx.Hash().Hex())

	// 90s timeout for Sepolia's ~12s block times
	if err := waitForTx(ctx, ethClient, tx.Hash(), 90*time.Second); err != nil {
		return fmt.Errorf("deposit tx failed: %w", err)
	}

	// =========================================================================
	// Step 3: Wait for deposit to be processed and verify balance
	// =========================================================================
	printHeader("Step 3: Verify Canton Balances")

	printStep("Waiting for deposit to be processed by relayer...")
	time.Sleep(5 * time.Second) // Initial delay for relayer to pick up event

	// Poll for balance update
	var user1Balance, user2Balance string
	deadline := time.Now().Add(depositTimeout)
	for time.Now().Before(deadline) {
		user1Balance, err = getBalance(testCfg.Services.APIServerURL, user1Key)
		if err == nil && user1Balance != "0" && user1Balance != "" {
			break
		}
		printInfo("Waiting for balance... (current: %s)", user1Balance)
		time.Sleep(5 * time.Second)
	}

	if user1Balance == "0" || user1Balance == "" {
		return fmt.Errorf("user1 balance not updated after deposit (timeout: %s)", depositTimeout)
	}
	printSuccess("User1 Canton balance: %s", user1Balance)

	user2Balance, _ = getBalance(testCfg.Services.APIServerURL, user2Key)
	printSuccess("User2 Canton balance: %s", user2Balance)

	// =========================================================================
	// Step 4: Transfer tokens on Canton side
	// =========================================================================
	printHeader("Step 4: Transfer Tokens (User1 → User2)")

	transferAmount := testCfg.Amounts.TransferAmount
	printStep("Transferring %s tokens from User1 to User2...", transferAmount)

	txID, err := transferTokens(testCfg.Services.APIServerURL, user1Key, user2Addr.Hex(), transferAmount)
	if err != nil {
		return fmt.Errorf("failed to transfer tokens: %w", err)
	}
	printSuccess("Transfer completed, tx: %s", truncate(txID, 30))

	// Wait for balance to update
	time.Sleep(3 * time.Second)

	// =========================================================================
	// Step 5: Verify final balances
	// =========================================================================
	printHeader("Step 5: Verify Final Balances")

	// Wait for cache to update
	deadline = time.Now().Add(balanceTimeout)
	var newUser2Balance string
	for time.Now().Before(deadline) {
		newUser2Balance, _ = getBalance(testCfg.Services.APIServerURL, user2Key)
		if newUser2Balance != user2Balance && newUser2Balance != "" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	user1Balance, _ = getBalance(testCfg.Services.APIServerURL, user1Key)
	user2Balance, _ = getBalance(testCfg.Services.APIServerURL, user2Key)

	printSuccess("User1 final balance: %s", user1Balance)
	printSuccess("User2 final balance: %s", user2Balance)

	// =========================================================================
	// Step 6: Test ERC20 metadata endpoints
	// =========================================================================
	printHeader("Step 6: Test ERC20 Metadata Endpoints")

	// Test erc20_name
	name, err := callERC20Method(testCfg.Services.APIServerURL, user1Key, "erc20_name")
	if err != nil {
		printWarning("erc20_name failed: %v", err)
	} else {
		printSuccess("erc20_name: %s", name)
	}

	// Test erc20_symbol
	symbol, err := callERC20Method(testCfg.Services.APIServerURL, user1Key, "erc20_symbol")
	if err != nil {
		printWarning("erc20_symbol failed: %v", err)
	} else {
		printSuccess("erc20_symbol: %s", symbol)
	}

	// Test erc20_decimals
	decimals, err := callERC20Method(testCfg.Services.APIServerURL, user1Key, "erc20_decimals")
	if err != nil {
		printWarning("erc20_decimals failed: %v", err)
	} else {
		printSuccess("erc20_decimals: %s", decimals)
	}

	// Test erc20_totalSupply
	totalSupply, err := callERC20Method(testCfg.Services.APIServerURL, user2Key, "erc20_totalSupply")
	if err != nil {
		printWarning("erc20_totalSupply failed: %v", err)
	} else {
		printSuccess("erc20_totalSupply: %s", totalSupply)
	}

	// Test from User2 as well
	printStep("Testing metadata from User2...")
	name2, _ := callERC20Method(testCfg.Services.APIServerURL, user2Key, "erc20_name")
	symbol2, _ := callERC20Method(testCfg.Services.APIServerURL, user2Key, "erc20_symbol")
	printSuccess("User2 can call erc20_name: %s, erc20_symbol: %s", name2, symbol2)

	return nil
}

// =============================================================================
// Database Helpers (for local mode whitelisting)
// =============================================================================

// whitelistUsers adds users to the API server whitelist in PostgreSQL
func whitelistUsers(cfg *TestConfig, users ...common.Address) error {
	if cfg.Database.Host == "" {
		return fmt.Errorf("database config not provided")
	}

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User, cfg.Database.Password, cfg.Database.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	for _, addr := range users {
		_, err := db.Exec(
			"INSERT INTO whitelist (evm_address) VALUES ($1) ON CONFLICT DO NOTHING",
			addr.Hex(),
		)
		if err != nil {
			return fmt.Errorf("failed to whitelist %s: %w", addr.Hex(), err)
		}
		printSuccess("Whitelisted %s", addr.Hex())
	}

	return nil
}

// =============================================================================
// API Server RPC Helpers
// =============================================================================

// signEIP191 creates an EIP-191 personal signature (same format as eth_sign / cast wallet sign)
func signEIP191(message string, privateKey *ecdsa.PrivateKey) (string, error) {
	// EIP-191: "\x19Ethereum Signed Message:\n" + len(message) + message
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256Hash([]byte(prefix + message))
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return "", err
	}
	// Adjust v value for Ethereum (27 or 28)
	if signature[64] < 27 {
		signature[64] += 27
	}
	return "0x" + hex.EncodeToString(signature), nil
}

func rpcCall(url string, privateKey *ecdsa.PrivateKey, method string, params interface{}) (*RPCResponse, error) {
	// Create signature using EIP-191 personal sign format
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%s:%d", method, timestamp)
	sigHex, err := signEIP191(message, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	// Create request
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Signature", sigHex)
	httpReq.Header.Set("X-Message", message)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(respBody))
	}

	return &rpcResp, nil
}

func registerUser(url string, privateKey *ecdsa.PrivateKey, name string) (string, error) {
	resp, err := rpcCall(url, privateKey, "user_register", map[string]interface{}{})
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		// Check if already registered
		if resp.Error.Code == -32005 || strings.Contains(resp.Error.Message, "already registered") {
			printWarning("%s already registered, computing fingerprint from address", name)
			// Compute fingerprint from address (same as API server's ComputeFingerprint)
			addr := crypto.PubkeyToAddress(privateKey.PublicKey)
			fingerprint := crypto.Keccak256Hash(addr.Bytes()).Hex()
			return fingerprint, nil
		}
		// Check if not whitelisted (helpful error message)
		if resp.Error.Code == -32004 || strings.Contains(resp.Error.Message, "not whitelisted") {
			return "", fmt.Errorf("%s not whitelisted - for remote mode, ensure users are pre-whitelisted; for local mode, use -local flag", name)
		}
		return "", fmt.Errorf("RPC error: %s", resp.Error.Message)
	}

	var result RegisterResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse result: %w", err)
	}

	return result.Fingerprint, nil
}

func getBalance(url string, privateKey *ecdsa.PrivateKey) (string, error) {
	resp, err := rpcCall(url, privateKey, "erc20_balanceOf", map[string]interface{}{})
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", fmt.Errorf("RPC error: %s", resp.Error.Message)
	}

	var result BalanceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse result: %w", err)
	}

	return result.Balance, nil
}

func transferTokens(url string, privateKey *ecdsa.PrivateKey, to, amount string) (string, error) {
	params := map[string]interface{}{
		"to":     to,
		"amount": amount,
	}

	resp, err := rpcCall(url, privateKey, "erc20_transfer", params)
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", fmt.Errorf("RPC error: %s", resp.Error.Message)
	}

	var result TransferResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse result: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("transfer failed")
	}

	return result.TxID, nil
}

func callERC20Method(url string, privateKey *ecdsa.PrivateKey, method string) (string, error) {
	resp, err := rpcCall(url, privateKey, method, map[string]interface{}{})
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", fmt.Errorf("RPC error: %s", resp.Error.Message)
	}

	// Handle different result types
	var strResult string
	if err := json.Unmarshal(resp.Result, &strResult); err == nil {
		return strResult, nil
	}

	// Try as object with specific field
	var objResult map[string]interface{}
	if err := json.Unmarshal(resp.Result, &objResult); err == nil {
		// Try common fields
		for _, key := range []string{"totalSupply", "decimals", "name", "symbol"} {
			if v, ok := objResult[key]; ok {
				return fmt.Sprintf("%v", v), nil
			}
		}
	}

	return string(resp.Result), nil
}

// =============================================================================
// Ethereum Helpers
// =============================================================================

func parsePrivateKey(keyHex string) (*ecdsa.PrivateKey, common.Address, error) {
	keyHex = strings.TrimPrefix(keyHex, "0x")
	key, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return nil, common.Address{}, err
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return key, addr, nil
}

func parseTokenAmount(amount string, decimals int) (*big.Int, error) {
	// Parse as float and convert to wei
	parts := strings.Split(amount, ".")
	whole := parts[0]
	frac := ""
	if len(parts) > 1 {
		frac = parts[1]
	}

	// Pad or trim fractional part
	if len(frac) < decimals {
		frac = frac + strings.Repeat("0", decimals-len(frac))
	} else if len(frac) > decimals {
		frac = frac[:decimals]
	}

	combined := whole + frac
	result := new(big.Int)
	result.SetString(combined, 10)
	return result, nil
}

func getTransactor(ctx context.Context, client *ethclient.Client, key *ecdsa.PrivateKey, chainID int64) (*bind.TransactOpts, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(chainID))
	if err != nil {
		return nil, err
	}

	nonce, err := client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, err
	}
	auth.Nonce = big.NewInt(int64(nonce))

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	auth.GasPrice = gasPrice
	auth.GasLimit = 300000

	return auth, nil
}

func waitForTx(ctx context.Context, client *ethclient.Client, txHash common.Hash, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			if receipt.Status == 1 {
				return nil
			}
			return fmt.Errorf("transaction reverted")
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for transaction")
}

func fingerprintToBytes32(fingerprint string) [32]byte {
	var result [32]byte
	fingerprint = strings.TrimPrefix(fingerprint, "0x")
	data, err := hex.DecodeString(fingerprint)
	if err != nil {
		panic(fmt.Sprintf("fingerprintToBytes32: invalid hex string %q: %v", fingerprint, err))
	}
	if len(data) > 32 {
		panic(fmt.Sprintf("fingerprintToBytes32: fingerprint too long (%d bytes, max 32)", len(data)))
	}
	copy(result[:], data)
	return result
}

// =============================================================================
// Output Helpers
// =============================================================================

func printHeader(msg string) {
	fmt.Printf("\n%s══════════════════════════════════════════════════════════════════════%s\n", colorBlue, colorReset)
	fmt.Printf("%s  %s%s\n", colorBlue, msg, colorReset)
	fmt.Printf("%s══════════════════════════════════════════════════════════════════════%s\n", colorBlue, colorReset)
}

func printStep(format string, args ...interface{}) {
	fmt.Printf("%s>>> %s%s\n", colorCyan, fmt.Sprintf(format, args...), colorReset)
}

func printSuccess(format string, args ...interface{}) {
	fmt.Printf("%s✓ %s%s\n", colorGreen, fmt.Sprintf(format, args...), colorReset)
}

func printWarning(format string, args ...interface{}) {
	fmt.Printf("%s⚠ %s%s\n", colorYellow, fmt.Sprintf(format, args...), colorReset)
}

func printError(format string, args ...interface{}) {
	fmt.Printf("%s✗ %s%s\n", colorRed, fmt.Sprintf(format, args...), colorReset)
}

func printInfo(format string, args ...interface{}) {
	fmt.Printf("    %s\n", fmt.Sprintf(format, args...))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
