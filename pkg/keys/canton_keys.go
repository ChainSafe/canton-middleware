// Package keys provides Canton key generation and encryption for custodial key management.
// This package is used to generate Canton keypairs for users and encrypt them for secure storage.
// Uses secp256k1 (same curve as Ethereum) for compatibility with user wallets.
package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/hkdf"
)

// ASN.1 OIDs for EC public key and secp256k1 curve
var (
	oidECPublicKey = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	oidSecp256k1   = asn1.ObjectIdentifier{1, 3, 132, 0, 10}
)

type spkiAlgorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.ObjectIdentifier
}

type subjectPublicKeyInfo struct {
	Algorithm        spkiAlgorithmIdentifier
	SubjectPublicKey asn1.BitString
}

type ecdsaSignature struct {
	R, S *big.Int
}

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

// CantonKeyPairFromPrivateKey reconstructs a full keypair from a 32-byte private key.
func CantonKeyPairFromPrivateKey(privKey []byte) (*CantonKeyPair, error) {
	if len(privKey) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes, got %d", len(privKey))
	}
	ecdsaKey, err := crypto.ToECDSA(privKey)
	if err != nil {
		return nil, fmt.Errorf("invalid secp256k1 private key: %w", err)
	}
	return &CantonKeyPair{
		PublicKey:  crypto.CompressPubkey(&ecdsaKey.PublicKey),
		PrivateKey: privKey,
	}, nil
}

// PublicKeyHex returns the public key as a hex string (for display/logging)
func (kp *CantonKeyPair) PublicKeyHex() string {
	return fmt.Sprintf("%x", kp.PublicKey)
}

// SPKIPublicKey returns the public key in X.509 SubjectPublicKeyInfo DER format.
func (kp *CantonKeyPair) SPKIPublicKey() ([]byte, error) {
	ecdsaPub, err := crypto.DecompressPubkey(kp.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decompress public key: %w", err)
	}
	uncompressed := elliptic.Marshal(ecdsaPub.Curve, ecdsaPub.X, ecdsaPub.Y)

	return asn1.Marshal(subjectPublicKeyInfo{
		Algorithm: spkiAlgorithmIdentifier{
			Algorithm:  oidECPublicKey,
			Parameters: oidSecp256k1,
		},
		SubjectPublicKey: asn1.BitString{
			Bytes:     uncompressed,
			BitLength: len(uncompressed) * 8,
		},
	})
}

// Fingerprint returns the Canton key fingerprint: multihash-encoded SHA-256
// of the SPKI public key bytes with hash purpose 12.
func (kp *CantonKeyPair) Fingerprint() (string, error) {
	spki, err := kp.SPKIPublicKey()
	if err != nil {
		return "", fmt.Errorf("encode SPKI public key: %w", err)
	}
	var purpose [4]byte
	binary.BigEndian.PutUint32(purpose[:], 12)
	h := sha256.Sum256(append(purpose[:], spki...))
	// Multihash encoding: 0x12 (SHA-256 algo) + 0x20 (32 byte length) + hash
	mh := append([]byte{0x12, 0x20}, h[:]...)
	return fmt.Sprintf("%x", mh), nil
}

// PublicKeyBase64 returns the public key as a base64 string
func (kp *CantonKeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PublicKey)
}

// PrivateKeyHex returns the private key as a hex string with 0x prefix (for MetaMask import)
func (kp *CantonKeyPair) PrivateKeyHex() string {
	return fmt.Sprintf("0x%x", kp.PrivateKey)
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

// SignDER signs a message with SHA-256 and returns an ASN.1 DER-encoded ECDSA signature.
// This is the format Canton requires for Interactive Submission and topology signing.
func (kp *CantonKeyPair) SignDER(message []byte) ([]byte, error) {
	hash := sha256.Sum256(message)
	return kp.SignHashDER(hash[:])
}

// SignHashDER signs a pre-hashed 32-byte digest and returns an ASN.1 DER-encoded signature.
// Use this when Canton provides the hash directly (PrepareSubmission, GenerateExternalPartyTopology).
func (kp *CantonKeyPair) SignHashDER(hash []byte) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("hash must be 32 bytes, got %d", len(hash))
	}

	privateKey, err := crypto.ToECDSA(kp.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %w", err)
	}

	rawSig, err := crypto.Sign(hash, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	r := new(big.Int).SetBytes(rawSig[:32])
	s := new(big.Int).SetBytes(rawSig[32:64])

	derBytes, err := asn1.Marshal(ecdsaSignature{R: r, S: s})
	if err != nil {
		return nil, fmt.Errorf("failed to DER-encode signature: %w", err)
	}
	return derBytes, nil
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

// KeyCipher encrypts and decrypts Canton private keys.
type KeyCipher interface {
	Encrypt(key []byte) (string, error)
	Decrypt(encryptedKey string) ([]byte, error)
}

// MasterKeyCipher implements KeyCipher using AES-256-GCM with a 32-byte master key.
type MasterKeyCipher struct {
	masterKey []byte
}

// NewMasterKeyCipher creates a MasterKeyCipher from a 32-byte master key.
func NewMasterKeyCipher(masterKey []byte) *MasterKeyCipher {
	return &MasterKeyCipher{masterKey: masterKey}
}

// Encrypt encrypts a private key using AES-256-GCM.
func (c *MasterKeyCipher) Encrypt(key []byte) (string, error) {
	return encryptPrivateKey(key, c.masterKey)
}

// Decrypt decrypts an encrypted private key using AES-256-GCM.
func (c *MasterKeyCipher) Decrypt(encryptedKey string) ([]byte, error) {
	return decryptPrivateKey(encryptedKey, c.masterKey)
}

// encryptPrivateKey encrypts the private key using AES-256-GCM with the provided master key.
// Returns the encrypted key as a base64-encoded string containing: nonce || ciphertext || tag
func encryptPrivateKey(privateKey []byte, masterKey []byte) (string, error) {
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

// decryptPrivateKey decrypts an encrypted private key using AES-256-GCM.
// The encrypted string should be base64-encoded containing: nonce || ciphertext || tag
func decryptPrivateKey(encrypted string, masterKey []byte) ([]byte, error) {
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

// DeriveEVMAddressFromPublicKey derives an EVM address from a compressed secp256k1 public key.
// This is used for Canton native users to generate an EVM-compatible address for MetaMask access.
//
// If decompression fails (e.g., invalid public key), it falls back to using the Keccak256
// hash of the compressed key (last 20 bytes with 0x prefix).
func DeriveEVMAddressFromPublicKey(compressedPubKey []byte) string {
	pubKey, err := crypto.DecompressPubkey(compressedPubKey)
	if err != nil {
		// Fallback: use hash of compressed key if decompression fails
		hash := crypto.Keccak256Hash(compressedPubKey)
		return "0x" + hash.Hex()[26:]
	}
	addr := crypto.PubkeyToAddress(*pubKey)
	return addr.Hex()
}
