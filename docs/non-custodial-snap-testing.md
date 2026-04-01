# Testing Non-Custodial Transfers with MetaMask Snap

This guide covers how to test the non-custodial prepare/execute transfer API, both locally without a Snap and with a real MetaMask Snap integration.

## Overview

Non-custodial mode lets external signers (e.g., a MetaMask Snap) control their own Canton secp256k1 keys. The server never touches the private key. Two-step flows are used for both registration and transfers:

1. **Registration**: `prepare-topology` → client signs topology hash → `register`
2. **Transfer**: `prepare` → client signs transaction hash → `execute`

Authentication on all endpoints uses EIP-191 `personal_sign` via `X-Signature` / `X-Message` headers.

---

## Prerequisites

```bash
# Bootstrap local environment (Canton, Anvil, Postgres, API server, Relayer)
./scripts/testing/bootstrap-local.sh --clean
```

Services after bootstrap:

| Service | URL |
|---------|-----|
| API Server | `http://localhost:8081` |
| Relayer | `http://localhost:8080` |
| Canton gRPC | `localhost:5011` |
| Anvil (Ethereum) | `http://localhost:8545` |
| PostgreSQL | `localhost:5432` |

---

## Quick Test (No Snap Required)

The integration test script simulates what a Snap would do using local Canton keypairs:

```bash
go run scripts/testing/test-prepare-execute.go \
  -config config.e2e-local.yaml \
  -api-url http://localhost:8081
```

This runs the full happy path: register external user, mint tokens, prepare transfer, sign locally, execute, verify balances.

---

## Cryptographic Requirements

The Snap must implement two distinct signing operations:

### 1. EIP-191 Signature (Authentication)

Standard MetaMask `personal_sign`. Used on every HTTP request.

- **Algorithm**: ECDSA over Keccak-256 (secp256k1)
- **Format**: 65-byte `R || S || V`, hex-encoded with `0x` prefix
- **Message prefix**: `\x19Ethereum Signed Message:\n{len}{message}`

### 2. Canton DER Signature (Transaction/Topology Signing)

This is the non-standard operation the Snap must provide. Canton uses a different hash and encoding than Ethereum.

- **Algorithm**: ECDSA over SHA-256 (secp256k1)
- **Input**: Raw hash bytes from the server (hex-decoded `transaction_hash` or `topology_hash`)
- **Process**: `SHA-256(input_bytes)` → ECDSA sign the 32-byte digest
- **Format**: ASN.1 DER encoded `SEQUENCE { INTEGER r, INTEGER s }` — **not** the Ethereum `R || S || V` format
- **Output**: hex-encoded with `0x` prefix

### 3. Canton Key Fingerprint

The fingerprint is a multihash of the SPKI-encoded public key. It is computed once during registration and reused for all transfers.

- Encode the uncompressed public key as X.509 SubjectPublicKeyInfo DER (OIDs: `1.2.840.10045.2.1` + `1.3.132.0.10`)
- Prepend 4-byte big-endian purpose value `12` to the SPKI bytes
- SHA-256 hash the result
- Prepend multihash prefix: `0x12` (SHA-256 identifier) + `0x20` (32-byte length)
- Result: 68-character hex string like `1220abcd...`

The server returns this fingerprint during registration (`public_key_fingerprint`). The Snap should store it.

---

## Replay Protection

Transfer endpoints (`/api/v2/transfer/*`) require timed messages to prevent signature replay.

**Message format**: `{action}:{unix_timestamp_seconds}`

- Prepare: `transfer:1710000000`
- Execute: `execute:1710000000`

The server rejects messages where the timestamp differs from server time by more than **5 minutes**.

Registration endpoints (`/register/*`) do **not** enforce this format.

---

## API Reference

### Step 1: Register External User

#### 1a. Prepare Topology

```
POST /register/prepare-topology
```

**Headers:**

| Header | Value |
|--------|-------|
| `Content-Type` | `application/json` |
| `X-Signature` | EIP-191 signature (hex, `0x`-prefixed) |
| `X-Message` | Any message string (e.g., `register-external-1710000000`) |

**Request:**
```json
{
  "canton_public_key": "02abc123..."
}
```

`canton_public_key` is the 33-byte compressed secp256k1 public key, hex-encoded. The `0x` prefix is optional.

**Response `200`:**
```json
{
  "topology_hash": "0xdef456...",
  "public_key_fingerprint": "1220abcdef...",
  "registration_token": "550e8400-e29b-41d4-a716-446655440000"
}
```

The `registration_token` expires after **5 minutes**.

#### 1b. Register with Topology Signature

Sign the `topology_hash` with the Canton key:
1. Hex-decode `topology_hash` (strip `0x` prefix) to get raw bytes
2. `SHA-256(raw_bytes)` → 32-byte digest
3. ECDSA sign the digest with the Canton private key
4. DER-encode the signature
5. Hex-encode with `0x` prefix

```
POST /register
```

**Request:**
```json
{
  "signature": "0x<EIP-191 signature>",
  "message": "<message that was signed>",
  "key_mode": "external",
  "canton_public_key": "02abc123...",
  "registration_token": "550e8400-e29b-41d4-a716-446655440000",
  "topology_signature": "0x3045022100..."
}
```

The `canton_public_key` must match the key from step 1a.

**Response `200`:**
```json
{
  "party": "user_f39Fd6e5::1220abc...",
  "fingerprint": "1220e9707d0e...",
  "mapping_cid": "00abc...",
  "evm_address": "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
  "key_mode": "external"
}
```

Store the `public_key_fingerprint` from step 1a — this is the Canton key fingerprint required as the `signed_by` field in execute transfer requests.

---

### Step 2: Transfer Tokens

#### 2a. Prepare Transfer

```
POST /api/v2/transfer/prepare
```

**Headers:**

| Header | Value |
|--------|-------|
| `Content-Type` | `application/json` |
| `X-Signature` | EIP-191 signature of `X-Message` |
| `X-Message` | `transfer:{unix_seconds}` (e.g., `transfer:1710000000`) |

**Request:**
```json
{
  "to": "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
  "amount": "100",
  "token": "DEMO"
}
```

Valid `token` values: `DEMO`, `PROMPT`.

**Response `200`:**
```json
{
  "transfer_id": "a1b2c3d4-...",
  "transaction_hash": "0x789abc...",
  "party_id": "user_f39Fd6e5::1220abc...",
  "expires_at": "2024-03-10T15:04:05Z"
}
```

The `transfer_id` is single-use — once executed or expired, it cannot be reused.

#### 2b. Execute Transfer

Sign the `transaction_hash` with the Canton key (same process as topology signing):
1. Hex-decode `transaction_hash` (strip `0x`) to get raw bytes
2. `SHA-256(raw_bytes)` → 32-byte digest
3. ECDSA sign with Canton private key
4. DER-encode
5. Hex-encode with `0x` prefix

```
POST /api/v2/transfer/execute
```

**Headers:**

| Header | Value |
|--------|-------|
| `Content-Type` | `application/json` |
| `X-Signature` | EIP-191 signature of `X-Message` |
| `X-Message` | `execute:{unix_seconds}` (e.g., `execute:1710000000`) |

**Request:**
```json
{
  "transfer_id": "a1b2c3d4-...",
  "signature": "0x3045022100...",
  "signed_by": "1220abcdef..."
}
```

`signed_by` is the `public_key_fingerprint` returned during registration (step 1a).

**Response `200`:**
```json
{
  "status": "completed"
}
```

---

## Error Reference

### Transfer Prepare (`/api/v2/transfer/prepare`)

| Status | Message | Cause |
|--------|---------|-------|
| 401 | `authentication required` | Missing `X-Signature` or `X-Message` |
| 401 | `message expired or invalid format` | Timestamp missing, unparseable, or >5 min old |
| 401 | `invalid signature` | EIP-191 recovery failed |
| 400 | `to, amount, and token are required` | Missing fields |
| 400 | `invalid recipient address: must be a 0x-prefixed 40-hex-char EVM address` | Bad `to` format |
| 400 | `invalid amount: must be a positive decimal number` | Non-positive or non-numeric |
| 400 | `unsupported token` | Unknown token symbol |
| 401 | `user not found` | Sender not registered |
| 400 | `prepare/execute API requires key_mode=external` | Sender is custodial |
| 400 | `recipient not found` | Recipient not registered |
| 400 | `insufficient balance` | Not enough tokens |

### Transfer Execute (`/api/v2/transfer/execute`)

| Status | Message | Cause |
|--------|---------|-------|
| 401 | `authentication required` | Missing headers |
| 401 | `message expired or invalid format` | Bad timestamp |
| 400 | `transfer_id, signature, and signed_by are required` | Missing fields |
| 403 | `signature fingerprint does not match registered key` | `signed_by` mismatch |
| 400 | `invalid DER signature` | Bad hex encoding |
| 404 | `transfer not found` | Unknown or already-consumed transfer ID |
| 410 | `transfer expired` | TTL exceeded (default 2 min) |

### Registration (`/register/prepare-topology`)

| Status | Message | Cause |
|--------|---------|-------|
| 401 | `signature and message required` | Missing auth |
| 400 | `canton_public_key is required` | Missing key |
| 409 | `user already registered` | EVM address taken |
| 403 | `address not whitelisted for registration` | Not on whitelist |
| 400 | `invalid canton_public_key` | Not a valid 33-byte compressed key |

### Registration (`/register` with `key_mode=external`)

| Status | Message | Cause |
|--------|---------|-------|
| 400 | `registration_token, topology_signature, and canton_public_key are required...` | Missing fields |
| 409 | `user already registered` | Concurrent registration race |
| 404 | `registration token not found or already used` | Token consumed or invalid |
| 410 | `registration token expired` | >5 min since prepare-topology |
| 400 | `canton_public_key does not match the key from prepare-topology` | Key mismatch between steps |
| 400 | `invalid topology_signature hex` | Bad hex |
| 409 | `Canton party already allocated for this user` | Canton-level conflict |

---

## Testing with a Real MetaMask Snap

### Snap RPC Methods to Implement

The Snap needs to expose these JSON-RPC methods:

#### `canton_generateKey`

Generate a secp256k1 keypair and store the private key in Snap secure storage.

**Returns:**
```json
{
  "publicKey": "02abc123...",
  "fingerprint": "1220abcdef..."
}
```

#### `canton_signHash`

Sign a raw hash with the stored Canton private key using SHA-256 + ECDSA, DER-encoded.

**Params:**
```json
{
  "hash": "0x789abc..."
}
```

**Process inside the Snap:**
1. Hex-decode `hash` to bytes
2. `SHA-256(bytes)` → 32-byte digest
3. ECDSA sign digest with stored private key (secp256k1)
4. ASN.1 DER encode: `SEQUENCE { INTEGER r, INTEGER s }`
5. Hex-encode result

**Returns:**
```json
{
  "signature": "0x3045022100..."
}
```

#### `canton_getPublicKey`

Return the stored compressed public key and fingerprint.

**Returns:**
```json
{
  "publicKey": "02abc123...",
  "fingerprint": "1220abcdef..."
}
```

### Dapp Integration Flow

```javascript
// 1. Generate Canton key (one-time, stored in Snap)
const { publicKey, fingerprint } = await ethereum.request({
  method: 'wallet_invokeSnap',
  params: { snapId: 'npm:canton-snap', request: { method: 'canton_generateKey' } }
});

// 2. Prepare topology
const evmMsg = `register-external-${Date.now()}`;
const evmSig = await ethereum.request({
  method: 'personal_sign',
  params: [evmMsg, accounts[0]]
});

const topoResp = await fetch('/register/prepare-topology', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Signature': evmSig,
    'X-Message': evmMsg
  },
  body: JSON.stringify({ canton_public_key: publicKey })
});
const { topology_hash, registration_token, public_key_fingerprint } = await topoResp.json();

// 3. Sign topology with Canton key (via Snap)
const { signature: topoSig } = await ethereum.request({
  method: 'wallet_invokeSnap',
  params: {
    snapId: 'npm:canton-snap',
    request: { method: 'canton_signHash', params: { hash: topology_hash } }
  }
});

// 4. Complete registration
const regMsg = `register-external-${Date.now()}`;
const regSig = await ethereum.request({
  method: 'personal_sign',
  params: [regMsg, accounts[0]]
});

await fetch('/register', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    signature: regSig,
    message: regMsg,
    key_mode: 'external',
    canton_public_key: publicKey,
    registration_token: registration_token,
    topology_signature: topoSig
  })
});

// 5. Prepare transfer
const ts = Math.floor(Date.now() / 1000);
const transferMsg = `transfer:${ts}`;
const transferSig = await ethereum.request({
  method: 'personal_sign',
  params: [transferMsg, accounts[0]]
});

const prepResp = await fetch('/api/v2/transfer/prepare', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Signature': transferSig,
    'X-Message': transferMsg
  },
  body: JSON.stringify({ to: recipientAddress, amount: '100', token: 'DEMO' })
});
const { transfer_id, transaction_hash } = await prepResp.json();

// 6. Sign transaction hash with Canton key (via Snap)
const { signature: txSig } = await ethereum.request({
  method: 'wallet_invokeSnap',
  params: {
    snapId: 'npm:canton-snap',
    request: { method: 'canton_signHash', params: { hash: transaction_hash } }
  }
});

// 7. Execute transfer
const execTs = Math.floor(Date.now() / 1000);
const execMsg = `execute:${execTs}`;
const execSig = await ethereum.request({
  method: 'personal_sign',
  params: [execMsg, accounts[0]]
});

const execResp = await fetch('/api/v2/transfer/execute', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Signature': execSig,
    'X-Message': execMsg
  },
  body: JSON.stringify({
    transfer_id: transfer_id,
    signature: txSig,
    signed_by: public_key_fingerprint
  })
});
const { status } = await execResp.json(); // "completed"
```

### Manual Testing with curl

Replace `<evm_sig>` and `<msg>` with actual EIP-191 signed values. Use the Snap or a local script to generate Canton signatures.

```bash
# Step 1a: Prepare topology
curl -s http://localhost:8081/register/prepare-topology \
  -H "Content-Type: application/json" \
  -H "X-Signature: <evm_sig>" \
  -H "X-Message: <msg>" \
  -d '{"canton_public_key": "02abc..."}' | jq .

# Step 1b: Register (after signing topology_hash with Canton key)
curl -s http://localhost:8081/register \
  -H "Content-Type: application/json" \
  -d '{
    "signature": "<evm_sig>",
    "message": "<msg>",
    "key_mode": "external",
    "canton_public_key": "02abc...",
    "registration_token": "<from step 1a>",
    "topology_signature": "0x3045..."
  }' | jq .

# Step 2a: Prepare transfer (note the timed message format)
curl -s http://localhost:8081/api/v2/transfer/prepare \
  -H "Content-Type: application/json" \
  -H "X-Signature: <evm_sig>" \
  -H "X-Message: transfer:$(date +%s)" \
  -d '{"to": "0x7099...", "amount": "100", "token": "DEMO"}' | jq .

# Step 2b: Execute (after signing transaction_hash with Canton key)
curl -s http://localhost:8081/api/v2/transfer/execute \
  -H "Content-Type: application/json" \
  -H "X-Signature: <evm_sig>" \
  -H "X-Message: execute:$(date +%s)" \
  -d '{
    "transfer_id": "<from step 2a>",
    "signature": "0x3045...",
    "signed_by": "1220abcdef..."
  }' | jq .
```

---

## Timing Constraints

| Constraint | Default | Description |
|------------|---------|-------------|
| Transfer message expiry | 5 min | `X-Message` timestamp must be within 5 min of server time |
| Prepared transfer TTL | 2 min | Time to execute after prepare before the transfer expires |
| Registration token TTL | 5 min | Time to complete step 2 after prepare-topology |
| Background cache cleanup | 30 sec | Interval for removing expired cache entries |
