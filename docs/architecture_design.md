# Architecture Design

## Component Interaction Sequence Diagram

This diagram illustrates the interaction between the main components of the Canton-Ethereum Bridge Relayer, including the initialization, event streaming loops, and the core processing logic.

```mermaid
sequenceDiagram
    participant Main
    participant Engine
    participant Store as Database
    participant C2E as TransferProcessor (Canton->Eth)
    participant E2C as TransferProcessor (Eth->Canton)
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

    Engine->>C2E: NewTransferProcessor(CantonSource, EthereumDestination)
    Engine->>E2C: NewTransferProcessor(EthereumSource, CantonDestination)

    par Start Canton->Eth Processor
        Engine->>C2E: Start(offset)
        activate C2E
        C2E->>CS: StreamEvents(offset)
        activate CS
        loop Every Event
            CS-->>C2E: Event (DepositRequest)
            C2E->>Store: GetTransfer(id)
            alt Not Processed
                C2E->>Store: CreateTransfer(Pending)
                C2E->>ED: SubmitTransfer(event)
                activate ED
                ED-->>C2E: txHash
                deactivate ED
                C2E->>Store: UpdateTransferStatus(Completed)
            end
        end
        deactivate CS
        deactivate C2E
    and Start Eth->Canton Processor
        Engine->>E2C: Start(offset)
        activate E2C
        E2C->>ES: StreamEvents(offset)
        activate ES
        loop Every Event
            ES-->>E2C: Event (Lock/Burn)
            E2C->>Store: GetTransfer(id)
            alt Not Processed
                E2C->>Store: CreateTransfer(Pending)
                E2C->>CD: SubmitTransfer(event)
                activate CD
                CD-->>E2C: txHash
                deactivate CD
                E2C->>Store: UpdateTransferStatus(Completed)
            end
        end
        deactivate ES
        deactivate E2C
    and Start Reconciliation Loop
        Engine->>Engine: reconcile() (Loop 5m)
    end

    deactivate Engine
```

## Component Roles

*   **Main**: Entry point. Initializes configuration, database connection, clients, and the Engine. Starts the HTTP server for metrics/API.
*   **Engine**: Orchestrator. Manages the lifecycle of the application. It initializes the bidirectional processors and the reconciliation loop. It handles graceful shutdown.
*   **TransferProcessor**: The core worker. There are two instances: one for Canton->Ethereum and one for Ethereum->Canton. It uses a generic `Source` and `Destination` interface to abstract the logic of "Listen -> Persist -> Submit".
*   **Source (Interface)**: Abstraction for fetching events (e.g., `CantonSource` streams from Canton Ledger API).
*   **Destination (Interface)**: Abstraction for submitting transactions (e.g., `EthereumDestination` submits to a smart contract).
*   **Store**: Persistence layer (PostgreSQL) for tracking transfer state and chain offsets.
