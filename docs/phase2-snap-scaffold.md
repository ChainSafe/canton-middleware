# Phase 2: MetaMask Snap Scaffold

**Goal:** Transform the canton-snap repo from a plain TypeScript crypto library into a working MetaMask Snap that derives keys from the user's seed phrase and signs Canton transactions.

## Current State (Phase 1)

```
canton-snap/
├── src/
│   ├── spki.ts          -- compressedPubKey → SPKI DER (proven, DO NOT TOUCH)
│   ├── fingerprint.ts   -- SPKI DER → multihash fingerprint (proven, DO NOT TOUCH)
│   ├── sign.ts          -- (privateKey, hash) → DER signature (proven, DO NOT TOUCH)
│   └── index.ts         -- re-exports
├── test/
│   ├── vectors.json     -- Go-generated test vectors
│   └── crypto.test.ts   -- 28 cross-validation tests (all passing)
├── package.json         -- @noble/curves, @noble/hashes
├── tsconfig.json
└── vitest.config.ts
```

## Target State (Phase 2)

```
canton-snap/
├── packages/
│   └── snap/
│       ├── snap.manifest.json
│       ├── snap.config.ts
│       ├── package.json
│       ├── tsconfig.json
│       ├── src/
│       │   ├── index.ts              -- onRpcRequest dispatcher
│       │   ├── keyDerivation.ts      -- BIP-44 key derivation from MetaMask seed
│       │   ├── dialogs.ts            -- Confirmation dialog builders
│       │   ├── types.ts              -- RPC param/response interfaces
│       │   ├── spki.ts               -- (moved from root, unchanged)
│       │   ├── fingerprint.ts        -- (moved from root, unchanged)
│       │   └── sign.ts              -- (moved from root, unchanged)
│       └── test/
│           ├── index.test.ts         -- Snap RPC handler tests
│           ├── keyDerivation.test.ts -- BIP-44 derivation tests
│           └── vectors.json          -- (moved from root, unchanged)
├── packages/
│   └── test-dapp/                    -- Optional: minimal test page
│       └── index.html
├── package.json                      -- Workspace root
└── README.md
```

## Key Derivation

### Path: `m/44'/60'/1'/0/{keyIndex}`

| Segment | Value | Reason |
|---------|-------|--------|
| Purpose | 44' | BIP-44 standard |
| Coin type | 60' | secp256k1 / Ethereum — only coin type available via `snap_getBip44Entropy` |
| Account | 1' | Canton namespace — avoids collision with ETH account at 0' |
| Change | 0 | External chain (standard) |
| Index | keyIndex | Supports multiple Canton identities (default 0) |

### How it works

1. `snap_getBip44Entropy({ coinType: 60 })` — MetaMask returns the BIP-44 node at `m/44'/60'`
2. `SLIP10Node.fromJSON(node)` — reconstitute using `@metamask/key-tree`
3. `.derive(["bip32:1'", "bip32:0", "bip32:0"])` — walk to `m/44'/60'/1'/0/0`
4. Extract 32-byte private key from the leaf node
5. Derive compressed public key via `secp256k1.getPublicKey(privateKey, true)`

The key is re-derived from the seed on every Snap invocation. Private keys are **never** persisted to `snap_manageState`. The Snap runtime is ephemeral — keys exist in memory only during the RPC call.

## Snap RPC Methods

### `canton_getPublicKey`

| | |
|---|---|
| **Params** | `{ keyIndex?: number }` (default 0) |
| **Returns** | `{ compressedPubKey: string, spkiDer: string, fingerprint: string }` (all hex) |
| **Dialog** | "Export Canton Public Key?" — shows fingerprint, user must approve |

Used during registration. The dApp sends the returned `compressedPubKey` to `POST /register/prepare-topology`.

### `canton_signHash`

| | |
|---|---|
| **Params** | `{ hash: string, keyIndex?: number, metadata?: { operation, tokenSymbol, amount, recipient, sender } }` |
| **Returns** | `{ derSignature: string, fingerprint: string }` (hex) |
| **Dialog** | Shows transaction details (operation, token, amount, recipient) if metadata provided; otherwise shows raw hash |

Used for transfer signing. The dApp sends `hash` from `POST /api/v2/transfer/prepare` response, gets back a DER signature for `POST /api/v2/transfer/execute`.

### `canton_signTopology`

| | |
|---|---|
| **Params** | `{ hash: string, keyIndex?: number }` |
| **Returns** | `{ derSignature: string, fingerprint: string }` (hex) |
| **Dialog** | "Approve Canton Registration?" — explains this links MetaMask to a Canton identity |

Same crypto as `canton_signHash` but with distinct dialog text. Registration is a fundamentally different action from signing a transfer — users deserve clear, separate messaging.

### `canton_getFingerprint`

| | |
|---|---|
| **Params** | `{ keyIndex?: number }` (default 0) |
| **Returns** | `{ fingerprint: string }` (hex) |
| **Dialog** | None — fingerprint alone is not sensitive |

Lightweight lookup for the dApp to check which key is registered.

## Snap Permissions

```json
{
  "snap_getBip44Entropy": [{ "coinType": 60 }],
  "snap_dialog": {},
  "snap_manageState": {},
  "endowment:rpc": { "dapps": true }
}
```

**No `endowment:network-access`.** The Snap is a pure signing oracle. All network communication happens in the dApp or the Go middleware. This minimizes attack surface.

`snap_manageState` is retained for future use (persisting registered fingerprints, user preferences) even though private keys are never stored.

## Confirmation Dialogs

### Export Public Key
```
┌─────────────────────────────────────┐
│  Export Canton Public Key           │
│                                     │
│  A dApp is requesting your Canton   │
│  Network public key for party       │
│  registration.                      │
│                                     │
│  Fingerprint:                       │
│  ┌─────────────────────────────┐   │
│  │ 1220ea5e78baa16cdeb93bf8... │   │
│  └─────────────────────────────┘   │
│                                     │
│  This does NOT expose your          │
│  private key.                       │
│                                     │
│  [Reject]              [Approve]    │
└─────────────────────────────────────┘
```

### Sign Transfer
```
┌─────────────────────────────────────┐
│  Sign Canton Transaction            │
│  ─────────────────────────────────  │
│  Operation:  Transfer               │
│  Token:      DEMO                   │
│  Amount:     100.50                 │
│  To:         0xabcdef...            │
│  From:       0x123456...            │
│  ─────────────────────────────────  │
│  Hash:                              │
│  ┌─────────────────────────────┐   │
│  │ a1b2c3d4e5f6...             │   │
│  └─────────────────────────────┘   │
│                                     │
│  [Reject]              [Approve]    │
└─────────────────────────────────────┘
```

### Sign Topology (Registration)
```
┌─────────────────────────────────────┐
│  Approve Canton Registration        │
│                                     │
│  Sign the topology transaction to   │
│  register your Canton Network       │
│  identity.                          │
│                                     │
│  This links your MetaMask wallet    │
│  to a Canton party.                 │
│  ─────────────────────────────────  │
│  Topology hash:                     │
│  ┌─────────────────────────────┐   │
│  │ 7f8e9a0b1c2d...             │   │
│  └─────────────────────────────┘   │
│                                     │
│  [Reject]              [Approve]    │
└─────────────────────────────────────┘
```

## Dependencies

### New (to add)

| Package | Role |
|---------|------|
| `@metamask/snaps-sdk` ^6.x | Snap API types, UI components (panel, heading, text, etc.) |
| `@metamask/snaps-cli` ^7.x | Build tooling (`mm-snap build`, `mm-snap serve`) |
| `@metamask/key-tree` ^10.x | BIP-32/44 key derivation from `snap_getBip44Entropy` node |
| `@metamask/snaps-jest` ^9.x | Jest preset for testing Snap RPC handlers (devDep) |

### Keep (unchanged from Phase 1)

| Package | Role |
|---------|------|
| `@noble/curves` ^1.8 | secp256k1 ECDSA (proven crypto) |
| `@noble/hashes` ^1.7 | SHA-256 (proven crypto) |

## Build System

Snaps require a specific bundler. `@metamask/snaps-cli` provides `mm-snap build` which:
- Bundles `src/index.ts` into a single `dist/bundle.js` using webpack
- Applies SES (Secure EcmaScript) compatibility transforms
- Regenerates `shasum` in `snap.manifest.json`

**Config:** `snap.config.ts` specifies the entry point and output.

**Commands:**
- `mm-snap build` — production bundle
- `mm-snap watch` — development with auto-rebuild
- `mm-snap serve` — serve bundle on localhost:8080 for local testing

## What NOT to Change

These files are proven by Phase 1 cross-validation (28 tests, byte-identical to Go, Canton integration test passed):

- `spki.ts` — SPKI DER encoding
- `fingerprint.ts` — Multihash fingerprint computation
- `sign.ts` — DER-encoded ECDSA signing

Move them into `packages/snap/src/` but do not modify their logic.

## Task Breakdown

| Task | Description | Depends On | Est. |
|------|-------------|------------|------|
| **S1** | Restructure repo as Snap workspace: move crypto into `packages/snap/src/`, add snap manifest, snap config, workspace package.json | — | 1-2 hours |
| **S2** | Add Snap dependencies (`@metamask/snaps-sdk`, `snaps-cli`, `key-tree`), verify `mm-snap build` produces a valid bundle with SES compatibility | S1 | 1 hour |
| **S3** | Implement `keyDerivation.ts` — BIP-44 derivation via `snap_getBip44Entropy` + `@metamask/key-tree` | S2 | 1-2 hours |
| **S4** | Implement `types.ts` — RPC param/response interfaces | — | 20 min |
| **S5** | Implement `dialogs.ts` — confirmation dialog builders using snaps-sdk UI components | S2 | 45 min |
| **S6** | Implement `index.ts` — `onRpcRequest` handler dispatching to 4 RPC methods, wiring key derivation + dialogs + crypto | S3, S4, S5 | 1-2 hours |
| **S7** | Update `snap.manifest.json` with `snap_getBip44Entropy` permission, rebuild, verify shasum | S6 | 15 min |
| **S8** | Migrate Phase 1 vitest cross-validation tests into new structure, verify they still pass | S1 | 30 min |
| **S9** | Write Snap integration tests with `@metamask/snaps-jest` — all 4 RPC methods (approve + reject flows) | S6, S7 | 2-3 hours |
| **S10** | Build minimal test dApp (`packages/test-dapp/index.html`) — install snap, get key, sign hash buttons | S7 | 1 hour |
| **S11** | Manual E2E smoke test: Flask + test dApp + canton-middleware local env — full registration and transfer flow | S10 + canton env | 1-2 hours |

### Parallelism

```
S1 ──> S2 ──> S3 ──┐
                    ├──> S6 ──> S7 ──> S9 ──> S11
S4 ────────────────┤         └──> S10 ──┘
S5 ────────────────┘
S1 ──> S8 (can run anytime after S1)
```

S1, S4, S5 can start in parallel. S3 depends on S2. S6 is the main integration point. S8 can run independently after S1. S9, S10, S11 are sequential at the end.

**Total estimate: ~1 week**

## Testing on MetaMask

### Flask (Development)
Install MetaMask Flask (separate Chrome extension). Build and serve the snap locally (`mm-snap build && mm-snap serve`). Install via `local:http://localhost:8080` snap ID.

### Production MetaMask
Production MetaMask supports Snaps natively (no longer Flask-only). Same installation flow but may have stricter SES enforcement — verify the bundle works on both.

## Security Properties

1. **Private key never leaves MetaMask.** Derived from seed in Snap sandbox, used for signing, discarded when Snap process exits.
2. **No network access.** Snap cannot exfiltrate keys or data.
3. **User confirms every sensitive operation.** Public key export and signing require dialog approval.
4. **Deterministic key recovery.** Same seed → same Canton key. User's existing MetaMask backup recovers everything.
5. **Auditable.** ~300 lines of snap-specific code (key derivation + dialogs + dispatch). Crypto modules are already proven against Go.
