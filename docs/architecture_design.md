# Architecture Design

## Component Interaction Sequence Diagram

This diagram illustrates the interaction between the main components of the Canton-Ethereum Bridge Relayer, including the initialization, event streaming loops, and the core processing logic.

```mermaid
sequenceDiagram
    participant Main
    participant Engine
    participant Store as Database
    participant CP as CantonProcessor
    participant EP as EthereumProcessor
    participant CS as CantonSource
    participant ED as EthereumDestination
    participant ES as EthereumSource
    participant CD as CantonDestination

    Note over Main: Application Start

    Main->>Store: Connect()
    Main->>Engine: NewEngine(config, clients, store)
    Main->>Engine: Start()

    activate Engine
    Engine->>Store: LoadOffsets()
    Store-->>Engine: Offsets (Canton & Eth)

    Note over Engine: Initialize Processors

    Engine->>CP: NewProcessor(CantonSource, EthereumDestination)
    Engine->>EP: NewProcessor(EthereumSource, CantonDestination)

    par Start Canton Processor
        Engine->>CP: Start(offset)
        activate CP
        CP->>CS: StreamEvents(offset)
        activate CS
        loop Every Event
            CS-->>CP: Event (DepositRequest)
            CP->>Store: GetTransfer(id)
            alt Not Processed
                CP->>Store: CreateTransfer(Pending)
                CP->>ED: SubmitTransfer(event)
                activate ED
                ED-->>CP: txHash
                deactivate ED
                CP->>Store: UpdateTransferStatus(Completed)
            end
        end
        deactivate CS
        deactivate CP
    and Start Ethereum Processor
        Engine->>EP: Start(offset)
        activate EP
        EP->>ES: StreamEvents(offset)
        activate ES
        loop Every Event
            ES-->>EP: Event (Lock/Burn)
            EP->>Store: GetTransfer(id)
            alt Not Processed
                EP->>Store: CreateTransfer(Pending)
                EP->>CD: SubmitTransfer(event)
                activate CD
                CD-->>EP: txHash
                deactivate CD
                EP->>Store: UpdateTransferStatus(Completed)
            end
        end
        deactivate ES
        deactivate EP
    and Start Reconciliation Loop
        Engine->>Engine: reconcile() (Loop 5m)
    end

    deactivate Engine
```

## Component Roles

*   **Main**: Entry point. Initializes configuration, database connection, clients, and the Engine. Starts the HTTP server for metrics/API.
*   **Engine**: Orchestrator. Manages the lifecycle of the application. It initializes the bidirectional processors and the reconciliation loop. It handles graceful shutdown.
*   **Processor**: The core worker. There are two instances: one for Canton->Ethereum and one for Ethereum->Canton. It abstracts the logic of "Listen -> Persist -> Submit".
*   **Source (Interface)**: Abstraction for fetching events (e.g., `CantonSource` streams from Canton Ledger API).
*   **Destination (Interface)**: Abstraction for submitting transactions (e.g., `EthereumDestination` submits to a smart contract).
*   **Store**: Persistence layer (PostgreSQL) for tracking transfer state and chain offsets.
