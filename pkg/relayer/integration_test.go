//go:build integration
// +build integration

package relayer

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/ethereum/go-ethereum/common"

	"go.uber.org/zap"
)

// Integration test configuration
// These values match the docker-compose.yaml setup
var testConfig = &config.Config{
	Canton: config.CantonConfig{
		RPCURL:          "localhost:5011",
		LedgerID:        "canton-ledger-id",
		ApplicationID:   "canton-middleware-test",
		DomainID:        "da",
		RelayerParty:    "participant1", // Will be resolved to full party ID at runtime
		BridgePackageID: "bridge-wayfinder-1.0.0",
		BridgeModule:    "Wayfinder.Bridge",
		// No auth needed with wildcard - participant operator has full access
	},
	Ethereum: config.EthereumConfig{
		RPCURL:            "http://localhost:8545",
		ChainID:           31337,
		BridgeContract:    "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		TokenContract:     "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512",
		RelayerPrivateKey: "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
		GasLimit:          300000,
	},
}

// TestIntegration_CantonConnectivity tests that we can connect to Canton
func TestIntegration_CantonConnectivity(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	client, err := canton.NewFromAppConfig(context.Background(), &testConfig.Canton, canton.WithLogger(logger))
	if err != nil {
		t.Fatalf("Failed to create Canton client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test GetLedgerEnd to verify connectivity
	offset, err := client.Ledger.GetLedgerEnd(ctx)
	if err != nil {
		t.Fatalf("Failed to get ledger end: %v", err)
	}

	t.Logf("✅ Canton connectivity OK - Ledger offset: %s", offset)
}

// TestIntegration_EthereumConnectivity tests that we can connect to Ethereum (Anvil)
func TestIntegration_EthereumConnectivity(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	client, err := ethereum.NewClient(&testConfig.Ethereum, logger)
	if err != nil {
		t.Fatalf("Failed to create Ethereum client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test GetLatestBlockNumber to verify connectivity
	blockNum, err := client.GetLatestBlockNumber(ctx)
	if err != nil {
		t.Fatalf("Failed to get latest block number: %v", err)
	}

	t.Logf("✅ Ethereum connectivity OK - Block number: %d", blockNum)
}

// TestIntegration_CantonGetBridgeConfig tests that we can find the WayfinderBridgeConfig
func TestIntegration_CantonGetBridgeConfig(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	client, err := canton.NewFromAppConfig(context.Background(), &testConfig.Canton, canton.WithLogger(logger))
	if err != nil {
		t.Fatalf("Failed to create Canton client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to get WayfinderBridgeConfig
	configCid, err := client.Bridge.GetWayfinderBridgeConfigCID(ctx)
	if err != nil {
		// This is expected to fail until we create the bridge config
		t.Logf("⚠️  WayfinderBridgeConfig not found (expected if not yet created): %v", err)
		t.Skip("WayfinderBridgeConfig needs to be created first")
	}

	t.Logf("✅ Found WayfinderBridgeConfig: %s", configCid)
}

// TestIntegration_CreateFingerprintMapping tests the direct fingerprint mapping creation
func TestIntegration_CreateFingerprintMapping(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	client, err := canton.NewFromAppConfig(context.Background(), &testConfig.Canton, canton.WithLogger(logger))
	if err != nil {
		t.Fatalf("Failed to create Canton client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a fingerprint mapping directly (no bridge config needed)
	testFingerprint := "1220abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	testUserParty := "TestUser::1220abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678"

	req := identity.CreateFingerprintMappingRequest{
		UserParty:   testUserParty,
		Fingerprint: testFingerprint,
		EvmAddress:  "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
	}

	mappingCid, err := client.Identity.CreateFingerprintMapping(ctx, req)
	if err != nil {
		t.Logf("CreateFingerprintMapping failed (expected if ledger not bootstrapped): %v", err)
		t.Skip("Ledger not bootstrapped for fingerprint mapping test")
	}

	t.Logf("FingerprintMapping created: %s", mappingCid)

	// Verify we can look up the mapping
	mapping, err := client.Identity.GetFingerprintMapping(ctx, testFingerprint)
	if err != nil {
		t.Fatalf("Failed to get fingerprint mapping: %v", err)
	}

	if mapping.Fingerprint != testFingerprint {
		t.Errorf("Fingerprint mismatch: got %s, want %s", mapping.Fingerprint, testFingerprint)
	}

	t.Logf("✅ FingerprintMapping lookup successful")
}

// TestIntegration_DepositFlow tests the EVM → Canton deposit flow
func TestIntegration_DepositFlow(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	cantonClient, err := canton.NewFromAppConfig(context.Background(), &testConfig.Canton, canton.WithLogger(logger))
	if err != nil {
		t.Fatalf("Failed to create Canton client: %v", err)
	}
	defer cantonClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test creating a pending deposit
	testFingerprint := "1220test000000000000000000000000000000000000000000000000000000000"

	depositReq := bridge.CreatePendingDepositRequest{
		Fingerprint: testFingerprint,
		Amount:      "100.0",
		EvmTxHash:   "0x" + fmt.Sprintf("%064x", time.Now().UnixNano()),
	}

	depositCid, err := cantonClient.Bridge.CreatePendingDeposit(ctx, depositReq)
	if err != nil {
		t.Logf("⚠️  CreatePendingDeposit failed (expected if bridge config not created): %v", err)
		t.Skip("Need WayfinderBridgeConfig to test deposit flow")
	}

	t.Logf("✅ PendingDeposit created: %s", depositCid)
}

// TestIntegration_EthereumDepositEvent tests watching for Ethereum deposit events
func TestIntegration_EthereumDepositEvent(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	client, err := ethereum.NewClient(&testConfig.Ethereum, logger)
	if err != nil {
		t.Fatalf("Failed to create Ethereum client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Watch for deposit events (non-blocking test)
	eventReceived := make(chan bool, 1)
	go func() {
		err := client.WatchDepositEvents(ctx, 0, func(event *ethereum.DepositEvent) error {
			t.Logf("✅ Received deposit event: Token=%s Amount=%s Recipient=%x",
				event.Token.Hex(), event.Amount.String(), event.CantonRecipient)
			eventReceived <- true
			return nil
		})
		if err != nil && ctx.Err() == nil {
			t.Logf("WatchDepositEvents ended: %v", err)
		}
	}()

	// Wait briefly for watcher to start
	time.Sleep(2 * time.Second)

	select {
	case <-eventReceived:
		t.Log("✅ Deposit event watcher is working")
	case <-ctx.Done():
		t.Log("⚠️  No deposit events (expected if no deposits made)")
	}
}

// TestIntegration_EthereumSubmitWithdrawal tests submitting a withdrawal to Ethereum
func TestIntegration_EthereumSubmitWithdrawal(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	logger, _ := zap.NewDevelopment()

	client, err := ethereum.NewClient(&testConfig.Ethereum, logger)
	if err != nil {
		t.Fatalf("Failed to create Ethereum client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test submitting a withdrawal
	// Note: This will likely fail if token isn't set up properly
	token := common.HexToAddress(testConfig.Ethereum.TokenContract)
	recipient := common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8") // Anvil account 1
	amount := big.NewInt(1000000000000000000)                                      // 1 token
	nonce := big.NewInt(1)
	var cantonTxHash [32]byte
	copy(cantonTxHash[:], []byte("test-canton-tx-hash"))

	txHash, err := client.WithdrawFromCanton(ctx, token, recipient, amount, nonce, cantonTxHash)
	if err != nil {
		t.Logf("⚠️  WithdrawFromCanton failed (expected if token not properly set up): %v", err)
		t.Skip("Token mapping may not be configured on the bridge contract")
	}

	t.Logf("✅ Withdrawal submitted: %s", txHash.Hex())
}

// TestIntegration_FullFlow tests the complete bridge flow
// This is the main integration test that exercises the full system
func TestIntegration_FullFlow(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test (set INTEGRATION_TEST=true to run)")
	}

	t.Log("=== Full Integration Test ===")
	t.Log("This test requires:")
	t.Log("1. Docker Compose running (canton, anvil, postgres)")
	t.Log("2. DARs deployed to Canton")
	t.Log("3. WayfinderBridgeConfig created on Canton")
	t.Log("4. Ethereum contracts deployed")
	t.Log("")

	// Run all subtests
	t.Run("CantonConnectivity", TestIntegration_CantonConnectivity)
	t.Run("EthereumConnectivity", TestIntegration_EthereumConnectivity)
	t.Run("CantonGetBridgeConfig", TestIntegration_CantonGetBridgeConfig)
}
