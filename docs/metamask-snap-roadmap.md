# MetaMask Snap Integration Roadmap

## Overview

Now that the codebase uses **secp256k1** (Ethereum's curve), we can implement **trustless Canton signing** via MetaMask Snaps. Users will control their own Canton keys, eliminating trust in the middleware server.

## Architecture Comparison

### Current: Custodial Model
```
User → MetaMask (EVM sig) → API Server (Canton sig) → Canton
       ✅ User controls       ❌ Server controls
```

### Target: Trustless Model
```
User → MetaMask Snap (EVM sig + Canton sig) → API Server (relay) → Canton
       ✅ User controls       ✅ User controls
```

## Implementation Phases

### Phase 1: Server-Side Interactive Submission API ✅ Complete

**Status:** Fully implemented and tested on both local and DevNet environments.

**Completed:**
- ✅ secp256k1 key generation and Canton key fingerprint computation
- ✅ AES-256-GCM key encryption/decryption with `CANTON_MASTER_KEY`
- ✅ Key storage in PostgreSQL
- ✅ External party allocation via `AllocateExternalParty` (topology signing)
- ✅ Interactive Submission API: `PrepareSubmission` / `ExecuteSubmission`
- ✅ CIP-56 token transfers via Interactive Submission for all external party users
- ✅ Splice `HoldingV1` and `TransferFactory` interface compliance
- ✅ Splice Registry API for TransferFactory discovery (Canton Loop interop)
- ✅ End-to-end tested: MetaMask-to-native, native-to-native, native-to-MetaMask transfers

**Key implementation files:**
- `pkg/cantonsdk/token/` -- CIP-56 token operations (transfer, mint, query)
- `pkg/cantonsdk/identity/` -- External party allocation, fingerprint mappings
- `pkg/cantonsdk/ledger/` -- gRPC client with ISA support
- `pkg/service/` -- Token service coordinating Interactive Submission
- `pkg/keys/` -- secp256k1 Canton keypair management

### Phase 2: MetaMask Snap Development 🔄 (Next)

**Status:** Ready to start

**Goals:**
- Create Canton signing Snap
- Implement secp256k1 signing with user's Ethereum key
- Build UI for transaction approval

**Tasks:**

#### 2.1 Snap Package Setup
```bash
mkdir -p packages/canton-snap
cd packages/canton-snap
npm init -y
npm install --save @metamask/snaps-sdk @metamask/snaps-types
```

**Files to Create:**
```
packages/canton-snap/
├── snap.manifest.json      # Snap configuration
├── package.json
├── src/
│   ├── index.ts           # Main snap entry point
│   ├── canton/
│   │   ├── signer.ts      # secp256k1 Canton signing
│   │   ├── types.ts       # Canton types
│   │   └── format.ts      # Transaction formatting
│   └── ui/
│       └── transaction.tsx # Transaction approval UI
└── test/
    └── index.test.ts
```

#### 2.2 Core Snap Implementation

**snap.manifest.json:**
```json
{
  "version": "1.0.0",
  "description": "Sign Canton transactions with your Ethereum key",
  "proposedName": "Canton Signer",
  "initialPermissions": {
    "snap_dialog": {},
    "snap_getBip44Entropy": [
      { "coinType": 60 }  // Ethereum - use same key!
    ]
  }
}
```

**src/index.ts:**
```typescript
import { OnRpcRequestHandler } from '@metamask/snaps-types';
import { signCantonTransaction } from './canton/signer';
import { panel, heading, text } from '@metamask/snaps-ui';

export const onRpcRequest: OnRpcRequestHandler = async ({ request }) => {
  switch (request.method) {
    case 'canton_getPublicKey':
      return await getCantonPublicKey();

    case 'canton_signTransaction':
      // Show transaction details
      const approved = await snap.request({
        method: 'snap_dialog',
        params: {
          type: 'confirmation',
          content: panel([
            heading('Canton Transaction'),
            text(`Transfer ${request.params.amount} tokens`),
            text(`To: ${request.params.recipient}`),
          ]),
        },
      });

      if (!approved) throw new Error('User rejected');

      return await signCantonTransaction(request.params);

    default:
      throw new Error('Method not found');
  }
};
```

**src/canton/signer.ts:**
```typescript
import { getBIP44AddressKeyDeriver } from '@metamask/key-tree';
import * as secp256k1 from 'secp256k1';
import { sha256 } from '@noble/hashes/sha256';

export async function getCantonPrivateKey(): Promise<Uint8Array> {
  // Get entropy for Ethereum's coin type (60)
  const entropy = await snap.request({
    method: 'snap_getBip44Entropy',
    params: { coinType: 60 },
  });

  // Derive the key at m/44'/60'/0'/0/0 (same as ETH account)
  const deriver = await getBIP44AddressKeyDeriver(entropy);
  const { privateKey } = await deriver(0);

  return privateKey;
}

export async function signCantonTransaction(params: {
  transactionHash: string;
  party: string;
}): Promise<CantonSignature> {
  const privateKey = await getCantonPrivateKey();
  const hash = Buffer.from(params.transactionHash.replace('0x', ''), 'hex');

  // Sign with secp256k1 (same as Ethereum)
  const signature = secp256k1.ecdsaSign(hash, privateKey);

  // Format for Canton (DER encoding)
  const derSignature = secp256k1.signatureExport(signature.signature);

  return {
    format: 'SIGNATURE_FORMAT_DER',
    signature: Buffer.from(derSignature).toString('base64'),
    signed_by: params.party,
    signing_algorithm_spec: 'SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256',
  };
}

export async function getCantonPublicKey(): Promise<string> {
  const privateKey = await getCantonPrivateKey();
  const publicKey = secp256k1.publicKeyCreate(privateKey, true); // compressed
  return Buffer.from(publicKey).toString('hex');
}
```

#### 2.3 Testing

```typescript
// test/index.test.ts
import { expect } from '@jest/globals';
import { installSnap } from '@metamask/snaps-jest';

describe('Canton Snap', () => {
  it('should get public key', async () => {
    const { request } = await installSnap();
    const response = await request({
      method: 'canton_getPublicKey',
    });
    expect(response).toMatch(/^[0-9a-f]{66}$/); // 33 bytes hex
  });

  it('should sign transaction', async () => {
    const { request } = await installSnap();
    const response = await request({
      method: 'canton_signTransaction',
      params: {
        transactionHash: '0x1234...',
        party: 'user_abc::1220...',
      },
    });
    expect(response.signature).toBeDefined();
  });
});
```

### Phase 3: API Server Integration 🔜

**Status:** Depends on Phase 1 & 2

**Goals:**
- Add endpoints for prepare/execute flow
- Support client-side signatures
- Maintain backwards compatibility with custodial mode

**New API Endpoints:**

```typescript
// POST /api/v2/transfer/prepare
{
  "from": "0xf39Fd6...",
  "to": "0x70997...",
  "amount": "100.0"
}

Response:
{
  "transaction_hash": "0x1234...",
  "party_id": "user_f39Fd6::1220...",
  "fingerprint": "0xabc...",
  "expires_at": "2026-01-29T12:00:00Z"
}

// POST /api/v2/transfer/execute
{
  "transaction_hash": "0x1234...",
  "signature": "base64-encoded-signature",
  "party_id": "user_f39Fd6::1220..."
}

Response:
{
  "tx_id": "...",
  "status": "submitted"
}
```

**Go Implementation:**

```go
// pkg/api/transfer_v2.go
type PrepareTransferRequest struct {
    From   string `json:"from"`
    To     string `json:"to"`
    Amount string `json:"amount"`
}

type PrepareTransferResponse struct {
    TransactionHash string    `json:"transaction_hash"`
    PartyID         string    `json:"party_id"`
    Fingerprint     string    `json:"fingerprint"`
    ExpiresAt       time.Time `json:"expires_at"`
}

func (h *Handler) PrepareTransfer(w http.ResponseWriter, r *http.Request) {
    var req PrepareTransferRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Verify EVM signature (authentication)
    evmAddress, err := h.verifyEVMAuth(r)
    if err != nil {
        h.writeError(w, 401, "unauthorized")
        return
    }

    // Get user's Canton party
    user, err := h.db.GetUserByEVMAddress(evmAddress)
    if err != nil {
        h.writeError(w, 404, "user not found")
        return
    }

    // Prepare Canton transaction (server-side)
    prepared, err := h.cantonClient.PrepareTransfer(r.Context(), &canton.PrepareTransferRequest{
        UserParty:  user.CantonParty,
        ToAddress:  req.To,
        Amount:     req.Amount,
    })
    if err != nil {
        h.writeError(w, 500, "prepare failed")
        return
    }

    // Return unsigned transaction hash to client
    h.writeJSON(w, 200, PrepareTransferResponse{
        TransactionHash: prepared.TransactionHash,
        PartyID:         user.CantonParty,
        Fingerprint:     user.Fingerprint,
        ExpiresAt:       time.Now().Add(5 * time.Minute),
    })
}

type ExecuteTransferRequest struct {
    TransactionHash string `json:"transaction_hash"`
    Signature       string `json:"signature"`
    PartyID         string `json:"party_id"`
}

func (h *Handler) ExecuteTransfer(w http.ResponseWriter, r *http.Request) {
    var req ExecuteTransferRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Verify the signature is from the correct party
    // (prevents replay attacks)

    // Submit to Canton with user's signature
    err := h.cantonClient.ExecuteTransfer(r.Context(), &canton.ExecuteTransferRequest{
        TransactionHash: req.TransactionHash,
        Signature:       req.Signature,
        PartyID:         req.PartyID,
    })
    if err != nil {
        h.writeError(w, 500, "execution failed")
        return
    }

    h.writeJSON(w, 200, map[string]string{
        "status": "submitted",
    })
}
```

### Phase 4: Frontend Integration 🔜

**Status:** Depends on Phase 2 & 3

**Goals:**
- Detect if Canton Snap is installed
- Fall back to custodial mode if not available
- Provide smooth UX for transaction signing

**React Example:**

```typescript
// hooks/useCantonSnap.ts
import { useEffect, useState } from 'react';

const SNAP_ID = 'npm:@chainsafe/canton-snap';

export function useCantonSnap() {
  const [isInstalled, setIsInstalled] = useState(false);
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    checkSnapInstalled();
  }, []);

  async function checkSnapInstalled() {
    try {
      const snaps = await window.ethereum.request({
        method: 'wallet_getSnaps',
      });
      setIsInstalled(SNAP_ID in snaps);
    } catch (err) {
      setIsInstalled(false);
    }
  }

  async function connectSnap() {
    await window.ethereum.request({
      method: 'wallet_requestSnaps',
      params: { [SNAP_ID]: {} },
    });
    setIsConnected(true);
  }

  async function signTransaction(txHash: string, party: string) {
    return await window.ethereum.request({
      method: 'wallet_invokeSnap',
      params: {
        snapId: SNAP_ID,
        request: {
          method: 'canton_signTransaction',
          params: { transactionHash: txHash, party },
        },
      },
    });
  }

  return { isInstalled, isConnected, connectSnap, signTransaction };
}

// components/TransferButton.tsx
export function TransferButton({ to, amount }: Props) {
  const { isInstalled, signTransaction } = useCantonSnap();
  const [loading, setLoading] = useState(false);

  async function handleTransfer() {
    setLoading(true);

    try {
      if (isInstalled) {
        // Trustless mode: user signs with Snap
        const prepared = await fetch('/api/v2/transfer/prepare', {
          method: 'POST',
          body: JSON.stringify({ to, amount }),
        }).then(r => r.json());

        const signature = await signTransaction(
          prepared.transaction_hash,
          prepared.party_id
        );

        await fetch('/api/v2/transfer/execute', {
          method: 'POST',
          body: JSON.stringify({
            transaction_hash: prepared.transaction_hash,
            signature,
            party_id: prepared.party_id,
          }),
        });
      } else {
        // Custodial mode: server signs
        await fetch('/api/transfer', {
          method: 'POST',
          body: JSON.stringify({ to, amount }),
        });
      }

      toast.success('Transfer submitted!');
    } finally {
      setLoading(false);
    }
  }

  return (
    <button onClick={handleTransfer} disabled={loading}>
      {loading ? 'Processing...' : `Transfer ${amount}`}
      {isInstalled && <span>🔒 Trustless</span>}
    </button>
  );
}
```

### Phase 5: Testing & Security Audit 🔜

**Testing Checklist:**
- [ ] Unit tests for Snap signing
- [ ] Integration tests for prepare/execute flow
- [ ] End-to-end tests with real Canton
- [ ] Test key recovery scenarios
- [ ] Test replay attack prevention
- [ ] Load testing for concurrent transactions

**Security Considerations:**
- [ ] Transaction expiry (prevent stale signatures)
- [ ] Nonce management (prevent replay)
- [ ] Signature validation (verify party matches)
- [ ] Rate limiting on prepare endpoint
- [ ] Audit smart contract upgrade paths

### Phase 6: Production Deployment 🔜

**Deployment Checklist:**
- [ ] Publish Snap to MetaMask Snap Store
- [ ] Update API server with v2 endpoints
- [ ] Migrate existing users (optional)
- [ ] Documentation for users
- [ ] Support for both modes (custodial + trustless)

## Migration Strategy

### Option A: Dual Mode (Recommended)

Support both custodial and trustless modes simultaneously:

```typescript
enum KeyManagementMode {
  CUSTODIAL = 'custodial',   // Server holds keys
  TRUSTLESS = 'trustless',   // User holds keys via Snap
}

// Database: Add column
ALTER TABLE users ADD COLUMN key_mode VARCHAR(20) DEFAULT 'custodial';

// API: Detect mode and route accordingly
if (user.key_mode === 'custodial') {
  // Sign with server key
  signature = await serverSign(transaction);
} else {
  // Return unsigned transaction for client signing
  return prepareTransaction(transaction);
}
```

### Option B: Full Migration

Migrate all users to trustless mode:

1. Announce migration timeline
2. Prompt users to install Snap
3. Deprecate custodial endpoints
4. Remove server-side key storage

## Success Metrics

- **Trustless Adoption:** % of users using Snap vs custodial
- **Transaction Success Rate:** Successful executions / total attempts
- **UX Metrics:** Time to complete transfer (custodial vs trustless)
- **Security:** Zero compromised user keys

## Timeline Estimate

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Interactive API | 1-2 weeks | secp256k1 migration ✅ |
| Phase 2: MetaMask Snap | 2-3 weeks | Phase 1 |
| Phase 3: API Integration | 1-2 weeks | Phase 1, 2 |
| Phase 4: Frontend | 1 week | Phase 2, 3 |
| Phase 5: Testing | 2 weeks | Phase 4 |
| Phase 6: Deployment | 1 week | Phase 5 |
| **Total** | **8-11 weeks** | |

## Resources

- [MetaMask Snaps Documentation](https://docs.metamask.io/snaps/)
- [Canton Interactive Submission API](../pkg/cantonsdk/lapi/v2/) -- Generated protobuf stubs
- [secp256k1 Implementation](../pkg/keys/canton_keys.go)
- [Architecture](./ARCHITECTURE.md) -- External party model and ISA overview

## Questions & Decisions

### Q: Should we support both custodial and trustless modes?
**A:** Yes (Option A). Gradual adoption, fallback for users without Snap.

### Q: How to handle users who lose their Snap?
**A:** Snap keys are recoverable via MetaMask seed phrase (same as Ethereum).

### Q: What if Canton doesn't support secp256k1?
**A:** Canton DOES support secp256k1 (`SIGNING_KEY_SPEC_EC_SECP256K1` in crypto.proto).

### Q: Performance impact of client-side signing?
**A:** Minimal - signing is fast, network latency dominates (2-step flow).

## Next Action

**Start Phase 2**: Build the MetaMask Snap for client-side Canton signing. Phase 1 (server-side Interactive Submission) is complete and running in production.
