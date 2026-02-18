//go:build ignore

// Register Native Canton User Script
//
// Allocates an external Canton party and registers it as a "native" user
// with the API server. The Canton signing key is passed to the handler so
// the API server can perform Interactive Submission on the user's behalf.
//
// Designed for local demo purposes where SKIP_CANTON_SIG_VERIFY=true is set.
//
// Usage:
//   go run scripts/testing/register-native-user.go -config config.e2e-local.yaml
//
// Output:
//   - Prints EVM address and private key (for MetaMask import)
//   - Saves details to native-user-info.json

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.e2e-local.yaml", "Path to config file")
	apiURL     = flag.String("api-url", "http://localhost:8081", "API server URL")
	partyHint  = flag.String("party-hint", "", "Party hint for allocation (default: auto-generated)")
	outputFile = flag.String("output", "native-user-info.json", "Output file for user info")
)

type NativeUserInfo struct {
	EVMAddress   string `json:"evm_address"`
	PrivateKey   string `json:"private_key"`
	CantonParty  string `json:"canton_party"`
	Fingerprint  string `json:"fingerprint"`
	RegisteredAt string `json:"registered_at"`
}

func main() {
	flag.Parse()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Register Native Canton User (External Party)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	fmt.Println(">>> Connecting to Canton...")
	cantonClient, err := canton.NewFromAppConfig(ctx, &cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("    Connected!")
	fmt.Println()

	hint := *partyHint
	if hint == "" {
		hint = fmt.Sprintf("native_%d", time.Now().Unix())
	}

	// Generate Canton keypair and allocate external party
	fmt.Println(">>> Generating Canton keypair...")
	cantonKeyPair, err := keys.GenerateCantonKeyPair()
	if err != nil {
		fmt.Printf("ERROR: Failed to generate Canton keypair: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(">>> Allocating external Canton party...")
	fmt.Printf("    Party hint: %s\n", hint)

	spkiKey, err := cantonKeyPair.SPKIPublicKey()
	if err != nil {
		fmt.Printf("ERROR: Failed to encode SPKI public key: %v\n", err)
		os.Exit(1)
	}
	party, err := cantonClient.Identity.AllocateExternalParty(ctx, hint, spkiKey, cantonKeyPair)
	if err != nil {
		fmt.Printf("ERROR: Failed to allocate external party: %v\n", err)
		os.Exit(1)
	}

	partyID := party.PartyID
	fmt.Printf("    Party ID: %s\n", partyID)
	fmt.Println()

	// Register with API server, passing Canton key for Interactive Submission
	fmt.Println(">>> Registering with API server...")

	userInfo, err := registerNativeUser(partyID, cantonKeyPair.PrivateKeyHex())
	if err != nil {
		fmt.Printf("ERROR: Registration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("    Registration successful!")
	fmt.Println()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Registration Complete - MetaMask Import Info")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  EVM Address (copy this):")
	fmt.Printf("    %s\n", userInfo.EVMAddress)
	fmt.Println()
	fmt.Println("  Private Key (for MetaMask import, keep secret!):")
	fmt.Printf("    %s\n", userInfo.PrivateKey)
	fmt.Println()
	fmt.Println("  Canton Party ID:")
	fmt.Printf("    %s\n", userInfo.CantonParty)
	fmt.Println()
	fmt.Println("  Fingerprint:")
	fmt.Printf("    %s\n", userInfo.Fingerprint)
	fmt.Println()

	userInfo.RegisteredAt = time.Now().Format(time.RFC3339)
	jsonData, _ := json.MarshalIndent(userInfo, "", "  ")
	if err := os.WriteFile(*outputFile, jsonData, 0600); err != nil {
		fmt.Printf("Warning: Failed to save to %s: %v\n", *outputFile, err)
	} else {
		fmt.Printf("  User info saved to: %s\n", *outputFile)
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Next Steps:")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  1. Import to MetaMask:")
	fmt.Println("     - Click account icon -> 'Import Account'")
	fmt.Println("     - Select 'Private Key'")
	fmt.Println("     - Paste the private key shown above")
	fmt.Println()
	fmt.Println("  2. Add DEMO token to MetaMask:")
	fmt.Println("     - Token Address: 0xDE30000000000000000000000000000000000001")
	fmt.Println("     - Symbol: DEMO")
	fmt.Println("     - Decimals: 18")
	fmt.Println()
	fmt.Println("  3. Open Native Wallet Viewer:")
	fmt.Println("     - Go to http://localhost:8081/web/native-wallet-viewer.html")
	fmt.Println("     - Enter the private key to load wallet")
	fmt.Println("     - Use the address to lookup user details")
	fmt.Println()
}

func registerNativeUser(partyID, cantonPrivKeyHex string) (*NativeUserInfo, error) {
	reqBody := map[string]string{
		"canton_party_id":  partyID,
		"canton_signature": "",
		"message":          fmt.Sprintf("Register for Canton Bridge: %d", time.Now().Unix()),
	}
	if cantonPrivKeyHex != "" {
		reqBody["canton_private_key"] = cantonPrivKeyHex
	}

	jsonBody, _ := json.Marshal(reqBody)

	resp, err := http.Post(*apiURL+"/register", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("%s", errResp.Error)
	}

	var result struct {
		EVMAddress  string `json:"evm_address"`
		PrivateKey  string `json:"private_key"`
		Fingerprint string `json:"fingerprint"`
		Party       string `json:"party"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &NativeUserInfo{
		EVMAddress:  result.EVMAddress,
		PrivateKey:  result.PrivateKey,
		CantonParty: result.Party,
		Fingerprint: result.Fingerprint,
	}, nil
}
