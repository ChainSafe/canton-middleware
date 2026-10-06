# Canton Middleware Architecture

This document describes how MetaMask, the API Server, the Relayer, and the Canton Ledger work together to enable ERC-20 compatible token operations on Canton Network.

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              USER LAYER                                      │
│                                                                              │
│    ┌──────────────────┐                      ┌──────────────────┐           │
│    │  🦊 MetaMask     │                      │  Native Canton   │           │
│    │  (EVM Wallet)    │                      │  User / CLI      │           │
│    └────────┬─────────┘                      └────────┬─────────┘           │
│             │ eth_sendRawTransaction                  │ eth_sendRawTransaction│
│             │ eth_call, eth_getBalance                │ (via /eth endpoint) │
└─────────────┼─────────────────────────────────────────┼─────────────────────┘
              │                                         │
              ▼                                         │
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MIDDLEWARE LAYER                                   │
│                                                                              │
│  ┌─────────────────────────────┐      ┌─────────────────────────┐           │
│  │      API SERVER             │      │        RELAYER          │           │
│  │      (Port 8081)            │      │    (Background Service) │           │
│  │                             │      │                         │           │
│  │  • /eth - JSON-RPC facade   │      │  • Ethereum → Canton    │           │
│  │  • /register - User signup  │      │  • Canton → Ethereum    │           │
│  │  • /health - Status check   │      │  • Event processing     │           │
│  │                             │      │                         │           │
│  │  Custodial key management:  │      │  Bridges PROMPT token   │           │
│  │  Holds Canton keys for all  │      │  between chains         │           │
│  │  registered users           │      │                         │           │
│  └──────────────┬──────────────┘      └──────────────┬──────────┘           │
│                 │                                    │                       │
│                 └─────────────┬───────────────────────┘                      │
│                               │                                              │
│                    ┌──────────▼──────────┐                                   │
│                    │     PostgreSQL      │                                   │
│                    │     (Port 5432)     │                                   │
│                    │                     │                                   │
│                    │  • User registry    │                                   │
│                    │  • Balance cache    │                                   │
│                    │  • Transfer state   │                                   │
│                    │  • Chain offsets    │                                   │
│                    └──────────┬──────────┘                                   │
└───────────────────────────────┼──────────────────────────────────────────────┘
                                │
              gRPC + OAuth2     │
                                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CANTON LEDGER                                        │
│                        (Source of Truth)                                     │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                      DAML Smart Contracts (CIP-56)                     │  │
│  │                                                                        │  │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐     │  │
│  │  │ FingerprintMapping│  │   CIP56Holding   │  │    TokenMeta     │     │  │
│  │  │                  │  │                  │  │                  │     │  │
│  │  │ Links EVM addr   │  │ Actual token     │  │ DEMO: Native     │     │  │
│  │  │ to Canton Party  │  │ balances per     │  │ PROMPT: Bridged  │     │  │
│  │  │                  │  │ user per token   │  │                  │     │  │
│  │  └──────────────────┘  └──────────────────┘  └──────────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Ports: 5011 (gRPC), 5013 (HTTP)                                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                ▲
                                │ Bridge Events (PROMPT only)
                                │
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ETHEREUM (Anvil Local / Sepolia Testnet)                  │
│                                                                              │
│  ┌──────────────────────────┐    ┌──────────────────────────┐               │
│  │    Bridge Contract       │◄───│    PROMPT Token          │               │
│  │                          │    │    (ERC-20)              │               │
│  │  • depositToCanton()     │    │                          │               │
│  │  • withdrawToEthereum()  │    │  Local: 0x5FbDB231...    │               │
│  │                          │    │                          │               │
│  │  Local: 0xe7f1725E...    │    │                          │               │
│  └──────────────────────────┘    └──────────────────────────┘               │
│                                                                              │
│  Port: 8545 (Anvil)                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Components

### API Server

The API Server provides an **Ethereum JSON-RPC compatible interface** that allows MetaMask and other EVM wallets to interact with Canton tokens.

| Endpoint | Purpose |
|----------|---------|
| `/eth` | JSON-RPC facade (eth_call, eth_sendRawTransaction, etc.) |
| `/register` | User registration with EIP-191 signature |
| `/health` | Health check endpoint |

**Key responsibilities:**
- Translate ERC-20 calls to CIP-56 DAML operations
- Manage custodial Canton keys for all registered users
- Cache balances in PostgreSQL for fast queries
- Expose an operator-run tool to reconcile the database cache against the Canton ledger

### Relayer

The Relayer bridges PROMPT tokens between Ethereum and Canton.

**Bidirectional processing:**
- **Ethereum → Canton**: Watches for `depositToCanton()` events, mints CIP-56 tokens
- **Canton → Ethereum**: Watches for withdrawal requests, releases ERC-20 tokens

**Design principles:**
- At-least-once delivery with idempotency
- Crash recovery via persisted offsets
- Database-backed deduplication

### Indexer

A standalone Go service (`cmd/indexer`) that subscribes to the Canton Ledger API and materialises CIP-56
contract state into PostgreSQL.

- Streams Holding creations and archivals, TransferFactory choices and bridge events from the Ledger API
- Aggregates UTXO-style holdings deterministically into per-party balances and per-token total supply
- Serves balance, supply and event queries over HTTP to the API server and to external consumers
- Deployed per node, scoped to the visibility of the participant it connects to, so each operator indexes
  only what their participant can see
- Published as a multi-architecture image at `ghcr.io/chainsafe/canton-indexer`

The API server can be configured to serve ERC-20 reads from the indexer rather than querying the ledger
directly, via `token_provider.mode: indexer`.

### PostgreSQL Database

Serves as a **distributed indexer** and cache:
- User registry (EVM address ↔ Canton Party mapping)
- Balance cache for fast MetaMask queries
- Transfer state tracking for idempotency
- Chain offsets for crash recovery

### Canton Ledger

The **source of truth** for all token balances.

**DAML Contracts:**
- `FingerprintMapping` - Links EVM addresses to Canton parties
- `CIP56Holding` - Actual token balances (one contract per user per token)
- `TokenMeta` - Token configuration (DEMO native, PROMPT bridged)

---

## Token Types

| Token | Type | Virtual Address | Description |
|-------|------|-----------------|-------------|
| **DEMO** | Native Canton | `0xDE30000000000000000000000000000000000001` | Created directly on Canton |
| **PROMPT** | Bridged ERC-20 | `0x5FbDB2315678afecb367f032d93F642f64180aa3` | Bridged from Ethereum |

---

## Data Flows

### Flow 1: MetaMask Transfer (DEMO or PROMPT)

User sends tokens to another user via MetaMask.

```
┌──────────┐     ┌───────────┐     ┌──────────┐     ┌────────┐
│ MetaMask │     │API Server │     │PostgreSQL│     │ Canton │
└────┬─────┘     └─────┬─────┘     └────┬─────┘     └───┬────┘
     │                 │                │               │
     │ eth_sendRawTx   │                │               │
     │ (ERC-20 transfer)               │               │
     │────────────────>│                │               │
     │                 │                │               │
     │                 │ Decode tx, verify whitelist    │
     │                 │ Get sender/recipient parties   │
     │                 │────────────────>│               │
     │                 │                │               │
     │                 │ TransferAsUserByFingerprint    │
     │                 │───────────────────────────────>│
     │                 │                │               │
     │                 │                │  CIP56 Transfer
     │                 │                │  (exercises choice)
     │                 │                │               │
     │                 │<──────────────────────────────│
     │                 │                │               │
     │                 │ Update balance cache           │
     │                 │────────────────>│               │
     │                 │                │               │
     │  tx receipt     │                │               │
     │<────────────────│                │               │
     │                 │                │               │
```

### Flow 2: Bridge Deposit (Ethereum → Canton)

User deposits PROMPT tokens from Ethereum to Canton.

```
┌──────────┐     ┌──────────┐     ┌─────────┐     ┌──────────┐     ┌────────┐
│   User   │     │ Ethereum │     │ Relayer │     │PostgreSQL│     │ Canton │
└────┬─────┘     └────┬─────┘     └────┬────┘     └────┬─────┘     └───┬────┘
     │                │                │               │               │
     │ depositToCanton()               │               │               │
     │───────────────>│                │               │               │
     │                │                │               │               │
     │                │ Emit Deposit event             │               │
     │                │───────────────>│               │               │
     │                │                │               │               │
     │                │                │ Check if processed            │
     │                │                │───────────────>│               │
     │                │                │               │               │
     │                │                │ Create pending transfer       │
     │                │                │───────────────>│               │
     │                │                │               │               │
     │                │                │ BridgeMint (CIP56)            │
     │                │                │──────────────────────────────>│
     │                │                │               │               │
     │                │                │               │  Create Holding
     │                │                │               │               │
     │                │                │<─────────────────────────────│
     │                │                │               │               │
     │                │                │ Update status = completed     │
     │                │                │───────────────>│               │
     │                │                │               │               │
```

### Flow 3: Balance Reconciliation (operator-run)

An operator rebuilds the API server's database cache from the Canton ledger. This is a maintenance tool,
not a background loop; see Reconciliation under Relayer Design Principles.

```
┌───────────┐     ┌──────────┐     ┌────────┐
│API Server │     │PostgreSQL│     │ Canton │
└─────┬─────┘     └────┬─────┘     └───┬────┘
      │                │               │
      │ GetAllCIP56Holdings            │
      │───────────────────────────────>│
      │                │               │
      │<──────────────────────────────│
      │  [list of all holdings]        │
      │                │               │
      │ Group by party, sum by token   │
      │                │               │
      │ For each registered user:      │
      │ UpdateBalanceByCantonPartyID   │
      │───────────────>│               │
      │                │               │
      │ Repeat...      │               │
      │───────────────>│               │
      │                │               │
```

---

## Relayer Processor Pattern

The Relayer uses a generic **Processor** pattern for bidirectional event handling:

```mermaid
sequenceDiagram
    participant Engine
    participant Processor
    participant Source
    participant Store as Database
    participant Destination

    Engine->>Processor: Start(offset)
    Processor->>Source: StreamEvents(offset)
    
    loop For each event
        Source-->>Processor: Event
        Processor->>Store: Check if processed
        alt Not yet processed
            Processor->>Store: CreateTransfer(Pending)
            Processor->>Destination: SubmitTransfer(event)
            Destination-->>Processor: txHash
            Processor->>Store: UpdateStatus(Completed)
        end
    end
```

**Two processor instances:**
1. **Canton → Ethereum**: `CantonSource` + `EthereumDestination`
2. **Ethereum → Canton**: `EthereumSource` + `CantonDestination`

---

## External Party Model

All users are allocated as **external parties** on Canton. This is a key architectural decision that removes the ~200 internal party limit and enables interoperability with wallets like Canton Loop.

| Party Type | Key Management | Submission Method | Limit |
|------------|---------------|-------------------|-------|
| **Internal** | Participant node holds keys | `CommandService.SubmitAndWait` | ~200 per participant |
| **External** (used) | API server holds keys custodially | Interactive Submission API | No practical limit |

### Interactive Submission API

All token transfers use the Interactive Submission flow:

```
1. PrepareSubmission  →  Canton returns transaction hash to sign
2. Sign hash          →  API server signs with user's custodial secp256k1 key
3. ExecuteSubmission  →  Canton executes the signed transaction
```

The API server stores each user's Canton signing key encrypted at rest (AES-256-GCM with `CANTON_MASTER_KEY`). When a user sends a transfer via `/eth`, the server decrypts their key, signs the prepared transaction, and submits it.

---

## Canton Integration

### gRPC Ledger API

The middleware connects to Canton via the **Daml Ledger gRPC API v2**:

```
Canton Participant Node
    │
    ├── Ledger API (gRPC, port 5011)
    │   ├── InteractiveSubmissionService - Prepare/execute (external parties)
    │   ├── CommandService - Submit transactions (relayer/operator)
    │   ├── UpdateService - Stream events
    │   └── StateService - Query active contracts
    │
    └── HTTP API (port 5013) - Health/version checks
```

**Why gRPC (not JSON API):**
- First-class, production-grade API
- Streaming with offsets for reliability
- Built-in deduplication via command IDs
- Generated Go stubs from protobuf definitions

> **Note:** gRPC connections through ALBs/proxies require ALPN to be disabled (grpc-go >= 1.67 enforces ALPN by default). The SDK uses `expcreds.NewTLSWithALPNDisabled` to handle this.

### Authentication

Canton uses **JWT-based authorization** via OAuth2:

```go
// JWT claims structure
{
  "actAs": ["BridgeOperatorParty"],  // Can submit commands
  "readAs": ["BridgeOperatorParty"], // Can read events
  "exp": 1234567890
}
```

The middleware authenticates via OAuth2 to obtain JWT tokens. User transfers are submitted by the operator party using Interactive Submission with the user's external party key.

---

## Relayer Design Principles

### Core Philosophy

1. **Bidirectional & Independent** - Two separate one-way flows that don't depend on each other
2. **At-Least-Once Delivery** - Every event is guaranteed to be processed at least once
3. **Idempotency** - Database-backed deduplication prevents duplicate processing
4. **Crash Recovery** - Persisted offsets allow resuming from exact position after restart

### State Management

**Offsets (Checkpoints):**
- Canton: `LedgerOffset` (absolute string)
- Ethereum: `BlockNumber`
- Stored in `chain_state` table
- Updated only after successful processing

**Idempotency:**
- Every event has a unique ID
- Canton→Eth: Canton Event ID
- Eth→Canton: Hash of `(TxHash, LogIndex)`
- Checked against `transfers` table before processing

### Reconciliation

Two distinct mechanisms share this name.

**Relayer reconciliation (automatic, every 60 seconds).** The relayer engine runs a reconciliation loop
(`pkg/relayer/engine/engine.go`) that updates chain head positions, queries pending transfers in both
directions from the bridge store, and retries ones that have become stuck. Duration and per-run outcome are
exported as Prometheus metrics (`reconciliation_duration_seconds`, `reconciliation_runs_total`).

**Balance reconciliation (operator-run).** Rebuilding the API server's cached balances from the Canton
ledger is available as an operator tool (`pkg/reconciler`, invoked by `scripts/utils/reconcile.go`) rather
than as a background loop. It queries all `CIP56Holding` contracts from Canton, groups them by party and
token, and updates the cached balances in PostgreSQL. Routine balance correctness does not depend on it:
the indexer maintains balances by streaming ledger events, and Canton remains the source of truth for any
read that matters.

---

## Key Design Decisions

### 1. External Parties with Custodial Key Management (secp256k1)

All users are **external parties** on Canton, with the API Server custodially holding their **secp256k1** signing keys:
- Same elliptic curve as Ethereum (enables future MetaMask Snap trustless signing)
- Keys encrypted at rest with `CANTON_MASTER_KEY` (AES-256-GCM)
- Stored in PostgreSQL `users` table
- Transactions use the **Interactive Submission API** (PrepareSubmission/ExecuteSubmission)

This enables MetaMask users to interact without Canton tooling and removes the ~200 internal party limit. Trade-off: Users trust the API Server with their Canton keys (mitigated by future MetaMask Snap integration).

### 2. PostgreSQL as Cache, Canton as Source of Truth

- Fast balance queries from PostgreSQL
- Relayer reconciliation retries stuck transfers every 60 seconds
- Canton ledger is always authoritative

### 3. Fingerprint-Based Identity

Users are identified by `keccak256(evmAddress)`:
- Links EVM identity to Canton party
- Enables cross-chain user lookup
- Stored in `FingerprintMapping` DAML contract

### 4. Virtual Token Addresses

Native Canton tokens use synthetic addresses:
- DEMO: `0xDE30000000000000000000000000000000000001`
- Allows MetaMask to "import" Canton-native tokens

---

## Ports Summary

| Service | Port | Protocol |
|---------|------|----------|
| API Server | 8081 | HTTP (JSON-RPC) |
| Anvil (Ethereum) | 8545 | HTTP (JSON-RPC) |
| Canton gRPC | 5011 | gRPC |
| Canton HTTP | 5013 | HTTP |
| PostgreSQL | 5432 | PostgreSQL |
| Relayer Metrics | 9090 | HTTP |
| Indexer | 8082 | HTTP (query API) |
| Indexer Metrics | 9092 | HTTP |
