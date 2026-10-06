# CIP-0086: ERC-20 Middleware for Canton Network

This document describes how the Canton Middleware implements [CIP-0086: ERC-20 Middleware and Distributed Indexer for Canton Network](https://github.com/global-synchronizer-foundation/cips/commit/9a646bce15ec273bf728f18d18ba685f30f015ad).

---

## Current Implementation Status

| Component | Status | Description |
|-----------|--------|-------------|
| **API Server** | ✅ Complete | Ethereum JSON-RPC facade at `/eth` |
| **User Registration** | ✅ Complete | EIP-191 signature + whitelist |
| **DEMO Token** | ✅ Complete | Native Canton CIP-56 token |
| **PROMPT Token** | ✅ Complete | Bridged ERC-20 from Ethereum |
| **Relayer (Deposits)** | ✅ Complete | Ethereum → Canton minting |
| **Relayer (Withdrawals)** | ✅ Complete | Canton → Ethereum releases |
| **Balance Reconciliation** | ✅ Complete | Operator-run rebuild from Canton; relayer reconciles stuck transfers every 60s |
| **External Party Model** | ✅ Complete | All users allocated as external parties via Interactive Submission API |
| **Native User Support** | ✅ Complete | Canton external party registration with EVM keypair |
| **Splice Registry API** | ✅ Complete | TransferFactory discovery for Canton Loop / external wallets |
| **Canton Loop Integration** | 🔜 Planned | Connect to global synchronizer |

---

## Architecture Overview

```
┌─────────────────┐          ┌─────────────────┐
│   MetaMask      │          │  Native Canton  │
│   (EVM Wallet)  │          │  User / CLI     │
└────────┬────────┘          └────────┬────────┘
         │                            │
         │ eth_sendRawTransaction     │ Direct gRPC
         │ eth_call                   │
         ▼                            ▼
┌─────────────────────────────────────────────────┐
│              API SERVER (Port 8081)              │
│                                                  │
│  • /eth - Ethereum JSON-RPC (MetaMask compat)   │
│  • /register - User registration (EIP-191)      │
│  • Custodial Canton key management              │
│  • Balance caching in PostgreSQL                │
└─────────────────────────────────────────────────┘
         │                            │
         │                            │
         ▼                            ▼
┌─────────────────────────────────────────────────┐
│           CANTON LEDGER (Source of Truth)        │
│                                                  │
│  CIP-56 DAML Contracts:                         │
│  • FingerprintMapping - EVM ↔ Canton identity   │
│  • CIP56Holding - Token balances                │
│  • TokenMeta - Token configuration              │
└─────────────────────────────────────────────────┘
```

---

## The ERC-20 API Server

A **standard Ethereum JSON-RPC interface** that exposes Canton holdings with ERC-20 semantics. Users interact using MetaMask—no Canton tooling required.

### Supported Operations

| Operation | Method | Description |
|-----------|--------|-------------|
| Query balance | `eth_call` → `balanceOf(address)` | Returns CIP-56 holding balance |
| Transfer tokens | `eth_sendRawTransaction` → `transfer(to, amount)` | Executes CIP-56 transfer |
| Token metadata | `eth_call` → `name()`, `symbol()`, `decimals()` | Returns token info |
| Total supply | `eth_call` → `totalSupply()` | Aggregated from holdings |
| User registration | `POST /register` | Links EVM address to Canton party |

### Token Addresses

| Token | Type | Address |
|-------|------|---------|
| **DEMO** | Native Canton | `0xDE30000000000000000000000000000000000001` |
| **PROMPT** | Bridged ERC-20 | Deployment-specific |

---

## The Canton-EVM Bridge (Relayer)

A bi-directional relay between Ethereum and Canton for the PROMPT token.

### Deposit Flow (Ethereum → Canton)

```
1. User calls depositToCanton() on Bridge Contract
2. Relayer detects DepositToCanton event
3. Relayer resolves recipient via fingerprint mapping
4. Relayer mints CIP-56 tokens on Canton
5. User sees PROMPT balance in MetaMask
```

### Withdrawal Flow (Canton → Ethereum)

```
1. User initiates withdrawal via API
2. Canton creates WithdrawalEvent contract
3. Relayer detects withdrawal event
4. Relayer burns CIP-56 tokens on Canton
5. Relayer releases ERC-20 from Bridge Contract
6. User receives PROMPT on Ethereum
```

---

## CIP-0086 Compliance

### ERC-20 Middleware ✅

- Standard Ethereum JSON-RPC interface compatible with MetaMask and Web3 tooling
- EVM wallet authentication via EIP-191 signatures
- Abstracts DAML contracts behind familiar ERC-20 method selectors

### Distributed Indexer ✅

- PostgreSQL stores cached balances for fast queries
- Real-time sync via relayer event processing
- Relayer reconciliation retries stuck transfers every 60 seconds; balance rebuild is an operator tool

### Cross-Chain Interoperability ✅

- Bi-directional bridge between Ethereum and Canton
- Fingerprint-based identity linking (`keccak256(evmAddress)`)
- Atomic deposit/withdrawal flows

---

## Custodial Model with User-Owned Holdings

The bridge uses a **custodial model** for key management while maintaining **user-owned holdings** on Canton.

### Key Characteristics

| Aspect | Description |
|--------|-------------|
| **Holdings Ownership** | Each CIP56Holding belongs to the user's Canton party |
| **Party Type** | All users are **external parties** (no ~200 internal party limit) |
| **Key Management** | API server custodially holds secp256k1 Canton signing keys (AES-256-GCM encrypted) |
| **Signing** | Transactions use the **Interactive Submission API** (PrepareSubmission/ExecuteSubmission) |

### Visibility

| Party | Can See |
|-------|---------|
| **Bridge Operator** | Mappings, events, transfers they facilitate |
| **User Parties** | Their own holdings and transfers |
| **Other Canton Participants** | Nothing |

### Privacy Guarantees

1. **No global visibility** - Balances aren't broadcast network-wide
2. **User-owned assets** - Holdings belong to user parties, not the operator
3. **User isolation** - Users can't see each other's holdings
4. **Custodial convenience** - Users interact via MetaMask without managing Canton keys

---

## Multi-Token Support

A single deployment can bridge **multiple EVM tokens**, with users holding assets in their own Canton parties.

```
┌─────────────────────────────────────────────────────────┐
│                   Participant Node                      │
│                                                         │
│  EVM Bridges          User Holdings (per party)         │
│  ───────────          ─────────────────────────         │
│  PROMPT Bridge  ←──→  Alice::... owns CIP56Holding     │
│  USDC Bridge    ←──→  Bob::... owns CIP56Holding       │
│  WETH Bridge    ←──→  Carol::... owns CIP56Holding     │
│                                                         │
│  API Server (custodial keys) + Relayer                 │
│  handles all tokens                                     │
└─────────────────────────────────────────────────────────┘
```

### Capabilities

| Capability | Description |
|------------|-------------|
| **Multi-asset portfolios** | Hold multiple tokens simultaneously |
| **Unified identity** | One registration covers all bridged assets |
| **Single API** | One endpoint for all tokens |

---

## Use Cases

| Operator Type | Value Proposition |
|---------------|-------------------|
| **Exchange** | Canton custody with ERC-20 compatibility |
| **Asset Manager** | Private multi-token portfolios |
| **Payment Processor** | Multi-stablecoin settlements |
| **DeFi Protocol** | Canton privacy with EVM tooling |

---

## References

- [CIP-0086: ERC-20 Middleware and Distributed Indexer](https://github.com/global-synchronizer-foundation/cips/commit/9a646bce15ec273bf728f18d18ba685f30f015ad)
- [Global Synchronizer Foundation CIPs](https://github.com/global-synchronizer-foundation/cips)
- [CIP-56 Token Standard](https://github.com/digital-asset/daml-finance)
- [Architecture](./ARCHITECTURE.md) - System design and external party model
- [API Documentation](./API_DOCUMENTATION.md) - Endpoint reference
