// Package keys provides Canton key generation and encryption for custodial key management.
// This package is used to generate Canton keypairs for users and encrypt them for secure storage.
package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// CantonKeyPair represents a Canton signing keypair
type CantonKeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateCantonKeyPair generates a new ed25519 keypair for Canton signing
func GenerateCantonKeyPair() (*CantonKeyPair, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 keypair: %w", err)
	}
	return &CantonKeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
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

	// ed25519 seed is 32 bytes
	seed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(hkdfReader, seed); err != nil {
		return nil, fmt.Errorf("failed to derive key seed: %w", err)
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	return &CantonKeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
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

// EncryptPrivateKey encrypts the private key using AES-256-GCM with the provided master key.
// Returns the encrypted key as a base64-encoded string containing: nonce || ciphertext || tag
func EncryptPrivateKey(privateKey ed25519.PrivateKey, masterKey []byte) (string, error) {
	if len(masterKey) != 32 {
		return "", fmt.Errorf("master key must be 32 bytes (AES-256)")
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
	// The private key is 64 bytes for ed25519 (seed + public key)
	ciphertext := gcm.Seal(nonce, nonce, privateKey, nil)

	// Return as base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPrivateKey decrypts an encrypted private key using AES-256-GCM.
// The encrypted string should be base64-encoded containing: nonce || ciphertext || tag
func DecryptPrivateKey(encrypted string, masterKey []byte) (ed25519.PrivateKey, error) {
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

	// Verify it's a valid ed25519 private key (64 bytes)
	if len(plaintext) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("decrypted key has wrong size: got %d, want %d", len(plaintext), ed25519.PrivateKeySize)
	}

	return ed25519.PrivateKey(plaintext), nil
}

// Sign signs a message with the private key
func (kp *CantonKeyPair) Sign(message []byte) []byte {
	return ed25519.Sign(kp.PrivateKey, message)
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
