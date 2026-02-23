# Canton Middleware

MetaMask-compatible middleware for Canton Network, enabling ERC-20 style interactions with Splice-compliant CIP-56 tokens.

## Overview

This project provides:
- **API Server** -- Ethereum JSON-RPC facade that translates MetaMask transactions to Canton CIP-56 operations
- **Relayer** -- Bridges PROMPT tokens between Ethereum and Canton
- **Interoperability** -- Native Canton users (e.g. Canton Loop) and MetaMask users can seamlessly transfer tokens to each other
- **Splice Token Standard (CIP-0056)** -- All tokens implement the Splice `HoldingV1`, `TransferFactory`, and `Metadata` interfaces
- **Splice Registry API** -- External wallets discover `TransferFactory` contracts for explicit disclosure during transfers
- **External Parties** -- All users are allocated as external parties using the Interactive Submission API (no ~200 internal party limit)

## Quick Start (Local Development)

```bash
# 1. Bootstrap: starts Docker, registers users, mints tokens
./scripts/testing/bootstrap-local.sh --clean

# 2. Test: runs all 8 interop + bridge test steps
go run scripts/testing/interop-demo.go
```

Both scripts auto-detect all dynamic configuration (domain IDs, party IDs, contract addresses) from the running Docker containers. See the [Local Interop Testing Guide](docs/LOCAL_INTEROP_TESTING.md) for full details.

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
| User Registration | http://localhost:8081/register |
| Splice Registry API | http://localhost:8081/registry/transfer-instruction/v1/transfer-factory |
| API Server Health | http://localhost:8081/health |
| Canton gRPC | localhost:5011 |
| Canton HTTP | http://localhost:5013 |
| Anvil (Ethereum) | http://localhost:8545 |
| PostgreSQL | localhost:5432 |

---

## Architecture

```
┌─────────────────┐                    ┌─────────────────┐
│    MetaMask     │                    │  Canton Loop /  │
│   (EVM Wallet)  │                    │  Native Canton  │
└────────┬────────┘                    └────────┬────────┘
         │ JSON-RPC                             │ gRPC / Registry API
         ▼                                      ▼
┌──────────────────────────────────────────────────────────┐
│                     API SERVER                            │
│  • /eth         - JSON-RPC facade (MetaMask-compatible)   │
│  • /register    - User registration (EVM + Canton native) │
│  • /registry/…  - Splice Registry API (TransferFactory)   │
│  • Custodial Canton key management (AES-256-GCM)          │
│  • Balance reconciliation with Canton                     │
└────────────────────────┬─────────────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
        ▼                ▼                ▼
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  PostgreSQL │   │   Canton    │   │   Relayer   │
│  (Users/    │   │  (CIP-56    │   │  (ERC-20    │
│   Balances) │   │   Ledger)   │   │   Bridge)   │
└─────────────┘   └─────────────┘   └──────┬──────┘
                                           │
                                    ┌──────▼──────┐
                                    │  Ethereum   │
                                    │  (Anvil /   │
                                    │   Mainnet)  │
                                    └─────────────┘
```

### How Transfers Work

All users are **external parties** on Canton. They hold their own signing keys and use the **Interactive Submission API** (prepare/sign/execute) instead of the standard `CommandService`.

```
MetaMask / cast send / Canton Loop
        │
        ▼
  /eth endpoint (JSON-RPC)  or  Interactive Submission (gRPC)
        │
        ▼
  Resolve Canton signing key from encrypted DB store
        │
        ▼
  PrepareSubmission → sign with user's key → ExecuteSubmission
        │
        ▼
  Canton Ledger: TransferFactory_Transfer (Splice-compliant CIP-56)
```

### Tokens

| Token | Type | Description |
|-------|------|-------------|
| DEMO | Native Canton (CIP-56) | Minted directly on Canton via `CIP56Manager`, implements Splice `HoldingV1` |
| PROMPT | Bridged ERC-20 | Bridged from Ethereum via the Wayfinder bridge, also uses Splice `HoldingV1` |

Both tokens carry Splice-standard metadata (`TextMap Text`) with DNS-prefixed keys (e.g. `splice.chainsafe.io/symbol`). Metadata is propagated through all transfers via the `TransferFactory`.

---

## Project Structure

```
canton-middleware/
├── cmd/
│   ├── api-server/              # API server entry point
│   └── relayer/                 # Relayer entry point
├── pkg/
│   ├── apidb/                   # API database operations (users, balances, whitelist)
│   ├── app/                     # Application wiring (api server, http server)
│   ├── auth/                    # JWT and EVM authentication
│   ├── cantonsdk/               # Canton SDK
│   │   ├── bridge/              #   ERC-20 ↔ Canton bridge operations
│   │   ├── client/              #   High-level SDK facade
│   │   ├── identity/            #   Party allocation, fingerprint mappings
│   │   ├── lapi/v2/             #   Generated Ledger API v2 protobufs
│   │   ├── ledger/              #   Low-level gRPC client (state, commands, auth)
│   │   ├── token/               #   CIP-56 token operations (mint, burn, transfer)
│   │   └── values/              #   Daml value encoding/decoding helpers
│   ├── config/                  # Configuration loading
│   ├── db/                      # Relayer database schema
│   ├── ethereum/                # Ethereum client and ABI
│   ├── ethrpc/                  # Ethereum JSON-RPC server
│   ├── keys/                    # Custodial key management (secp256k1, AES-256-GCM)
│   ├── registration/            # User registration handler
│   ├── registry/                # Splice Registry API handler
│   ├── relayer/                 # Bridge relayer logic
│   └── service/                 # Token service layer
├── contracts/
│   ├── canton-erc20/            # DAML contracts (CIP-56, Splice-compliant)
│   └── ethereum-wayfinder/      # Solidity bridge contracts
├── scripts/
│   ├── setup/                   # Bootstrap and infrastructure scripts
│   ├── testing/                 # Test and demo scripts
│   ├── bridge/                  # Bridge operation scripts
│   ├── demo/                    # Demo scripts
│   ├── remote/                  # Remote deployment scripts
│   ├── utils/                   # Diagnostic utilities
│   ├── lib/                     # Shared bash functions
│   └── archive/                 # Archived migration scripts
├── docs/                        # Documentation
├── deployments/                 # Docker, Canton, Prometheus configs
└── proto/                       # Protobuf definitions
```

---

## Scripts

### Testing (`scripts/testing/`)

| Script | Description |
|--------|-------------|
| `bootstrap-local.sh` | Full local bootstrap (Docker, users, tokens) |
| `interop-demo.go` | 8-step interop + bridge test suite |
| `demo-activity.go` | Display Canton token activity (holdings, configs, events) |
| `canton-transfer-demo.go` | Demo transfers and reconciliation |
| `register-native-user.go` | Register a native Canton user |
| `register-user.go` | Register an EVM user |
| `e2e-local.go` | End-to-end local tests |
| `test-reconcile.go` | Test balance reconciliation |
| `test-whitelist.go` | Test whitelist functionality |

### Setup (`scripts/setup/`)

| Script | Description |
|--------|-------------|
| `bootstrap-all.sh` | Full automated setup (alternative to bootstrap-local.sh) |
| `bootstrap-bridge.go` | Bootstrap bridge contracts on Canton |
| `bootstrap-demo.go` | Mint DEMO tokens to users |
| `docker-bootstrap.sh` | Docker-specific bootstrap |
| `setup-local.sh` | Local environment setup |
| `setup-devnet.sh` | DevNet setup |
| `build-dars.sh` | Build DAML archives |
| `generate-protos.sh` | Regenerate protobuf Go code |

### Bridge (`scripts/bridge/`)

| Script | Description |
|--------|-------------|
| `bridge-activity.go` | Display recent bridge activity |
| `bridge-deposit.go` | Deposit ERC-20 to Canton |
| `get-holding-cid.go` | Get holding contract ID for withdrawals |
| `initiate-withdrawal.go` | Initiate Canton → Ethereum withdrawal |
| `cleanup-withdrawals.go` | Clean up processed withdrawals |

### Utilities (`scripts/utils/`)

| Script | Description |
|--------|-------------|
| `check-user-holdings.go` | Check user holdings on Canton |
| `verify-canton-holdings.go` | Verify/compare holdings (DB vs Canton) |
| `list-parties.go` | List known parties |
| `list-users.go` | List registered users |
| `check-mappings.go` | Check fingerprint mappings |
| `reconcile.go` | Manual balance reconciliation |
| `reset-demo-state.go` | Reset demo state |
| `query-canton-holdings.sh` | Query Canton holdings (bash) |
| `metamask-info.sh` | Show MetaMask config |
| `mock-oauth2-server.go` | Local OAuth2 mock server |

---

## Development

### Build

```bash
go mod download
go build ./...
```

### Lint

```bash
golangci-lint run ./...
```

### Manual Setup

```bash
export CANTON_MASTER_KEY=$(openssl rand -base64 32)
export SKIP_CANTON_SIG_VERIFY=true

docker compose up -d

# Wait for healthy status, then bootstrap
go run scripts/setup/bootstrap-bridge.go -config config.e2e-local.yaml
go run scripts/setup/bootstrap-demo.go -config config.e2e-local.yaml
```

### Verify Setup

```bash
curl http://localhost:8081/health
curl -X POST http://localhost:8081/registry/transfer-instruction/v1/transfer-factory
go run scripts/utils/check-user-holdings.go -config config.e2e-local.yaml
```

---

## Configuration

| Environment | Config File |
|-------------|-------------|
| Local testing | `config.e2e-local.yaml` |
| Docker | `config.docker.yaml` |
| API server (Docker) | `config.api-server.docker.yaml` |
| Local devnet | `config.local-devnet.yaml` |
| Example | `config.example.yaml` |

Key Canton-specific fields (auto-detected by bootstrap):

| Field | Description |
|-------|-------------|
| `domain_id` | Canton synchronizer domain ID |
| `relayer_party` | Bridge relayer / token issuer party |
| `instrument_admin` | Party administering token instruments |
| `instrument_id` | Token instrument identifier (e.g. `PROMPT`) |
| `cip56_package_id` | `cip56-token` DAR package hash |
| `splice_holding_package_id` | Splice `HoldingV1` interface DAR hash |
| `splice_transfer_package_id` | Splice `TransferFactory` interface DAR hash |
| `bridge_package_id` | `bridge-wayfinder` DAR package hash |

---

## Documentation

| Document | Description |
|----------|-------------|
| [Local Interop Testing](docs/LOCAL_INTEROP_TESTING.md) | Full local bootstrap and 8-step interop test guide |
| [API Documentation](docs/API_DOCUMENTATION.md) | Endpoint reference (JSON-RPC, Registration, Splice Registry) |
| [Architecture](docs/ARCHITECTURE.md) | System design and data flows |
| [Setup & Testing](docs/SETUP_AND_TESTING.md) | Environment configuration |
| [CIP-0086 Overview](docs/CIP-0086-OVERVIEW.md) | CIP-0086 compliance |
| [Deployment Requirements](docs/WAYFINDER_DEPLOYMENT_REQUIREMENTS.md) | Production deployment checklist |

---

## License

[License details here]
