//go:build ignore

package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	apiURL    = "https://middleware-api-prod1.02.chainsafe.dev/rpc"
	user1Key  = "eacbff42147f4a4493e2212a70ace9e0ef4e40532e5aa3e049a0eb355e8fc5be"
	user2Key  = "e9fe9a4abcaed48276a80923e1514779a7d7a872d5a9c2fbb2681062e115390f"
	user2Addr = "0x79B3ff7ca5D5eeeF4d60bcEcD5C1294e0F328431"
)

func main() {
	key1, _ := crypto.HexToECDSA(user1Key)
	key2, _ := crypto.HexToECDSA(user2Key)

	fmt.Println("=== Checking Mainnet Canton Balances ===")
	fmt.Println()

	// Check User1 balance
	bal1, err := getBalance(key1)
	if err != nil {
		fmt.Printf("User1 balance error: %v\n", err)
	} else {
		fmt.Printf("✓ User1 balance: %s PROMPT\n", bal1)
	}

	// Check User2 balance
	bal2, err := getBalance(key2)
	if err != nil {
		fmt.Printf("User2 balance error: %v\n", err)
	} else {
		fmt.Printf("✓ User2 balance: %s PROMPT\n", bal2)
	}

	fmt.Println()
	fmt.Println("=== Testing Transfer (User1 → User2, 1.0 PROMPT) ===")
	fmt.Println()

	// Transfer 1.0 PROMPT from User1 to User2
	err = transfer(key1, user2Addr, "1.0")
	if err != nil {
		fmt.Printf("✗ Transfer failed: %v\n", err)
		return
	}
	fmt.Println("✓ Transfer successful!")

	// Wait and check new balances
	time.Sleep(2 * time.Second)
	fmt.Println()
	fmt.Println("=== Final Balances ===")

	bal1, _ = getBalance(key1)
	bal2, _ = getBalance(key2)
	fmt.Printf("✓ User1 final balance: %s PROMPT\n", bal1)
	fmt.Printf("✓ User2 final balance: %s PROMPT\n", bal2)
}

func signEIP191(message string, key *ecdsa.PrivateKey) string {
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256Hash([]byte(prefix + message))
	sig, _ := crypto.Sign(hash.Bytes(), key)
	if sig[64] < 27 {
		sig[64] += 27
	}
	return "0x" + hex.EncodeToString(sig)
}

func rpcCall(key *ecdsa.PrivateKey, method string, params map[string]interface{}) (json.RawMessage, error) {
	ts := time.Now().Unix()
	msg := fmt.Sprintf("%s:%d", method, ts)
	sig := signEIP191(msg, key)

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "method": method, "params": params, "id": 1,
	})

	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Message", msg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respBody, &rpcResp)
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("%s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func getBalance(key *ecdsa.PrivateKey) (string, error) {
	result, err := rpcCall(key, "erc20_balanceOf", map[string]interface{}{})
	if err != nil {
		return "", err
	}
	var bal struct {
		Balance string `json:"balance"`
	}
	json.Unmarshal(result, &bal)
	return bal.Balance, nil
}

func transfer(key *ecdsa.PrivateKey, to, amount string) error {
	_, err := rpcCall(key, "erc20_transfer", map[string]interface{}{"to": to, "amount": amount})
	return err
}

