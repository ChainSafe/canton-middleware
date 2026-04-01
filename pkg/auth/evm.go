package auth

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// VerifyEIP191Signature verifies an EIP-191 personal_sign signature
// Returns the recovered Ethereum address if valid
func VerifyEIP191Signature(message, signature string) (common.Address, error) {
	// Decode signature from hex
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		return common.Address{}, fmt.Errorf("invalid signature hex: %w", err)
	}

	if len(sigBytes) != 65 {
		return common.Address{}, fmt.Errorf("invalid signature length: expected 65, got %d", len(sigBytes))
	}

	// Ethereum signature has recovery id (v) at the end
	// v can be 0, 1, 27, or 28 - normalize to 0 or 1
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	// Create the EIP-191 prefixed message hash
	prefixedMsg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	msgHash := crypto.Keccak256Hash([]byte(prefixedMsg))

	// Recover the public key
	pubKey, err := crypto.SigToPub(msgHash.Bytes(), sigBytes)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to recover public key: %w", err)
	}

	// Derive the address from the public key
	addr := crypto.PubkeyToAddress(*pubKey)
	return addr, nil
}

// ComputeFingerprint computes the fingerprint from an EVM address
// The fingerprint is used to link Canton parties to EVM addresses
func ComputeFingerprint(evmAddress string) string {
	// Normalize the address to checksummed format
	addr := common.HexToAddress(evmAddress)
	// Create fingerprint as keccak256 hash of the address bytes
	hash := crypto.Keccak256Hash(addr.Bytes())
	return hash.Hex()
}

// ValidateEVMAddress checks if a string is a valid EVM address
func ValidateEVMAddress(address string) bool {
	if !strings.HasPrefix(address, "0x") {
		return false
	}
	if len(address) != 42 {
		return false
	}
	_, err := hex.DecodeString(address[2:])
	return err == nil
}

// NormalizeAddress returns a checksummed EVM address
func NormalizeAddress(address string) string {
	return common.HexToAddress(address).Hex()
}

// ValidateTimedMessage checks that a message contains a Unix timestamp suffix
// (format: "{prefix}:{unix_seconds}") and that it is within maxAge of now.
// This provides replay protection: captured signatures expire after maxAge.
func ValidateTimedMessage(msg string, maxAge time.Duration) error {
	idx := strings.LastIndex(msg, ":")
	if idx < 0 || idx == len(msg)-1 {
		return fmt.Errorf("message must contain a colon-separated Unix timestamp (e.g. transfer:1710000000)")
	}

	tsStr := msg[idx+1:]
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp in message: %w", err)
	}

	msgTime := time.Unix(ts, 0)
	age := time.Since(msgTime)
	if age < 0 {
		age = -age
	}
	if age > maxAge {
		return fmt.Errorf("message expired: timestamp is %s old (max %s)", age.Truncate(time.Second), maxAge)
	}

	return nil
}
