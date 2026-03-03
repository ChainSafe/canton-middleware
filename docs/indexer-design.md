# Canton Middleware — Distributed Indexer Design

> **Spec:** [CIP-0086](https://github.com/canton-foundation/cips/blob/main/cip-0086/cip-0086.md)
> **Phase 1 scope:** CIP-56 tokens (DEMO + PROMPT) only. No Canton Coin.

---

## Table of Contents

1. [Goals & Scope](#1-goals--scope)
2. [Architecture Overview](#2-architecture-overview)
3. [DAML Contract Change: TokenTransferEvent](#3-daml-contract-change-tokentransferevent)
4. [cantonsdk/streaming — Generic Ledger Stream](#4-cantonsdk-streaming--generic-ledger-stream)
5. [Indexer Pipeline: Fetcher → Parser → Processor](#5-indexer-pipeline-fetcher--parser--processor)
6. [Database Schema (Go migrations)](#6-database-schema-go-migrations)
7. [Store Interface & PostgreSQL Implementation](#7-store-interface--postgresql-implementation)
8. [Service Layer](#8-service-layer)
9. [API/HTTP Layer](#9-apihttp-layer)
10. [Authentication (JWT-only)](#10-authentication-jwt-only)
11. [File Layout](#11-file-layout)
12. [cmd/indexer Entry Point](#12-cmdindexer-entry-point)
13. [Configuration](#13-configuration)

---

## 1. Goals & Scope

### What the indexer does
- Subscribe to the Canton Ledger API v2 (`UpdateService.GetUpdates`) and stream `TokenTransferEvent` contracts.
- Maintain a queryable, off-chain projection of:
  - Per-party token balances (`token_balances`)
  - Transfer event history (`transfer_events`)
  - Global token statistics: `totalSupply`, `circulatingSupply` (`token_stats`)
- Expose an HTTP API implementing CIP-0086 query endpoints.

### What it does NOT do
- It does **not** depend on `pkg/userstore` or look up users by EVM address.
- It does **not** issue or verify EVM signatures.
- It does **not** know about EVM addresses at all. The canonical identity is `canton_party_id`.
- The existing api-server acts as the EVM → `canton_party_id` translator and issues JWTs.

### Privacy model
All per-party queries (balance, transfer history) require a JWT containing the `canton_party_id` claim. The JWT is validated using a shared JWKS endpoint. Total supply and token metadata are public (ERC-20 convention).

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                      CANTON LEDGER                               │
│    UpdateService.GetUpdates (gRPC server-streaming)              │
└───────────────────────────┬──────────────────────────────────────┘
                            │ TokenTransferEvent contracts
                            ▼
┌──────────────────────────────────────────────────────────────────┐
│                 pkg/indexer/fetcher                              │
│  cantonsdk/streaming.Stream[TokenTransferEvent]                  │
│  Checkpoint management (ledger offset)                           │
└───────────────────────────┬──────────────────────────────────────┘
                            │ raw events
                            ▼
┌──────────────────────────────────────────────────────────────────┐
│                 pkg/indexer/parser                               │
│  DAML value → Go struct (protobuf decode)                        │
│  Emits: ParsedEvent{Type, From, To, Amount, Symbol, TxHash, ...} │
└───────────────────────────┬──────────────────────────────────────┘
                            │ ParsedEvent
                            ▼
┌──────────────────────────────────────────────────────────────────┐
│                 pkg/indexer/processor                            │
│  Applies event to DB in a single transaction:                    │
│   • insert transfer_events row                                   │
│   • upsert token_balances (MINT +, BURN -, TRANSFER +/-)        │
│   • update token_stats (totalSupply, circulatingSupply)          │
│   • update ledger_checkpoints                                    │
└───────────────────────────┬──────────────────────────────────────┘
                            │ writes
                            ▼
┌──────────────────────────────────────────────────────────────────┐
│             pkg/indexer/store (PostgreSQL via Bun)               │
│  Tables: transfer_events, token_balances, token_stats,           │
│          indexed_tokens, ledger_checkpoints                      │
└───────────────────────────┬──────────────────────────────────────┘
                            │ reads
                            ▼
┌──────────────────────────────────────────────────────────────────┐
│             pkg/indexer/api/http                                 │
│  GET /v1/tokens                                                  │
│  GET /v1/tokens/{symbol}/supply                                  │
│  GET /v1/tokens/{symbol}/balance/{partyID}  [JWT required]      │
│  GET /v1/tokens/{symbol}/transfers/{partyID} [JWT required]      │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. DAML Contract Change: TokenTransferEvent

### New DAML template (additive — no breaking changes)

```daml
-- TokenTransferEvent is emitted as a side-effect from:
--   • CIP56TransferFactory.Transfer → eventType = "TRANSFER"
--   • TokenAdmin.Mint             → eventType = "MINT"  (fromParty = None)
--   • TokenAdmin.Burn             → eventType = "BURN"  (toParty = None)
--
-- The existing MintEvent / BurnEvent templates are kept for backward compat.

template TokenTransferEvent
  with
    issuer          : Party
    fromParty       : Optional Party    -- None for mints
    toParty         : Optional Party    -- None for burns
    amount          : Decimal
    tokenSymbol     : Text
    eventType       : Text              -- "MINT" | "BURN" | "TRANSFER"
    timestamp       : Time
    evmTxHash       : Optional Text     -- set for bridge-originated events
    evmDestination  : Optional Text     -- set for bridge burns
    userFingerprint : Optional Text     -- Keccak256(evmAddress) if known
    auditObservers  : [Party]
  where
    signatory issuer
    observer
      optional [] (\p -> [p]) fromParty,
      optional [] (\p -> [p]) toParty,
      auditObservers
```

### Why a unified template?
- ERC-20 defines a single `Transfer(from, to, value)` event. Indexers subscribe to one stream, not three.
- Mints: `fromParty = None` (equivalent to ERC-20 address(0)).
- Burns: `toParty = None`.
- Keeps `auditObservers` for compliance-level visibility without breaking Canton privacy.
- The indexer has a single `getUpdates` subscription — simpler reconnect/offset tracking.

### Existing templates untouched
`CIP56Holding`, `CIP56TransferFactory`, `MintEvent`, `BurnEvent` — no changes required.

---

## 4. cantonsdk/streaming — Generic Ledger Stream

Rather than duplicating the stream/reconnect logic from `bridge/client.go`, we extract it into a reusable package.

### New file: `pkg/cantonsdk/streaming/stream.go`

```go
package streaming

import (
    "context"
    "errors"
    "io"
    "time"

    lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
    "github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
    "go.uber.org/zap"
)

const (
    defaultReconnectDelay    = 5 * time.Second
    defaultMaxReconnectDelay = 60 * time.Second
)

// DecodeFunc decodes a raw DAML record value into a typed event T.
// It should return (nil, nil) to skip the event (e.g. archived contracts).
type DecodeFunc[T any] func(event *lapiv2.Event) (*T, error)

// Config holds parameters for a generic ledger stream.
type Config struct {
    // UpdateFormat defines which templates / parties to subscribe to.
    UpdateFormat *lapiv2.UpdateFormat

    // InitialOffset is the ledger offset to start from (exclusive).
    // Use "0" or "" to start from the beginning.
    InitialOffset string

    ReconnectDelay    time.Duration
    MaxReconnectDelay time.Duration
}

// Stream subscribes to Canton's GetUpdates RPC and decodes events into T.
// It reconnects automatically on transient errors with exponential back-off.
// The returned channel is closed when ctx is cancelled or EOF is reached.
func Stream[T any](
    ctx context.Context,
    l ledger.Ledger,
    cfg Config,
    decode DecodeFunc[T],
    logger *zap.Logger,
) (<-chan *T, <-chan error) {
    outCh := make(chan *T, 64)
    errCh := make(chan error, 1)

    reconnectDelay := cfg.ReconnectDelay
    if reconnectDelay == 0 {
        reconnectDelay = defaultReconnectDelay
    }
    maxDelay := cfg.MaxReconnectDelay
    if maxDelay == 0 {
        maxDelay = defaultMaxReconnectDelay
    }

    go func() {
        defer close(outCh)
        defer close(errCh)

        currentOffset := cfg.InitialOffset
        delay := reconnectDelay

        for {
            select {
            case <-ctx.Done():
                return
            default:
            }

            err := streamOnce(ctx, l, cfg.UpdateFormat, currentOffset, decode, outCh, &currentOffset, logger)
            if err == nil || errors.Is(err, io.EOF) || ctx.Err() != nil {
                return
            }

            if isAuthError(err) {
                l.InvalidateToken()
                delay = reconnectDelay
            }

            logger.Warn("stream error, reconnecting", zap.Error(err), zap.Duration("in", delay))
            select {
            case <-ctx.Done():
                return
            case <-time.After(delay):
            }
            delay = min(delay*2, maxDelay)
        }
    }()

    return outCh, errCh
}
```

### Usage in indexer/fetcher

```go
cfg := streaming.Config{
    InitialOffset: checkpoint.Offset,
    UpdateFormat: &lapiv2.UpdateFormat{
        IncludeTransactions: &lapiv2.TransactionFormat{
            EventFormat: &lapiv2.EventFormat{
                FiltersByParty: map[string]*lapiv2.Filters{
                    indexerParty: {
                        Cumulative: []*lapiv2.CumulativeFilter{{
                            IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
                                TemplateFilter: &lapiv2.TemplateFilter{
                                    TemplateId: &lapiv2.Identifier{
                                        PackageId:  tokenPackageID,
                                        ModuleName: "CIP56.TokenTransfer",
                                        EntityName: "TokenTransferEvent",
                                    },
                                },
                            },
                        }},
                    },
                },
                Verbose: true,
            },
            TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
        },
    },
}

eventCh, _ := streaming.Stream[ParsedEvent](ctx, ledger, cfg, parser.Decode, logger)
```

> **WebSocket note:** Canton's gRPC endpoint uses HTTP/2 server-streaming — not WebSocket. WebSocket is a Phase 2 option for _indexer → client_ push notifications only (not for Canton → indexer).

---

## 5. Indexer Pipeline: Fetcher → Parser → Processor

### fetcher

```go
// pkg/indexer/fetcher/fetcher.go
package fetcher

type Fetcher struct {
    ledger    ledger.Ledger
    store     store.Store
    cfg       Config
    logger    *zap.Logger
}

// Run subscribes to Canton, recovers the last checkpoint offset, and
// forwards decoded events to the processor.
func (f *Fetcher) Run(ctx context.Context, processor *processor.Processor) error {
    checkpoint, err := f.store.GetCheckpoint(ctx)
    if err != nil {
        return fmt.Errorf("load checkpoint: %w", err)
    }

    eventCh, errCh := streaming.Stream[parser.RawEvent](ctx, f.ledger, f.buildStreamConfig(checkpoint.Offset), nil, f.logger)

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case err := <-errCh:
            return err
        case raw, ok := <-eventCh:
            if !ok {
                return nil
            }
            parsed, err := parser.Decode(raw)
            if err != nil {
                f.logger.Warn("decode error", zap.Error(err))
                continue
            }
            if err := processor.Apply(ctx, parsed); err != nil {
                return fmt.Errorf("processor: %w", err)
            }
        }
    }
}
```

### parser

```go
// pkg/indexer/parser/parser.go
package parser

type ParsedEvent struct {
    EventType      string          // "MINT" | "BURN" | "TRANSFER"
    FromParty      string          // "" for mints
    ToParty        string          // "" for burns
    Amount         *big.Int
    TokenSymbol    string
    LedgerOffset   string
    TransactionID  string
    Timestamp      time.Time
    EvmTxHash      string          // optional
    EvmDestination string          // optional
    UserFingerprint string         // optional
}

// Decode extracts a ParsedEvent from a Canton ACS-delta event.
// Returns nil to skip non-TokenTransferEvent records.
func Decode(event *lapiv2.Event) (*ParsedEvent, error) {
    created := event.GetCreated()
    if created == nil {
        return nil, nil // archived contract, skip
    }

    rec := created.GetCreateArguments()
    // ... decode DAML record fields using pkg/cantonsdk/values helpers ...
    return &ParsedEvent{...}, nil
}
```

### processor

```go
// pkg/indexer/processor/processor.go
package processor

type Processor struct {
    store store.Store
}

// Apply writes a single parsed event atomically: updates balances, stats, and checkpoint.
func (p *Processor) Apply(ctx context.Context, e *parser.ParsedEvent) error {
    return p.store.RunInTx(ctx, func(ctx context.Context, tx store.Tx) error {
        // 1. Insert transfer_events
        if err := tx.InsertTransferEvent(ctx, toTransferEventDao(e)); err != nil {
            return err
        }

        // 2. Upsert token_balances
        switch e.EventType {
        case "MINT":
            if err := tx.AdjustBalance(ctx, e.ToParty, e.TokenSymbol, e.Amount, true); err != nil {
                return err
            }
        case "BURN":
            if err := tx.AdjustBalance(ctx, e.FromParty, e.TokenSymbol, new(big.Int).Neg(e.Amount), false); err != nil {
                return err
            }
        case "TRANSFER":
            if err := tx.AdjustBalance(ctx, e.FromParty, e.TokenSymbol, new(big.Int).Neg(e.Amount), false); err != nil {
                return err
            }
            if err := tx.AdjustBalance(ctx, e.ToParty, e.TokenSymbol, e.Amount, false); err != nil {
                return err
            }
        }

        // 3. Update token_stats
        if err := tx.UpdateTokenStats(ctx, e); err != nil {
            return err
        }

        // 4. Advance checkpoint
        return tx.SetCheckpoint(ctx, e.LedgerOffset)
    })
}
```

---

## 6. Database Schema (Go migrations)

**Migration package:** `pkg/migrations/indexerdb/`

### service.go

```go
// pkg/migrations/indexerdb/service.go
package indexerdb

import "github.com/uptrace/bun/migrate"

var Migrations = migrate.NewMigrations()
```

### 1_create_ledger_checkpoints.go

```go
package indexerdb

import (
    "context"
    "log"
    "github.com/chainsafe/canton-middleware/pkg/indexer/store"
    mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
    "github.com/uptrace/bun"
)

func init() {
    Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
        log.Println("creating ledger_checkpoints table...")
        return mghelper.CreateSchema(ctx, db, &store.LedgerCheckpointDao{})
    }, func(ctx context.Context, db *bun.DB) error {
        log.Println("dropping ledger_checkpoints table...")
        return mghelper.DropTables(ctx, db, &store.LedgerCheckpointDao{})
    })
}
```

### 2_create_indexed_tokens.go

```go
package indexerdb

func init() {
    Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
        log.Println("creating indexed_tokens table...")
        return mghelper.CreateSchema(ctx, db, &store.IndexedTokenDao{})
    }, func(ctx context.Context, db *bun.DB) error {
        return mghelper.DropTables(ctx, db, &store.IndexedTokenDao{})
    })
}
```

### 3_create_transfer_events.go

```go
package indexerdb

func init() {
    Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
        log.Println("creating transfer_events table...")
        if err := mghelper.CreateSchema(ctx, db, &store.TransferEventDao{}); err != nil {
            return err
        }
        return mghelper.CreateModelIndexes(ctx, db, &store.TransferEventDao{},
            "from_party",
            "to_party",
            "token_symbol",
            "ledger_offset",
        )
    }, func(ctx context.Context, db *bun.DB) error {
        return mghelper.DropTables(ctx, db, &store.TransferEventDao{})
    })
}
```

### 4_create_token_balances.go

```go
package indexerdb

func init() {
    Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
        log.Println("creating token_balances table...")
        // NOTE: No evm_address column. The indexer is Canton-native.
        // The api-server translates EVM addresses to party_ids before calling the indexer.
        if err := mghelper.CreateSchema(ctx, db, &store.TokenBalanceDao{}); err != nil {
            return err
        }
        return mghelper.CreateModelIndexes(ctx, db, &store.TokenBalanceDao{},
            "party_id",
            "token_symbol",
        )
    }, func(ctx context.Context, db *bun.DB) error {
        return mghelper.DropTables(ctx, db, &store.TokenBalanceDao{})
    })
}
```

### 5_create_token_stats.go

```go
package indexerdb

func init() {
    Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
        log.Println("creating token_stats table...")
        return mghelper.CreateSchema(ctx, db, &store.TokenStatDao{})
    }, func(ctx context.Context, db *bun.DB) error {
        return mghelper.DropTables(ctx, db, &store.TokenStatDao{})
    })
}
```

### DAO models (pkg/indexer/store/models.go)

```go
package store

import (
    "time"
    "github.com/uptrace/bun"
)

// LedgerCheckpointDao tracks the last successfully processed ledger offset.
// There is always exactly one row (id = 1).
type LedgerCheckpointDao struct {
    bun.BaseModel `bun:"table:ledger_checkpoints"`
    ID            int64     `bun:"id,pk"`
    Offset        string    `bun:"offset,notnull,default:'0',type:varchar(255)"`
    UpdatedAt     time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
}

// IndexedTokenDao holds metadata about each tracked CIP-56 token.
type IndexedTokenDao struct {
    bun.BaseModel  `bun:"table:indexed_tokens"`
    Symbol         string    `bun:"symbol,pk,type:varchar(50)"`
    PackageID      string    `bun:"package_id,notnull,type:varchar(255)"`
    Issuer         string    `bun:"issuer,notnull,type:varchar(255)"` // canton party ID
    Decimals       int       `bun:"decimals,notnull,default:10"`
    CreatedAt      time.Time `bun:"created_at,nullzero,default:current_timestamp"`
}

// TransferEventDao is an immutable record of a single mint / burn / transfer.
type TransferEventDao struct {
    bun.BaseModel  `bun:"table:transfer_events"`
    ID             int64     `bun:"id,pk,autoincrement"`
    EventType      string    `bun:"event_type,notnull,type:varchar(10)"`  // MINT|BURN|TRANSFER
    FromParty      string    `bun:"from_party,type:varchar(255)"`         // empty for mints
    ToParty        string    `bun:"to_party,type:varchar(255)"`           // empty for burns
    Amount         string    `bun:"amount,notnull,type:numeric(38,18)"`
    TokenSymbol    string    `bun:"token_symbol,notnull,type:varchar(50)"`
    LedgerOffset   string    `bun:"ledger_offset,notnull,type:varchar(255)"`
    TransactionID  string    `bun:"transaction_id,notnull,type:varchar(255)"`
    Timestamp      time.Time `bun:"timestamp,notnull"`
    EvmTxHash      string    `bun:"evm_tx_hash,type:varchar(66)"`
    EvmDestination string    `bun:"evm_destination,type:varchar(42)"`
    UserFingerprint string   `bun:"user_fingerprint,type:varchar(128)"`
}

// TokenBalanceDao stores the current balance for a (party, token) pair.
// NOTE: No evm_address — the indexer speaks canton_party_id exclusively.
type TokenBalanceDao struct {
    bun.BaseModel `bun:"table:token_balances"`
    PartyID       string    `bun:"party_id,pk,type:varchar(255)"`
    TokenSymbol   string    `bun:"token_symbol,pk,type:varchar(50)"`
    Balance       string    `bun:"balance,notnull,default:'0',type:numeric(38,18)"`
    UpdatedAt     time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
}

// TokenStatDao holds aggregate statistics per token (totalSupply, circulatingSupply).
type TokenStatDao struct {
    bun.BaseModel       `bun:"table:token_stats"`
    TokenSymbol         string    `bun:"token_symbol,pk,type:varchar(50)"`
    TotalSupply         string    `bun:"total_supply,notnull,default:'0',type:numeric(38,18)"`
    CirculatingSupply   string    `bun:"circulating_supply,notnull,default:'0',type:numeric(38,18)"`
    UpdatedAt           time.Time `bun:"updated_at,nullzero,default:current_timestamp"`
}
```

---

## 7. Store Interface & PostgreSQL Implementation

### pkg/indexer/store/store.go

```go
package store

import (
    "context"
    "math/big"
)

// TransferEventFilter is used to filter transfer_events queries.
type TransferEventFilter struct {
    PartyID     string // required for user queries; empty for admin queries
    TokenSymbol string // optional
    Limit       int
    Offset      int
}

// Tx is a transactional subset of Store for use inside RunInTx.
type Tx interface {
    InsertTransferEvent(ctx context.Context, dao *TransferEventDao) error
    AdjustBalance(ctx context.Context, partyID, tokenSymbol string, delta *big.Int, isMint bool) error
    UpdateTokenStats(ctx context.Context, e *TokenStatUpdate) error
    SetCheckpoint(ctx context.Context, offset string) error
}

// Store is the primary data access interface for the indexer.
// It deliberately has NO userstore dependency.
// All queries are keyed by canton_party_id, not evm_address.
type Store interface {
    // GetCheckpoint returns the last processed ledger offset.
    GetCheckpoint(ctx context.Context) (*LedgerCheckpointDao, error)

    // GetTokenBalance returns the balance for a specific (party, token) pair.
    GetTokenBalance(ctx context.Context, partyID, tokenSymbol string) (*TokenBalanceDao, error)

    // GetTokenStat returns aggregate statistics for a token.
    GetTokenStat(ctx context.Context, tokenSymbol string) (*TokenStatDao, error)

    // ListIndexedTokens returns all tracked tokens.
    ListIndexedTokens(ctx context.Context) ([]*IndexedTokenDao, error)

    // ListTransferEvents returns a paginated list of transfer events matching the filter.
    // Returns events and total count (for pagination).
    ListTransferEvents(ctx context.Context, filter TransferEventFilter) ([]*TransferEventDao, int, error)

    // UpsertIndexedToken registers or updates a tracked token.
    UpsertIndexedToken(ctx context.Context, dao *IndexedTokenDao) error

    // RunInTx executes fn inside a single database transaction.
    RunInTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}
```

### pkg/indexer/store/pg/pg.go (skeleton)

```go
package pg

import (
    "context"
    "database/sql"
    "math/big"

    "github.com/chainsafe/canton-middleware/pkg/indexer/store"
    "github.com/uptrace/bun"
)

type Store struct {
    db *bun.DB
}

func New(db *bun.DB) *Store {
    return &Store{db: db}
}

func (s *Store) GetCheckpoint(ctx context.Context) (*store.LedgerCheckpointDao, error) {
    dao := &store.LedgerCheckpointDao{}
    err := s.db.NewSelect().Model(dao).Where("id = 1").Scan(ctx)
    if err == sql.ErrNoRows {
        // First run — no checkpoint yet, start from offset 0
        return &store.LedgerCheckpointDao{ID: 1, Offset: "0"}, nil
    }
    return dao, err
}

func (s *Store) GetTokenBalance(ctx context.Context, partyID, tokenSymbol string) (*store.TokenBalanceDao, error) {
    dao := &store.TokenBalanceDao{}
    err := s.db.NewSelect().Model(dao).
        Where("party_id = ? AND token_symbol = ?", partyID, tokenSymbol).
        Scan(ctx)
    if err == sql.ErrNoRows {
        // No balance yet — return zero
        return &store.TokenBalanceDao{PartyID: partyID, TokenSymbol: tokenSymbol, Balance: "0"}, nil
    }
    return dao, err
}

func (s *Store) RunInTx(ctx context.Context, fn func(ctx context.Context, tx store.Tx) error) error {
    return s.db.RunInTx(ctx, nil, func(ctx context.Context, bunTx bun.Tx) error {
        return fn(ctx, &pgTx{tx: bunTx})
    })
}

// ... remaining methods omitted for brevity
```

---

## 8. Service Layer

### pkg/indexer/service/service.go

```go
package service

import (
    "context"
    "math/big"

    "github.com/chainsafe/canton-middleware/pkg/indexer/store"
)

// Service implements business logic for indexer queries.
// It deliberately has no userstore, no EVM dependency.
type Service struct {
    store store.Store
}

func New(s store.Store) *Service {
    return &Service{store: s}
}

type BalanceResponse struct {
    PartyID     string `json:"party_id"`
    TokenSymbol string `json:"token_symbol"`
    Balance     string `json:"balance"`
    Decimals    int    `json:"decimals"`
}

type SupplyResponse struct {
    TokenSymbol       string `json:"token_symbol"`
    TotalSupply       string `json:"total_supply"`
    CirculatingSupply string `json:"circulating_supply"`
}

type TransferListResponse struct {
    Events []*TransferEventResponse `json:"events"`
    Total  int                      `json:"total"`
}

// GetBalance returns the token balance for a specific party.
func (s *Service) GetBalance(ctx context.Context, partyID, tokenSymbol string) (*BalanceResponse, error) {
    dao, err := s.store.GetTokenBalance(ctx, partyID, tokenSymbol)
    if err != nil {
        return nil, err
    }
    return &BalanceResponse{
        PartyID:     dao.PartyID,
        TokenSymbol: dao.TokenSymbol,
        Balance:     dao.Balance,
    }, nil
}

// GetSupply returns total and circulating supply for a token. Public — no auth required.
func (s *Service) GetSupply(ctx context.Context, tokenSymbol string) (*SupplyResponse, error) {
    dao, err := s.store.GetTokenStat(ctx, tokenSymbol)
    if err != nil {
        return nil, err
    }
    return &SupplyResponse{
        TokenSymbol:       dao.TokenSymbol,
        TotalSupply:       dao.TotalSupply,
        CirculatingSupply: dao.CirculatingSupply,
    }, nil
}

// ListTransfers returns paginated transfer events for a party. Requires auth.
func (s *Service) ListTransfers(ctx context.Context, partyID, tokenSymbol string, limit, offset int) (*TransferListResponse, error) {
    events, total, err := s.store.ListTransferEvents(ctx, store.TransferEventFilter{
        PartyID:     partyID,
        TokenSymbol: tokenSymbol,
        Limit:       limit,
        Offset:      offset,
    })
    if err != nil {
        return nil, err
    }
    // map DAOs to response structs ...
    return &TransferListResponse{Total: total}, nil
}
```

---

## 9. API/HTTP Layer

### pkg/indexer/api/http/server.go

```go
package http

import (
    "net/http"

    "github.com/chainsafe/canton-middleware/pkg/indexer/service"
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
    svc    *service.Service
    router *chi.Mux
    cfg    Config
}

func New(svc *service.Service, cfg Config) *Server {
    s := &Server{svc: svc, cfg: cfg}
    s.router = chi.NewRouter()
    s.router.Use(middleware.Logger)
    s.router.Use(middleware.Recoverer)

    // Public routes — no auth
    s.router.Get("/v1/tokens", s.listTokens)
    s.router.Get("/v1/tokens/{symbol}/supply", s.getSupply)

    // Protected routes — JWT required
    s.router.Group(func(r chi.Router) {
        r.Use(authMiddleware(cfg.Auth))
        r.Get("/v1/tokens/{symbol}/balance/{partyID}", s.getBalance)
        r.Get("/v1/tokens/{symbol}/transfers/{partyID}", s.listTransfers)
    })

    return s
}

func (s *Server) getBalance(w http.ResponseWriter, r *http.Request) {
    // extract caller party from JWT claims context
    caller := r.Context().Value(principalKey{}).(string)
    partyID := chi.URLParam(r, "partyID")

    // enforce: caller can only query their own balance
    if caller != partyID {
        writeError(w, http.StatusForbidden, errors.New("forbidden: cannot query another party's balance"))
        return
    }

    resp, err := s.svc.GetBalance(r.Context(), partyID, chi.URLParam(r, "symbol"))
    if err != nil {
        writeError(w, http.StatusInternalServerError, err)
        return
    }
    writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getSupply(w http.ResponseWriter, r *http.Request) {
    resp, err := s.svc.GetSupply(r.Context(), chi.URLParam(r, "symbol"))
    if err != nil {
        writeError(w, http.StatusInternalServerError, err)
        return
    }
    writeJSON(w, http.StatusOK, resp)
}
```

### Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/tokens` | — | List all indexed tokens |
| GET | `/v1/tokens/{symbol}/supply` | — | Total + circulating supply |
| GET | `/v1/tokens/{symbol}/balance/{partyID}` | JWT | Party's balance (caller = partyID) |
| GET | `/v1/tokens/{symbol}/transfers/{partyID}` | JWT | Transfer history for party |
| GET | `/healthz` | — | Liveness probe |

> **GraphQL:** Leave room in the `Server` struct for a `/graphql` handler to be wired in Phase 2. The service layer needs no changes.

---

## 10. Authentication (JWT-only)

The indexer has **no userstore dependency**. Auth is purely JWT-based.

The api-server:
1. Verifies the user's EVM signature (existing `pkg/auth/evm.go`)
2. Looks up the user's `canton_party_id` via userstore
3. Issues a JWT containing `canton_party_id` as a claim
4. Client passes that JWT to the indexer

The indexer:
1. Extracts the Bearer token from `Authorization` header
2. Validates it against the shared JWKS endpoint
3. Extracts `canton_party_id` from claims
4. Uses that party ID to scope DB queries

```go
// pkg/indexer/api/http/auth.go

type Claims struct {
    CantonPartyID string `json:"canton_party_id"`
    jwt.RegisteredClaims
}

type principalKey struct{}

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

func validateJWT(token, jwksURL string) (*Claims, error) {
    // Fetch JWKS and validate signature. Use golang-jwt/jwt + JWKS HTTP fetch.
    // Cache the JWKS with a 5-minute TTL.
    // ...
}
```

---

## 11. File Layout

```
canton-middleware/
├── cmd/
│   └── indexer/
│       └── main.go                        # binary entry point
│
├── pkg/
│   ├── cantonsdk/
│   │   └── streaming/
│   │       └── stream.go                  # NEW — generic ledger stream
│   │
│   ├── indexer/
│   │   ├── fetcher/
│   │   │   └── fetcher.go                 # subscribes to Canton, drives processor
│   │   ├── parser/
│   │   │   └── parser.go                  # decodes DAML records → ParsedEvent
│   │   ├── processor/
│   │   │   └── processor.go               # applies events to DB atomically
│   │   ├── store/
│   │   │   ├── models.go                  # DAO structs (no evm_address on token_balances)
│   │   │   ├── store.go                   # Store interface
│   │   │   └── pg/
│   │   │       └── pg.go                  # PostgreSQL implementation (Bun)
│   │   ├── service/
│   │   │   └── service.go                 # business logic, no userstore dep
│   │   └── api/
│   │       └── http/
│   │           ├── server.go              # chi router, handler funcs
│   │           ├── auth.go                # JWT middleware
│   │           └── helpers.go             # writeJSON, writeError
│   │
│   └── migrations/
│       └── indexerdb/
│           ├── service.go                 # var Migrations = migrate.NewMigrations()
│           ├── 1_create_ledger_checkpoints.go
│           ├── 2_create_indexed_tokens.go
│           ├── 3_create_transfer_events.go
│           ├── 4_create_token_balances.go
│           └── 5_create_token_stats.go
│
└── daml/
    └── TokenTransferEvent.daml            # NEW template (additive)
```

---

## 12. cmd/indexer Entry Point

```go
// cmd/indexer/main.go
package main

import (
    "context"
    "log"
    "os/signal"
    "syscall"

    "github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
    "github.com/chainsafe/canton-middleware/pkg/db"
    "github.com/chainsafe/canton-middleware/pkg/indexer/api/http"
    "github.com/chainsafe/canton-middleware/pkg/indexer/fetcher"
    "github.com/chainsafe/canton-middleware/pkg/indexer/processor"
    "github.com/chainsafe/canton-middleware/pkg/indexer/service"
    "github.com/chainsafe/canton-middleware/pkg/indexer/store/pg"
    "github.com/chainsafe/canton-middleware/pkg/migrations/indexerdb"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    cfg := loadConfig()

    // Database
    bunDB, err := db.Connect(cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("db connect: %v", err)
    }
    if err := db.RunMigrations(ctx, bunDB, indexerdb.Migrations); err != nil {
        log.Fatalf("migrations: %v", err)
    }

    // Store, service, HTTP server
    indexerStore := pg.New(bunDB)
    svc := service.New(indexerStore)
    apiServer := http.New(svc, cfg.HTTP)

    // Canton ledger client (reuses existing cantonsdk/ledger)
    cantonLedger, err := ledger.New(cfg.Canton)
    if err != nil {
        log.Fatalf("ledger client: %v", err)
    }

    // Processor + fetcher
    proc := processor.New(indexerStore)
    f := fetcher.New(cantonLedger, indexerStore, cfg.Fetcher)

    // Run all components
    eg, ctx := errgroup.WithContext(ctx)
    eg.Go(func() error { return apiServer.ListenAndServe(cfg.HTTP.Addr) })
    eg.Go(func() error { return f.Run(ctx, proc) })

    if err := eg.Wait(); err != nil {
        log.Printf("indexer stopped: %v", err)
    }
}
```

---

## 13. Configuration

```go
// pkg/indexer/config.go
package indexer

type Config struct {
    DatabaseURL string

    Canton struct {
        GRPCAddr  string
        OAuthURL  string
        ClientID  string
        ClientSecret string
        PartyID   string // the indexer's own Canton party ID (observer on TokenTransferEvent)
    }

    Fetcher struct {
        TokenPackageID string   // package ID of the CIP-56 token package
        TokenSymbols   []string // ["DEMO", "PROMPT"] — which tokens to index
    }

    HTTP struct {
        Addr string
        Auth struct {
            JWKSUrl string // shared with api-server
        }
    }
}
```

---

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Canton streaming | Extend `cantonsdk` with a generic `streaming.Stream[T]` | Reuses existing reconnect pattern from `bridge/client.go` |
| Transfer detection | New `TokenTransferEvent` DAML template | Single subscription, cleaner than inferring from ACS diffs |
| Identity | `canton_party_id` only (no `evm_address`) | Indexer is Canton-native; api-server handles EVM↔party translation |
| Auth | JWT-only (`canton_party_id` claim) | No userstore dependency; api-server issues JWTs after EVM sig verify |
| ORM | Bun (same as rest of project) | Consistency, existing migration helpers (`mghelper`) |
| HTTP framework | `go-chi/chi` (same as api-server) | Consistency |
| Phase 2 | GraphQL via `/graphql` handler | Service layer unchanged; just add a new handler |
| Out of scope | Canton Coin, WebSocket push, realtime subscriptions | Phase 2 |
