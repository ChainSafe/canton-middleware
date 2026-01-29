# Migration from Ed25519 to secp256k1 for Canton Keys

## Summary

The Canton key management has been migrated from **Ed25519** to **secp256k1** (Ethereum's curve) to enable trustless wallet integration and prepare for MetaMask Snap support.

## Key Changes

### Cryptographic Changes

| Aspect | Ed25519 (Old) | secp256k1 (New) |
|--------|---------------|-----------------|
| **Curve** | Curve25519 | secp256k1 (same as Ethereum) |
| **Private Key Size** | 64 bytes | 32 bytes |
| **Public Key Size** | 32 bytes | 33 bytes (compressed) |
| **Signature Algorithm** | EdDSA | ECDSA with SHA-256 |
| **Signature Size** | 64 bytes | 64 bytes (R \|\| S) |
| **Canton Key Spec** | `SIGNING_KEY_SPEC_EC_CURVE25519` | `SIGNING_KEY_SPEC_EC_SECP256K1` |
| **Canton Algo Spec** | `SIGNING_ALGORITHM_SPEC_ED25519` | `SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256` |
| **Signature Format** | `SIGNATURE_FORMAT_CONCAT` | `SIGNATURE_FORMAT_DER` |

### Modified Files

#### 1. `pkg/keys/canton_keys.go`
- Changed from `crypto/ed25519` to `github.com/ethereum/go-ethereum/crypto`
- `CantonKeyPair` now uses `[]byte` for both keys instead of typed Ed25519 keys
- `GenerateCantonKeyPair()` now generates secp256k1 keys
- `DeriveCantonKeyPair()` derives secp256k1 keys using HKDF
- `Sign()` now returns `([]byte, error)` instead of just `[]byte`
- New `SignHash()` method for signing pre-hashed messages
- `Verify()` uses ECDSA signature recovery and verification
- `EncryptPrivateKey()` expects 32-byte keys (was 64 bytes)
- `DecryptPrivateKey()` validates 32-byte keys (was 64 bytes)

#### 2. `pkg/keys/store.go`
- `KeyStore` interface updated to use `[]byte` instead of `ed25519.PrivateKey`
- All implementations validate key size is exactly 32 bytes
- Removed `crypto/ed25519` import

#### 3. `pkg/keys/canton_keys_test.go`
- Updated all tests to use secp256k1 key sizes
- Added constants for key sizes:
  - `secp256k1PrivateKeySize = 32`
  - `secp256k1PublicKeySize = 33`
- Updated signature verification tests to handle error returns

#### 4. `pkg/registration/handler.go`
- No changes needed - already uses `cantonKeyPair.PrivateKey` which now returns `[]byte`
- Key storage works seamlessly with new secp256k1 keys

## Benefits of secp256k1

### 1. **Ethereum Compatibility**
- Same curve as Ethereum wallets (MetaMask, Ledger, etc.)
- Users can potentially use their Ethereum keys for Canton transactions
- Enables trustless wallet integration

### 2. **Smaller Private Keys**
- 32 bytes instead of 64 bytes
- Reduces storage requirements by 50%
- Faster encryption/decryption

### 3. **Industry Standard**
- Used by Bitcoin, Ethereum, and many other blockchains
- Well-tested and widely understood
- Abundant tooling and library support

### 4. **MetaMask Snap Ready**
- Canton officially supports secp256k1 (`SIGNING_KEY_SPEC_EC_SECP256K1`)
- MetaMask Snaps can access secp256k1 keys via `snap_getBip44Entropy`
- Enables client-side signing with Canton Interactive Submission API

## Database Impact

### Existing Encrypted Keys

⚠️ **IMPORTANT**: Existing Ed25519 keys in the database will **NOT** work with the new code.

**Migration Options:**

1. **Fresh Start** (Recommended for development)
   - Drop and recreate the `users` table
   - Users re-register with new secp256k1 keys

2. **Dual-Format Support** (For production)
   - Add a `key_type` column to track Ed25519 vs secp256k1
   - Maintain both code paths during transition
   - Gradually migrate users to secp256k1

### Database Schema

No schema changes required! The `canton_private_key_encrypted` column remains TEXT and stores base64-encoded encrypted keys. The format is:

```
Old (Ed25519): base64(nonce || AES-GCM-encrypted-64-bytes || tag)
New (secp256k1): base64(nonce || AES-GCM-encrypted-32-bytes || tag)
```

## Testing

All tests pass with the new secp256k1 implementation:

```bash
go test ./pkg/keys/... -v
# PASS: All 9 tests
```

Test coverage includes:
- ✅ Key generation (secp256k1)
- ✅ Key derivation (deterministic)
- ✅ Signing and verification
- ✅ Encryption/decryption
- ✅ Public key encoding (hex/base64)
- ✅ Master key management

## Canton Configuration

### For Custodial Model (Current)

No configuration changes needed - the middleware automatically uses secp256k1 for new registrations.

### For Trustless Model (Future - MetaMask Snap)

When using the Interactive Submission API with client-side signing:

```go
// Client provides secp256k1 public key during registration
publicKey := getPublicKeyFromMetaMaskSnap()

// Register with Canton specifying secp256k1
registerRequest := &RegisterUserRequest{
    CantonPublicKey: publicKey,
    KeySpec: crypto.SigningKeySpec_SIGNING_KEY_SPEC_EC_SECP256K1,
}
```

## Next Steps

### 1. **Update Canton API Calls** (Required)
Add support for specifying key spec when registering parties:
- Update `AllocateParty` to register secp256k1 public key
- Update `RegisterUser` to use correct signing algorithm spec

### 2. **Implement Interactive Submission API** (For Trustless)
```go
// pkg/canton/interactive.go
func (c *Client) PrepareTransfer(ctx context.Context, req *PrepareTransferRequest) (*PreparedTransaction, error)
func (c *Client) ExecuteTransfer(ctx context.Context, req *ExecuteTransferRequest) error
```

### 3. **Build MetaMask Snap** (For Trustless)
Create Canton signing Snap that:
- Derives Canton keys from Ethereum keys
- Signs Canton transaction hashes
- Displays transaction details to users

### 4. **Update Documentation**
- Architecture diagrams showing secp256k1 flow
- API documentation for key registration
- MetaMask Snap integration guide

## Backwards Compatibility

⚠️ **Breaking Change**: This migration is **not backwards compatible** with Ed25519 keys.

**Impact:**
- Existing encrypted Ed25519 keys cannot be decrypted with new code
- Development/staging environments: reset database
- Production: implement migration strategy (dual-format support)

**Compatibility Matrix:**

| Code Version | Ed25519 Keys | secp256k1 Keys |
|--------------|--------------|----------------|
| Old (Ed25519) | ✅ Works | ❌ Fails |
| New (secp256k1) | ❌ Fails | ✅ Works |

## Security Considerations

### Key Storage
- ✅ Same AES-256-GCM encryption as before
- ✅ 32-byte keys are still secure (256-bit security)
- ✅ Master key management unchanged

### Signature Security
- ✅ ECDSA with SHA-256 is industry standard
- ✅ secp256k1 has 128-bit security level
- ✅ Signatures are non-malleable with proper implementation

### Known Issues
- ⚠️ secp256k1 signatures have malleability concerns (use low-S enforcement)
- ⚠️ ECDSA requires secure random number generation (uses crypto/rand)

## References

- [Canton Crypto Proto](../proto/daml/com/daml/ledger/api/v2/crypto.proto)
- [Ethereum go-ethereum/crypto](https://github.com/ethereum/go-ethereum/tree/master/crypto)
- [secp256k1 Specification](https://www.secg.org/sec2-v2.pdf)
- [MetaMask Snaps Documentation](https://docs.metamask.io/snaps/)

## Questions?

For questions about the migration, see:
- Architecture discussion in GitHub issue #XX
- Technical details in `pkg/keys/canton_keys.go`
- Test examples in `pkg/keys/canton_keys_test.go`
