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
│   ├── relayer/           # Main relayer application
│   └── tools/             # CLI tools for management
├── contracts/
│   ├── ethereum/          # Solidity contracts
│   └── canton/            # Canton contracts
├── pkg/
│   ├── canton/            # Canton client and utilities
│   ├── ethereum/          # Ethereum client and utilities
│   ├── relayer/           # Core relayer logic
│   ├── db/                # Database models and queries
│   └── config/            # Configuration handling
├── internal/
│   ├── metrics/           # Prometheus metrics
│   └── security/          # Key management, signing
└── deployments/           # Docker, k8s configs
```

## Development

### Prerequisites

- Go 1.23+
- PostgreSQL 15+
- Access to Canton Network Partner Node
- Ethereum node access (Infura or similar)

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

See `docs/configuration.md` for detailed configuration options.

## Documentation

See the [docs](docs/) directory for detailed documentation:
- [Implementation Plan](BRIDGE_IMPLEMENTATION_PLAN.md)
- [Architecture](docs/architecture.md)
- [API Reference](docs/api.md)

## Security

This is a centralized bridge operated by a single relayer. Users must trust the relayer operator. See [Security Considerations](BRIDGE_IMPLEMENTATION_PLAN.md#security-considerations) for details.

## License

TBD
