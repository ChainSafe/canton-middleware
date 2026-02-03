# Canton Middleware

MetaMask-compatible middleware for Canton Network, enabling ERC-20 style interactions with CIP-56 tokens.

## Overview

This project provides:
- **API Server**: Ethereum JSON-RPC facade that translates MetaMask transactions to Canton CIP-56 operations
- **Relayer**: Bridges PROMPT tokens between Ethereum and Canton
- **Interoperability**: Native Canton users and MetaMask users can seamlessly transfer tokens to each other

## Quick Start (Local Development)

```bash
# Clone the repository
git clone https://github.com/ChainSafe/canton-middleware.git
cd canton-middleware

# Run the bootstrap script (handles everything)
./scripts/setup/bootstrap-all.sh
```

This will:
1. Generate encryption keys (`CANTON_MASTER_KEY`)
2. Start Docker services (Canton, Anvil, PostgreSQL, API Server, Relayer)
3. Wait for healthy status
4. Register test users
5. Mint DEMO tokens (500 each) and deposit PROMPT tokens

### Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.23+ | https://go.dev/dl/ |
| Docker | Latest | https://docs.docker.com/get-docker/ |
| Docker Compose | v2+ | (included with Docker Desktop) |
| Foundry | Latest | `curl -L https://foundry.paradigm.xyz \| bash && foundryup` |

### MetaMask Configuration

After bootstrap completes, configure MetaMask:

| Setting | Value |
|---------|-------|
| Network Name | Canton Local |
| RPC URL | `http://localhost:8081/eth` |
| Chain ID | 31337 |
| Currency Symbol | ETH |

**Test Accounts (Anvil defaults):**

| User | Address | Private Key |
|------|---------|-------------|
| User 1 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` |
| User 2 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d` |

**Token Addresses:**

| Token | Address | Type |
|-------|---------|------|
| DEMO | `0xDE30000000000000000000000000000000000001` | Native Canton token |
| PROMPT | `0x5FbDB2315678afecb367f032d93F642f64180aa3` | Bridged ERC-20 |

### Service Endpoints

| Service | URL |
|---------|-----|
| API Server (MetaMask RPC) | http://localhost:8081/eth |
| API Server Health | http://localhost:8081/health |
| Canton gRPC | localhost:5011 |
| Canton HTTP | http://localhost:5013 |
| Anvil (Ethereum) | http://localhost:8545 |
| PostgreSQL | localhost:5432 |

---

## Architecture

```
┌─────────────────┐                    ┌─────────────────┐
│    MetaMask     │                    │  Native Canton  │
│   (EVM Wallet)  │                    │   User / CLI    │
└────────┬────────┘                    └────────┬────────┘
         │ JSON-RPC                             │ gRPC
         ▼                                      │
┌─────────────────────────────────────────────────────────┐
│                    API SERVER                            │
│  • /eth - JSON-RPC facade (eth_call, eth_sendRawTx)     │
│  • /register - User registration                         │
│  • Custodial Canton key management                       │
│  • Balance reconciliation with Canton                    │
└─────────────────────────┬───────────────────────────────┘
                          │
         ┌────────────────┼────────────────┐
         │                │                │
         ▼                ▼                ▼
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  PostgreSQL │   │   Canton    │   │   Relayer   │
│  (Cache)    │   │  (CIP-56)   │   │  (Bridge)   │
└─────────────┘   └─────────────┘   └──────┬──────┘
                                           │
                                    ┌──────▼──────┐
                                    │  Ethereum   │
                                    │  (ERC-20)   │
                                    └─────────────┘
```

**Key Components:**
- **API Server** - Translates ERC-20 calls to CIP-56 DAML operations
- **Relayer** - Bridges PROMPT token between Ethereum ↔ Canton
- **PostgreSQL** - Caches user data and balances for fast queries
- **Canton Ledger** - Source of truth for all token balances

---

## Project Structure

```
canton-middleware/
├── cmd/
│   ├── api-server/           # API server entry point
│   └── relayer/              # Relayer entry point
├── pkg/
│   ├── apidb/                # Database operations
│   ├── auth/                 # JWT and EVM authentication
│   ├── canton/               # Canton client (gRPC)
│   │   └── lapi/v2/          # Generated Ledger API protobufs
│   ├── config/               # Configuration handling
│   ├── db/                   # Database schema
│   ├── ethereum/             # Ethereum client
│   ├── ethrpc/               # JSON-RPC server
│   ├── keys/                 # Key management (secp256k1)
│   ├── registration/         # User registration
│   └── service/              # Token service
├── scripts/
│   ├── setup/                # Bootstrap and setup scripts
│   ├── testing/              # Test and demo scripts
│   ├── bridge/               # Bridge operation scripts
│   ├── utils/                # Diagnostic utilities
│   ├── lib/                  # Shared bash functions
│   └── archive/              # Migration scripts (reference)
├── contracts/
│   ├── canton-erc20/         # DAML contracts (CIP-56)
│   └── ethereum-wayfinder/   # Solidity contracts
├── docs/                     # Documentation
└── deployments/              # Docker, deployment configs
```

---

## Scripts

### Setup (`scripts/setup/`)

| Script | Description |
|--------|-------------|
| `bootstrap-all.sh` | Full automated setup (recommended) |
| `bootstrap-bridge.go` | Bootstrap bridge contracts on Canton |
| `bootstrap-demo.go` | Mint DEMO tokens to users |
| `setup-local.sh` | Alternative local setup |
| `setup-devnet.sh` | DevNet setup |

### Testing (`scripts/testing/`)

| Script | Description |
|--------|-------------|
| `canton-transfer-demo.go` | Demo transfers and reconciliation |
| `register-native-user.go` | Register native Canton user |
| `register-user.go` | Register EVM user |
| `e2e-local.go` | End-to-end local tests |

### Utilities (`scripts/utils/`)

| Script | Description |
|--------|-------------|
| `query-canton-holdings.sh` | Query Canton holdings |
| `verify-canton-holdings.go` | Verify/compare holdings |
| `metamask-info.sh` | Show MetaMask config |
| `check-mappings.go` | Check fingerprint mappings |

---

## Development

### Build

```bash
# Install dependencies
go mod download

# Build binaries
make build

# Run tests
make test
```

### Manual Setup

```bash
# Set environment variables
export CANTON_MASTER_KEY=$(openssl rand -base64 32)
export SKIP_CANTON_SIG_VERIFY=true

# Start services
docker compose up -d

# Wait for healthy status
docker compose ps

# Run bootstrap scripts
go run scripts/setup/bootstrap-bridge.go -config config.e2e-local.yaml
go run scripts/setup/bootstrap-demo.go -config config.e2e-local.yaml
```

### Verify Setup

```bash
# Health checks
curl http://localhost:8081/health
curl http://localhost:5013/v2/version

# Check balances
./scripts/utils/query-canton-holdings.sh

# Compare DB vs Canton
go run scripts/utils/verify-canton-holdings.go -config config.e2e-local.yaml -compare
```

---

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/ARCHITECTURE.md) | System design and data flows |
| [Setup & Testing](docs/SETUP_AND_TESTING.md) | Local setup and environment configuration |
| [API Documentation](docs/API_DOCUMENTATION.md) | Endpoint reference |
| [CIP-0086 Overview](docs/CIP-0086-OVERVIEW.md) | CIP-0086 compliance |
| [Deployment Requirements](docs/WAYFINDER_DEPLOYMENT_REQUIREMENTS.md) | Production deployment checklist |

---

## Configuration

Configuration files:

| Environment | Config File |
|-------------|-------------|
| Local | `config.e2e-local.yaml` |
| Docker | `config.docker.yaml` |
| Example | `config.example.yaml` |

See [Setup & Testing Guide](docs/SETUP_AND_TESTING.md) for DevNet/Production configuration.

---

## License

[License details here]
