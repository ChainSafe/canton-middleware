// Package keys provides Canton key generation and encryption for custodial key management.
// This package is used to generate Canton keypairs for users and encrypt them for secure storage.
// Uses secp256k1 (same curve as Ethereum) for compatibility with user wallets.
package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/hkdf"
)

// CantonKeyPair represents a Canton signing keypair using secp256k1
type CantonKeyPair struct {
	PublicKey  []byte // 33-byte compressed secp256k1 public key
	PrivateKey []byte // 32-byte secp256k1 private key
}

// GenerateCantonKeyPair generates a new secp256k1 keypair for Canton signing
// This uses the same curve as Ethereum for potential wallet integration
func GenerateCantonKeyPair() (*CantonKeyPair, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secp256k1 keypair: %w", err)
	}

	// Get the 32-byte private key
	privateKeyBytes := crypto.FromECDSA(privateKey)

	// Get the compressed 33-byte public key
	publicKeyBytes := crypto.CompressPubkey(&privateKey.PublicKey)

	return &CantonKeyPair{
		PublicKey:  publicKeyBytes,
		PrivateKey: privateKeyBytes,
	}, nil
}

// DeriveCantonKeyPair deterministically derives a Canton keypair from an EVM address and server seed.
// This allows the same keypair to be regenerated if needed (though the encrypted key is preferred).
// Uses HKDF with SHA-256 for key derivation.
func DeriveCantonKeyPair(evmAddress string, serverSeed []byte) (*CantonKeyPair, error) {
	if len(serverSeed) < 32 {
		return nil, fmt.Errorf("server seed must be at least 32 bytes")
	}

	// Create deterministic seed using HKDF
	info := []byte("canton-key-" + evmAddress)
	hkdfReader := hkdf.New(sha256.New, serverSeed, nil, info)

	// secp256k1 private key is 32 bytes
	privateKeyBytes := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, privateKeyBytes); err != nil {
		return nil, fmt.Errorf("failed to derive key seed: %w", err)
	}

	// Convert to ECDSA private key
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key: %w", err)
	}

	// Get compressed public key
	publicKeyBytes := crypto.CompressPubkey(&privateKey.PublicKey)

	return &CantonKeyPair{
		PublicKey:  publicKeyBytes,
		PrivateKey: privateKeyBytes,
	}, nil
}

// PublicKeyHex returns the public key as a hex string (for display/logging)
func (kp *CantonKeyPair) PublicKeyHex() string {
	return fmt.Sprintf("%x", kp.PublicKey)
}

// PublicKeyBase64 returns the public key as a base64 string
func (kp *CantonKeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PublicKey)
}

// PrivateKeyHex returns the private key as a hex string with 0x prefix (for MetaMask import)
func (kp *CantonKeyPair) PrivateKeyHex() string {
	return fmt.Sprintf("0x%x", kp.PrivateKey)
}

// EncryptPrivateKey encrypts the private key using AES-256-GCM with the provided master key.
// Returns the encrypted key as a base64-encoded string containing: nonce || ciphertext || tag
func EncryptPrivateKey(privateKey []byte, masterKey []byte) (string, error) {
	if len(masterKey) != 32 {
		return "", fmt.Errorf("master key must be 32 bytes (AES-256)")
	}

	if len(privateKey) != 32 {
		return "", fmt.Errorf("private key must be 32 bytes (secp256k1)")
	}

	// Create AES cipher
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the private key
	// The private key is 32 bytes for secp256k1
	ciphertext := gcm.Seal(nonce, nonce, privateKey, nil)

	// Return as base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPrivateKey decrypts an encrypted private key using AES-256-GCM.
// The encrypted string should be base64-encoded containing: nonce || ciphertext || tag
func DecryptPrivateKey(encrypted string, masterKey []byte) ([]byte, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (AES-256)")
	}

	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	// Verify it's a valid secp256k1 private key (32 bytes)
	if len(plaintext) != 32 {
		return nil, fmt.Errorf("decrypted key has wrong size: got %d, want 32", len(plaintext))
	}

	return plaintext, nil
}

// Sign signs a message with the private key using ECDSA with SHA-256
// Returns the signature in DER format (compatible with Canton)
func (kp *CantonKeyPair) Sign(message []byte) ([]byte, error) {
	// Convert private key bytes to ECDSA private key
	privateKey, err := crypto.ToECDSA(kp.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %w", err)
	}

	// Hash the message with SHA-256
	hash := sha256.Sum256(message)

	// Sign with ECDSA
	signature, err := crypto.Sign(hash[:], privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// The crypto.Sign function returns a 65-byte signature [R || S || V]
	// For Canton, we need just the DER-encoded signature without the recovery ID
	// We'll return the R and S values (first 64 bytes)
	return signature[:64], nil
}

// SignHash signs a pre-hashed message (useful when Canton provides the hash)
func (kp *CantonKeyPair) SignHash(hash []byte) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("hash must be 32 bytes")
	}

	privateKey, err := crypto.ToECDSA(kp.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %w", err)
	}

	signature, err := crypto.Sign(hash, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return signature[:64], nil
}

// Verify verifies a signature against a message
func (kp *CantonKeyPair) Verify(message, signature []byte) bool {
	// Hash the message
	hash := sha256.Sum256(message)

	if len(signature) != 64 {
		return false
	}

	// Add recovery ID (V) for verification (need to try both 0 and 1)
	sig := make([]byte, 65)
	copy(sig, signature)

	// Try recovery ID 0
	sig[64] = 0
	recoveredPub, err := crypto.SigToPub(hash[:], sig)
	if err == nil {
		// Compare public keys by converting both to addresses
		expectedAddr := crypto.PubkeyToAddress(*recoveredPub)
		actualPub, err := crypto.DecompressPubkey(kp.PublicKey)
		if err == nil {
			actualAddr := crypto.PubkeyToAddress(*actualPub)
			if expectedAddr == actualAddr {
				return true
			}
		}
	}

	// Try recovery ID 1
	sig[64] = 1
	recoveredPub, err = crypto.SigToPub(hash[:], sig)
	if err == nil {
		expectedAddr := crypto.PubkeyToAddress(*recoveredPub)
		actualPub, err := crypto.DecompressPubkey(kp.PublicKey)
		if err == nil {
			actualAddr := crypto.PubkeyToAddress(*actualPub)
			return expectedAddr == actualAddr
		}
	}

	return false
}

// GenerateMasterKey generates a new random 32-byte master key for encrypting Canton keys.
// This should be stored securely (environment variable, secrets manager, etc.)
func GenerateMasterKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}
	return key, nil
}

// MasterKeyFromBase64 decodes a base64-encoded master key
func MasterKeyFromBase64(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

// MasterKeyToBase64 encodes a master key as base64 for storage
func MasterKeyToBase64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}
