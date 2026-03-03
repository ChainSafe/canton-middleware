package keys

import (
	"encoding/base64"
	"testing"
)

const (
	secp256k1PrivateKeySize = 32 // secp256k1 private key is 32 bytes
	secp256k1PublicKeySize  = 33 // Compressed secp256k1 public key is 33 bytes
)

func TestGenerateCantonKeyPair(t *testing.T) {
	kp, err := GenerateCantonKeyPair()
	if err != nil {
		t.Fatalf("GenerateCantonKeyPair failed: %v", err)
	}

	// Check key sizes
	if len(kp.PublicKey) != secp256k1PublicKeySize {
		t.Errorf("Expected public key size %d, got %d", secp256k1PublicKeySize, len(kp.PublicKey))
	}
	if len(kp.PrivateKey) != secp256k1PrivateKeySize {
		t.Errorf("Expected private key size %d, got %d", secp256k1PrivateKeySize, len(kp.PrivateKey))
	}

	// Verify the keypair works for signing
	message := []byte("test message")
	signature, err := kp.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if !kp.Verify(message, signature) {
		t.Error("Signature verification failed")
	}
}

func TestDeriveCantonKeyPair(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	evmAddress := "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

	// Derive keypair twice - should get same result
	kp1, err := DeriveCantonKeyPair(evmAddress, seed)
	if err != nil {
		t.Fatalf("DeriveCantonKeyPair failed: %v", err)
	}

	kp2, err := DeriveCantonKeyPair(evmAddress, seed)
	if err != nil {
		t.Fatalf("DeriveCantonKeyPair (2nd call) failed: %v", err)
	}

	// Keys should be identical
	if kp1.PublicKeyHex() != kp2.PublicKeyHex() {
		t.Error("Derived public keys don't match")
	}

	// Different address should give different key
	kp3, err := DeriveCantonKeyPair("0x70997970C51812dc3A010C7d01b50e0d17dc79C8", seed)
	if err != nil {
		t.Fatalf("DeriveCantonKeyPair (different address) failed: %v", err)
	}
	if kp1.PublicKeyHex() == kp3.PublicKeyHex() {
		t.Error("Different addresses produced same key")
	}
}

func TestDeriveCantonKeyPairShortSeed(t *testing.T) {
	shortSeed := make([]byte, 16)
	_, err := DeriveCantonKeyPair("0xtest", shortSeed)
	if err == nil {
		t.Error("Expected error for short seed, got nil")
	}
}

func TestEncryptDecryptPrivateKey(t *testing.T) {
	// Generate a keypair
	kp, err := GenerateCantonKeyPair()
	if err != nil {
		t.Fatalf("GenerateCantonKeyPair failed: %v", err)
	}

	// Generate master key
	masterKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	// Encrypt
	encrypted, err := encryptPrivateKey(kp.PrivateKey, masterKey)
	if err != nil {
		t.Fatalf("EncryptPrivateKey failed: %v", err)
	}

	// Should be base64 encoded
	_, err = base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Errorf("Encrypted key is not valid base64: %v", err)
	}

	// Decrypt
	decrypted, err := decryptPrivateKey(encrypted, masterKey)
	if err != nil {
		t.Fatalf("DecryptPrivateKey failed: %v", err)
	}

	// Should match original
	if len(decrypted) != len(kp.PrivateKey) {
		t.Errorf("Decrypted key length mismatch: got %d, want %d", len(decrypted), len(kp.PrivateKey))
	}
	for i := range decrypted {
		if decrypted[i] != kp.PrivateKey[i] {
			t.Errorf("Decrypted key byte %d mismatch", i)
			break
		}
	}

	// Decrypted key should still work for signing
	message := []byte("test message")
	decryptedKP := &CantonKeyPair{
		PrivateKey: decrypted,
		PublicKey:  kp.PublicKey,
	}
	signature, err := decryptedKP.Sign(message)
	if err != nil {
		t.Fatalf("Sign with decrypted key failed: %v", err)
	}
	if !kp.Verify(message, signature) {
		t.Error("Signature with decrypted key failed verification")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	kp, _ := GenerateCantonKeyPair()
	masterKey1, _ := GenerateMasterKey()
	masterKey2, _ := GenerateMasterKey()

	// Encrypt with key 1
	encrypted, err := encryptPrivateKey(kp.PrivateKey, masterKey1)
	if err != nil {
		t.Fatalf("EncryptPrivateKey failed: %v", err)
	}

	// Try to decrypt with key 2 - should fail
	_, err = decryptPrivateKey(encrypted, masterKey2)
	if err == nil {
		t.Error("Expected error decrypting with wrong key, got nil")
	}
}

func TestEncryptInvalidMasterKeySize(t *testing.T) {
	kp, _ := GenerateCantonKeyPair()

	// Master key too short
	shortKey := make([]byte, 16)
	_, err := encryptPrivateKey(kp.PrivateKey, shortKey)
	if err == nil {
		t.Error("Expected error for short master key")
	}

	// Master key too long
	longKey := make([]byte, 64)
	_, err = encryptPrivateKey(kp.PrivateKey, longKey)
	if err == nil {
		t.Error("Expected error for long master key")
	}
}

func TestMasterKeyConversion(t *testing.T) {
	// Generate
	masterKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	// Convert to base64
	b64 := MasterKeyToBase64(masterKey)

	// Convert back
	recovered, err := MasterKeyFromBase64(b64)
	if err != nil {
		t.Fatalf("MasterKeyFromBase64 failed: %v", err)
	}

	// Should match
	if len(recovered) != len(masterKey) {
		t.Errorf("Recovered key length mismatch")
	}
	for i := range recovered {
		if recovered[i] != masterKey[i] {
			t.Errorf("Recovered key byte %d mismatch", i)
			break
		}
	}
}

func TestMasterKeyFromBase64Invalid(t *testing.T) {
	// Invalid base64
	_, err := MasterKeyFromBase64("not-valid-base64!!!")
	if err == nil {
		t.Error("Expected error for invalid base64")
	}

	// Valid base64 but wrong length
	shortB64 := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err = MasterKeyFromBase64(shortB64)
	if err == nil {
		t.Error("Expected error for wrong key length")
	}
}

func TestPublicKeyEncodings(t *testing.T) {
	kp, _ := GenerateCantonKeyPair()

	// Hex encoding
	hex := kp.PublicKeyHex()
	if len(hex) != secp256k1PublicKeySize*2 {
		t.Errorf("Hex encoding length wrong: got %d, want %d", len(hex), secp256k1PublicKeySize*2)
	}

	// Base64 encoding
	b64 := kp.PublicKeyBase64()
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Errorf("Base64 decoding failed: %v", err)
	}
	if len(decoded) != secp256k1PublicKeySize {
		t.Errorf("Decoded length wrong: got %d, want %d", len(decoded), secp256k1PublicKeySize)
	}
}
