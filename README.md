# Canton-Ethereum Token Bridge

A centralized token bridge connecting CIP-56 tokens on Canton Network with ERC-20 tokens on Ethereum Mainnet.

## Overview

This bridge enables bidirectional token transfers between Canton Network and Ethereum Mainnet through a single relayer node that runs as a sidecar to a Canton Network Partner Node.

## Architecture

- **Canton Bridge Contract**: Manages CIP-56 token locks/unlocks and wrapped token minting/burning
- **Ethereum Bridge Contract**: Manages ERC-20 token locks/unlocks and wrapped token minting/burning  
- **Relayer Node**: Go-based service that monitors both chains and orchestrates cross-chain transfers

## Project Structure

```
canton-middleware/
├── cmd/
│   └── relayer/              # Main relayer application
├── contracts/
│   ├── ethereum/             # Solidity contracts
│   └── canton-erc20/         # Canton DAML contracts
├── pkg/
│   ├── canton/               # Canton client and utilities
│   │   └── lapi/             # Generated Ledger API protobufs (v2)
│   ├── ethereum/             # Ethereum client and utilities
│   ├── relayer/              # Core relayer logic
│   ├── db/                   # Database models and queries
│   └── config/               # Configuration handling
├── scripts/
│   ├── test-bridge.sh        # Automated end-to-end testing
│   ├── bootstrap-bridge.go   # Bootstrap Canton contracts
│   ├── register-user.go      # Register user fingerprint mapping
│   ├── query-holdings.go     # Query Canton holdings
│   └── initiate-withdrawal.go # Initiate Canton→EVM withdrawal
├── internal/
│   └── metrics/              # Prometheus metrics
└── deployments/              # Docker, k8s configs
```

## Development

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Foundry (forge, cast, anvil)
- Canton Docker image (see [Local Testing](#local-testing))

### Setup

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Build relayer
go build -o bin/relayer ./cmd/relayer
```

### Configuration

See `config.example.yaml` for detailed configuration options.

## Local Testing

For local development and testing, the bridge can be run entirely in Docker using a local Canton node and Anvil (local Ethereum).

### Prerequisites

1. **Build the Canton Docker image** from [ChainSafe/canton-docker](https://github.com/ChainSafe/canton-docker):

```bash
git clone https://github.com/ChainSafe/canton-docker.git
cd canton-docker
./build_container.sh
```

2. **Install Foundry** (for `cast` CLI):

```bash
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

### Running Tests

```bash
# Full test with clean environment (recommended for first run)
./scripts/test-bridge-local.sh --clean

# Skip Docker setup (if services are already running)
./scripts/test-bridge-local.sh --skip-docker
```

The test script will:
- Start Docker services (Canton, Anvil, PostgreSQL)
- Deploy and verify contracts
- Bootstrap the Canton bridge
- Register a test user
- Start the relayer
- Execute deposit (EVM→Canton) and withdrawal (Canton→EVM) flows
- Print a summary of results

For detailed step-by-step instructions, see [Bridge Testing Guide](docs/BRIDGE_TESTING_GUIDE.md).

> **Note**: Local testing uses Docker containers. Production deployments will connect to live Canton Network Partner Nodes.

## Documentation

See the [docs](docs/) directory for detailed documentation:
- [Bridge Testing Guide](docs/BRIDGE_TESTING_GUIDE.md) - Local testing with Docker
- [Testnet Migration Guide](docs/TESTNET_MIGRATION_GUIDE.md) - Moving to Canton Network testnet
- [Architecture Design](docs/architecture_design.md)
- [Relayer Logic](docs/relayer-logic.md)
- [Canton Integration](docs/canton-integration.md)
