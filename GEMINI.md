# Gemini Context & Instructions

## Project Overview
Canton-Ethereum Bridge is a centralized relayer connecting Canton Network (CIP-56) and Ethereum (ERC-20).
- **Core Logic**: `pkg/relayer`
- **Canton Client**: `pkg/canton` (gRPC)
- **Ethereum Client**: `pkg/ethereum` (go-ethereum)

## Critical Conventions

### 1. Protobuf Imports
- **V2 API**: ALWAYS import `github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2` with alias `lapiv2`.
- **Reason**: Avoids naming collisions and ensures clarity between API versions.

### 2. Relayer Architecture
- **Processor Pattern**: The core pattern for bidirectional syncing (`pkg/relayer/processor.go`). Do not create separate "CantonProcessor" or "EthereumProcessor" types; use the generic `Processor` struct with `Source` and `Destination` interfaces.
- **Source/Destination Adapters**: Use `CantonSource`, `EthereumSource`, `CantonDestination`, `EthereumDestination` in `handlers.go` to adapt chain-specific logic to the generic interfaces.
- **State Management**: All state changes must go through `BridgeStore` (`pkg/db`).

### 3. Testing
- **Unit Tests**: Run with `go test ./...`.
- **Mocks**: Use `pkg/relayer/mocks_test.go` for relayer tests. Do not create ad-hoc mocks if possible.

## Documentation Map
- **Architecture**: `docs/architecture_design.md`
- **Canton Details**: `docs/canton-integration.md`
- **Relayer Logic**: `docs/relayer-logic.md`
- **Next Steps**: `docs/next_steps.md`
