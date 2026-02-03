package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

// VerifyCantonSignature verifies a Canton signature from Loop wallet's signMessage.
// Canton uses secp256k1 (same curve as Ethereum).
// The partyID format is "hint::fingerprint" where fingerprint is a hex-encoded hash.
// Returns true if the signature is valid for the given party.
func VerifyCantonSignature(partyID, message, signature string) (bool, error) {
	// Extract fingerprint from party ID
	fingerprint, err := ExtractFingerprintFromPartyID(partyID)
	if err != nil {
		return false, fmt.Errorf("invalid party ID: %w", err)
	}

	// Decode signature (base64 from Canton dApp SDK)
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		// Try hex decoding as fallback
		sigBytes, err = hex.DecodeString(strings.TrimPrefix(signature, "0x"))
		if err != nil {
			return false, fmt.Errorf("failed to decode signature: %w", err)
		}
	}

	// Canton signatures are typically 64 bytes (R || S) without recovery ID
	// or 65 bytes with recovery ID
	if len(sigBytes) != 64 && len(sigBytes) != 65 {
		return false, fmt.Errorf("invalid signature length: expected 64 or 65, got %d", len(sigBytes))
	}

	// Hash the message with SHA-256 (Canton's standard)
	msgHash := sha256.Sum256([]byte(message))

	// If signature is 64 bytes, we need to try both recovery IDs
	if len(sigBytes) == 64 {
		sig := make([]byte, 65)
		copy(sig, sigBytes)

		// Try recovery ID 0
		sig[64] = 0
		if verifySignatureAgainstFingerprint(msgHash[:], sig, fingerprint) {
			return true, nil
		}

		// Try recovery ID 1
		sig[64] = 1
		if verifySignatureAgainstFingerprint(msgHash[:], sig, fingerprint) {
			return true, nil
		}

		return false, fmt.Errorf("signature verification failed")
	}

	// 65-byte signature with recovery ID
	return verifySignatureAgainstFingerprint(msgHash[:], sigBytes, fingerprint), nil
}

// verifySignatureAgainstFingerprint recovers the public key from signature and verifies
// it matches the expected fingerprint
func verifySignatureAgainstFingerprint(hash, signature []byte, expectedFingerprint string) bool {
	// Recover the public key from signature
	pubKey, err := crypto.SigToPub(hash, signature)
	if err != nil {
		return false
	}

	// Get compressed public key bytes
	pubKeyBytes := crypto.CompressPubkey(pubKey)

	// Compute fingerprint from public key
	// Canton fingerprint is typically the SHA-256 hash of the public key, hex-encoded
	computedHash := sha256.Sum256(pubKeyBytes)
	computedFingerprint := hex.EncodeToString(computedHash[:])

	// Compare fingerprints (case-insensitive)
	return strings.EqualFold(computedFingerprint, expectedFingerprint)
}

// ExtractFingerprintFromPartyID extracts the fingerprint from a Canton party ID.
// Party ID format: "hint::fingerprint" where fingerprint is hex-encoded.
// The fingerprint may have a "1220" prefix (multihash prefix for SHA-256).
func ExtractFingerprintFromPartyID(partyID string) (string, error) {
	parts := strings.Split(partyID, "::")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid party ID format: expected 'hint::fingerprint', got %q", partyID)
	}

	fingerprint := parts[1]

	// Remove 0x prefix if present
	fingerprint = strings.TrimPrefix(fingerprint, "0x")

	// Remove multihash prefix "1220" if present (SHA-256 multihash)
	if strings.HasPrefix(fingerprint, "1220") && len(fingerprint) > 68 {
		fingerprint = fingerprint[4:]
	}

	// Validate fingerprint is hex
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return "", fmt.Errorf("invalid fingerprint hex: %w", err)
	}

	return fingerprint, nil
}

// ExtractHintFromPartyID extracts the hint portion from a Canton party ID.
// Party ID format: "hint::fingerprint"
func ExtractHintFromPartyID(partyID string) (string, error) {
	parts := strings.Split(partyID, "::")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid party ID format: expected 'hint::fingerprint', got %q", partyID)
	}
	return parts[0], nil
}

// IsCantonPartyID checks if a string looks like a Canton party ID.
// Returns true if it contains "::" separator.
func IsCantonPartyID(s string) bool {
	return strings.Contains(s, "::")
}

// ComputeFingerprintFromPublicKey computes a Canton-style fingerprint from a compressed public key.
func ComputeFingerprintFromPublicKey(publicKey []byte) string {
	hash := sha256.Sum256(publicKey)
	return hex.EncodeToString(hash[:])
}

// ValidateCantonPartyID validates that a string is a properly formatted Canton party ID.
func ValidateCantonPartyID(partyID string) error {
	_, err := ExtractFingerprintFromPartyID(partyID)
	return err
}
