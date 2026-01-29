# secp256k1 API Changes - Quick Reference

## Key Generation

### Old (Ed25519)
```go
import "crypto/ed25519"

kp, err := keys.GenerateCantonKeyPair()
// kp.PublicKey: ed25519.PublicKey (32 bytes)
// kp.PrivateKey: ed25519.PrivateKey (64 bytes)
```

### New (secp256k1)
```go
kp, err := keys.GenerateCantonKeyPair()
// kp.PublicKey: []byte (33 bytes, compressed)
// kp.PrivateKey: []byte (32 bytes)
```

## Signing

### Old (Ed25519)
```go
message := []byte("sign this")
signature := kp.Sign(message) // []byte, no error
// signature: 64 bytes
```

### New (secp256k1)
```go
message := []byte("sign this")
signature, err := kp.Sign(message) // ([]byte, error)
if err != nil {
    // handle error
}
// signature: 64 bytes (R || S)
```

## Verification

### Old (Ed25519)
```go
import "crypto/ed25519"

valid := ed25519.Verify(kp.PublicKey, message, signature)
```

### New (secp256k1)
```go
valid := kp.Verify(message, signature) // built into CantonKeyPair
```

## Encryption

### Old (Ed25519)
```go
// Encrypts 64-byte private key
encrypted, err := keys.EncryptPrivateKey(kp.PrivateKey, masterKey)
```

### New (secp256k1)
```go
// Encrypts 32-byte private key
encrypted, err := keys.EncryptPrivateKey(kp.PrivateKey, masterKey)
// Same API, different key size
```

## Decryption

### Old (Ed25519)
```go
privateKey, err := keys.DecryptPrivateKey(encrypted, masterKey)
// privateKey: ed25519.PrivateKey (64 bytes)
```

### New (secp256k1)
```go
privateKey, err := keys.DecryptPrivateKey(encrypted, masterKey)
// privateKey: []byte (32 bytes)
```

## KeyStore Interface

### Old (Ed25519)
```go
type KeyStore interface {
    GetUserKey(evmAddress string) (partyID string, privateKey ed25519.PrivateKey, err error)
    SetUserKey(evmAddress, cantonPartyID string, privateKey ed25519.PrivateKey) error
}
```

### New (secp256k1)
```go
type KeyStore interface {
    GetUserKey(evmAddress string) (partyID string, privateKey []byte, err error)
    SetUserKey(evmAddress, cantonPartyID string, privateKey []byte) error
    // privateKey must be exactly 32 bytes
}
```

## Canton Specifications

### Old (Ed25519)
```go
// Canton Proto Values
SigningKeySpec: SIGNING_KEY_SPEC_EC_CURVE25519
SigningAlgorithmSpec: SIGNING_ALGORITHM_SPEC_ED25519
SignatureFormat: SIGNATURE_FORMAT_CONCAT
```

### New (secp256k1)
```go
// Canton Proto Values
SigningKeySpec: SIGNING_KEY_SPEC_EC_SECP256K1
SigningAlgorithmSpec: SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256
SignatureFormat: SIGNATURE_FORMAT_DER
```

## Working with Raw Keys

### Old (Ed25519)
```go
// Create from existing key
privateKey := ed25519.PrivateKey(rawBytes) // 64 bytes

// Extract public key
publicKey := privateKey.Public().(ed25519.PublicKey)

// Sign directly
signature := ed25519.Sign(privateKey, message)
```

### New (secp256k1)
```go
import "github.com/ethereum/go-ethereum/crypto"

// Create from existing key
privateKey, err := crypto.ToECDSA(rawBytes) // 32 bytes

// Extract public key
publicKeyBytes := crypto.CompressPubkey(&privateKey.PublicKey) // 33 bytes

// Sign with hash
hash := sha256.Sum256(message)
signature, err := crypto.Sign(hash[:], privateKey)
// signature: 65 bytes [R || S || V], use first 64 for Canton
```

## Key Derivation

### Old (Ed25519)
```go
import "crypto/ed25519"

seed := make([]byte, 32)
privateKey := ed25519.NewKeyFromSeed(seed)
```

### New (secp256k1)
```go
import "github.com/ethereum/go-ethereum/crypto"

seed := make([]byte, 32)
privateKey, err := crypto.ToECDSA(seed)
```

## Migration Example

### Converting existing code:

**Before:**
```go
// Registration handler
cantonKeyPair, err := keys.GenerateCantonKeyPair()
if err != nil {
    return err
}

// Sign a message
signature := cantonKeyPair.Sign(message)

// Store key
err = keyStore.SetUserKey(evmAddress, partyID, cantonKeyPair.PrivateKey)
```

**After:**
```go
// Registration handler
cantonKeyPair, err := keys.GenerateCantonKeyPair()
if err != nil {
    return err
}

// Sign a message - NOW RETURNS ERROR
signature, err := cantonKeyPair.Sign(message)
if err != nil {
    return fmt.Errorf("signing failed: %w", err)
}

// Store key - same API, different size
err = keyStore.SetUserKey(evmAddress, partyID, cantonKeyPair.PrivateKey)
```

## Key Size Constants

```go
// pkg/keys/canton_keys_test.go
const (
    secp256k1PrivateKeySize = 32 // secp256k1 private key
    secp256k1PublicKeySize  = 33 // Compressed secp256k1 public key
)
```

## Ethereum Compatibility

The new secp256k1 implementation is fully compatible with Ethereum keys:

```go
import (
    "github.com/ethereum/go-ethereum/crypto"
    "github.com/chainsafe/canton-middleware/pkg/keys"
)

// Generate an Ethereum key
ethKey, _ := crypto.GenerateKey()

// Use it as a Canton key
cantonKey := &keys.CantonKeyPair{
    PrivateKey: crypto.FromECDSA(ethKey),
    PublicKey:  crypto.CompressPubkey(&ethKey.PublicKey),
}

// Sign Canton messages with Ethereum key
signature, err := cantonKey.Sign(cantonMessage)

// This enables trustless wallet integration!
```

## Common Pitfalls

### ❌ Don't do this:
```go
// Trying to use old Ed25519 keys
oldEncryptedKey := "..." // from database
privateKey, err := keys.DecryptPrivateKey(oldEncryptedKey, masterKey)
// FAILS: wrong key size (64 bytes vs 32 bytes expected)
```

### ✅ Do this instead:
```go
// Migrate or regenerate keys
// Option 1: Regenerate
user.CantonPrivateKeyEncrypted = nil // clear old key
cantonKeyPair, _ := keys.GenerateCantonKeyPair()

// Option 2: Implement migration
if isEd25519Key(encryptedKey) {
    migrateToSecp256k1(user)
}
```

### ❌ Don't forget error handling:
```go
signature := kp.Sign(message) // Compile error! Sign now returns error
```

### ✅ Handle errors:
```go
signature, err := kp.Sign(message)
if err != nil {
    return fmt.Errorf("signing failed: %w", err)
}
```

## Testing Changes

### Old test:
```go
func TestSigning(t *testing.T) {
    kp, _ := keys.GenerateCantonKeyPair()
    signature := kp.Sign(message)
    if !ed25519.Verify(kp.PublicKey, message, signature) {
        t.Error("verification failed")
    }
}
```

### New test:
```go
func TestSigning(t *testing.T) {
    kp, _ := keys.GenerateCantonKeyPair()
    signature, err := kp.Sign(message)
    if err != nil {
        t.Fatalf("signing failed: %v", err)
    }
    if !kp.Verify(message, signature) {
        t.Error("verification failed")
    }
}
```
