# DevNet Interoperability Testing Guide

This guide walks you through running the DEMO token interoperability test against the ChainSafe DevNet. It demonstrates:

- **Splice Token Standard (CIP-0056) compliance** -- all tokens use the Splice `HoldingV1`, `TransferFactory`, and `Metadata` interfaces
- **External party allocation** -- all users (MetaMask and native) are created as external parties using the Interactive Submission API
- **DEMO token interoperability** -- bidirectional transfers between MetaMask (EVM) users and native Canton external parties via the `/eth` JSON-RPC endpoint

> **Note:** This guide covers DEMO token only. PROMPT/bridging is not available on DevNet. For the full local flow (including PROMPT bridge), see [LOCAL_INTEROP_TESTING.md](LOCAL_INTEROP_TESTING.md).

## Prerequisites

- **Docker** with Docker Compose v2
- **Go 1.24+** ([install guide](https://go.dev/doc/install))
- **Foundry/Cast** ([install guide](https://book.getfoundry.sh/getting-started/installation)) -- used for EIP-191 signing and Ethereum interactions
- **DevNet credentials** in `config.api-server.devnet.yaml` (OAuth2 client ID/secret, Canton endpoint)

## Quick Start (Three Commands)

```bash
# 1. Bootstrap: starts PostgreSQL + API Server, registers users, mints DEMO
./scripts/remote/bootstrap-remote.sh --devnet

# 2. Test: runs DEMO interop steps (Part A only)
DATABASE_HOST=localhost go run scripts/testing/interop-demo.go \
  --config config.api-server.devnet.yaml --skip-prompt

# 3. Reset (optional): archive all holdings, remint 500 DEMO per user
DATABASE_HOST=localhost go run scripts/utils/reset-demo-state.go \
  -config config.api-server.devnet.yaml
```

## Architecture Overview

Only PostgreSQL and the API Server run locally. Canton runs on the ChainSafe DevNet (remote gRPC over TLS with OAuth2 authentication).

```
┌─────────────────────────────────────────────────────┐
│  Local (Docker)                                     │
│                                                     │
│  ┌──────────────┐       ┌─────────────────────────┐ │
│  │  PostgreSQL   │◄─────│     API Server (:8081)   │ │
│  │  (:5432)      │       │  /eth  /register  /health│ │
│  └──────────────┘       └───────────┬─────────────┘ │
│                                     │               │
└─────────────────────────────────────│───────────────┘
                                      │ gRPC over TLS
                                      │ + OAuth2
                                      ▼
                          ┌───────────────────────┐
                          │  ChainSafe DevNet      │
                          │  Canton Ledger API     │
                          │  (5North Participant)   │
                          └───────────────────────┘
```

| Service | Location | Port | Description |
|---------|----------|------|-------------|
| PostgreSQL | Local (Docker) | 5432 | Database for users, whitelist, encrypted Canton keys |
| API Server | Local (Docker) | 8081 | ERC-20 JSON-RPC facade with `/eth` and `/register` endpoints |
| Canton | Remote (DevNet) | 443 | ChainSafe 5North participant node (gRPC over TLS) |

### What's Different from Local

| | Local | DevNet |
|---|---|---|
| Canton | Docker container (localhost:5011) | Remote TLS (canton-ledger-api-grpc-dev1.chainsafe.dev:443) |
| Auth | Mock OAuth2 (localhost:8088) | Auth0 (dev-2j3m40ajwym1zzaq.eu.auth0.com) |
| Anvil | Docker container (localhost:8545) | Not used |
| Relayer | Docker container | Not used |
| PROMPT bridge | Available | Not available |
| Tokens | DEMO + PROMPT | DEMO only |

## Step 1: Bootstrap

```bash
./scripts/remote/bootstrap-remote.sh --devnet
```

This single command:

1. Generates `CANTON_MASTER_KEY` (encrypts stored Canton signing keys)
2. Starts PostgreSQL and API Server containers via `docker-compose.remote.yaml`
3. Waits for services to be healthy
4. Whitelists and registers two test users (EIP-191 signatures, external party allocation on DevNet)
5. Bootstraps 500 DEMO tokens to each user via the Canton Ledger API

**Options:**
```bash
./scripts/remote/bootstrap-remote.sh --devnet                    # Default (500 DEMO per user)
./scripts/remote/bootstrap-remote.sh --devnet --demo-amount 1000 # Custom DEMO amount
```

**Expected state after bootstrap:**

| User | Type | DEMO |
|------|------|------|
| User 1 (`0xf39F...`) | External (MetaMask) | 500 |
| User 2 (`0x7099...`) | External (MetaMask) | 500 |

## Step 2: Run the Interop Test

```bash
DATABASE_HOST=localhost go run scripts/testing/interop-demo.go \
  --config config.api-server.devnet.yaml --skip-prompt
```

`DATABASE_HOST=localhost` overrides the config's `database.host` (which is set to `postgres` for Docker networking) so the Go script can connect directly.

### What the Test Covers (Part A: DEMO Token Interoperability)

| Step | Description |
|------|-------------|
| 1 | **Allocate External Native Parties** -- Creates `native_interop_1` and `native_interop_2` as external parties on Canton DevNet, registers them with the API server, and whitelists their EVM addresses |
| 2 | **MetaMask -> Native** -- User 1 (MetaMask) sends 100 DEMO to Native User 1 via `cast send` to `/eth` |
| 3 | **Native -> Native** -- Native User 1 sends 100 DEMO to Native User 2 via `cast send` to `/eth` |
| 4 | **Native -> MetaMask** -- Native User 2 sends 100 DEMO back to User 1 via `cast send` to `/eth` |

### Expected Final State

| User | DEMO |
|------|------|
| User 1 (`0xf39F...`) | 500 |
| User 2 (`0x7099...`) | 500 |
| Native User 1 | 0 |
| Native User 2 | 0 |

## Step 3: Reset State (Optional)

To re-run the test from a clean state without rebuilding Docker:

```bash
DATABASE_HOST=localhost go run scripts/utils/reset-demo-state.go \
  -config config.api-server.devnet.yaml
```

This script:
1. Archives (burns) all DEMO holdings on Canton
2. Mints fresh 500 DEMO to each registered user
3. Removes native interop users from the database
4. Reconciles database balances with Canton

Preview what will happen without making changes:
```bash
DATABASE_HOST=localhost go run scripts/utils/reset-demo-state.go \
  -config config.api-server.devnet.yaml --dry-run
```

## Test Accounts

### MetaMask Users (Anvil Default Accounts)

| | Address | Private Key |
|-|---------|-------------|
| User 1 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` |
| User 2 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d` |

### Native Users (Created by Interop Demo)

Native users are allocated as external parties during the test. Each gets a fresh secp256k1 Canton keypair and a derived EVM address. Their Canton signing keys are stored (encrypted) in the API server's database.

### MetaMask Network Configuration

| Setting | Value |
|---------|-------|
| Network Name | Canton DevNet |
| RPC URL | `http://localhost:8081/eth` |
| Chain ID | `1337` |
| Currency | ETH |

### Token Address (for MetaMask Import)

| Token | Address | Decimals |
|-------|---------|----------|
| DEMO | `0xDE30000000000000000000000000000000000001` | 18 |

## Troubleshooting

### API Server fails to start

```bash
docker compose -f docker-compose.remote.yaml logs api-server
```

Common causes:
- DevNet OAuth2 credentials expired or misconfigured in `config.api-server.devnet.yaml`
- Canton endpoint unreachable (firewall, VPN, DNS)
- Missing `CANTON_MASTER_KEY` (bootstrap script auto-generates this)

### Canton connection errors (TLS / OAuth2)

```bash
# Check if the Canton endpoint is reachable
openssl s_client -connect canton-ledger-api-grpc-dev1.chainsafe.dev:443 </dev/null

# Check OAuth2 token endpoint
curl -s -X POST https://dev-2j3m40ajwym1zzaq.eu.auth0.com/oauth/token \
  -H "Content-Type: application/json" \
  -d '{"grant_type": "client_credentials", "client_id": "...", "client_secret": "...", "audience": "..."}' | jq
```

### Registration fails ("party allocation failed")

Check API server logs for the underlying Canton error:
```bash
docker compose -f docker-compose.remote.yaml logs api-server --tail 30
```

### DATABASE_HOST override not working

The `DATABASE_HOST` environment variable must override the config's `database.host` field. If the Go scripts can't connect to PostgreSQL, check:
```bash
# Verify PostgreSQL is running
docker compose -f docker-compose.remote.yaml ps postgres

# Test direct connection
psql -h localhost -U postgres -d erc20_api -c "SELECT count(*) FROM users;"
```

### Stale state from previous runs

```bash
# Option A: Reset via script (preserves registered users, remints DEMO)
DATABASE_HOST=localhost go run scripts/utils/reset-demo-state.go \
  -config config.api-server.devnet.yaml

# Option B: Full clean slate (destroys all state)
docker compose -f docker-compose.remote.yaml down -v
./scripts/remote/bootstrap-remote.sh --devnet
```

### Port conflicts

If port 8081 or 5432 is in use:
```bash
lsof -i :8081
lsof -i :5432
```

## Cleanup

```bash
docker compose -f docker-compose.remote.yaml down -v
```

This stops all local containers and removes volumes (database data). Canton state on DevNet is unaffected.

## Related Documentation

- [Local Interop Testing](LOCAL_INTEROP_TESTING.md) -- Full local testing with PROMPT bridge
- [DevNet Setup](DEVNET_SETUP.md) -- Initial DevNet deployment and configuration
- [Architecture Design](architecture_design.md) -- System architecture overview
- [API Documentation](API_DOCUMENTATION.md) -- API server endpoints
