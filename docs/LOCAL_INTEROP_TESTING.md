# Local Interoperability Testing Guide

This guide walks you through running the full Canton-EVM interoperability test locally. It demonstrates:

- **DEMO token interoperability** -- bidirectional transfers between MetaMask (EVM) users and native Canton Ledger API users
- **PROMPT token bridging** -- depositing ERC-20 tokens from Ethereum into Canton and transferring them on-ledger

## Prerequisites

- **Docker** with Docker Compose v2
- **Go 1.23+** ([install guide](https://go.dev/doc/install))
- **Foundry/Cast** ([install guide](https://book.getfoundry.sh/getting-started/installation)) -- used for EIP-191 signing and Ethereum interactions
- **DAML SDK 3.4.8** (only needed to rebuild DARs; pre-built DARs are included)

## Quick Start (Two Commands)

```bash
# 1. Bootstrap: starts Docker, registers users, mints tokens
./scripts/testing/bootstrap-local.sh --clean

# 2. Test: runs all 10 interop + bridge test steps
go run scripts/testing/interop-demo.go
```

That's it. Both scripts auto-detect all dynamic configuration (domain IDs, party IDs, contract addresses) from the running Docker containers.

> **Go module cache note:** If `go run` fails with "no required module provides package" errors, your `GOMODCACHE` may be pointing to a sandboxed or empty cache directory. Fix by explicitly setting it before running:
>
> ```bash
> export GOMODCACHE="$HOME/go/pkg/mod"
> ./scripts/testing/bootstrap-local.sh --clean
> go run scripts/testing/interop-demo.go
> ```
>
> The bootstrap script sets this automatically, but external tools (e.g. Cursor IDE sandbox) can override it. If you hit this issue, export `GOMODCACHE` in your shell profile (`~/.zshrc` or `~/.bashrc`) to make it permanent.

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

## Step 1: Bootstrap

```bash
./scripts/testing/bootstrap-local.sh --clean
```

This single command does everything from scratch:

1. Starts Docker services (Canton, Anvil, PostgreSQL, OAuth2 mock, API server, relayer)
2. Waits for all services to be healthy
3. Extracts dynamic config from the bootstrap container (domain ID, relayer party, contract addresses)
4. Auto-updates `config.e2e-local.yaml` so subsequent scripts use the correct values
5. Whitelists and registers two test users via the API server (EIP-191 signatures)
6. Bootstraps 500 DEMO tokens to each user

**Options:**
```bash
./scripts/testing/bootstrap-local.sh --clean        # Full clean slate (removes volumes)
./scripts/testing/bootstrap-local.sh --skip-docker   # Skip Docker start (services already running)
./scripts/testing/bootstrap-local.sh --verbose       # Verbose Docker output
```

**Expected state after bootstrap:**
| User | DEMO | PROMPT |
|------|------|--------|
| User 1 (`0xf39F...`) | 500 | 0 |
| User 2 (`0x7099...`) | 500 | 0 |

## Step 2: Run the Interop Test

```bash
go run scripts/testing/interop-demo.go
```

**Options:**
```bash
go run scripts/testing/interop-demo.go --skip-prompt   # Skip PROMPT bridge tests
go run scripts/testing/interop-demo.go --skip-demo     # Skip DEMO interop tests
```

## What the Test Covers

The interop demo runs 10 automated steps across two parts:

### Part A: DEMO Token Interoperability (Steps 1--6)

Tests bidirectional transfers between MetaMask users and native Canton Ledger API users using the native DEMO token.

| Step | Description |
|------|-------------|
| 1 | **Allocate Native Parties** -- Creates `native_interop_1` and `native_interop_2` on Canton (not registered with API server) |
| 2 | **MetaMask → Native** -- User 1 (MetaMask) sends 100 DEMO to Native User 1 |
| 3 | **Native → Native** -- Native User 1 sends 100 DEMO to Native User 2 via Ledger API |
| 4 | **Native → MetaMask** -- Native User 2 sends 100 DEMO back to User 1 |
| 5 | **Register Native User** -- Registers Native User 1 with the API server, generating a MetaMask-compatible EVM keypair |
| 6 | **MetaMask → Registered Native** -- User 1 sends 100 DEMO to the newly registered Native User 1 |

### Part B: PROMPT Token Bridge (Steps 7--10)

Tests the full ERC-20 bridge lifecycle: Ethereum deposit → Canton balance → Canton transfer.

| Step | Description |
|------|-------------|
| 7 | **Deposit PROMPT** -- Approves and deposits 100 PROMPT from Anvil (Ethereum) to Canton via the bridge contract |
| 8 | **Verify Canton Balance** -- Polls until the relayer processes the deposit and PROMPT appears on Canton |
| 9 | **Transfer on Canton** -- Sends 25 PROMPT from User 1 to User 2 via the API server's `eth_sendRawTransaction` |
| 10 | **Verify Final Balances** -- Confirms User 1 has 75 PROMPT and User 2 has 25 PROMPT |

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

| Token | Address | Decimals | Notes |
|-------|---------|----------|-------|
| PROMPT | Auto-detected from deployer | 18 | ERC-20 bridged from Ethereum |
| DEMO | `0xDE30000000000000000000000000000000000001` | 18 | Synthetic address for native Canton token |

> PROMPT and bridge contract addresses are deterministic on first deployment but change if Docker volumes are recreated. The bootstrap script and interop test auto-detect them from `docker logs deployer`.

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

### `no required module provides package` errors

Go can't find downloaded dependencies. This typically happens when `GOMODCACHE` points to a sandboxed or empty directory (common in Cursor IDE or CI environments).

```bash
# Check where Go is looking for modules
go env GOMODCACHE

# If it points to a temp/sandbox path, override it:
export GOMODCACHE="$HOME/go/pkg/mod"

# Then re-download and retry
go mod download
./scripts/testing/bootstrap-local.sh --skip-docker
```

The bootstrap script exports `GOMODCACHE="$HOME/go/pkg/mod"` internally, but if the parent shell has already set it to something else, you may need to export it before invoking the script.

### `USER_NOT_FOUND` warnings during party allocation

When the interop demo allocates native Canton parties, it attempts to call `GrantCanActAs` via Canton's **User Management Service** to register the OAuth client (`local-test-client`) as authorized to act as the new party. This call fails with `USER_NOT_FOUND` because the mock OAuth user is not registered in Canton's User Management Service.

**Why the demo still works:** Command submission (`CommandService.SubmitAndWaitForTransaction`) and party allocation are handled by the Canton participant node's authentication layer, which is separate from the User Management Service. In the local Docker setup, the participant trusts the authenticated caller (via the mock OAuth token) to act as any party it has locally allocated. The `GrantCanActAs` step is only needed in production Canton deployments with strict user management enforcement.

These warnings are expected in local testing and can be safely ignored.

### Stale state from previous runs

```bash
# Full cleanup and re-bootstrap
./scripts/testing/bootstrap-local.sh --clean
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
