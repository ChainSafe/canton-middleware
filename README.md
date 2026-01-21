# Canton-Ethereum Token Bridge

A token bridge connecting CIP-56 tokens on Canton Network with ERC-20 tokens on Ethereum.

## Quick Start (Local Testing)

```bash
# Clone with submodules
git clone --recursive https://github.com/ChainSafe/canton-middleware.git
cd canton-middleware

# Run the setup script (handles everything automatically)
./scripts/setup-local.sh
```

This will:
1. Check prerequisites (Docker, Go)
2. Initialize git submodules (canton-erc20 contracts)
3. Build DAML DARs (if DAML SDK is installed)
4. Start all Docker services
5. Run the end-to-end test

### Prerequisites

- **Docker** and **Docker Compose** (required)
- **Go 1.23+** (required)
- **DAML SDK** (optional - only needed to rebuild DARs)
- **Foundry** (optional - for direct Ethereum interactions)

### Test with MetaMask

After setup, you can test transfers using MetaMask:

```bash
./scripts/metamask-test.sh
```

This walks you through:
- Adding the local Canton network to MetaMask
- Importing test accounts
- Transferring PROMPT and DEMO tokens

### Service Endpoints (Local)

| Service | URL |
|---------|-----|
| Anvil (Ethereum) | http://localhost:8545 |
| API Server | http://localhost:8081 |
| Relayer | http://localhost:8080 |
| Canton HTTP | http://localhost:5013 |
| Canton gRPC | localhost:5011 |

### Test Accounts

Using Anvil's default mnemonic (`test test test test test test test test test test test junk`):

| Account | Address | Private Key |
|---------|---------|-------------|
| User 1 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` |
| User 2 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d` |

### Token Addresses

| Token | Address | Description |
|-------|---------|-------------|
| PROMPT | `0x5FbDB2315678afecb367f032d93F642f64180aa3` | Bridged ERC-20 token |
| DEMO | `0xDE30000000000000000000000000000000000001` | Native Canton token |

---

## Overview

This bridge enables bidirectional token transfers between Canton Network and Ethereum through a relayer node that runs as a sidecar to a Canton Network Partner Node.

## Architecture

- **Canton Bridge Contract**: Manages CIP-56 token locks/unlocks and wrapped token minting/burning
- **Ethereum Bridge Contract**: Manages ERC-20 token locks/unlocks and wrapped token minting/burning  
- **Relayer Node**: Go-based service that monitors both chains and orchestrates cross-chain transfers
- **API Server**: EVM JSON-RPC facade for MetaMask compatibility

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

- Go 1.23+
- Docker & Docker Compose
- DAML SDK (optional, for rebuilding contracts)
- Foundry (optional, for direct Ethereum interactions)

### Setup

```bash
# Clone with submodules
git clone --recursive https://github.com/ChainSafe/canton-middleware.git
cd canton-middleware

# Install Go dependencies
go mod download

# Run unit tests
go test ./...

# Build binaries
go build -o bin/relayer ./cmd/relayer
go build -o bin/api-server ./cmd/api-server
```

### Configuration

See `config.example.yaml` for detailed configuration options.

## Local Testing

### Quick Start

```bash
# Full automated setup and test
./scripts/setup-local.sh

# Or setup only (no tests)
./scripts/setup-local.sh --setup-only

# Clean and rebuild everything
./scripts/setup-local.sh --clean
```

### Manual Testing

```bash
# Start services
docker compose up -d --build

# Run E2E test
./scripts/e2e-local.sh

# Test with MetaMask
./scripts/metamask-test.sh
```

### Available Scripts

| Script | Description |
|--------|-------------|
| `scripts/setup-local.sh` | Full automated setup (submodules, DARs, Docker, tests) |
| `scripts/e2e-local.sh` | End-to-end test (deposit, transfer, DEMO token) |
| `scripts/metamask-test.sh` | Interactive MetaMask testing guide |
| `scripts/test-bridge.sh` | Interactive bridge testing menu |

For detailed instructions, see [Local Testing Guide](docs/LOCAL_TESTING_GUIDE.md).

## Documentation

See the [docs](docs/) directory for detailed documentation:
- [Bridge Testing Guide](docs/BRIDGE_TESTING_GUIDE.md) - Local testing with Docker
- [Testnet Migration Guide](docs/TESTNET_MIGRATION_GUIDE.md) - Moving to Canton Network testnet
- [Architecture Design](docs/architecture_design.md)
- [Relayer Logic](docs/relayer-logic.md)
- [Canton Integration](docs/canton-integration.md)
