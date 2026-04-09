//go:build e2e

// Package util provides shared cryptographic utilities for the E2E test
// framework. It is imported by both the shim and dsl packages to avoid
// duplicating signing helpers.
package util

import (
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
)

// SignEIP191 produces a 0x-prefixed EIP-191 personal_sign signature of message
// using the hex-encoded ECDSA private key. The recovery ID is normalised to
// 27 or 28 to match the api-server's VerifyEIP191Signature expectation.
func SignEIP191(hexKey, message string) (string, error) {
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return "", fmt.Errorf("parse key: %w", err)
	}
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256Hash([]byte(prefix + message))
	sig, err := crypto.Sign(hash.Bytes(), key)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	sig[64] += 27 // normalise recovery ID to Ethereum convention (27/28)
	return "0x" + hex.EncodeToString(sig), nil
}
