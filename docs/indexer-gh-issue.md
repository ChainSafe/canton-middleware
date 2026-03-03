# Feature: Distributed Indexer for CIP-56 Tokens (CIP-0086)

## Summary

Implement a standalone `indexer` service that streams events from the Canton Ledger API and maintains a queryable, off-chain projection of CIP-56 token activity — balances, transfer history, and supply statistics — exposed via a REST API.

Full design doc: [`docs/indexer-design.md`](./indexer-design.md)

---

## Motivation

The current api-server (`pkg/ethrpc`) answers `eth_getBalance` / `eth_call` requests by querying Canton directly on every call. This works for low-traffic scenarios but doesn't scale:

- Each RPC call requires a Canton gRPC round-trip
- No history/events are queryable
- CIP-0086 requires a dedicated indexer HTTP API

---

## Key Design Decisions

1. **Unified `TokenTransferEvent` DAML template** — New additive template emitted on MINT / BURN / TRANSFER. Single subscription covers all cases. `fromParty = None` for mints, `toParty = None` for burns.

2. **`cantonsdk/streaming` package** — Generic `Stream[T]` wrapper over `GetUpdates` gRPC with exponential back-off reconnect. Extracted from the existing `bridge/client.go` `StreamWithdrawalEvents` pattern.

3. **Canton-native identity** — `canton_party_id` is the primary key everywhere. No `evm_address` in the indexer DB. The api-server acts as the EVM→party translator.

4. **JWT-only auth** — No `userstore` dependency. The indexer validates a JWT (issued by api-server) and extracts the `canton_party_id` claim. Total supply endpoints are public (ERC-20 spec).

5. **Independent binary** — `cmd/indexer` runs separately from `cmd/api-server`. Has its own DB (separate migration package `pkg/migrations/indexerdb/`). Can be scaled independently.

---

## New Packages

| Package | Description |
|---------|-------------|
| `pkg/cantonsdk/streaming` | Generic Canton ledger stream with reconnect |
| `pkg/indexer/fetcher` | Manages ledger subscription + checkpoint |
| `pkg/indexer/parser` | Decodes DAML records → `ParsedEvent` |
| `pkg/indexer/processor` | Applies events to DB atomically |
| `pkg/indexer/store` | Store interface + PostgreSQL (Bun) impl |
| `pkg/indexer/service` | Business logic for queries |
| `pkg/indexer/api/http` | chi-based HTTP handlers |
| `pkg/migrations/indexerdb` | Bun migrations for the indexer DB |
| `cmd/indexer` | Binary entry point |

---

## Database Tables

| Table | Primary Key | Description |
|-------|-------------|-------------|
| `ledger_checkpoints` | `id` | Last processed ledger offset (single row) |
| `indexed_tokens` | `symbol` | Tracked CIP-56 tokens (DEMO, PROMPT) |
| `transfer_events` | `id` | Immutable event log (MINT/BURN/TRANSFER) |
| `token_balances` | `(party_id, token_symbol)` | Current balance per party |
| `token_stats` | `token_symbol` | `total_supply`, `circulating_supply` |

> `token_balances` has **no `evm_address` column** — indexer is Canton-native.

---

## HTTP Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/tokens` | — | List indexed tokens |
| GET | `/v1/tokens/{symbol}/supply` | — | Total + circulating supply |
| GET | `/v1/tokens/{symbol}/balance/{partyID}` | JWT (`canton_party_id` = partyID) | Party balance |
| GET | `/v1/tokens/{symbol}/transfers/{partyID}` | JWT | Transfer history |
| GET | `/healthz` | — | Liveness probe |

---

## DAML Change Required

Add `TokenTransferEvent` template to the CIP-56 token package (additive, no breaking changes):

```daml
template TokenTransferEvent
  with
    issuer        : Party
    fromParty     : Optional Party   -- None for mints
    toParty       : Optional Party   -- None for burns
    amount        : Decimal
    tokenSymbol   : Text
    eventType     : Text             -- "MINT" | "BURN" | "TRANSFER"
    timestamp     : Time
    evmTxHash     : Optional Text
    auditObservers : [Party]
  where
    signatory issuer
    observer optional [] (\p -> [p]) fromParty,
             optional [] (\p -> [p]) toParty,
             auditObservers
```

---

## Migration Path

1. Add `TokenTransferEvent` to DAML contracts + redeploy
2. Implement `pkg/cantonsdk/streaming`
3. Implement `pkg/indexer/` packages + migrations
4. Add `cmd/indexer` binary + config
5. Wire into Docker Compose (Phase 1: single instance)
6. Update api-server to issue JWTs containing `canton_party_id`

---

## Out of Scope (Phase 1)

- Canton Coin indexing
- WebSocket push subscriptions (indexer → client)
- GraphQL API (planned for Phase 2 — service layer is ready)
- Multi-node / sharded indexer
