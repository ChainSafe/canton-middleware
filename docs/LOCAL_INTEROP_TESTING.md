# Local Interoperability Testing Guide

This guide walks you through running the full Canton-EVM interoperability demo locally. It demonstrates bidirectional token transfers between MetaMask (EVM) users and native Canton Ledger API users.

## Prerequisites

- **Docker** with Docker Compose v2
- **Go 1.22+** ([install guide](https://go.dev/doc/install))
- **DAML SDK 3.4.8** (only needed to rebuild DARs; pre-built DARs are included)
  ```bash
  daml version  # Should show 3.4.8
  ```

## Architecture Overview

The local stack consists of the following services:

| Service | Port | Description |
|---------|------|-------------|
| Canton | 5011 (gRPC), 5013 (HTTP) | Canton participant node with two participants and a sequencer |
| Anvil | 8545 | Local Ethereum node (Foundry) |
| PostgreSQL | 5432 | Database for the API server |
| Mock OAuth2 | 8088 | OAuth2 token provider for Canton authentication |
| API Server | 8081 | ERC-20 JSON-RPC facade (MetaMask-compatible) |
| Relayer | - | Bridges EVM deposits/withdrawals to Canton |
| Bootstrap | - | One-shot container that sets up parties, DARs, and configs |

### Tokens

| Token | Type | Description |
|-------|------|-------------|
| DEMO | Native Canton (CIP-56) | Minted directly on Canton, no EVM counterpart |
| PROMPT | Bridged ERC-20 | Bridged from Ethereum via the Wayfinder bridge |

### How It Works

- **MetaMask users** interact through the API server, which translates Ethereum JSON-RPC calls into Canton Ledger API operations.
- **Native Canton users** interact directly via the Canton Ledger API (gRPC).
- Both user types hold the same `CIP56Holding` contracts on Canton, enabling seamless interoperability.

## Quick Start

### 1. Clone the Repository

```bash
git clone --recurse-submodules https://github.com/chainsafe/canton-middleware.git
cd canton-middleware
```

If you already cloned without submodules:
```bash
git submodule update --init --recursive
```

### 2. Build DAML Contracts (Optional)

Pre-built DARs are included. If you need to rebuild:

```bash
./scripts/build-dars.sh
```

### 3. Run the Full Bootstrap

This single command starts all services, deploys contracts, registers test users, and mints tokens:

```bash
./scripts/setup/bootstrap-all.sh
```

**What it does:**
1. Generates a master key for Canton authentication
2. Starts Docker services (Canton, Anvil, PostgreSQL, OAuth2 mock, API server, relayer)
3. Waits for all services to be healthy
4. Deploys DAML packages (DARs) to Canton
5. Allocates a `BridgeIssuer` party
6. Creates `TokenConfig` and `CIP56Manager` for DEMO and PROMPT tokens
7. Creates `WayfinderBridgeConfig` for PROMPT bridging
8. Registers two MetaMask test users
9. Mints 500 DEMO to each user
10. Deposits 100 PROMPT to User 1 via the Ethereum bridge

**Options:**
```bash
./scripts/setup/bootstrap-all.sh --skip-prompt       # Skip PROMPT deposit
./scripts/setup/bootstrap-all.sh --demo-amount 1000   # Custom DEMO amount per user
./scripts/setup/bootstrap-all.sh --skip-shutdown       # Don't restart existing services
```

**Expected final state:**
| User | DEMO | PROMPT |
|------|------|--------|
| User 1 (0xf39F...) | 500 | 100 |
| User 2 (0x7099...) | 500 | 0 |

### 4. Verify the Setup

```bash
go run scripts/testing/demo-activity.go -config .test-config.yaml
```

This runs a quick smoke test: transfers between MetaMask users and verifies balances.

### 5. Run the Interoperability Demo

```bash
go run scripts/testing/interop-demo.go -config .test-config.yaml
```

## Step-by-Step Walkthrough

The interop demo executes the following steps:

### Step 0: Verify Existing MetaMask Users
Confirms the two bootstrap users exist with their DEMO balances.

### Step 1: Allocate Native Canton Parties
Creates two new Canton parties (`native_interop_1`, `native_interop_2`) that are **not** registered with the API server. These represent users who interact with Canton directly via the Ledger API.

### Step 2: MetaMask User -> Native User
Transfers 100 DEMO from MetaMask User 1 to Native User 1. This proves MetaMask users can send tokens to native Canton participants.

### Step 3: Native User -> Native User
Transfers 100 DEMO from Native User 1 to Native User 2. This transfer happens entirely via the Canton Ledger API, simulating direct Canton interactions.

### Step 4: Native User -> MetaMask User
Transfers 100 DEMO from Native User 2 back to MetaMask User 1. This proves native Canton users can send tokens to MetaMask users. The transfer includes **holding merge** (combining the existing and new holdings into one).

### Step 5: Register Native User with API Server
Registers Native User 1 with the API server, giving them a MetaMask-compatible EVM address and private key. After registration, they can import this key into MetaMask.

### Step 6: Cross-Type Transfer
Transfers 100 DEMO from MetaMask User 1 to the newly registered Native User 1 (now accessible via MetaMask). Runs reconciliation to update the API server database.

## Test Accounts

### MetaMask Users (Pre-configured)

| | Address | Private Key |
|-|---------|-------------|
| User 1 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` |
| User 2 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d` |

### MetaMask Network Configuration

| Setting | Value |
|---------|-------|
| Network Name | Canton Local |
| RPC URL | `http://localhost:8081/eth` |
| Chain ID | `31337` |
| Currency | ETH |

### Token Addresses (for MetaMask import)

| Token | Address | Decimals |
|-------|---------|----------|
| PROMPT | `0x5FbDB2315678afecb367f032d93F642f64180aa3` | 18 |
| DEMO | `0xDE30000000000000000000000000000000000001` | 18 |

## Troubleshooting

### Docker services fail to start

```bash
# Check container status
docker compose ps

# Check logs for a specific service
docker compose logs canton
docker compose logs bootstrap
```

### Canton not connected to synchronizer

Canton takes a few seconds to start. The bootstrap script retries automatically. If it times out:

```bash
# Verify Canton is healthy
docker compose exec canton curl -s http://localhost:5013/v2/state/connected-synchronizers | jq
```

### Bootstrap fails at "Allocating BridgeIssuer party"

Check Canton logs for errors:
```bash
docker compose logs canton 2>&1 | grep -i "error\|exception"
```

### Package ID mismatch

If you rebuilt DARs, the package IDs change. The bootstrap script auto-detects them, but if you're running scripts manually, update the config:

```bash
# Get the current bridge-wayfinder package ID
daml damlc inspect contracts/canton-erc20/daml/bridge-wayfinder/.daml/dist/bridge-wayfinder-*.dar | grep "^package"

# Update in config files
# bridge_package_id in: config.docker-local.yaml, .test-config.yaml, etc.
```

### `USER_NOT_FOUND` warnings during party allocation

When the interop demo allocates native Canton parties, it attempts to call `GrantCanActAs` via Canton's **User Management Service** to register the OAuth client (`local-test-client`) as authorized to act as the new party. This call fails with `USER_NOT_FOUND` because the mock OAuth user is not registered in Canton's User Management Service.

**Why the demo still works:** Command submission (`CommandService.SubmitAndWaitForTransaction`) and party allocation are handled by the Canton participant node's authentication layer, which is separate from the User Management Service. In the local Docker setup, the participant trusts the authenticated caller (via the mock OAuth token) to act as any party it has locally allocated. The `GrantCanActAs` step is only needed in production Canton deployments with strict user management enforcement.

These warnings are expected in local testing and can be safely ignored.

### Stale state from previous runs

```bash
# Full cleanup
docker compose down -v

# Re-run bootstrap
./scripts/setup/bootstrap-all.sh
```

### Port conflicts

If ports 5011, 5013, 8081, 8088, or 8545 are in use:
```bash
# Check what's using the port
lsof -i :8081
```

## Cleanup

```bash
docker compose down -v
```

This stops all containers and removes all volumes (database data, Canton state).

## Related Documentation

- [Architecture Design](architecture_design.md) -- System architecture and component overview
- [Bridge Testing Guide](BRIDGE_TESTING_GUIDE.md) -- DAML contract-level testing
- [Devnet Setup](DEVNET_SETUP.md) -- Deploying to a Canton devnet
- [API Documentation](API_DOCUMENTATION.md) -- API server endpoints
