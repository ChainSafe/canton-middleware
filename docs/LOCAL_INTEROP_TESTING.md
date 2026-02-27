# Local Interoperability Testing Guide

This guide walks you through running the full Canton-EVM interoperability test locally. It demonstrates:

- **Splice Token Standard (CIP-0056) compliance** -- all tokens use the Splice `HoldingV1`, `TransferFactory`, and `Metadata` interfaces, enabling interoperability with wallets like Canton Loop
- **External party allocation** -- all users (MetaMask and native) are created as external parties using the Interactive Submission API
- **DEMO token interoperability** -- bidirectional transfers between MetaMask (EVM) users and native Canton external parties via the `/eth` JSON-RPC endpoint
- **PROMPT token bridging** -- depositing ERC-20 tokens from Ethereum into Canton and transferring them on-ledger
- **Metadata propagation** -- token metadata (symbol, name, decimals) is preserved through transfers and bridge operations

## Prerequisites

- **Docker** with Docker Compose v2
- **Go 1.24+** ([install guide](https://go.dev/doc/install))
- **Foundry/Cast** ([install guide](https://book.getfoundry.sh/getting-started/installation)) -- used for EIP-191 signing and Ethereum interactions
- **DAML SDK 3.4.8** (only needed to rebuild DARs; pre-built DARs are included)

## Quick Start (Two Commands)

```bash
# 1. Bootstrap: starts Docker, registers users as external parties, mints tokens
./scripts/testing/bootstrap-local.sh --clean

# 2. Test: runs all 8 interop + bridge test steps
go run scripts/testing/interop-demo.go
```

That's it. Both scripts auto-detect all dynamic configuration (domain IDs, party IDs, contract addresses) from the running Docker containers.

The bootstrap script auto-generates `CANTON_MASTER_KEY` (for encrypting stored Canton signing keys) and sets `SKIP_CANTON_SIG_VERIFY=true` (for local testing). No manual environment setup is needed.

> **Go module cache note:** If `go run` fails with "no required module provides package" errors, set `GOMODCACHE` before running:
>
> ```bash
> export GOMODCACHE="$HOME/go/pkg/mod"
> go run scripts/testing/interop-demo.go
> ```

## Architecture Overview

The local stack consists of the following services:

| Service | Port | Description |
|---------|------|-------------|
| Canton | 5011 (gRPC), 5013 (HTTP) | Canton participant node with two participants and a sequencer |
| Anvil | 8545 | Local Ethereum node (Foundry) |
| PostgreSQL | 5432 | Database for the API server (users, whitelist, encrypted Canton keys) |
| Mock OAuth2 | 8088 | OAuth2 token provider for Canton authentication |
| API Server | 8081 | ERC-20 JSON-RPC facade with `/eth` and `/register` endpoints |
| Relayer | - | Bridges EVM deposits/withdrawals to Canton |
| Bootstrap | - | One-shot container that sets up parties, DARs, and configs |

### Tokens

| Token | Type | Description |
|-------|------|-------------|
| DEMO | Native Canton (CIP-56) | Minted directly on Canton via `CIP56Manager`, implements Splice `HoldingV1` |
| PROMPT | Bridged ERC-20 | Bridged from Ethereum via the Wayfinder bridge, also uses Splice `HoldingV1` |

Both tokens carry Splice-standard metadata (`TextMap Text`) with DNS-prefixed keys (e.g., `splice.chainsafe.io/symbol`). Metadata is propagated through all transfers via the `TransferFactory`.

### How It Works

All users are **external parties** on Canton. External parties hold their own signing keys and use the **Interactive Submission API** (prepare/sign/execute) instead of the standard `CommandService`. This removes the ~200 internal party limit and enables interoperability with wallets like Canton Loop.

- **MetaMask users** register via `/register` with an EIP-191 signature. The API server generates a secp256k1 Canton keypair, allocates an external party, and stores the encrypted signing key.
- **Native Canton users** are allocated externally (via the SDK), then registered via `/register` with their Canton party ID and signing key, which the API server stores for Interactive Submission.
- All transfers route through the `/eth` JSON-RPC endpoint using `eth_sendRawTransaction`. The API server resolves the sender's Canton signing key from the database, then uses Interactive Submission to execute the transfer on Canton.

```
MetaMask / cast send
        |
        v
  /eth endpoint (JSON-RPC)
        |
        v
  TokenService.Transfer()
        |
        v
  PrepareSubmission -> sign with user's key -> ExecuteSubmission
        |
        v
  Canton Ledger (CIP-56 Holding transfer)
```

## Step 1: Bootstrap

```bash
./scripts/testing/bootstrap-local.sh --clean
```

This single command does everything from scratch:

1. Starts Docker services (Canton, Anvil, PostgreSQL, OAuth2 mock, API server, relayer)
2. Waits for all services to be healthy
3. Extracts dynamic config from the bootstrap container (domain ID, relayer party, contract addresses)
4. Auto-updates `config.e2e-local.yaml` so subsequent scripts use the correct values
5. Whitelists and registers two test users via the API server (EIP-191 signatures, external party allocation)
6. Bootstraps 500 DEMO tokens to each user

**Options:**
```bash
./scripts/testing/bootstrap-local.sh --clean        # Full clean slate (removes volumes)
./scripts/testing/bootstrap-local.sh --skip-docker   # Skip Docker start (services already running)
./scripts/testing/bootstrap-local.sh --verbose       # Verbose Docker output
```

**Expected state after bootstrap:**
| User | Type | DEMO | PROMPT |
|------|------|------|--------|
| User 1 (`0xf39F...`) | External (MetaMask) | 500 | 0 |
| User 2 (`0x7099...`) | External (MetaMask) | 500 | 0 |

## Step 2: Run the Interop Test

```bash
go run scripts/testing/interop-demo.go                      # full test (DEMO + PROMPT)
go run scripts/testing/interop-demo.go --skip-prompt         # DEMO only
go run scripts/testing/interop-demo.go --skip-demo           # PROMPT bridge only
```

> For DevNet testing (DEMO only, remote Canton), see [DEVNET_INTEROP_TESTING.md](DEVNET_INTEROP_TESTING.md).

## What the Test Covers

The interop demo runs 8 automated steps across two parts:

### Part A: DEMO Token Interoperability (Steps 1--4)

Tests bidirectional transfers between MetaMask users and native Canton external parties using the native DEMO token. All transfers go through the `/eth` endpoint using Interactive Submission.

| Step | Description |
|------|-------------|
| 1 | **Allocate External Native Parties** -- Creates `native_interop_1` and `native_interop_2` as external parties on Canton, registers them with the API server (passing Canton signing keys), and whitelists their EVM addresses |
| 2 | **MetaMask -> Native** -- User 1 (MetaMask) sends 100 DEMO to Native User 1 via `cast send` to `/eth` |
| 3 | **Native -> Native** -- Native User 1 sends 100 DEMO to Native User 2 via `cast send` to `/eth` |
| 4 | **Native -> MetaMask** -- Native User 2 sends 100 DEMO back to User 1 via `cast send` to `/eth` |

### Part B: PROMPT Token Bridge (Steps 5--8)

Tests the full ERC-20 bridge lifecycle: Ethereum deposit -> Canton balance -> Canton transfer.

| Step | Description |
|------|-------------|
| 5 | **Deposit PROMPT** -- Approves and deposits 100 PROMPT from Anvil (Ethereum) to Canton via the bridge contract |
| 6 | **Verify Canton Balance** -- Polls until the relayer processes the deposit and PROMPT appears on Canton |
| 7 | **Transfer on Canton** -- Sends 25 PROMPT from User 1 to User 2 via `cast send` to `/eth` |
| 8 | **Verify Final Balances** -- Confirms User 1 has 75 PROMPT and User 2 has 25 PROMPT |

### Expected Final State

| User | DEMO | PROMPT |
|------|------|--------|
| User 1 (`0xf39F...`) | 500 | 75 |
| User 2 (`0x7099...`) | 500 | 25 |
| Native User 1 | 0 | 0 |
| Native User 2 | 0 | 0 |

## Test Accounts

### MetaMask Users (Pre-configured Anvil accounts)

| | Address | Private Key |
|-|---------|-------------|
| User 1 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` |
| User 2 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d` |

### Native Users (Created by interop demo)

Native users are allocated as external parties during the test. Each gets a fresh secp256k1 Canton keypair and a derived EVM address. Their Canton signing keys are stored (encrypted) in the API server's database so it can sign Interactive Submission transactions on their behalf.

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

## Key Concepts

### External Parties and Interactive Submission

Canton has two types of parties:

- **Internal parties** are created by `AllocateParty`. The participant node manages their keys and signs transactions via `CommandService.SubmitAndWait`. Limited to ~200 per participant.
- **External parties** are created by `AllocateExternalParty`. Their signing keys are held externally. Transactions use the Interactive Submission API: `PrepareSubmission` -> sign hash with external key -> `ExecuteSubmission`. No practical limit.

This middleware uses external parties exclusively. The API server stores each user's Canton signing key (AES-256-GCM encrypted with `CANTON_MASTER_KEY`) and signs Interactive Submission transactions on their behalf.

### Canton Key Fingerprints

Canton identifies signing keys by their **fingerprint**: a multihash-encoded SHA-256 of the X.509 SubjectPublicKeyInfo (SPKI) DER-encoded public key. The fingerprint is used in the `SignedBy` field of signature messages. The `CantonKeyPair.Fingerprint()` method computes this.

## Troubleshooting

### Docker services fail to start

```bash
docker compose ps
docker compose logs canton
docker compose logs bootstrap
```

### Canton not connected to synchronizer

Canton takes a few seconds to start. The bootstrap script retries automatically. If it times out:

```bash
docker compose exec canton curl -s http://localhost:5013/v2/state/connected-synchronizers | jq
```

### Bootstrap fails at user registration ("party allocation failed")

Check the API server logs for the underlying Canton error:
```bash
docker logs erc20-api-server --tail 20
```

Common causes:
- Canton node not ready yet (retry after a few seconds)
- Key format issue (should be SPKI DER-encoded, not raw)

### CANTON_MASTER_KEY issues

The bootstrap script auto-generates this key. If you're running services manually, set it:
```bash
export CANTON_MASTER_KEY=$(openssl rand -base64 32)
docker compose up -d api-server
```

The API server will refuse to start without it. The key must be the same one used during user registration (it encrypts the stored Canton signing keys).

### Transfer fails with "no key resolver configured"

The API server's Canton SDK client doesn't have a `KeyResolver`. This means `CANTON_MASTER_KEY` was not set when the API server started.

### Stale state from previous runs

```bash
./scripts/testing/bootstrap-local.sh --clean
```

### Port conflicts

If ports 5011, 5013, 8081, 8088, or 8545 are in use:
```bash
lsof -i :8081
```

## Cleanup

```bash
docker compose down -v
```

This stops all containers and removes all volumes (database data, Canton state).

## Canton Configuration

The bootstrap process auto-detects and sets several Canton-specific configuration fields in `config.e2e-local.yaml`:

| Field | Description |
|-------|-------------|
| `domain_id` | Canton synchronizer domain ID (auto-detected from bootstrap container) |
| `relayer_party` | Canton party ID for the bridge relayer / token issuer |
| `instrument_admin` | Party that administers token instruments (set to `relayer_party`) |
| `cip56_package_id` | Package hash of the `cip56-token` DAR |
| `bridge_package_id` | Package hash of the `bridge-wayfinder` DAR |
| `core_package_id` | Package hash of the `bridge-core` DAR |
| `splice_holding_package_id` | Package hash of the Splice `HoldingV1` interface DAR |
| `splice_transfer_package_id` | Package hash of the Splice `TransferFactory` interface DAR |

These are set automatically by `bootstrap-local.sh` and `docker-bootstrap.sh`. You should not need to edit them manually unless you rebuild the DAML contracts (which changes their package hashes).

## Related Documentation

- [DevNet Interop Testing](DEVNET_INTEROP_TESTING.md) -- DEMO token testing against ChainSafe DevNet
- [Architecture Design](architecture_design.md) -- System architecture and component overview
- [Bridge Testing Guide](BRIDGE_TESTING_GUIDE.md) -- DAML contract-level testing
- [Devnet Setup](DEVNET_SETUP.md) -- Deploying to a Canton devnet
- [API Documentation](API_DOCUMENTATION.md) -- API server endpoints
