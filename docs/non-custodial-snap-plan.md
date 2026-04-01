# Non-Custodial Signing via MetaMask Snap — Implementation Plan

## Problem Statement

The Canton middleware currently holds every user's Canton signing key on the server (custodial model). While this enables a seamless MetaMask experience (users add a custom RPC network and use MetaMask's native Send UI), it makes the server a single point of compromise — if breached, every user's Canton key is exposed.

We need a non-custodial option where the user's Canton signing key never leaves their control.

## Why MetaMask Can't Sign Canton Transactions Directly

Canton and Ethereum use the **same elliptic curve** (secp256k1) but **different hash functions**:

| Aspect | Canton | MetaMask/Ethereum |
|--------|--------|-------------------|
| Hash algorithm | SHA-256 | Keccak-256 |
| Signature encoding | ASN.1 DER | Raw 65-byte (r \|\| s \|\| v) |
| Curve | secp256k1 | secp256k1 |

An ECDSA signature is mathematically bound to the specific hash it was computed over. MetaMask always applies Keccak-256 before signing — there is no setting, API, or wrapping trick to change this. A signature over `keccak256(data)` will never verify against `sha256(data)`. This is a fundamental property of ECDSA security, not a software limitation.

### Approaches Ruled Out

| Approach | Why It Fails |
|----------|-------------|
| EIP-712 structured signing | Still Keccak-256 under the hood |
| Raw ECDSA extraction | Signature is bound to the hash — mathematically impossible to re-target |
| `eth_sign` raw hash | Deprecated, disabled by default, MetaMask may still re-hash, terrible UX (blind signing) |
| EIP-4337 account abstraction | Problem is on Canton's side, not Ethereum's |
| WalletConnect custom methods | Just a transport — the wallet still can't do SHA-256 signing |

## Solution: MetaMask Snap

[MetaMask Snaps](https://metamask.io/snaps/) are sandboxed JavaScript plugins that run inside MetaMask. A snap can:

- **Derive a secp256k1 key** from the user's existing seed phrase via BIP-44
- **Sign with SHA-256 + DER encoding** — exactly what Canton requires
- **Show a confirmation dialog** inside MetaMask before every signature
- **Never expose the private key** — it stays in MetaMask's encrypted vault, no network access

The Canton key is deterministically derived from the same seed phrase MetaMask already uses. One backup recovers both Ethereum and Canton keys.

## Architecture: Hybrid (Custodial + Non-Custodial)

Both signing modes coexist. A `key_mode` field on the user record routes between flows.

```
 "Add Network" user (custodial)        Snap user (non-custodial)
        │                                       │
        │ MetaMask native Send UI               │ Web dApp UI
        ▼                                       ▼
   ┌──────────────────┐              ┌─────────────────────────┐
   │ /eth JSON-RPC    │              │ /api/v2/transfer/       │
   │ facade (existing)│              │ prepare → execute       │
   └────────┬─────────┘              └────────────┬────────────┘
            │ server signs                        │ snap signs
            │ with custodial key                  │ in MetaMask
            ▼                                     ▼
   ┌───────────────────────────────────────────────────────┐
   │           Canton Interactive Submission API            │
   │           PrepareSubmission → ExecuteSubmission        │
   └───────────────────────────────────────────────────────┘
```

- **Custodial users**: Add the middleware as a custom network in MetaMask, use the native Send UI. Zero friction. No changes to existing flow.
- **Non-custodial users**: Install the Canton Snap, interact through a web dApp that orchestrates prepare/sign/execute. The server never sees their private key.
- Both produce identical Canton ledger state.

## Snap Design

### Key Derivation

Derivation path: **`m/44'/60'/1'/0/0`**

- Reuses coin type 60 (secp256k1) with account index 1 to segregate from ETH keys
- `snap_getBip44Entropy` provides the BIP-44 node at `m/44'/60'`; the snap derives children via `@metamask/key-tree`
- The snap does NOT get access to MetaMask's actual ETH private key — these are separate keys from the same seed

### Snap RPC Methods

| Method | Purpose | User Dialog |
|--------|---------|-------------|
| `canton_getPublicKey` | Export compressed pubkey + SPKI DER + fingerprint for registration | "Export Canton public key?" |
| `canton_signHash` | Sign a 32-byte SHA-256 hash, return DER signature | Shows operation, token, amount, recipient |
| `canton_signTopology` | Sign topology hash during registration | "Approve Canton party registration?" |
| `canton_getFingerprint` | Quick fingerprint lookup | None |
| `canton_getState` | Return registered key indices | None |

### Snap Permissions

| Permission | Purpose |
|------------|---------|
| `snap_getBip44Entropy` (coinType 60) | Derive Canton secp256k1 keys from seed |
| `snap_dialog` | Show confirmation dialogs for signing |
| `snap_manageState` | Persist key index and fingerprint mappings |
| `endowment:rpc` (dapps: true) | Allow dApp to call snap RPC methods |

**No network access.** The snap is a pure signing oracle — all data flows through the dApp.

### Confirmation Dialog

```
┌─────────────────────────────────────┐
│  Canton Network Transaction         │
│                                     │
│  Operation:  Transfer               │
│  Token:      DEMO                   │
│  Amount:     100.50                 │
│  Recipient:  party-abc123...        │
│  Sender:     party-def456...        │
│                                     │
│  Hash:       a1b2c3d4...           │
│                                     │
│  ⚠ Verify the details match your   │
│  intent before approving.           │
│                                     │
│  [Reject]              [Approve]    │
└─────────────────────────────────────┘
```

## Flows

### Non-Custodial Registration

```
User → dApp: clicks "Register with Snap"
dApp → Snap: canton_getPublicKey()
Snap → User: "Export Canton key?" → Approve
Snap → dApp: { compressedPubKey, spkiDer, fingerprint }
dApp → MetaMask: personal_sign (EIP-191, proves ETH address)
dApp → Server: POST /register/prepare-topology { canton_public_key, signature, message }
Server → Canton: GenerateExternalPartyTopology(pubkey)
Server → dApp: { topology_hash, public_key_fingerprint, registration_token }
dApp → Snap: canton_signTopology(topology_hash)
Snap → User: "Approve Canton registration?" → Approve
Snap → dApp: { derSignature }
dApp → Server: POST /register { key_mode=external, registration_token, topology_signature, canton_public_key }
Server → Canton: AllocateExternalPartyWithSignature(topology, signature)
Server → dApp: { partyId, fingerprint, key_mode=external }
```

### Non-Custodial Transfer

```
User → dApp: initiates transfer (token, amount, recipient)
dApp → Server: POST /api/v2/transfer/prepare { to, amount, token }
       (authenticated via X-Signature / X-Message headers)
Server → Canton: PrepareSubmission(transfer command)
Server → Cache: store PreparedTransfer (2-5 min TTL)
Server → dApp: { transfer_id, transaction_hash, party_id, expires_at }
dApp → Snap: canton_signHash(hash, metadata)
Snap → User: "Sign transfer: 100 DEMO → Alice?" → Approve
Snap → dApp: { derSignature, fingerprint }
dApp → Server: POST /api/v2/transfer/execute { transfer_id, signature, signed_by }
Server: verify signature against stored public key
Server → Canton: ExecuteSubmissionAndWait(prepared_tx, signature)
Server → dApp: { status: "completed" }
```

## Server-Side Status

### Already Implemented (on main, PRs #152-155)

- `POST /api/v2/transfer/prepare` and `POST /api/v2/transfer/execute` endpoints (`pkg/transfer/`)
- `POST /register/prepare-topology` and `POST /register` (external mode) (`pkg/user/service/`)
- `PrepareTransfer()` and `ExecuteTransfer()` on the Canton token SDK (`pkg/cantonsdk/token/`)
- `GenerateExternalPartyTopology()` and `AllocateExternalPartyWithSignature()` (`pkg/cantonsdk/identity/`)
- User model with `KeyMode` field (custodial/external) (`pkg/user/`)
- In-memory caches with TTL for both transfers and topology
- EVM timed message authentication with replay protection
- Unit tests and mocks

### Server-Side Fixes Needed

1. **Signature verification before forwarding to Canton** — Add `keys.VerifyDER(publicKey, hash, signature)` in `Execute()`. Currently garbage signatures get forwarded to Canton, wasting a gRPC round-trip.

2. **Ownership check in Execute** — Verify `pt.PartyID == sender.CantonPartyID` to prevent user B from executing user A's prepared transfer if they know the transfer ID.

3. **Store SPKI public key on user record** — Needed for server-side signature verification (currently only the fingerprint is stored).

4. **Consider bumping cache TTL** — Currently 2 minutes. A MetaMask Snap dialog adds latency; 3-5 minutes is safer for first-time users.

## Security Model

### Threat Analysis

| Threat | Severity | Mitigation |
|--------|----------|------------|
| Server compromise exposes custodial keys | High | Non-custodial users unaffected — key never on server |
| Malicious dApp sends wrong metadata with correct hash | Medium | Snap displays metadata for user verification (same as hardware wallets) |
| Replay of transfer ID | Low | Single-use (deleted after execute), TTL expiry |
| Man-in-the-middle modifies hash | Medium | HTTPS; modified hash won't match PreparedTransaction, Canton rejects |
| Snap supply chain attack | Medium | No network access, pinned versions, auditable code, npm 2FA |
| Private key extraction from snap | Low | SES sandbox, no network, key exists in memory only during signing |
| User B executes user A's prepared transfer | Medium | Fix: add ownership check (pt.PartyID == sender.CantonPartyID) |

### Key Security Properties

1. **Private key never leaves MetaMask** — snap has no network access, cannot exfiltrate
2. **Server never sees the private key** — receives only the public key at registration and signatures at execution
3. **User confirms every signing operation** — snap dialog cannot be bypassed
4. **Server validates everything** — verifies DER signature against stored public key before forwarding to Canton

## Migration Strategy

### Phase 1: Coexistence
Both modes operate simultaneously. `key_mode` routes between them. New users choose at registration time.

### Phase 2: Voluntary Migration
Existing custodial users can migrate:
1. Install the snap
2. dApp gets snap's public key
3. `POST /api/v1/migrate-to-snap` registers new key with Canton (key rotation via topology API)
4. Server deletes encrypted private key from DB
5. Canton party ID stays the same — only the signing key changes

### Phase 3: Optional Deprecation
If desired, new registrations can default to non-custodial. Custodial mode remains available for institutional/API users.

## Development Phases

| Phase | Scope | Estimate |
|-------|-------|----------|
| **1. Crypto compatibility** | TypeScript signing/encoding modules. Cross-validation tests against Go's `pkg/keys/`. Prove Canton accepts TypeScript-generated signatures. | 1-2 weeks |
| **2. Snap scaffold** | Working MetaMask Snap with key derivation, signing, dialogs. Test on MetaMask Flask. | 1 week |
| **3. Server fixes** | Signature verification, ownership check, store SPKI pubkey, bump cache TTL. | 2-3 days |
| **4. Frontend/dApp** | Snap install flow, registration UI, transfer UI, mode detection. | 1 week |
| **5. Migration + hardening** | Custodial-to-snap migration endpoint, error handling, E2E tests. | 1 week |
| **6. Publish** | npm publish, Snaps directory submission, security audit. | Ongoing |

**Phase 1 is the critical risk reducer** — it proves Canton accepts TypeScript-generated signatures before building anything else.

## Why This Approach

1. **Additive, not disruptive** — existing custodial MetaMask flow is untouched
2. **Server-side is mostly done** — prepare/execute endpoints, registration, caches, user model all merged
3. **One seed phrase backs up everything** — Canton key derived from MetaMask seed
4. **Familiar UX** — snap signing dialogs appear inside MetaMask, not a separate app
5. **Auditable and minimal** — snap is ~200 lines of crypto code, no network access, sandboxed
6. **Users choose their trust model** — custodial for convenience, snap for security
