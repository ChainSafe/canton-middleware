# Relayer Logic & Design Principles

This document explains the core logic behind the Canton-Ethereum Relayer. It is intended for developers who want to understand *how* the relayer works under the hood, beyond just the component structure.

## Core Philosophy

The relayer is designed with the following principles:

1.  **Bidirectional & Independent**: The bridge operates as two independent one-way flows (Canton→Ethereum and Ethereum→Canton).
2.  **At-Least-Once Delivery**: The system guarantees that every event is processed at least once.
3.  **Idempotency**: The system relies on the database to ensure that events are not processed multiple times (deduplication).
4.  **Crash Recovery**: The system persists its progress (offsets) so it can resume exactly where it left off after a restart.

## The Processor Pattern

The heart of the relayer is the `TransferProcessor` struct (`pkg/relayer/processor.go`). The `Engine` initializes two instances of this processor, one for each direction, using generic `Source` and `Destination` interfaces.

Each processor abstracts the flow into three stages:

1.  **Source (Stream)**: Listen for events from the source chain.
2.  **Store (Persist)**: Record the event in the local database.
3.  **Destination (Submit)**: Submit the corresponding transaction to the destination chain.

### The Event Loop

The processor runs a continuous loop:

```go
for event := range sourceStream {
    // 1. Check Idempotency
    if store.HasProcessed(event.ID) {
        continue
    }

    // 2. Persist Pending State
    store.CreateTransfer(event, StatusPending)

    // 3. Execute Action
    txHash, err := destination.SubmitTransfer(event)

    // 4. Update State
    if err != nil {
        store.UpdateStatus(event.ID, StatusFailed)
    } else {
        store.UpdateStatus(event.ID, StatusCompleted, txHash)
    }
}
```

## State Management

### 1. Offsets (Checkpoints)
To ensure no events are missed, the relayer tracks its position on both chains.

*   **Canton**: Uses `LedgerOffset` (absolute string).
*   **Ethereum**: Uses `BlockNumber`.

On startup, the `Engine` loads the last successfully processed offset from the `chain_state` table.
*   If an offset exists, it resumes streaming from that point.
*   If no offset exists (first run), it starts from `BEGIN` (Canton) or a configured `StartBlock` (Ethereum).

**Crucially**, the offset is updated in the database *only after* a transfer is successfully processed. This ensures that if the relayer crashes mid-process, it will re-read the last unprocessed event upon restart.

### 2. Idempotency (Deduplication)
Because we might re-process events (e.g., after a crash or during reconciliation), the system must handle duplicate events gracefully.

*   **Event ID**: Every event has a unique ID.
    *   Canton→Eth: The Canton Event ID.
    *   Eth→Canton: A hash of `(TransactionHash, LogIndex)`.
*   **Check**: Before processing any event, the processor checks the `transfers` table. If a record with that ID exists, the event is skipped (or checked for retry).

## Reconciliation (Recovery)

Network failures or transaction reverts can cause transfers to get stuck in a `pending` or `failed` state. The `Engine` runs a periodic **Reconciliation Loop** (every 5 minutes) to handle these cases.

**Logic:**
1.  Query the database for all transfers with status `pending` or `failed`.
2.  For each stuck transfer:
    *   Check the status on the destination chain (did the tx actually confirm?).
    *   If confirmed, mark as `completed`.
    *   If failed/missing, re-submit the transaction (with a new nonce if needed).

*(Note: The current implementation logs stuck transfers. Automatic retry logic is the next planned feature.)*

## Error Handling Strategy

*   **Transient Errors** (e.g., RPC timeout): The stream will disconnect. The processor loop will exit, and the Engine (or a supervisor) will restart it, resuming from the last offset.
*   **Permanent Errors** (e.g., Invalid Data): The transfer is marked as `failed` in the DB. It requires manual intervention or the reconciliation loop to fix.

## Concurrency

*   The `Engine` uses `sync.WaitGroup` to manage the lifecycles of the two processors and the reconciliation loop.
*   Each processor runs in its own goroutine.
*   Database access is thread-safe (via `sql.DB` connection pooling).
