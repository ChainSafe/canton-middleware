//go:build ignore

// e2e-local.go - Fully local end-to-end test for Canton-Ethereum bridge
//
// This script runs a complete E2E test using only local Docker services:
// - Anvil (local Ethereum)
// - Canton (local ledger)
// - PostgreSQL
// - Mock OAuth2 server
// - Relayer + API Server
//
// Test Flow:
// 1. Start Docker services (if not running)
// 2. Wait for all services to be healthy
// 3. Mint test tokens to users on Anvil
// 4. Register users on API server
// 5. Deposit tokens from Anvil to Canton
// 6. Verify Canton balances
// 7. Transfer tokens between users on Canton
// 8. Initiate withdrawal from Canton to Anvil
// 9. Verify final balances
// 10. Optionally cleanup Docker services
//
// Usage:
//   go run scripts/e2e-local.go [-cleanup] [-skip-docker] [-verbose]
//
// Flags:
//   -cleanup      Stop and remove Docker services after test
//   -skip-docker  Skip Docker compose start (assume services are running)
//   -verbose      Enable verbose output

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

	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	_ "github.com/lib/pq"
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

// ERC20 ABI for encoding calls
const erc20ABI = `[
	{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},
	{"constant":false,"inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"symbol","outputs":[{"name":"","type":"string"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"totalSupply","outputs":[{"name":"","type":"uint256"}],"type":"function"}
]`

// LocalTestConfig holds configuration for local E2E testing
type LocalTestConfig struct {
	Users struct {
		User1 UserConfig `yaml:"user1"`
		User2 UserConfig `yaml:"user2"`
	} `yaml:"users"`

	Services struct {
		RelayerURL    string `yaml:"relayer_url"`
		APIServerURL  string `yaml:"api_server_url"`
		EthRPCURL     string `yaml:"eth_rpc_url"`
		RegisterURL   string `yaml:"register_url"`
	} `yaml:"services"`

	EthRPC struct {
		ChainID      uint64 `yaml:"chain_id"`
		TokenAddress string `yaml:"token_address"`
	} `yaml:"eth_rpc"`

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

	Local struct {
		ComposeFiles []string `yaml:"compose_files"`
		AnvilURL     string   `yaml:"anvil_url"`
		CantonURL    string   `yaml:"canton_url"`
	} `yaml:"local"`
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

type RegisterResult struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mappingCid,omitempty"`
}

type BalanceResult struct {
	Balance string `json:"balance"`
	Address string `json:"address"`
}

type TransferResult struct {
	Success bool   `json:"success"`
	TxID    string `json:"txId,omitempty"`
}

var (
	configPath = flag.String("config", "config.e2e-local.yaml", "Path to local test config")
	cleanup    = flag.Bool("cleanup", false, "Stop and remove Docker services after test")
	skipDocker = flag.Bool("skip-docker", false, "Skip Docker compose start")
	verbose    = flag.Bool("verbose", false, "Enable verbose output")
)

func main() {
	flag.Parse()

	printHeader("Canton-Ethereum Bridge Local E2E Test")

	// Load config
	printStep("Loading configuration...")
	cfg, err := loadConfig(*configPath)
	if err != nil {
		printError("Failed to load config: %v", err)
		os.Exit(1)
	}
	printSuccess("Configuration loaded")

	// Start Docker services
	if !*skipDocker {
		if err := startDockerServices(cfg); err != nil {
			printError("Failed to start Docker services: %v", err)
			os.Exit(1)
		}
	} else {
		printInfo("Skipping Docker compose start (assuming services are running)")
	}

	// Ensure cleanup on exit if requested
	if *cleanup {
		defer func() {
			printHeader("Cleanup")
			if err := stopDockerServices(cfg); err != nil {
				printWarning("Failed to stop Docker services: %v", err)
			}
		}()
	}

	// Wait for services to be healthy
	printStep("Waiting for services to be healthy...")
	if err := waitForServices(cfg); err != nil {
		printError("Services not ready: %v", err)
		os.Exit(1)
	}
	printSuccess("All services are healthy")

	// Run the E2E test
	ctx := context.Background()
	if err := runE2ETest(ctx, cfg); err != nil {
		printError("E2E test failed: %v", err)
		os.Exit(1)
	}

	printHeader("Local E2E Test Completed Successfully!")
}

func loadConfig(path string) (*LocalTestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg LocalTestConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if cfg.Services.RelayerURL == "" {
		cfg.Services.RelayerURL = "http://localhost:8080"
	}
	if cfg.Services.APIServerURL == "" {
		cfg.Services.APIServerURL = "http://localhost:8081"
	}
	if cfg.Services.EthRPCURL == "" {
		cfg.Services.EthRPCURL = cfg.Services.APIServerURL + "/eth"
	}
	if cfg.Services.RegisterURL == "" {
		cfg.Services.RegisterURL = cfg.Services.APIServerURL + "/register"
	}
	if cfg.EthRPC.ChainID == 0 {
		cfg.EthRPC.ChainID = 31337 // Default chain ID
	}
	if cfg.Local.AnvilURL == "" {
		cfg.Local.AnvilURL = "http://localhost:8545"
	}
	if cfg.Timeouts.DepositConfirmation == "" {
		cfg.Timeouts.DepositConfirmation = "60s"
	}
	if cfg.Timeouts.BalanceUpdate == "" {
		cfg.Timeouts.BalanceUpdate = "30s"
	}
	if len(cfg.Local.ComposeFiles) == 0 {
		cfg.Local.ComposeFiles = []string{"docker-compose.yaml", "docker-compose.local-test.yaml"}
	}

	return &cfg, nil
}

func startDockerServices(cfg *LocalTestConfig) error {
	printHeader("Starting Docker Services")

	// Build compose args
	args := []string{"compose"}
	for _, f := range cfg.Local.ComposeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "up", "-d", "--build")

	printStep("Running: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)
	if *verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose failed: %w", err)
	}

	printSuccess("Docker services started")
	return nil
}

func stopDockerServices(cfg *LocalTestConfig) error {
	printStep("Stopping Docker services...")

	args := []string{"compose"}
	for _, f := range cfg.Local.ComposeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "down", "-v")

	cmd := exec.Command("docker", args...)
	if *verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}

	printSuccess("Docker services stopped")
	return nil
}

func waitForServices(cfg *LocalTestConfig) error {
	maxAttempts := 60
	checkInterval := 3 * time.Second

	// Check Anvil (JSON-RPC, needs POST request)
	printStep("Waiting for Anvil...")
	if err := waitForAnvil(cfg.Local.AnvilURL, maxAttempts, checkInterval); err != nil {
		return fmt.Errorf("Anvil not ready: %w", err)
	}
	printSuccess("Anvil is ready")

	// Check other services (HTTP health endpoints)
	services := []struct {
		name string
		url  string
	}{
		{"API Server", strings.TrimSuffix(cfg.Services.APIServerURL, "/") + "/health"},
		{"Relayer", cfg.Services.RelayerURL + "/health"},
	}

	for _, svc := range services {
		printStep("Waiting for %s...", svc.name)
		if err := waitForEndpoint(svc.url, maxAttempts, checkInterval); err != nil {
			return fmt.Errorf("%s not ready: %w", svc.name, err)
		}
		printSuccess("%s is ready", svc.name)
	}

	return nil
}

func waitForAnvil(url string, maxAttempts int, interval time.Duration) error {
	// Anvil is JSON-RPC, so we need to POST a valid request
	rpcBody := []byte(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`)

	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewReader(rpcBody))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timeout after %d attempts", maxAttempts)
}

func waitForEndpoint(url string, maxAttempts int, interval time.Duration) error {
	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
				return nil
			}
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timeout after %d attempts", maxAttempts)
}

func runE2ETest(ctx context.Context, cfg *LocalTestConfig) error {
	depositTimeout, _ := time.ParseDuration(cfg.Timeouts.DepositConfirmation)
	balanceTimeout, _ := time.ParseDuration(cfg.Timeouts.BalanceUpdate)

	// Parse user keys
	user1Key, user1Addr, err := parsePrivateKey(cfg.Users.User1.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse user1 key: %w", err)
	}
	user2Key, user2Addr, err := parsePrivateKey(cfg.Users.User2.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse user2 key: %w", err)
	}

	printInfo("User1 Address: %s", user1Addr.Hex())
	printInfo("User2 Address: %s", user2Addr.Hex())

	// Connect to Anvil
	ethClient, err := ethclient.Dial(cfg.Local.AnvilURL)
	if err != nil {
		return fmt.Errorf("failed to connect to Anvil: %w", err)
	}
	defer ethClient.Close()

	chainID, err := ethClient.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain ID: %w", err)
	}
	printInfo("Chain ID: %s", chainID.String())

	// Contract addresses
	tokenAddr := common.HexToAddress(cfg.Contracts.TokenAddress)
	bridgeAddr := common.HexToAddress(cfg.Contracts.BridgeAddress)
	printInfo("Token: %s", tokenAddr.Hex())
	printInfo("Bridge: %s", bridgeAddr.Hex())

	// Get token address for eth RPC calls
	ethTokenAddr := cfg.EthRPC.TokenAddress
	if ethTokenAddr == "" {
		ethTokenAddr = tokenAddr.Hex() // Fall back to Anvil token address
	}

	// Create contract bindings
	token, err := contracts.NewPromptToken(tokenAddr, ethClient)
	if err != nil {
		return fmt.Errorf("failed to bind token contract: %w", err)
	}
	bridge, err := contracts.NewCantonBridge(bridgeAddr, ethClient)
	if err != nil {
		return fmt.Errorf("failed to bind bridge contract: %w", err)
	}

	// =========================================================================
	// Step 1: Verify User1 has tokens (deployer gets all tokens at deployment)
	// =========================================================================
	printHeader("Step 1: Verify Token Balance")

	depositAmount, err := parseTokenAmount(cfg.Amounts.TotalDeposit, 18)
	if err != nil {
		return fmt.Errorf("failed to parse deposit amount: %w", err)
	}

	// Check user1 token balance (deployer gets entire supply at deployment)
	balance, err := token.BalanceOf(&bind.CallOpts{}, user1Addr)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	if balance.Cmp(depositAmount) < 0 {
		return fmt.Errorf("User1 has insufficient tokens: %s (need %s). Make sure User1 is the deployer account", balance.String(), depositAmount.String())
	}
	printSuccess("User1 has sufficient tokens: %s", balance.String())

	// =========================================================================
	// Step 2: Whitelist users in PostgreSQL
	// =========================================================================
	printHeader("Step 2: Whitelist Users")

	if err := whitelistUsers(cfg, user1Addr, user2Addr); err != nil {
		return fmt.Errorf("failed to whitelist users: %w", err)
	}

	// =========================================================================
	// Step 3: Register users on API server
	// =========================================================================
	printHeader("Step 3: Register Users")

	user1Fingerprint, err := registerUser(cfg.Services.RegisterURL, user1Key, "User1")
	if err != nil {
		return fmt.Errorf("failed to register user1: %w", err)
	}
	printSuccess("User1 fingerprint: %s", truncate(user1Fingerprint, 20))

	user2Fingerprint, err := registerUser(cfg.Services.RegisterURL, user2Key, "User2")
	if err != nil {
		return fmt.Errorf("failed to register user2: %w", err)
	}
	printSuccess("User2 fingerprint: %s", truncate(user2Fingerprint, 20))

	// =========================================================================
	// Step 4: Approve and deposit to Canton
	// =========================================================================
	printHeader("Step 4: Deposit Tokens to Canton")

	// Approve bridge
	printStep("Approving bridge contract...")
	auth, err := getTransactor(ctx, ethClient, user1Key, chainID.Int64())
	if err != nil {
		return fmt.Errorf("failed to create transactor: %w", err)
	}

	tx, err := token.Approve(auth, bridgeAddr, depositAmount)
	if err != nil {
		return fmt.Errorf("failed to approve: %w", err)
	}
	printInfo("Approval tx: %s", tx.Hash().Hex())

	if err := waitForTx(ctx, ethClient, tx.Hash(), 30*time.Second); err != nil {
		return fmt.Errorf("approval failed: %w", err)
	}

	// Deposit to Canton
	printStep("Depositing to Canton...")
	auth, err = getTransactor(ctx, ethClient, user1Key, chainID.Int64())
	if err != nil {
		return fmt.Errorf("failed to create transactor: %w", err)
	}

	cantonRecipient := fingerprintToBytes32(user1Fingerprint)
	tx, err = bridge.DepositToCanton(auth, tokenAddr, depositAmount, cantonRecipient)
	if err != nil {
		return fmt.Errorf("failed to deposit: %w", err)
	}
	printInfo("Deposit tx: %s", tx.Hash().Hex())

	if err := waitForTx(ctx, ethClient, tx.Hash(), 30*time.Second); err != nil {
		return fmt.Errorf("deposit failed: %w", err)
	}
	printSuccess("Deposit submitted")

	// =========================================================================
	// Step 5: Verify Canton balance
	// =========================================================================
	printHeader("Step 5: Verify Canton Balance")

	printStep("Waiting for relayer to process deposit...")
	time.Sleep(5 * time.Second)

	var user1Balance string
	deadline := time.Now().Add(depositTimeout)
	for time.Now().Before(deadline) {
		user1Balance, err = getBalance(cfg.Services.EthRPCURL, ethTokenAddr, user1Key)
		if err == nil && user1Balance != "0" && user1Balance != "" {
			break
		}
		printInfo("Waiting for balance... (current: %s)", user1Balance)
		time.Sleep(3 * time.Second)
	}

	if user1Balance == "0" || user1Balance == "" {
		return fmt.Errorf("user1 balance not updated (timeout)")
	}
	printSuccess("User1 Canton balance: %s", user1Balance)

	// =========================================================================
	// Step 6: Transfer tokens on Canton
	// =========================================================================
	printHeader("Step 6: Transfer Tokens (User1 -> User2)")

	transferAmount := cfg.Amounts.TransferAmount
	printStep("Transferring %s tokens...", transferAmount)

	// Parse transfer amount to wei
	transferAmountWei, err := parseTokenAmount(transferAmount, 18)
	if err != nil {
		return fmt.Errorf("failed to parse transfer amount: %w", err)
	}

	txHash, err := transferTokens(cfg.Services.EthRPCURL, ethTokenAddr, cfg.EthRPC.ChainID, user1Key, user2Addr.Hex(), transferAmountWei.String())
	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}
	printSuccess("Transfer completed: %s", truncate(txHash, 30))

	// Wait for balance update
	time.Sleep(3 * time.Second)

	// =========================================================================
	// Step 7: Verify final balances
	// =========================================================================
	printHeader("Step 7: Final Balances")

	deadline = time.Now().Add(balanceTimeout)
	var user2Balance string
	for time.Now().Before(deadline) {
		user2Balance, _ = getBalance(cfg.Services.EthRPCURL, ethTokenAddr, user2Key)
		if user2Balance != "" && user2Balance != "0" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	user1Balance, _ = getBalance(cfg.Services.EthRPCURL, ethTokenAddr, user1Key)
	user2Balance, _ = getBalance(cfg.Services.EthRPCURL, ethTokenAddr, user2Key)

	printSuccess("User1 final balance: %s", user1Balance)
	printSuccess("User2 final balance: %s", user2Balance)

	// =========================================================================
	// Step 8: Test ERC20 metadata endpoints
	// =========================================================================
	printHeader("Step 8: Test ERC20 Metadata")

	name, _ := callERC20Method(cfg.Services.EthRPCURL, ethTokenAddr, "name")
	symbol, _ := callERC20Method(cfg.Services.EthRPCURL, ethTokenAddr, "symbol")
	decimals, _ := callERC20Method(cfg.Services.EthRPCURL, ethTokenAddr, "decimals")
	totalSupply, _ := callERC20Method(cfg.Services.EthRPCURL, ethTokenAddr, "totalSupply")

	printSuccess("Name: %s", name)
	printSuccess("Symbol: %s", symbol)
	printSuccess("Decimals: %s", decimals)
	printSuccess("Total Supply: %s", totalSupply)

	return nil
}

// =============================================================================
// Database Helpers
// =============================================================================

func whitelistUsers(cfg *LocalTestConfig, users ...common.Address) error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User, cfg.Database.Password, cfg.Database.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer db.Close()

	// Retry connection with timeout
	var lastErr error
	for i := 0; i < 10; i++ {
		if err := db.Ping(); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
			printInfo("Waiting for database connection (attempt %d/10)...", i+1)
		}
		time.Sleep(2 * time.Second)
	}

	if lastErr != nil {
		return fmt.Errorf("failed to connect to database after retries: %w", lastErr)
	}

	printInfo("Connected to database: %s", cfg.Database.Database)

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
// RPC Helpers
// =============================================================================

func signEIP191(message string, privateKey *ecdsa.PrivateKey) (string, error) {
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256Hash([]byte(prefix + message))
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return "", err
	}
	if signature[64] < 27 {
		signature[64] += 27
	}
	return "0x" + hex.EncodeToString(signature), nil
}

// ethRPCCall makes an Ethereum JSON-RPC call (no authentication)
func ethRPCCall(url string, method string, params interface{}) (json.RawMessage, error) {
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

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func registerUser(url string, privateKey *ecdsa.PrivateKey, name string) (string, error) {
	// Create signature
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("registration:%d", timestamp)
	sigHex, err := signEIP191(message, privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign message: %w", err)
	}

	// Create request body
	reqBody := map[string]string{
		"signature": sigHex,
		"message":   message,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Handle non-success status codes
	if resp.StatusCode != http.StatusOK {
		// Check if already registered
		if resp.StatusCode == http.StatusConflict {
			printWarning("%s already registered, computing fingerprint from address", name)
			addr := crypto.PubkeyToAddress(privateKey.PublicKey)
			fingerprint := crypto.Keccak256Hash(addr.Bytes()).Hex()
			return fingerprint, nil
		}
		// Check if not whitelisted
		if resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("%s not whitelisted", name)
		}
		return "", fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result RegisterResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse result: %w", err)
	}

	return result.Fingerprint, nil
}

func getBalance(ethRPCURL string, tokenAddress string, privateKey *ecdsa.PrivateKey) (string, error) {
	// Parse ABI
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return "", fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Get address from private key
	addr := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Encode balanceOf call
	data, err := parsedABI.Pack("balanceOf", addr)
	if err != nil {
		return "", fmt.Errorf("failed to pack balanceOf: %w", err)
	}

	// Make eth_call request
	params := []interface{}{
		map[string]interface{}{
			"to":   tokenAddress,
			"data": "0x" + hex.EncodeToString(data),
		},
		"latest",
	}

	respData, err := ethRPCCall(ethRPCURL, "eth_call", params)
	if err != nil {
		return "", err
	}

	// Decode result
	var resultHex string
	if err := json.Unmarshal(respData, &resultHex); err != nil {
		return "", fmt.Errorf("failed to parse result: %w", err)
	}

	// Parse hex to big.Int
	resultBytes, err := hex.DecodeString(strings.TrimPrefix(resultHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("failed to decode hex: %w", err)
	}

	balance := new(big.Int).SetBytes(resultBytes)
	return balance.String(), nil
}

func transferTokens(ethRPCURL string, tokenAddress string, chainID uint64, privateKey *ecdsa.PrivateKey, to, amount string) (string, error) {
	// Parse ABI
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return "", fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Parse recipient address
	toAddr := common.HexToAddress(to)

	// Parse amount
	amountBig := new(big.Int)
	if _, ok := amountBig.SetString(amount, 10); !ok {
		return "", fmt.Errorf("invalid amount: %s", amount)
	}

	// Encode transfer call
	data, err := parsedABI.Pack("transfer", toAddr, amountBig)
	if err != nil {
		return "", fmt.Errorf("failed to pack transfer: %w", err)
	}

	// Get sender address
	fromAddr := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Get nonce
	nonceResp, err := ethRPCCall(ethRPCURL, "eth_getTransactionCount", []interface{}{fromAddr.Hex(), "latest"})
	if err != nil {
		return "", fmt.Errorf("failed to get nonce: %w", err)
	}
	var nonceHex string
	if err := json.Unmarshal(nonceResp, &nonceHex); err != nil {
		return "", fmt.Errorf("failed to parse nonce: %w", err)
	}
	nonceBytes, _ := hex.DecodeString(strings.TrimPrefix(nonceHex, "0x"))
	nonce := new(big.Int).SetBytes(nonceBytes).Uint64()

	// Get gas price
	gasPriceResp, err := ethRPCCall(ethRPCURL, "eth_gasPrice", []interface{}{})
	if err != nil {
		return "", fmt.Errorf("failed to get gas price: %w", err)
	}
	var gasPriceHex string
	if err := json.Unmarshal(gasPriceResp, &gasPriceHex); err != nil {
		return "", fmt.Errorf("failed to parse gas price: %w", err)
	}
	gasPriceBytes, _ := hex.DecodeString(strings.TrimPrefix(gasPriceHex, "0x"))
	gasPrice := new(big.Int).SetBytes(gasPriceBytes)

	// Create transaction
	tokenAddr := common.HexToAddress(tokenAddress)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &tokenAddr,
		Value:    big.NewInt(0),
		Gas:      100000,
		GasPrice: gasPrice,
		Data:     data,
	})

	// Sign transaction
	signer := types.NewEIP155Signer(big.NewInt(int64(chainID)))
	signedTx, err := types.SignTx(tx, signer, privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Encode transaction
	txBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to encode transaction: %w", err)
	}

	// Send transaction
	txHashResp, err := ethRPCCall(ethRPCURL, "eth_sendRawTransaction", []interface{}{"0x" + hex.EncodeToString(txBytes)})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	var txHash string
	if err := json.Unmarshal(txHashResp, &txHash); err != nil {
		return "", fmt.Errorf("failed to parse tx hash: %w", err)
	}

	return txHash, nil
}

func callERC20Method(ethRPCURL string, tokenAddress string, method string) (string, error) {
	// Parse ABI
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return "", fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Encode method call (methods like name, symbol, decimals, totalSupply have no params)
	data, err := parsedABI.Pack(method)
	if err != nil {
		return "", fmt.Errorf("failed to pack %s: %w", method, err)
	}

	// Make eth_call request
	params := []interface{}{
		map[string]interface{}{
			"to":   tokenAddress,
			"data": "0x" + hex.EncodeToString(data),
		},
		"latest",
	}

	respData, err := ethRPCCall(ethRPCURL, "eth_call", params)
	if err != nil {
		return "", err
	}

	// Decode result
	var resultHex string
	if err := json.Unmarshal(respData, &resultHex); err != nil {
		return "", fmt.Errorf("failed to parse result: %w", err)
	}

	// Parse hex response
	resultBytes, err := hex.DecodeString(strings.TrimPrefix(resultHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("failed to decode hex: %w", err)
	}

	// Unpack based on method type
	switch method {
	case "name", "symbol":
		// String type
		var result string
		err := parsedABI.UnpackIntoInterface(&result, method, resultBytes)
		if err != nil {
			return "", fmt.Errorf("failed to unpack %s: %w", method, err)
		}
		return result, nil
	case "decimals":
		// uint8 type
		var result uint8
		err := parsedABI.UnpackIntoInterface(&result, method, resultBytes)
		if err != nil {
			return "", fmt.Errorf("failed to unpack decimals: %w", err)
		}
		return fmt.Sprintf("%d", result), nil
	case "totalSupply":
		// uint256 type
		var result *big.Int
		err := parsedABI.UnpackIntoInterface(&result, method, resultBytes)
		if err != nil {
			return "", fmt.Errorf("failed to unpack totalSupply: %w", err)
		}
		return result.String(), nil
	default:
		return "", fmt.Errorf("unsupported method: %s", method)
	}
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
	parts := strings.Split(amount, ".")
	whole := parts[0]
	frac := ""
	if len(parts) > 1 {
		frac = parts[1]
	}

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
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for tx")
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

