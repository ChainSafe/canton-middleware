# Canton ERC-20 Indexer — Design Document

> **Status:** Design / Pre-Implementation
> **CIP Reference:** CIP-0086 (ERC-20 Middleware & Distributed Indexer)
> **Scope:** Phase 1 — CIP-56 token indexer (DEMO + PROMPT); no Canton Coin yet

---

## Table of Contents

1. [Background & Motivation](#1-background--motivation)
2. [Current State & Gaps](#2-current-state--gaps)
3. [Key Design Questions Answered](#3-key-design-questions-answered)
4. [Architecture Overview](#4-architecture-overview)
5. [DAML Contract Change — Unified `TokenTransferEvent`](#5-daml-contract-change--unified-tokentransferevent)
6. [Component Deep-Dive](#6-component-deep-dive)
   - 6.1 [cantonsdk Streaming Client](#61-cantonsdk-streaming-client-new-package)
   - 6.2 [Fetcher](#62-fetcher)
   - 6.3 [Parser](#63-parser)
   - 6.4 [Processor](#64-processor)
   - 6.5 [Store — Models & PostgreSQL](#65-store--models--postgresql)
   - 6.6 [Database Migrations (Go code)](#66-database-migrations-go-code)
   - 6.7 [Service Layer](#67-service-layer)
   - 6.8 [API / HTTP Layer](#68-api--http-layer)
7. [File & Directory Layout](#7-file--directory-layout)
8. [Pseudo-code & Data Flows](#8-pseudo-code--data-flows)
9. [Configuration](#9-configuration)
10. [Integration with API Server](#10-integration-with-api-server)
11. [Open Questions & Future Work](#11-open-questions--future-work)

---

## 1. Background & Motivation

CIP-0086 mandates a **distributed indexer** that aggregates Canton token state and exposes
ERC-20-compatible HTTP endpoints. The current `reconciler` in `pkg/reconciler/` is a
periodic polling loop (snapshot-based) that:

- Queries all `CIP56Holding` active contracts every N seconds
- Aggregates per-party balances — only current state, no history
- Tracks only bridge (mint/burn) events via `bridge_events` table
- Misses transfers made directly on Canton (visible only via holdings snapshot)

**What the reconciler lacks:**
- Continuous streaming — balance lag between polls, events missed
- Transfer event history — can't answer "show me all transfers for party X"
- Resumability — replays from scratch on restart
- Independent HTTP query API
- Scalability — hard-coded to DEMO/PROMPT package IDs

The indexer is a **separate, independent binary** (`cmd/indexer`) with no dependency on
the api-server's user table or user registration flow. It is Canton-native: it speaks
`canton_party_id` as its primary identity. EVM address mapping is the api-server's
responsibility, not the indexer's.

---

## 2. Current State & Gaps

```
Current Architecture (reconciler, inside api-server process):

  StartPeriodicReconciliation(interval)
       │
       ▼  every N seconds
  ReconcileAll()
       ├── GetAllHoldings()         → StateService.GetActiveContracts() (snapshot)
       ├── SetBalanceByCantonPartyID()
       ├── SetTotalSupply()
       ├── GetMintEvents()          → active contract query (no streaming)
       └── GetBurnEvents()          → "Transfers are internal Canton operations, not tracked"

  PostgreSQL: user_token_balances, bridge_events, token_metrics

Gaps:
  ✗  No transfer history — only current balance
  ✗  Balance lag between reconcile intervals
  ✗  Not resumable (no ledger offset checkpoint)
  ✗  No independent HTTP query API
  ✗  Restarts replay from offset 0
  ✗  Coupled to api-server process and userstore
```

---

## 3. Key Design Questions Answered

### Q1: Use / extend cantonsdk for the fetcher?

**Yes — add `pkg/cantonsdk/streaming/` as a new generic streaming package.**

The existing `pkg/cantonsdk/bridge/client.go` already uses `UpdateService.GetUpdates`
(gRPC server-streaming) inside `StreamWithdrawalEvents`, with exponential backoff
reconnect, auth token invalidation on 401, and offset resumption. The new package
formalises this pattern as a reusable, generic streaming client. The indexer fetcher
delegates entirely to it.

**WebSocket note:** Canton's gRPC API does not support WebSocket. The Canton→indexer
connection is always gRPC HTTP/2 server-streaming. WebSocket is a Phase 2 option for the
indexer→client direction (real-time event subscriptions).

### Q2: Add `TransferEvent`? Use a unified event for all cases?

**Yes — add a single `TokenTransferEvent` DAML template covering MINT, BURN, TRANSFER.**

This mirrors ERC-20's `Transfer(address indexed from, address indexed to, uint256 value)`:

- **MINT**: `fromParty = None`
- **BURN**: `toParty = None`
- **TRANSFER**: both set

The indexer subscribes to **only this one template** — no inference heuristics, no holding
lifecycle correlation. Clean, deterministic, ERC-20 idiomatic.

**Does it violate Canton privacy?** No. The observer pattern is:
```
signatory issuer          ← indexer runs as issuer, sees all events
observer fromParty, toParty, auditObservers
```
Identical to the existing `MintEvent` / `BurnEvent` pattern. Parties only see events they
are party to. The indexer (as issuer) has full visibility — same as the current
reconciler. Existing `MintEvent` / `BurnEvent` are kept for backward compatibility.

**No return type changes needed.** In DAML, `create` inside a choice is a side-effect;
the new event is emitted without touching existing choice signatures.

### Q3: Why does the indexer NOT depend on userstore?

**The indexer is a Canton-native service. Its primary identity is `canton_party_id`.**

The EVM address → party_id mapping is a concern of the api-server, not the indexer.
Coupling the indexer to `userstore` would:
- Make it non-deployable independently (always needs api-server's DB schema)
- Break the separation of concerns (indexer = ledger aggregator, not user registry)
- Prevent it from serving non-EVM Canton parties in the future

**How callers query the indexer without userstore:**

| Caller | Flow |
|--------|------|
| **api-server** | Resolves EVM address → `canton_party_id` via its own userstore, then calls indexer with a JWT whose claims contain `canton_party_id`. The indexer never sees the EVM address. |
| **Direct client** (wallet, dApp) | Client sends a JWT issued by the api-server (or auth server) that contains `canton_party_id`. Indexer validates JWT, extracts `canton_party_id`, scopes query. |
| **Public queries** | `totalSupply`, token metadata — no auth, no party resolution needed. |

**The indexer's auth contract:** Validate JWT signature against the shared JWKS endpoint.
Extract `canton_party_id` from claims. Scope all queries to that party. Done.

### Q4: Separate admin vs. user API?

**No — two tiers: public and authenticated (by JWT party_id).**

In ERC-20, `totalSupply()`, `name()`, `symbol()`, `decimals()` are public. The indexer
follows the same model. An admin tier can be added later if needed.

---

## 4. Architecture Overview

```
┌───────────────────────────────────────────────────────────────────────────┐
│                        cmd/indexer (binary)                               │
│              (entry → pkg/app/indexer/server.go)                          │
│                                                                           │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │  pkg/cantonsdk/streaming  (NEW — reusable across the project)        │ │
│  │  StreamingClient.Subscribe(templateIDs, fromOffset)                  │ │
│  │    → UpdateService.GetUpdates (gRPC server-streaming)                │ │
│  │    → exponential backoff reconnect (mirrors StreamWithdrawalEvents)  │ │
│  └──────────────────────────────┬───────────────────────────────────────┘ │
│                                  │ chan LedgerTransaction                  │
│  ┌───────────────────────────────▼─────────────────────────────────────┐  │
│  │  pkg/indexer/fetcher                                                │  │
│  │  loads checkpoint from DB → delegates to cantonsdk/streaming        │  │
│  └───────────────────────────────┬─────────────────────────────────────┘  │
│                                   │ chan RawTransaction                    │
│  ┌────────────────────────────────▼────────────────────────────────────┐  │
│  │  pkg/indexer/parser                                                 │  │
│  │  decode TokenTransferEvent → classify MINT | BURN | TRANSFER        │  │
│  │  apply package whitelist filter                                     │  │
│  └────────────────────────────────┬────────────────────────────────────┘  │
│                                    │ chan []ParsedEvent (per tx)           │
│  ┌─────────────────────────────────▼───────────────────────────────────┐  │
│  │  pkg/indexer/processor                                              │  │
│  │  BEGIN TX                                                           │  │
│  │    INSERT transfer_events (idempotent via event_id UNIQUE)          │  │
│  │    UPSERT token_balances  (±delta by canton_party_id)               │  │
│  │    UPSERT token_stats     (total supply)                            │  │
│  │    UPDATE ledger_checkpoints                                        │  │
│  │  COMMIT  ← checkpoint committed atomically with events              │  │
│  └─────────────────────────────────┬───────────────────────────────────┘  │
│                                     │                                      │
│                               PostgreSQL                                   │
│                                     │                                      │
│  ┌──────────────────────────────────▼──────────────────────────────────┐  │
│  │  pkg/indexer/service  (Canton-native query service)                 │  │
│  │  All queries keyed by canton_party_id — no EVM address, no user     │  │
│  │  table. Caller is responsible for resolving EVM → party_id.         │  │
│  └──────────────────────────────────┬──────────────────────────────────┘  │
│                                      │                                     │
│  ┌───────────────────────────────────▼─────────────────────────────────┐  │
│  │  pkg/indexer/api  — HTTP :8082  (chi router)                        │  │
│  │                                                                     │  │
│  │  Auth: JWT validation only (shared JWKS with api-server)            │  │
│  │  Claims must contain canton_party_id.                               │  │
│  │  No userstore. No EVM sig verification.                             │  │
│  │                                                                     │  │
│  │  [public]  GET /v1/tokens                                           │  │
│  │  [public]  GET /v1/tokens/{symbol}                                  │  │
│  │  [public]  GET /v1/tokens/{symbol}/totalSupply                      │  │
│  │  [JWT]     GET /v1/balance/{partyID}[/{symbol}]                     │  │
│  │  [JWT]     GET /v1/transfers/{partyID}[/{symbol}]                   │  │
│  │  [JWT]     GET /v1/events/{partyID}                                 │  │
│  │            GET /health   GET /metrics                               │  │
│  │  (Phase 2: add /graph for GraphQL)                                  │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
         ▲                                          ▲
   Canton Ledger API v2                    Callers (api-server or direct clients)
   gRPC server-streaming                   JWT must contain canton_party_id claim

How api-server uses the indexer:
  EVM client  →  api-server (EVM sig auth + userstore lookup)
              →  api-server mints JWT with canton_party_id claim
              →  api-server calls indexer /v1/balance/{partyID} with that JWT
              ←  indexer returns balance for that party
              ←  api-server returns result to EVM client
```

---

## 5. DAML Contract Change — Unified `TokenTransferEvent`

### New template in `Events.daml`

```daml
-- contracts/canton-erc20/daml/cip56-token/src/CIP56/Events.daml

-- Unified transfer event covering mint, burn, and transfer.
-- Mirrors ERC-20 Transfer(from, to, value):
--   MINT:     fromParty = None,        toParty = Some recipient
--   BURN:     fromParty = Some owner,  toParty = None
--   TRANSFER: fromParty = Some sender, toParty = Some receiver
template TokenTransferEvent
  with
    issuer          : Party
    fromParty       : Optional Party    -- None for mints
    toParty         : Optional Party    -- None for burns
    amount          : Decimal
    tokenSymbol     : Text
    eventType       : Text              -- "MINT" | "BURN" | "TRANSFER"
    timestamp       : Time
    evmTxHash       : Optional Text     -- bridge deposit tx hash (mints only)
    evmDestination  : Optional Text     -- bridge withdrawal address (burns only)
    userFingerprint : Optional Text     -- EVM fingerprint, stored for bridge audit only
    auditObservers  : [Party]
  where
    signatory issuer
    observer
      optional [] (\p -> [p]) fromParty,
      optional [] (\p -> [p]) toParty,
      auditObservers
```

### Emit from `TokenConfig.IssuerMint` — no return type change

```daml
-- Config.daml — inside IssuerMint do-block, AFTER creating MintEvent:
    _ <- create TokenTransferEvent with
      issuer
      fromParty       = None
      toParty         = Some recipient
      amount
      tokenSymbol     = getSymbol meta
      eventType       = "MINT"
      timestamp       = eventTime
      evmTxHash
      evmDestination  = None
      userFingerprint = Some userFingerprint
      auditObservers
    pure (holdingCid, eventCid)    -- return type UNCHANGED
```

### Emit from `TokenConfig.IssuerBurn` — no return type change

```daml
-- Config.daml — inside IssuerBurn do-block, AFTER creating BurnEvent:
    _ <- create TokenTransferEvent with
      issuer
      fromParty       = Some holding.owner
      toParty         = None
      amount
      tokenSymbol     = getSymbol meta
      eventType       = "BURN"
      timestamp       = eventTime
      evmTxHash       = None
      evmDestination
      userFingerprint = Some userFingerprint
      auditObservers
    pure (remainderCid, eventCid)  -- return type UNCHANGED
```

### Emit from `CIP56TransferFactory.transferFactory_transferImpl`

```daml
-- TransferFactory.daml — AFTER creating receiverCid:
    _ <- create TokenTransferEvent with
      issuer         = admin
      fromParty      = Some sender
      toParty        = Some receiver
      amount
      tokenSymbol    = instrumentId.id
      eventType      = "TRANSFER"
      timestamp      = now
      evmTxHash      = None
      evmDestination = None
      userFingerprint = None   -- pure Canton transfer, no EVM context
      auditObservers  = []
    pure TransferInstructionResult with ...   -- return type UNCHANGED
```

> Existing `MintEvent` and `BurnEvent` are kept intact for the reconciler and bridge
> relayer during the migration window.

---

## 6. Component Deep-Dive

### 6.1 cantonsdk Streaming Client (new package)

Mirrors `StreamWithdrawalEvents` in `pkg/cantonsdk/bridge/client.go` exactly — same
backoff, same auth invalidation on 401, same reconnect-from-offset logic — but generic
enough for any template subscription.

```go
// pkg/cantonsdk/streaming/client.go
package streaming

// LedgerTransaction is a decoded, typed transaction from the GetUpdates stream.
type LedgerTransaction struct {
    UpdateID      string
    Offset        int64
    EffectiveTime time.Time
    Events        []LedgerEvent
}

// LedgerEvent is a single created or archived contract within a transaction.
type LedgerEvent struct {
    ContractID   string
    PackageID    string
    ModuleName   string
    TemplateName string
    IsCreated    bool
    Created      *lapiv2.CreatedEvent  // set when IsCreated=true
    Archived     *lapiv2.ArchivedEvent // set when IsCreated=false
}

// SubscribeRequest configures which templates to stream and from where.
type SubscribeRequest struct {
    FromOffset  int64
    TemplateIDs []*lapiv2.Identifier
}

// Client wraps UpdateService.GetUpdates with reconnection and auth handling.
type Client struct {
    ledger ledger.Ledger
    party  string
}

func New(l ledger.Ledger, party string) *Client {
    return &Client{ledger: l, party: party}
}

// Subscribe opens a live stream against the Canton ledger.
// Reconnects automatically with exponential backoff (5s → 60s, mirrors bridge client).
// lastOffset is updated after each received transaction. The caller commits it to DB
// so reconnects resume from the last safe point.
func (c *Client) Subscribe(
    ctx context.Context,
    req SubscribeRequest,
    lastOffset *int64,
) <-chan *LedgerTransaction {
    out := make(chan *LedgerTransaction, 100)
    go func() {
        defer close(out)
        backoff := newExponentialBackoff(5*time.Second, 60*time.Second)
        for {
            err := c.runStream(ctx, &req, lastOffset, out)
            if ctx.Err() != nil {
                return
            }
            // Reload offset — processor commits it to DB on each batch
            atomic.StoreInt64(&req.FromOffset, atomic.LoadInt64(lastOffset))
            log.Warn("canton stream disconnected, reconnecting",
                "err", err, "resume_offset", req.FromOffset)
            backoff.Wait(ctx)
        }
    }()
    return out
}

func (c *Client) runStream(
    ctx context.Context,
    req *SubscribeRequest,
    lastOffset *int64,
    out chan<- *LedgerTransaction,
) error {
    authCtx, err := c.ledger.AuthContext(ctx)
    if err != nil {
        return fmt.Errorf("auth: %w", err)
    }
    stream, err := c.ledger.Update().GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
        BeginExclusive: req.FromOffset,
        UpdateFormat: &lapiv2.UpdateFormat{
            IncludeTransactions: &lapiv2.TransactionFormat{
                EventFormat: &lapiv2.EventFormat{
                    FiltersByParty: map[string]*lapiv2.Filters{
                        c.party: buildTemplateFilters(req.TemplateIDs),
                    },
                    Verbose: true,
                },
                TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
            },
        },
    })
    if err != nil {
        if isAuthError(err) {
            c.ledger.InvalidateToken()
        }
        return err
    }
    for {
        resp, err := stream.Recv()
        if err != nil {
            if isAuthError(err) {
                c.ledger.InvalidateToken()
            }
            return err
        }
        tx := resp.GetTransaction()
        if tx == nil {
            continue // checkpoint or topology event — skip
        }
        lt := decodeLedgerTransaction(tx)
        atomic.StoreInt64(lastOffset, lt.Offset)
        select {
        case out <- lt:
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 6.2 Fetcher

Thin wrapper — loads checkpoint offset from DB, builds the template filter, delegates to
`cantonsdk/streaming`. No business logic here.

```go
// pkg/indexer/fetcher/fetcher.go
package fetcher

type Fetcher struct {
    streaming  *streaming.Client
    store      store.Store
    templateID *lapiv2.Identifier // TokenTransferEvent fully-resolved ID
    out        chan<- *streaming.LedgerTransaction
}

func New(
    s *streaming.Client,
    st store.Store,
    tplID *lapiv2.Identifier,
    out chan<- *streaming.LedgerTransaction,
) *Fetcher {
    return &Fetcher{streaming: s, store: st, templateID: tplID, out: out}
}

func (f *Fetcher) Start(ctx context.Context) error {
    cp, err := f.store.GetCheckpoint(ctx)
    if err != nil {
        return fmt.Errorf("load checkpoint: %w", err)
    }
    var lastOffset int64 = cp.LastProcessedOffset

    events := f.streaming.Subscribe(ctx, streaming.SubscribeRequest{
        FromOffset:  lastOffset,
        TemplateIDs: []*lapiv2.Identifier{f.templateID},
    }, &lastOffset)

    for {
        select {
        case tx, ok := <-events:
            if !ok {
                return nil
            }
            select {
            case f.out <- tx:
            case <-ctx.Done():
                return ctx.Err()
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 6.3 Parser

Since the indexer subscribes only to `TokenTransferEvent`, parsing is a straightforward
DAML record decode using the existing `cantonsdk/values` helpers. No inference needed.

```go
// pkg/indexer/parser/types.go
package parser

type EventType string

const (
    EventMint     EventType = "MINT"
    EventBurn     EventType = "BURN"
    EventTransfer EventType = "TRANSFER"
)

// ParsedEvent is a fully decoded TokenTransferEvent, ready for the processor.
// Primary identity is always canton_party_id — no EVM address at this layer.
type ParsedEvent struct {
    EventType       EventType
    TokenSymbol     string
    Amount          string    // decimal string, e.g. "1.5"
    FromPartyID     *string   // nil for mints
    ToPartyID       *string   // nil for burns
    UserFingerprint *string   // from DAML event — stored for bridge audit, not for auth
    EVMTxHash       *string   // bridge deposit
    EVMDestination  *string   // bridge withdrawal
    ContractID      string    // unique idempotency key (TokenTransferEvent contract ID)
    TxID            string
    LedgerOffset    int64
    EffectiveTime   time.Time
}
```

```go
// pkg/indexer/parser/cip56.go
package parser

// Uses cantonsdk/values helpers (values.RecordToMap, values.Text, etc.)
// — same pattern as bridge/decode.go
func decodeTokenTransferEvent(ce *lapiv2.CreatedEvent, tx *streaming.LedgerTransaction) *ParsedEvent {
    fields := values.RecordToMap(ce.CreateArguments)

    fromParty := optionalParty(fields["fromParty"])
    toParty   := optionalParty(fields["toParty"])

    var et EventType
    switch {
    case fromParty == nil && toParty != nil:
        et = EventMint
    case fromParty != nil && toParty == nil:
        et = EventBurn
    default:
        et = EventTransfer
    }

    amount, _ := values.Numeric(fields["amount"])
    return &ParsedEvent{
        EventType:       et,
        TokenSymbol:     values.Text(fields["tokenSymbol"]),
        Amount:          amount.String(),
        FromPartyID:     fromParty,
        ToPartyID:       toParty,
        UserFingerprint: optionalText(fields["userFingerprint"]),
        EVMTxHash:       optionalText(fields["evmTxHash"]),
        EVMDestination:  optionalText(fields["evmDestination"]),
        ContractID:      ce.ContractId,
        TxID:            tx.UpdateID,
        LedgerOffset:    tx.Offset,
        EffectiveTime:   tx.EffectiveTime,
    }
}
```

### 6.4 Processor

Atomic batch writer. Checkpoint update is inside the same DB transaction as the event
writes — guarantees exactly-once processing on restart.

```go
// pkg/indexer/processor/processor.go
package processor

func (proc *Processor) processBatch(ctx context.Context, events []*parser.ParsedEvent) error {
    if len(events) == 0 {
        return nil
    }
    lastOffset := events[len(events)-1].LedgerOffset

    return proc.store.RunInTx(ctx, func(ctx context.Context, tx store.Tx) error {
        for _, ev := range events {
            if err := proc.processEvent(ctx, tx, ev); err != nil {
                return fmt.Errorf("event %s: %w", ev.ContractID, err)
            }
        }
        return tx.UpdateCheckpoint(ctx, lastOffset)
    })
}

func (proc *Processor) processEvent(ctx context.Context, tx store.Tx, ev *parser.ParsedEvent) error {
    // Idempotent insert — ON CONFLICT (event_id) DO NOTHING
    inserted, err := tx.InsertTransferEvent(ctx, toTransferEventDao(ev))
    if err != nil {
        return err
    }
    if !inserted {
        return nil // already committed in a previous run
    }

    switch ev.EventType {
    case parser.EventMint:
        if err := tx.IncrementBalance(ctx, *ev.ToPartyID, ev.TokenSymbol, ev.Amount); err != nil {
            return err
        }
        return tx.IncrementTotalSupply(ctx, ev.TokenSymbol, ev.Amount)

    case parser.EventBurn:
        if err := tx.DecrementBalance(ctx, *ev.FromPartyID, ev.TokenSymbol, ev.Amount); err != nil {
            return err
        }
        return tx.DecrementTotalSupply(ctx, ev.TokenSymbol, ev.Amount)

    case parser.EventTransfer:
        if err := tx.DecrementBalance(ctx, *ev.FromPartyID, ev.TokenSymbol, ev.Amount); err != nil {
            return err
        }
        return tx.IncrementBalance(ctx, *ev.ToPartyID, ev.TokenSymbol, ev.Amount)
    }
    return nil
}
```

---

### 6.5 Store — Models & PostgreSQL

#### `pkg/indexer/store/model.go`

DAOs follow the exact Bun ORM pattern from `pkg/reconciler/store/model.go`.
**No EVM address in `TokenBalanceDao`** — the indexer is Canton-native. `evm_address`
is the api-server's concern.

```go
// pkg/indexer/store/model.go
package store

import (
    "time"
    "github.com/uptrace/bun"
)

// LedgerCheckpointDao — single-row table. Offset committed atomically with each
// processed batch, guaranteeing safe restart from this point.
type LedgerCheckpointDao struct {
    bun.BaseModel       `bun:"table:ledger_checkpoints,alias:lc"`
    ID                  int       `bun:"id,pk,default:1"`
    LastProcessedOffset int64     `bun:"last_processed_offset,notnull,default:0"`
    LastTxID            *string   `bun:"last_tx_id,type:varchar(255)"`
    UpdatedAt           time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
}

// IndexedTokenDao — registry of token contracts being indexed.
type IndexedTokenDao struct {
    bun.BaseModel `bun:"table:indexed_tokens,alias:it"`
    PackageID     string    `bun:"package_id,pk,type:varchar(255)"`
    TokenSymbol   string    `bun:"token_symbol,unique,notnull,type:varchar(50)"`
    ModuleName    string    `bun:"module_name,notnull,type:varchar(255)"`
    TemplateName  string    `bun:"template_name,notnull,type:varchar(255)"`
    Name          *string   `bun:"name,type:varchar(255)"`
    Decimals      int16     `bun:"decimals,notnull,default:18"`
    IssuerPartyID *string   `bun:"issuer_party_id,type:varchar(255)"`
    AddedAt       time.Time `bun:"added_at,nullzero,default:current_timestamp"`
}

// TransferEventDao — append-only event log.
// event_id = Canton contract ID of the TokenTransferEvent (globally unique).
// fingerprint is stored only because it comes from the DAML event itself (bridge audit).
// It is NOT used for auth or party resolution inside the indexer.
type TransferEventDao struct {
    bun.BaseModel  `bun:"table:transfer_events,alias:te"`
    ID             int64      `bun:"id,pk,autoincrement"`
    EventID        string     `bun:"event_id,unique,notnull,type:varchar(512)"`
    EventType      string     `bun:"event_type,notnull,type:varchar(20)"`      // MINT|BURN|TRANSFER
    TokenSymbol    string     `bun:"token_symbol,notnull,type:varchar(50)"`
    Amount         string     `bun:"amount,notnull,type:numeric(38,18)"`
    FromPartyID    *string    `bun:"from_party_id,type:varchar(255)"`          // nil for mints
    ToPartyID      *string    `bun:"to_party_id,type:varchar(255)"`            // nil for burns
    Fingerprint    *string    `bun:"fingerprint,type:varchar(128)"`            // from DAML event
    EVMTxHash      *string    `bun:"evm_tx_hash,type:varchar(255)"`
    EVMDestination *string    `bun:"evm_destination,type:varchar(42)"`
    TransactionID  *string    `bun:"transaction_id,type:varchar(255)"`
    LedgerOffset   int64      `bun:"ledger_offset,notnull"`
    EffectiveTime  time.Time  `bun:"effective_time,notnull"`
    IndexedAt      time.Time  `bun:"indexed_at,nullzero,default:current_timestamp"`
}

// TokenBalanceDao — incremental balance cache per party per token.
// Primary key: (party_id, token_symbol).
// NO evm_address — the indexer is Canton-native. EVM mapping is the api-server's job.
type TokenBalanceDao struct {
    bun.BaseModel `bun:"table:token_balances,alias:tb"`
    PartyID       string    `bun:"party_id,pk,type:varchar(255)"`
    TokenSymbol   string    `bun:"token_symbol,pk,type:varchar(50)"`
    Balance       string    `bun:"balance,notnull,default:0,type:numeric(38,18)"`
    UpdatedAt     time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
}

// TokenStatDao — aggregate stats per token.
type TokenStatDao struct {
    bun.BaseModel `bun:"table:token_stats,alias:ts"`
    TokenSymbol   string    `bun:"token_symbol,pk,type:varchar(50)"`
    TotalSupply   string    `bun:"total_supply,notnull,default:0,type:numeric(38,18)"`
    HolderCount   int64     `bun:"holder_count,notnull,default:0"`
    UpdatedAt     time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
}
```

#### `pkg/indexer/store/store.go`

```go
// pkg/indexer/store/store.go
package store

import "context"

//go:generate mockery --name Store --output ./mocks
type Store interface {
    GetCheckpoint(ctx context.Context) (*LedgerCheckpointDao, error)
    // Queries are keyed by canton_party_id — no EVM address resolution here.
    GetTokenBalance(ctx context.Context, partyID, tokenSymbol string) (*TokenBalanceDao, error)
    GetTokenStat(ctx context.Context, tokenSymbol string) (*TokenStatDao, error)
    ListIndexedTokens(ctx context.Context) ([]*IndexedTokenDao, error)
    ListTransferEvents(ctx context.Context, filter TransferEventFilter) ([]*TransferEventDao, int, error)
    UpsertIndexedToken(ctx context.Context, dao *IndexedTokenDao) error
    RunInTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

//go:generate mockery --name Tx --output ./mocks
type Tx interface {
    InsertTransferEvent(ctx context.Context, dao *TransferEventDao) (inserted bool, err error)
    IncrementBalance(ctx context.Context, partyID, tokenSymbol, amount string) error
    DecrementBalance(ctx context.Context, partyID, tokenSymbol, amount string) error
    IncrementTotalSupply(ctx context.Context, tokenSymbol, amount string) error
    DecrementTotalSupply(ctx context.Context, tokenSymbol, amount string) error
    UpdateCheckpoint(ctx context.Context, offset int64) error
}

// TransferEventFilter — all fields keyed by canton_party_id, not EVM address.
type TransferEventFilter struct {
    PartyID     *string // filter events where from_party_id OR to_party_id = this
    TokenSymbol *string
    EventType   *string
    Page        int
    PageSize    int
}
```

#### `pkg/indexer/store/pg.go` (key methods)

```go
// pkg/indexer/store/pg.go
package store

type pgStore struct{ db *bun.DB }

func NewStore(db *bun.DB) Store { return &pgStore{db: db} }

func (s *pgStore) GetTokenBalance(ctx context.Context, partyID, tokenSymbol string) (*TokenBalanceDao, error) {
    dao := new(TokenBalanceDao)
    err := s.db.NewSelect().Model(dao).
        Where("party_id = ? AND token_symbol = ?", partyID, tokenSymbol).
        Scan(ctx)
    if errors.Is(err, sql.ErrNoRows) {
        return &TokenBalanceDao{PartyID: partyID, TokenSymbol: tokenSymbol, Balance: "0"}, nil
    }
    return dao, err
}

func (s *pgStore) ListTransferEvents(ctx context.Context, f TransferEventFilter) ([]*TransferEventDao, int, error) {
    var rows []*TransferEventDao
    q := s.db.NewSelect().Model(&rows).OrderExpr("ledger_offset DESC")

    if f.PartyID != nil {
        q = q.Where("(from_party_id = ? OR to_party_id = ?)", *f.PartyID, *f.PartyID)
    }
    if f.TokenSymbol != nil {
        q = q.Where("token_symbol = ?", *f.TokenSymbol)
    }
    if f.EventType != nil {
        q = q.Where("event_type = ?", *f.EventType)
    }

    total, err := q.Count(ctx)
    if err != nil {
        return nil, 0, fmt.Errorf("count events: %w", err)
    }

    pageSize := f.PageSize
    if pageSize <= 0 { pageSize = 20 }
    page := f.Page
    if page <= 0 { page = 1 }

    err = q.Limit(pageSize).Offset((page - 1) * pageSize).Scan(ctx)
    return rows, total, err
}

func (s *pgStore) RunInTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
    return s.db.RunInTx(ctx, nil, func(ctx context.Context, bunTx bun.Tx) error {
        return fn(ctx, &pgTx{db: bunTx})
    })
}

type pgTx struct{ db bun.Tx }

func (t *pgTx) InsertTransferEvent(ctx context.Context, dao *TransferEventDao) (bool, error) {
    res, err := t.db.NewInsert().Model(dao).
        On("CONFLICT (event_id) DO NOTHING").
        Exec(ctx)
    if err != nil {
        return false, fmt.Errorf("insert transfer event: %w", err)
    }
    rows, _ := res.RowsAffected()
    return rows > 0, nil
}

func (t *pgTx) IncrementBalance(ctx context.Context, partyID, tokenSymbol, amount string) error {
    _, err := t.db.NewInsert().
        TableExpr("token_balances").
        ColumnExpr("party_id, token_symbol, balance, updated_at").
        Value("?, ?, ?, NOW()", partyID, tokenSymbol, amount).
        On("CONFLICT (party_id, token_symbol) DO UPDATE").
        Set("balance = token_balances.balance + EXCLUDED.balance").
        Set("updated_at = NOW()").
        Exec(ctx)
    return err
}

func (t *pgTx) DecrementBalance(ctx context.Context, partyID, tokenSymbol, amount string) error {
    _, err := t.db.NewUpdate().TableExpr("token_balances").
        Set("balance = balance - ?", amount).
        Set("updated_at = NOW()").
        Where("party_id = ? AND token_symbol = ?", partyID, tokenSymbol).
        Exec(ctx)
    return err
}

func (t *pgTx) UpdateCheckpoint(ctx context.Context, offset int64) error {
    _, err := t.db.NewUpdate().Model((*LedgerCheckpointDao)(nil)).
        Set("last_processed_offset = ?", offset).
        Set("updated_at = NOW()").
        Where("id = 1").
        Exec(ctx)
    return err
}
```

---

### 6.6 Database Migrations (Go code)

Package `indexerdb`, same pattern as `pkg/migrations/apidb/`.
Inline DAO structs per migration file keep migrations self-contained.

```go
// pkg/migrations/indexerdb/migrations.go
package indexerdb

import "github.com/uptrace/bun/migrate"

var Migrations = migrate.NewMigrations()
```

```go
// pkg/migrations/indexerdb/1_create_ledger_checkpoints.go
package indexerdb

import (
    "context"
    "log"
    "time"

    mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
    "github.com/uptrace/bun"
)

func init() {
    Migrations.MustRegister(
        func(ctx context.Context, db *bun.DB) error {
            log.Println("creating ledger_checkpoints table...")
            type dao struct {
                bun.BaseModel       `bun:"table:ledger_checkpoints"`
                ID                  int       `bun:"id,pk,default:1"`
                LastProcessedOffset int64     `bun:"last_processed_offset,notnull,default:0"`
                LastTxID            *string   `bun:"last_tx_id,type:varchar(255)"`
                UpdatedAt           time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
            }
            if err := mghelper.CreateSchema(ctx, db, (*dao)(nil)); err != nil {
                return err
            }
            _, err := db.ExecContext(ctx,
                `INSERT INTO ledger_checkpoints (id, last_processed_offset)
                 VALUES (1, 0) ON CONFLICT DO NOTHING;`)
            return err
        },
        func(ctx context.Context, db *bun.DB) error {
            log.Println("dropping ledger_checkpoints table...")
            type dao struct {
                bun.BaseModel `bun:"table:ledger_checkpoints"`
            }
            return mghelper.DropTables(ctx, db, (*dao)(nil))
        },
    )
}
```

```go
// pkg/migrations/indexerdb/2_create_indexed_tokens.go
package indexerdb

import (
    "context"
    "log"
    "time"

    mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
    "github.com/uptrace/bun"
)

func init() {
    Migrations.MustRegister(
        func(ctx context.Context, db *bun.DB) error {
            log.Println("creating indexed_tokens table...")
            type dao struct {
                bun.BaseModel `bun:"table:indexed_tokens"`
                PackageID     string    `bun:"package_id,pk,type:varchar(255)"`
                TokenSymbol   string    `bun:"token_symbol,unique,notnull,type:varchar(50)"`
                ModuleName    string    `bun:"module_name,notnull,type:varchar(255)"`
                TemplateName  string    `bun:"template_name,notnull,type:varchar(255)"`
                Name          *string   `bun:"name,type:varchar(255)"`
                Decimals      int16     `bun:"decimals,notnull,default:18"`
                IssuerPartyID *string   `bun:"issuer_party_id,type:varchar(255)"`
                AddedAt       time.Time `bun:"added_at,nullzero,default:current_timestamp"`
            }
            return mghelper.CreateSchema(ctx, db, (*dao)(nil))
        },
        func(ctx context.Context, db *bun.DB) error {
            log.Println("dropping indexed_tokens table...")
            type dao struct {
                bun.BaseModel `bun:"table:indexed_tokens"`
            }
            return mghelper.DropTables(ctx, db, (*dao)(nil))
        },
    )
}
```

```go
// pkg/migrations/indexerdb/3_create_transfer_events.go
package indexerdb

import (
    "context"
    "log"
    "time"

    mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
    "github.com/uptrace/bun"
)

func init() {
    Migrations.MustRegister(
        func(ctx context.Context, db *bun.DB) error {
            log.Println("creating transfer_events table...")
            type dao struct {
                bun.BaseModel  `bun:"table:transfer_events"`
                ID             int64      `bun:"id,pk,autoincrement"`
                EventID        string     `bun:"event_id,unique,notnull,type:varchar(512)"`
                EventType      string     `bun:"event_type,notnull,type:varchar(20)"`
                TokenSymbol    string     `bun:"token_symbol,notnull,type:varchar(50)"`
                Amount         string     `bun:"amount,notnull,type:numeric(38,18)"`
                FromPartyID    *string    `bun:"from_party_id,type:varchar(255)"`
                ToPartyID      *string    `bun:"to_party_id,type:varchar(255)"`
                Fingerprint    *string    `bun:"fingerprint,type:varchar(128)"`
                EVMTxHash      *string    `bun:"evm_tx_hash,type:varchar(255)"`
                EVMDestination *string    `bun:"evm_destination,type:varchar(42)"`
                TransactionID  *string    `bun:"transaction_id,type:varchar(255)"`
                LedgerOffset   int64      `bun:"ledger_offset,notnull"`
                EffectiveTime  time.Time  `bun:"effective_time,notnull"`
                IndexedAt      time.Time  `bun:"indexed_at,nullzero,default:current_timestamp"`
            }
            if err := mghelper.CreateSchema(ctx, db, (*dao)(nil)); err != nil {
                return err
            }
            // Indexes: all events for a party (sent or received), fingerprint, bridge
            if err := mghelper.CreateModelIndexes(ctx, db, (*dao)(nil), "from_party_id", "token_symbol"); err != nil {
                return err
            }
            if err := mghelper.CreateModelIndexes(ctx, db, (*dao)(nil), "to_party_id", "token_symbol"); err != nil {
                return err
            }
            if err := mghelper.CreateModelIndexes(ctx, db, (*dao)(nil), "fingerprint"); err != nil {
                return err
            }
            if err := mghelper.CreateModelIndexes(ctx, db, (*dao)(nil), "evm_tx_hash"); err != nil {
                return err
            }
            return mghelper.CreateModelIndexes(ctx, db, (*dao)(nil), "ledger_offset")
        },
        func(ctx context.Context, db *bun.DB) error {
            log.Println("dropping transfer_events table...")
            type dao struct {
                bun.BaseModel `bun:"table:transfer_events"`
            }
            return mghelper.DropTables(ctx, db, (*dao)(nil))
        },
    )
}
```

```go
// pkg/migrations/indexerdb/4_create_token_balances.go
package indexerdb

import (
    "context"
    "log"
    "time"

    mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
    "github.com/uptrace/bun"
)

func init() {
    Migrations.MustRegister(
        func(ctx context.Context, db *bun.DB) error {
            log.Println("creating token_balances table...")
            // NOTE: No evm_address column. The indexer is Canton-native.
            // EVM address resolution is the api-server's responsibility.
            type dao struct {
                bun.BaseModel `bun:"table:token_balances"`
                PartyID       string    `bun:"party_id,pk,type:varchar(255)"`
                TokenSymbol   string    `bun:"token_symbol,pk,type:varchar(50)"`
                Balance       string    `bun:"balance,notnull,default:0,type:numeric(38,18)"`
                UpdatedAt     time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
            }
            if err := mghelper.CreateSchema(ctx, db, (*dao)(nil)); err != nil {
                return err
            }
            return mghelper.CreateModelIndexes(ctx, db, (*dao)(nil), "token_symbol")
        },
        func(ctx context.Context, db *bun.DB) error {
            log.Println("dropping token_balances table...")
            type dao struct {
                bun.BaseModel `bun:"table:token_balances"`
            }
            return mghelper.DropTables(ctx, db, (*dao)(nil))
        },
    )
}
```

```go
// pkg/migrations/indexerdb/5_create_token_stats.go
package indexerdb

import (
    "context"
    "log"
    "time"

    mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
    "github.com/uptrace/bun"
)

func init() {
    Migrations.MustRegister(
        func(ctx context.Context, db *bun.DB) error {
            log.Println("creating token_stats table...")
            type dao struct {
                bun.BaseModel `bun:"table:token_stats"`
                TokenSymbol   string    `bun:"token_symbol,pk,type:varchar(50)"`
                TotalSupply   string    `bun:"total_supply,notnull,default:0,type:numeric(38,18)"`
                HolderCount   int64     `bun:"holder_count,notnull,default:0"`
                UpdatedAt     time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
            }
            return mghelper.CreateSchema(ctx, db, (*dao)(nil))
        },
        func(ctx context.Context, db *bun.DB) error {
            log.Println("dropping token_stats table...")
            type dao struct {
                bun.BaseModel `bun:"table:token_stats"`
            }
            return mghelper.DropTables(ctx, db, (*dao)(nil))
        },
    )
}
```

---

### 6.7 Service Layer

Canton-native query service. All methods keyed by `canton_party_id`. No userstore.
No EVM address. Mirrors `pkg/token/service.go` in structure.

```go
// pkg/indexer/service/service.go
package service

import (
    "context"
    "github.com/chainsafe/canton-middleware/pkg/indexer/store"
)

//go:generate mockery --name Service --output ./mocks
type Service interface {
    ListTokens(ctx context.Context) ([]*TokenInfo, error)
    GetTokenInfo(ctx context.Context, tokenSymbol string) (*TokenInfo, error)
    // All balance/history queries take canton_party_id as the primary identifier.
    // The caller (api-server) is responsible for resolving EVM address → party_id
    // before calling these methods.
    GetBalance(ctx context.Context, partyID, tokenSymbol string) (*Balance, error)
    GetTransferHistory(ctx context.Context, partyID string, filter TransferFilter) (*TransferPage, error)
}

type indexerService struct {
    store store.Store
}

func NewService(s store.Store) Service {
    return &indexerService{store: s}
}

func (s *indexerService) GetBalance(ctx context.Context, partyID, tokenSymbol string) (*Balance, error) {
    dao, err := s.store.GetTokenBalance(ctx, partyID, tokenSymbol)
    if err != nil {
        return nil, fmt.Errorf("get balance: %w", err)
    }
    token, err := s.store.GetTokenStat(ctx, tokenSymbol)
    if err != nil {
        return nil, fmt.Errorf("get token: %w", err)
    }
    return toBalance(dao, token), nil
}

func (s *indexerService) GetTransferHistory(ctx context.Context, partyID string, f TransferFilter) (*TransferPage, error) {
    rows, total, err := s.store.ListTransferEvents(ctx, store.TransferEventFilter{
        PartyID:     &partyID,
        TokenSymbol: f.TokenSymbol,
        EventType:   f.EventType,
        Page:        f.Page,
        PageSize:    f.PageSize,
    })
    if err != nil {
        return nil, fmt.Errorf("get transfer history: %w", err)
    }
    return toTransferPage(rows, total, f), nil
}
```

```go
// pkg/indexer/service/types.go
package service

type TokenInfo struct {
    Symbol      string `json:"symbol"`
    Name        string `json:"name"`
    Decimals    int    `json:"decimals"`
    TotalSupply string `json:"total_supply"`
    HolderCount int64  `json:"holder_count"`
}

// Balance is keyed by canton_party_id. The api-server maps EVM → party_id before
// calling the indexer and may re-map the response back to EVM context for its clients.
type Balance struct {
    PartyID         string `json:"party_id"`
    TokenSymbol     string `json:"token_symbol"`
    Balance         string `json:"balance"`          // raw (18 decimals)
    BalanceFormatted string `json:"balance_formatted"` // human readable
    Decimals        int    `json:"decimals"`
}

type TransferEvent struct {
    EventID         string  `json:"event_id"`
    EventType       string  `json:"event_type"`       // MINT | BURN | TRANSFER
    FromPartyID     *string `json:"from_party_id"`
    ToPartyID       *string `json:"to_party_id"`
    Amount          string  `json:"amount"`
    AmountFormatted string  `json:"amount_formatted"`
    TokenSymbol     string  `json:"token_symbol"`
    EVMTxHash       *string `json:"evm_tx_hash,omitempty"`
    LedgerOffset    int64   `json:"ledger_offset"`
    EffectiveTime   string  `json:"effective_time"`
}

type TransferPage struct {
    Total    int             `json:"total"`
    Page     int             `json:"page"`
    PageSize int             `json:"page_size"`
    Events   []TransferEvent `json:"events"`
}

type TransferFilter struct {
    TokenSymbol *string
    EventType   *string
    Page        int
    PageSize    int
}
```

---

### 6.8 API / HTTP Layer

**Auth: JWT only.** The JWT is issued by the api-server (or a shared auth service) after
authenticating the user via EVM signature. The JWT claims must contain `canton_party_id`.
The indexer validates the JWT signature against the shared JWKS endpoint and extracts
`canton_party_id` from claims — no userstore, no EVM signature verification here.

**Endpoints use `partyID` as path param**, not EVM address. The api-server is the
translator between EVM world and Canton-native world.

```go
// pkg/indexer/api/server.go
package api

func RegisterRoutes(r chi.Router, svc service.Service, cfg AuthConfig, logger *zap.Logger) {
    h := newHandler(svc, logger)

    // Public — no auth (totalSupply is public per ERC-20 spec)
    r.Get("/v1/tokens", h.listTokens)
    r.Get("/v1/tokens/{symbol}", h.getToken)
    r.Get("/v1/tokens/{symbol}/totalSupply", h.getTotalSupply)

    // JWT-authenticated — scoped to the party_id in the JWT claims
    r.Group(func(r chi.Router) {
        r.Use(authMiddleware(cfg))
        r.Get("/v1/balance/{partyID}", h.getBalance)
        r.Get("/v1/balance/{partyID}/{symbol}", h.getBalanceBySymbol)
        r.Get("/v1/transfers/{partyID}", h.getTransfers)
        r.Get("/v1/transfers/{partyID}/{symbol}", h.getTransfersBySymbol)
        r.Get("/v1/events/{partyID}", h.getTransfers) // alias
    })
}
```

```go
// pkg/indexer/api/middleware.go
package api

// AuthConfig holds the JWKS URL for JWT validation.
// No userstore reference — the indexer does not know about EVM addresses.
type AuthConfig struct {
    JWKSUrl string
}

// Claims are extracted from the JWT. The JWT is issued by the api-server
// and must carry canton_party_id so the indexer can scope queries.
type Claims struct {
    CantonPartyID string `json:"canton_party_id"`
    // Other standard JWT fields (exp, iat, sub) handled by the JWT library
}

type principalKey struct{}

// authMiddleware validates the JWT and stores the party_id in context.
// Only JWT Bearer tokens are accepted — no EVM signature verification.
func authMiddleware(cfg AuthConfig) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if bearer == "" {
                writeError(w, http.StatusUnauthorized, errors.New("Bearer token required"))
                return
            }
            claims, err := validateJWT(bearer, cfg.JWKSUrl)
            if err != nil {
                writeError(w, http.StatusUnauthorized, err)
                return
            }
            if claims.CantonPartyID == "" {
                writeError(w, http.StatusUnauthorized,
                    errors.New("JWT missing canton_party_id claim"))
                return
            }
            ctx := context.WithValue(r.Context(), principalKey{}, claims.CantonPartyID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// scopeCheck ensures the authenticated party can only read its own data.
func scopeCheck(r *http.Request, requestedPartyID string) error {
    partyID, ok := r.Context().Value(principalKey{}).(string)
    if !ok || partyID == "" {
        return errors.New("unauthenticated")
    }
    if partyID != requestedPartyID {
        return errors.New("access denied: can only query own party data")
    }
    return nil
}
```

```go
// pkg/indexer/api/handler.go
package api

func (h *handler) getBalance(w http.ResponseWriter, r *http.Request) {
    partyID := chi.URLParam(r, "partyID")
    if err := scopeCheck(r, partyID); err != nil {
        writeError(w, http.StatusForbidden, err)
        return
    }
    symbol := chi.URLParam(r, "symbol") // may be "" for all-tokens variant
    bal, err := h.svc.GetBalance(r.Context(), partyID, symbol)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err)
        return
    }
    writeJSON(w, http.StatusOK, bal)
}

func (h *handler) getTransfers(w http.ResponseWriter, r *http.Request) {
    partyID := chi.URLParam(r, "partyID")
    if err := scopeCheck(r, partyID); err != nil {
        writeError(w, http.StatusForbidden, err)
        return
    }
    page, _     := strconv.Atoi(r.URL.Query().Get("page"))
    pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
    symbol      := r.URL.Query().Get("token")
    evtType     := r.URL.Query().Get("type")

    f := service.TransferFilter{Page: page, PageSize: pageSize}
    if symbol != "" { f.TokenSymbol = &symbol }
    if evtType != "" { f.EventType = &evtType }

    result, err := h.svc.GetTransferHistory(r.Context(), partyID, f)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err)
        return
    }
    writeJSON(w, http.StatusOK, result)
}
```

**How the api-server calls the indexer on behalf of an EVM client:**

```go
// pkg/token/provider/indexer.go  (new provider in api-server)
// The api-server resolves EVM → party_id, mints a short-lived JWT, calls indexer.

func (p *IndexerProvider) GetBalance(ctx context.Context, tokenSymbol, fingerprint string) (string, error) {
    // 1. Resolve fingerprint → canton_party_id via userstore (api-server's own DB)
    user, err := p.userStore.GetUserByFingerprint(ctx, fingerprint)
    if err != nil {
        return "0", err
    }
    // 2. Mint a short-lived internal JWT with canton_party_id claim
    jwt, err := p.jwtIssuer.IssuePartyJWT(*user.CantonPartyID)
    if err != nil {
        return "0", err
    }
    // 3. Call indexer HTTP API — indexer sees only party_id, never EVM address
    return p.indexerClient.GetBalance(ctx, *user.CantonPartyID, tokenSymbol, jwt)
}
```

---

## 7. File & Directory Layout

```
canton-middleware/
│
├── cmd/
│   ├── api-server/                 existing
│   ├── relayer/                    existing
│   └── indexer/                    NEW
│       ├── main.go                 loads config → app/indexer.NewServer(cfg).Run()
│       └── migrate/
│           └── main.go             runs indexerdb migrations
│
├── pkg/
│   │
│   ├── app/
│   │   ├── api/                    existing (api-server orchestrator)
│   │   └── indexer/                NEW (mirrors pkg/app/api/)
│   │       └── server.go           wires streaming + fetcher + parser +
│   │                               processor + service + HTTP server
│   │
│   ├── cantonsdk/
│   │   ├── bridge/                 existing — unchanged
│   │   ├── token/                  existing — unchanged
│   │   ├── ledger/                 existing — unchanged
│   │   ├── lapi/v2/                existing — unchanged
│   │   └── streaming/              NEW — generic ledger streaming client
│   │       ├── client.go           Subscribe(), runStream(), reconnect loop
│   │       └── types.go            LedgerTransaction, LedgerEvent
│   │
│   ├── indexer/                    NEW — all indexer domain packages
│   │   │
│   │   ├── fetcher/
│   │   │   └── fetcher.go          loads checkpoint → delegates to cantonsdk/streaming
│   │   │
│   │   ├── parser/
│   │   │   ├── parser.go           routes LedgerTransaction → []ParsedEvent
│   │   │   ├── cip56.go            decodeTokenTransferEvent() via cantonsdk/values
│   │   │   ├── whitelist.go        ContractFilter, WhitelistFilter, AllFilter
│   │   │   └── types.go            ParsedEvent, EventType (MINT/BURN/TRANSFER)
│   │   │
│   │   ├── processor/
│   │   │   └── processor.go        atomic batch writer: events + balances + checkpoint
│   │   │
│   │   ├── store/
│   │   │   ├── model.go            DAOs (no evm_address in TokenBalanceDao)
│   │   │   ├── store.go            Store + Tx interfaces, TransferEventFilter
│   │   │   └── pg.go               pgStore + pgTx (Bun ORM)
│   │   │
│   │   ├── service/
│   │   │   ├── service.go          Service interface + impl, all methods by party_id
│   │   │   └── types.go            TokenInfo, Balance, TransferEvent, TransferPage
│   │   │
│   │   └── api/                    HTTP layer (add graph/ here in Phase 2)
│   │       ├── server.go           RegisterRoutes() on chi.Router
│   │       ├── handler.go          listTokens, getBalance, getTransfers
│   │       ├── middleware.go       authMiddleware (JWT only), scopeCheck
│   │       └── types.go            JSON response types
│   │
│   └── migrations/
│       ├── apidb/                  existing
│       └── indexerdb/              NEW
│           ├── migrations.go       var Migrations = migrate.NewMigrations()
│           ├── 1_create_ledger_checkpoints.go
│           ├── 2_create_indexed_tokens.go
│           ├── 3_create_transfer_events.go
│           ├── 4_create_token_balances.go
│           └── 5_create_token_stats.go
│
├── contracts/
│   └── canton-erc20/daml/cip56-token/src/CIP56/
│       ├── Events.daml             MODIFIED — add TokenTransferEvent
│       ├── Config.daml             MODIFIED — emit from IssuerMint/IssuerBurn
│       └── TransferFactory.daml   MODIFIED — emit from transfer choice
│
└── docs/
    ├── indexer-design.md           this document
    └── indexer-gh-issue.md         GitHub issue (condensed)
```

---

## 8. Pseudo-code & Data Flows

### Orchestrator — `pkg/app/indexer/server.go`

```go
// pkg/app/indexer/server.go
package indexer

type Server struct{ cfg *config.IndexerConfig }

func NewServer(cfg *config.IndexerConfig) *Server { return &Server{cfg: cfg} }

func (s *Server) Run() error {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    logger, _ := config.NewLogger(s.cfg.Logging)
    defer logger.Sync()

    dbBun, err := pgutil.ConnectDB(&s.cfg.Database)
    if err != nil { return err }
    defer dbBun.Close()

    idxStore := indexerstore.NewStore(dbBun)

    ledgerClient, err := ledger.New(s.cfg.Canton)
    if err != nil { return err }
    defer ledgerClient.Close()

    streamClient := streaming.New(ledgerClient, s.cfg.Canton.IssuerParty)

    templateID := &lapiv2.Identifier{
        PackageId:  s.cfg.Indexer.PackageID,
        ModuleName: "CIP56",
        EntityName: "TokenTransferEvent",
    }

    var filter parser.ContractFilter
    if len(s.cfg.Indexer.WhitelistedPackageIDs) > 0 {
        filter = parser.NewWhitelistFilter(s.cfg.Indexer.WhitelistedPackageIDs)
    } else {
        filter = &parser.AllFilter{}
    }

    txCh     := make(chan *streaming.LedgerTransaction, 500)
    parsedCh := make(chan []*parser.ParsedEvent, 100)

    f    := fetcher.New(streamClient, idxStore, templateID, txCh)
    p    := parser.New(txCh, parsedCh, filter)
    proc := processor.New(idxStore, parsedCh)

    svc := indexerservice.NewService(idxStore) // no userstore dependency
    r   := s.setupRouter(svc, logger)
    go apphttp.ServeAndWait(ctx, r, logger, &s.cfg.Query.Server)

    g, ctx := errgroup.WithContext(ctx)
    g.Go(func() error { return f.Start(ctx) })
    g.Go(func() error { return p.Start(ctx) })
    g.Go(func() error { return proc.Start(ctx) })

    logger.Info("indexer running")
    return g.Wait()
}

func (s *Server) setupRouter(svc indexerservice.Service, logger *zap.Logger) chi.Router {
    r := chi.NewRouter()
    r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
    r.Use(middleware.Timeout(60 * time.Second))
    r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("OK"))
    })
    indexerapi.RegisterRoutes(r, svc, indexerapi.AuthConfig{
        JWKSUrl: s.cfg.Query.JWKSUrl,
    }, logger)
    return r
}
```

### Call flow: EVM client → api-server → indexer

```
EVM Client (MetaMask)
    │  GET balance (EVM address, DEMO token)
    ▼
API Server
    │  1. Auth: VerifyEIP191Signature(evmAddress)     ← pkg/auth/evm.go
    │  2. Resolve: userStore.GetUserByEVMAddress()    ← userstore (api-server DB)
    │     → canton_party_id
    │  3. Issue: jwtIssuer.IssuePartyJWT(canton_party_id)
    │  4. Call: indexer /v1/balance/{canton_party_id}
    │           Authorization: Bearer <jwt with canton_party_id claim>
    ▼
Indexer
    │  5. Validate JWT → extract canton_party_id from claims
    │  6. scopeCheck: party_id matches URL param
    │  7. store.GetTokenBalance(canton_party_id, "DEMO")
    │  8. Return Balance{party_id, balance}
    ▼
API Server
    │  9. Map party_id → evm_address for client response (if needed)
    ▼
EVM Client  ← {"balance": "1000000000000000000"}
```

---

## 9. Configuration

```go
// pkg/config/indexer.go

type IndexerConfig struct {
    Logging LoggingConfig
    Database DatabaseConfig  // shared with api-server
    Canton   CantonConfig    // same issuer credentials as api-server
    Indexer  IndexerOptions
    Query    IndexerQueryConfig
}

type IndexerOptions struct {
    WhitelistedPackageIDs []string      `yaml:"whitelisted_package_ids"`
    PackageID             string        `yaml:"package_id"` // TokenTransferEvent package
    MaxReconnectBackoff   time.Duration `yaml:"max_reconnect_backoff"` // default 60s
}

type IndexerQueryConfig struct {
    Server  ServerConfig // host, port, timeouts
    JWKSUrl string       `yaml:"jwks_url"` // shared with api-server
}
```

```yaml
# config.indexer.yaml
logging:
  level: info
  format: json

database:
  host: localhost
  port: 5432
  name: canton_middleware  # same DB, indexer writes its own tables
  user: postgres
  password: ${POSTGRES_PASSWORD}

canton:
  endpoint: localhost:5011
  issuer_party: "Issuer::1220..."
  auth:
    type: oauth2
    client_id: ${CANTON_CLIENT_ID}
    client_secret: ${CANTON_CLIENT_SECRET}
    token_url: ${CANTON_TOKEN_URL}

indexer:
  package_id: "168483ce8a80e76f69f7392ceaa9ff57b1036b8fb41ccb3d410b087048195a92"
  whitelisted_package_ids:
    - "168483ce8a80e76f69f7392ceaa9ff57b1036b8fb41ccb3d410b087048195a92"  # DEMO
    - "<PROMPT package ID>"
  max_reconnect_backoff: 60s

query:
  server:
    host: 0.0.0.0
    port: 8082
  jwks_url: ${JWKS_URL}   # same JWKS as api-server for JWT validation
```

---

## 10. Integration with API Server

```
Shared PostgreSQL (canton_middleware DB):

  public.*       ← api-server writes
    users                   ← EVM ↔ party_id mapping lives here, NOT in indexer
    user_token_balances     ← DEPRECATED after migration
    bridge_events           ← DEPRECATED after migration
    token_metrics           ← DEPRECATED after migration

  indexer.*      ← indexer writes, api-server may read
    ledger_checkpoints
    indexed_tokens
    transfer_events         ← replaces bridge_events (richer, includes transfers)
    token_balances          ← replaces user_token_balances (keyed by party_id)
    token_stats             ← replaces token_metrics

Option A (recommended Phase 1):
  api-server issues a JWT → calls indexer HTTP API.
  Clean separation. No shared DB reads from api-server side.

Option B (simpler Phase 1 alternative):
  api-server reads indexer.token_balances directly via SQL
  (same DB, no HTTP hop needed). Requires api-server to know the indexer schema.
```

**Migration path:**
```
Step 1  Deploy indexer. It builds indexer.* tables from offset 0.
        Reconciler continues running in parallel.

Step 2  Validate: compare reconciler balances vs indexer.token_balances.
        Confirm TokenTransferEvent DAML upgrade is live and emitting.

Step 3  Switch api-server token provider to call indexer API (or read indexer tables).

Step 4  Disable reconciler. Remove after one release cycle.
```

---

## 11. Open Questions & Future Work

| Question | Decision |
|---|---|
| DB: same instance? | Yes — same DB, indexer.* schema |
| ORM? | Bun — consistent with project |
| HTTP router? | chi — consistent with project |
| Query port? | 8082 (api=8080, relayer=8081) |
| Canton auth? | Reuse issuer OAuth2 creds from existing config |
| JWT claim name for party_id? | `canton_party_id` (custom claim) |
| api-server call mode? | Option A (HTTP) initially; can collapse to Option B (shared DB read) |
| Docker Compose? | Add `indexer` + `indexer-migrate` services |

### Phase 2

1. **GraphQL** — add `pkg/indexer/graph/` alongside `pkg/indexer/api/`
2. **WebSocket push** — real-time event stream from processor → subscribed clients
3. **Canton Coin** — same code, different package IDs + Super Validator node
4. **Metrics** — `indexer_lag_offsets`, `events_per_second`, `batch_commit_duration_ms`
5. **Backfill** — `cmd/indexer/backfill/` to replay from offset 0 after package upgrades

---

*Created: 2026-03-02*
*CIP Reference: https://github.com/canton-foundation/cips/blob/main/cip-0086/cip-0086.md*
