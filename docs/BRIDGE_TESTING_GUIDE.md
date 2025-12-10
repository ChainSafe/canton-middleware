# Canton-Ethereum Bridge Testing Guide

This guide walks through building, deploying, and testing the Canton-Ethereum bridge from start to finish.

## Automated Testing

For quick testing, use the automated test script:

```bash
# Full test with clean environment
./scripts/test-bridge-local.sh --clean

# Skip Docker setup (if services are already running)
./scripts/test-bridge-local.sh --skip-docker
```

The script automates all steps below and prints results to the terminal.

> **⚠️ Prerequisite**: You must build the Canton Docker image first! See [Build Canton Docker Image](#build-canton-docker-image) below.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start](#quick-start)
3. [Step-by-Step Setup](#step-by-step-setup)
4. [Testing the Bridge](#testing-the-bridge)
5. [EVM → Canton Flow (Deposit)](#evm--canton-flow-deposit)
6. [Canton → EVM Flow (Withdrawal)](#canton--evm-flow-withdrawal)
7. [Troubleshooting](#troubleshooting)
8. [Testing Checklist](#testing-checklist)

---

## Prerequisites

- Docker & Docker Compose
- Go 1.21+
- Node.js 18+ (for Foundry/Cast)
- Foundry (forge, cast, anvil)
- Canton Docker image (`chainsafe/canton:3.4.8`)

### Install Foundry

```bash
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

### Build Canton Docker Image

The Canton Docker image must be built from the [ChainSafe/canton-docker](https://github.com/ChainSafe/canton-docker) repository:

```bash
# Clone the canton-docker repository
git clone https://github.com/ChainSafe/canton-docker.git
cd canton-docker

# Build the Docker image (downloads Canton 3.4.8 and creates chainsafe/canton:3.4.8)
./build_container.sh

# Verify the image was created
docker images | grep canton
# Should show: chainsafe/canton   3.4.8   ...
```

The build script will:
1. Download the Canton open-source release (version 3.4.8)
2. Unpack the release into a temporary directory
3. Build the Docker image tagged as `chainsafe/canton:3.4.8`

The image includes:
- **Canton**: The distributed ledger binaries (Open Source Edition)
- **grpcurl**: A command-line tool for interacting with gRPC servers
- **grpc_health_probe**: A command-line tool to perform health checks on gRPC applications

> **Note**: The `docker-compose.yaml` in this repository references `chainsafe/canton:3.4.8`. The image must be built locally before running `docker compose up`.

---

## Quick Start

**Recommended: Use the automated test script** (handles all setup automatically):

```bash
./scripts/test-bridge-local.sh --clean
```

**Manual quick start** (requires updating `config.yaml` with dynamic party/domain IDs):

```bash
# 1. Start all services
docker compose up -d

# 2. Wait for Canton to be healthy AND connected to synchronizer
docker compose ps  # Should show canton as "healthy"
# Also verify synchronizer connection:
curl -s http://localhost:5013/v2/state/connected-synchronizers | jq '.connectedSynchronizers | length'
# Should return 1

# 3. Allocate party and get domain ID (each fresh Canton has unique IDs!)
PARTY_RESP=$(curl -s -X POST http://localhost:5013/v2/parties \
  -H 'Content-Type: application/json' \
  -d '{"partyIdHint": "BridgeIssuer"}')
PARTY_ID=$(echo "$PARTY_RESP" | jq -r '.partyDetails.party')
DOMAIN_ID=$(curl -s http://localhost:5013/v2/state/connected-synchronizers | jq -r '.connectedSynchronizers[0].synchronizerId')
echo "Update config.yaml with:"
echo "  relayer_party: $PARTY_ID"
echo "  domain_id: $DOMAIN_ID"

# 4. Bootstrap the bridge contracts on Canton
go run scripts/bootstrap-bridge.go \
  -config config.local.yaml \
  -issuer "$PARTY_ID" \
  -package "6694b7794de78352c5893ded301e6cf0080db02cbdfa7fab23cfd9e8a56eb73d"

# 5. Register a test user (required for deposits)
go run scripts/register-user.go \
  -config config.local.yaml \
  -party "$PARTY_ID"

# 6. Start the relayer
go run cmd/relayer/main.go -config config.local.yaml

# 7. Check health
curl http://localhost:8080/health  # Should return "OK"
```

---

## Step-by-Step Setup

### Step 1: Start Docker Services

```bash
cd /path/to/canton-middleware

# Start all services (Anvil, Canton, PostgreSQL)
docker compose up -d

# Check status
docker compose ps
```

Expected output:
```
NAME       IMAGE                        STATUS                    PORTS
anvil      ghcr.io/foundry-rs/foundry   Up                        0.0.0.0:8545->8545/tcp
canton     chainsafe/canton:3.4.8       Up (healthy)              0.0.0.0:5011-5013->5011-5013/tcp
postgres   postgres:15-alpine           Up (healthy)              0.0.0.0:5432->5432/tcp
```

**Wait for Canton to be fully ready** (the test script does this automatically):

```bash
# 1. Wait for Docker health check
docker inspect --format='{{.State.Health.Status}}' canton
# Should return: healthy

# 2. Wait for HTTP API
curl -s http://localhost:5013/v2/version

# 3. Wait for synchronizer connection (IMPORTANT!)
curl -s http://localhost:5013/v2/state/connected-synchronizers | jq '.connectedSynchronizers | length'
# Should return: 1 (not 0)
```

### Step 2: Verify Ethereum Contracts Deployed

The `deployer` container automatically deploys the bridge contracts to Anvil.

```bash
# Check bridge contract
cast call 0x5FbDB2315678afecb367f032d93F642f64180aa3 "relayer()" --rpc-url http://localhost:8545

# Should return the relayer address:
# 0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266

# Check wrapped token contract name
cast call 0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512 "name()(string)" --rpc-url http://localhost:8545
# Should return: "Wrapped Canton Token"
```

### Step 3: Verify Canton DARs Uploaded

```bash
# List uploaded packages
curl -s http://localhost:5013/v2/packages | jq '.packageIds | length'
# Should return 30+ packages

# Verify the cip56-token package is uploaded (required for CIP56Manager)
CIP56_PACKAGE="e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe"
curl -s http://localhost:5013/v2/packages | jq ".packageIds | index(\"$CIP56_PACKAGE\")"
# Should return a number (not null)

# Check parties exist
curl -s http://localhost:5013/v2/parties | jq '.partyDetails[].party'
# Should include BridgeIssuer party (if allocated)
```

> **Note**: The deployer container uploads all DAR packages automatically. Wait for at least 30 packages AND the cip56-token package to be available before bootstrapping.

### Step 4: Allocate Bridge Issuer Party (if not done)

> **Prerequisite**: Canton must be connected to a synchronizer before allocating parties!

```bash
# First, verify Canton is connected to a synchronizer
curl -s http://localhost:5013/v2/state/connected-synchronizers | jq '.connectedSynchronizers | length'
# Must return at least 1 (not 0)

# Check if BridgeIssuer exists
curl -s http://localhost:5013/v2/parties | jq '.partyDetails[].party' | grep BridgeIssuer

# If not, allocate it:
curl -X POST http://localhost:5013/v2/parties \
  -H 'Content-Type: application/json' \
  -d '{"partyIdHint": "BridgeIssuer"}'
```

> **Important**: Each fresh Canton instance generates a **unique party fingerprint** and domain ID. After allocating the party, you must update `config.yaml`:
>
> ```bash
> # Get domain_id (synchronizer ID)
> DOMAIN_ID=$(curl -s http://localhost:5013/v2/state/connected-synchronizers | jq -r '.connectedSynchronizers[0].synchronizerId')
> echo "domain_id: $DOMAIN_ID"
> 
> # Get relayer_party (from the allocation response above)
> # Update these in config.yaml before running bootstrap
> ```
>
> The automated test script (`./scripts/test-bridge.sh`) handles this automatically.

### Step 5: Bootstrap Canton Bridge Contracts

```bash
# Run bootstrap script
go run scripts/bootstrap-bridge.go \
  -config config.local.yaml \
  -issuer "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e" \
  -package "6694b7794de78352c5893ded301e6cf0080db02cbdfa7fab23cfd9e8a56eb73d"
```

This creates:
- **CIP56Manager**: Token manager for minting/burning wrapped PROMPT
- **WayfinderBridgeConfig**: Bridge configuration contract

Expected output on first run:
```
>>> Step 3: Checking for existing WayfinderBridgeConfig...
    No existing config found, creating new one...
>>> Step 4: Creating CIP56Manager for PROMPT token...
    CIP56Manager Contract ID: 00...
>>> Step 5: Creating WayfinderBridgeConfig...
    WayfinderBridgeConfig Contract ID: 00...
```

On subsequent runs:
```
>>> Step 3: Checking for existing WayfinderBridgeConfig...
    [EXISTS] WayfinderBridgeConfig: 00...
Bridge is already bootstrapped!
```

### Step 6: Register Test User

**Important**: Before deposits can be processed, you must register a `FingerprintMapping` for the recipient on Canton.

```bash
# Register the BridgeIssuer as a user (for testing)
go run scripts/register-user.go \
  -config config.local.yaml \
  -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
```

The script:
1. Extracts the fingerprint from the party ID (removes `1220` prefix)
2. Checks if a mapping already exists
3. Creates a `FingerprintMapping` contract if needed

Expected output:
```
======================================================================
REGISTER USER - Create FingerprintMapping on Canton
======================================================================
Party:       BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e
Fingerprint: 47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e
EVM Address: 
...
USER REGISTERED SUCCESSFULLY
======================================================================
Party:            BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e
Fingerprint:      47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e
MappingCid:       00...

The user can now receive deposits with this fingerprint as bytes32:
  CANTON_RECIPIENT="0x47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
```

### Step 7: Start the Relayer

```bash
# Run relayer (foreground)
go run cmd/relayer/main.go -config config.local.yaml

# Or run in background
go run cmd/relayer/main.go -config config.local.yaml &
```

Expected logs:
```
INFO    Starting Canton-Ethereum Bridge Relayer
INFO    Database connection established
INFO    Connected to Canton Network
INFO    Connected to Ethereum
INFO    Relayer engine started
INFO    Starting deposit event poller
INFO    Starting Canton withdrawal event stream
```

### Step 8: Verify Everything is Running

```bash
# Health check
curl http://localhost:8080/health
# Returns: OK

# API status
curl http://localhost:8080/api/v1/status | jq
# Returns: {"status": "running", "version": "dev"}

# Metrics
curl http://localhost:9090/metrics | head -20
```

---

## Testing the Bridge

### Test Environment Info

| Component | Address/URL |
|-----------|-------------|
| Anvil RPC | http://localhost:8545 |
| Canton gRPC | localhost:5011 |
| Canton HTTP | http://localhost:5013 |
| Relayer API | http://localhost:8080 |
| Relayer Metrics | http://localhost:9090 |
| Bridge Contract | 0x5FbDB2315678afecb367f032d93F642f64180aa3 |
| Wrapped Token | 0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512 |
| Relayer Address | 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266 |
| Issuer Fingerprint | *Dynamic - generated on each fresh Canton start* |

> **Note**: The `Issuer Fingerprint` and `Domain ID` are dynamically generated by Canton on each fresh start. The test script (`./scripts/test-bridge.sh`) automatically captures and uses these values. For manual testing, you must extract them from the party allocation response and update `config.yaml`.

### Anvil Test Accounts

```
Account 0 (Relayer): 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
Private Key: 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80

Account 1 (User): 0x70997970C51812dc3A010C7d01b50e0d17dc79C8
Private Key: 0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d
```

---

## EVM → Canton Flow (Deposit)

**Flow: Lock PROMPT on EVM → Relayer detects → Mint wrapped PROMPT on Canton**

### Prerequisites

Ensure you have:
1. ✅ Docker services running (`docker compose up -d`)
2. ✅ Canton bridge bootstrapped (`scripts/bootstrap-bridge.go`)
3. ✅ User registered (`scripts/register-user.go`)
4. ✅ Relayer running (`cmd/relayer/main.go`)

### Step 1: Setup Bridge Contracts (One-time setup)

```bash
BRIDGE="0x5FbDB2315678afecb367f032d93F642f64180aa3"
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
RELAYER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
RELAYER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

# Grant MINTER_ROLE to relayer (required for withdrawals)
MINTER_ROLE="0x9f2df0fed2c77648de5860a4cc508cd0818c85b8b8a1ab4ceeef8d981c8956a6"
cast send $TOKEN "grantRole(bytes32,address)" $MINTER_ROLE $RELAYER \
  --rpc-url http://localhost:8545 \
  --private-key $RELAYER_KEY

# Add token mapping
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"  # "PROM" in hex
cast send $BRIDGE "addTokenMapping(address,bytes32,bool)" \
  $TOKEN $CANTON_TOKEN_ID true \
  --rpc-url http://localhost:8545 \
  --private-key $RELAYER_KEY

# Note: These will fail if already done - that's OK
```

### Step 2: Mint Test Tokens (for testing)

```bash
# Mint wrapped tokens to user account for testing
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
AMOUNT="1000000000000000000000"  # 1000 tokens with 18 decimals

cast send $TOKEN "mint(address,uint256)" $USER $AMOUNT \
  --rpc-url http://localhost:8545 \
  --private-key 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80

# Check balance
cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url http://localhost:8545
```

### Step 3: Approve Bridge to Spend Tokens

```bash
USER_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
BRIDGE="0x5FbDB2315678afecb367f032d93F642f64180aa3"
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
AMOUNT="1000000000000000000000"  # 1000 tokens

cast send $TOKEN "approve(address,uint256)" $BRIDGE $AMOUNT \
  --rpc-url http://localhost:8545 \
  --private-key $USER_KEY
```

### Step 4: Deposit to Canton

```bash
USER_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
BRIDGE="0x5FbDB2315678afecb367f032d93F642f64180aa3"
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
AMOUNT="100000000000000000000"  # 100 tokens

# Canton recipient fingerprint as bytes32 (32 bytes = 64 hex chars)
# This is the fingerprint WITHOUT the "1220" multihash prefix
CANTON_RECIPIENT="0x47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"

cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
  $TOKEN $AMOUNT $CANTON_RECIPIENT \
  --rpc-url http://localhost:8545 \
  --private-key $USER_KEY
```

### Step 5: Monitor Relayer

Watch the relayer logs for the deposit being processed:

```
INFO    Processing transfer    {"id": "0x...", "direction": "ethereum_to_canton", "amount": "100000000000000000000"}
INFO    Creating pending deposit    {"fingerprint": "47584945...", "amount": "100", "evm_tx_hash": "0x..."}
INFO    Processing deposit    {"deposit_cid": "00...", "mapping_cid": "00..."}
INFO    Deposit processed successfully    {"holding_cid": "00..."}
```

### Step 6: Verify on Canton

```bash
# Query holdings using the script
go run scripts/query-holdings.go \
  -config config.local.yaml \
  -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
```

Expected output shows the newly minted CIP56Holding:
```
Found 1 holding(s):

Holding #1:
  Contract ID: 00...
  Owner:       BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e
  Amount:      100.0000000000
```

### Step 7: Check Transfer Records

```bash
# Query relayer API for transfer history
curl http://localhost:8080/api/v1/transfers | jq
```

---

## Canton → EVM Flow (Withdrawal)

**Flow: Burn wrapped PROMPT on Canton → Relayer detects → Unlock/Mint PROMPT on EVM**

### Prerequisites

Ensure you have:
1. ✅ Completed deposit flow (user has CIP56Holding on Canton)
2. ✅ User has registered FingerprintMapping with EVM address

### Step 1: Query User's Holdings on Canton

First, find the user's CIP56Holding contract ID:

```bash
# Query active CIP56Holding contracts
go run scripts/query-holdings.go \
  -config config.local.yaml \
  -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
```

The script will output:
```
Found 1 holding(s):

Holding #1:
  Contract ID: 00...
  Owner:       BridgeIssuer::...
  Amount:      100.0000000000

To initiate a withdrawal, use:
  go run scripts/initiate-withdrawal.go ...
```

### Step 2: Initiate and Process Withdrawal

Use the withdrawal script (it handles both `InitiateWithdrawal` and `ProcessWithdrawal`):

```bash
go run scripts/initiate-withdrawal.go \
  -config config.local.yaml \
  -holding-cid "00..." \
  -amount "50.0" \
  -evm-destination "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
```

The script will:
1. Create a `WithdrawalRequest` via `InitiateWithdrawal`
2. Process it via `ProcessWithdrawal` (burns tokens, creates `WithdrawalEvent`)

Expected output:
```
>>> Initiating withdrawal...
    WithdrawalRequest CID: 00...

>>> Processing withdrawal (burning tokens)...
    WithdrawalEvent CID: 00...

WITHDRAWAL PROCESSED SUCCESSFULLY
```

### Step 3: Monitor Relayer

Watch the relayer logs for the withdrawal being processed:

```
INFO    Withdrawal event detected    {"amount": "50.0", "destination": "0x7099..."}
INFO    Submitting withdrawal to EVM
INFO    Withdrawal transaction submitted    {"tx_hash": "0xabc..."}
INFO    Completing withdrawal on Canton
```

### Step 4: Verify on EVM

```bash
# Check user's token balance increased
USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"

cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url http://localhost:8545
```

### Step 5: Check Transfer Records

```bash
# Query relayer API for transfer history
curl http://localhost:8080/api/v1/transfers | jq
```

---

## Troubleshooting

### Canton Not Healthy

```bash
# Check Canton logs
docker logs canton 2>&1 | tail -50

# Restart Canton
docker compose restart canton
```

### "PARTY_ALLOCATION_WITHOUT_CONNECTED_SYNCHRONIZER"

Canton must be connected to a synchronizer before allocating parties. Wait longer after startup:

```bash
# Check synchronizer connection
curl -s http://localhost:5013/v2/state/connected-synchronizers | jq '.connectedSynchronizers | length'
# Must return 1 or more

# If 0, wait a few more seconds and retry
```

### "PACKAGE_NOT_FOUND" for CIP56.Token:CIP56Manager

The cip56-token package hasn't been uploaded yet. Wait for the deployer to finish:

```bash
# Check if cip56-token package is uploaded
CIP56_PACKAGE="e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe"
curl -s http://localhost:5013/v2/packages | jq ".packageIds | index(\"$CIP56_PACKAGE\")"
# Should return a number, not null
```

### Relayer Connection Errors

```bash
# Check if Canton is accepting connections
grpcurl -plaintext localhost:5011 list

# Check if packages are uploaded
curl -s http://localhost:5013/v2/packages | jq '.packageIds | length'
```

### Template Not Found Errors

Ensure the correct package IDs are in `config.yaml`:
- `bridge_package_id`: bridge-wayfinder package
- `core_package_id`: bridge-core package (for WithdrawalEvent)

```bash
# Find package IDs
curl -s http://localhost:5013/v2/packages | jq '.packageIds[]'
```

### "No active WayfinderBridgeConfig found"

This was a bug in the V2 API usage - fixed in the codebase. The `GetActiveContracts` request requires `ActiveAtOffset` to be set to the current ledger end (not 0).

```bash
# Re-run bootstrap to create the config
go run scripts/bootstrap-bridge.go -config config.local.yaml \
  -issuer "BridgeIssuer::..." -package "..."
```

### "Missing user-id" Error

This was a bug - fixed by adding `UserId` field to all Canton command submissions.

### "No FingerprintMapping found for fingerprint"

The recipient fingerprint doesn't have a registered mapping on Canton:

```bash
# Register the user
go run scripts/register-user.go \
  -config config.local.yaml \
  -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
```

### Deposit Not Processing

1. Check relayer logs for errors
2. Verify token mapping exists on bridge contract
3. Verify user has FingerprintMapping on Canton
4. Check database for pending transfers:

```bash
docker exec postgres psql -U postgres -d relayer -c "SELECT id, status, error_message FROM transfers ORDER BY created_at DESC LIMIT 5;"
```

### "parser error: invalid string length" for CANTON_RECIPIENT

The Canton recipient must be exactly 32 bytes (64 hex characters). The full fingerprint includes a `1220` multihash prefix, making it 33 bytes. Use only the hash portion:

```bash
# Full fingerprint (33 bytes - TOO LONG):
# 122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e

# Hash portion (32 bytes - CORRECT):
CANTON_RECIPIENT="0x47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
```

---

## Testing Checklist

### Setup
- [ ] Docker services start correctly (`docker compose up -d`)
- [ ] Canton is healthy (`docker inspect --format='{{.State.Health.Status}}' canton` returns "healthy")
- [ ] Canton HTTP API is ready (`curl http://localhost:5013/v2/version`)
- [ ] Canton connected to synchronizer (count ≥ 1)
- [ ] Ethereum contracts verified (`cast call` returns expected values)
- [ ] Canton DARs uploaded (30+ packages)
- [ ] cip56-token package uploaded (`e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe`)
- [ ] BridgeIssuer party allocated
- [ ] `config.yaml` updated with new `domain_id` and `relayer_party`
- [ ] Bootstrap creates WayfinderBridgeConfig
- [ ] User FingerprintMapping registered
- [ ] MINTER_ROLE granted to relayer on token contract
- [ ] Token mapping added to bridge contract
- [ ] Relayer starts and passes health check

### EVM → Canton (Deposit)
- [ ] Test tokens minted to user
- [ ] Tokens approved for bridge
- [ ] `depositToCanton` transaction succeeds
- [ ] Relayer detects deposit event
- [ ] PendingDeposit created on Canton
- [ ] FingerprintMapping found
- [ ] CIP56Holding minted to user

### Canton → EVM (Withdrawal)
- [ ] User has CIP56Holding on Canton
- [ ] `InitiateWithdrawal` creates WithdrawalRequest
- [ ] `ProcessWithdrawal` burns tokens and creates WithdrawalEvent
- [ ] Relayer detects WithdrawalEvent
- [ ] `withdrawFromCanton` transaction succeeds on EVM
- [ ] Tokens minted to user on EVM
- [ ] Withdrawal marked complete

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              USER ACTIONS                                    │
└─────────────────────────────────────────────────────────────────────────────┘
                │                                      │
                ▼                                      ▼
┌───────────────────────────┐          ┌───────────────────────────┐
│      ETHEREUM (Anvil)     │          │     CANTON NETWORK        │
│                           │          │                           │
│  ┌─────────────────────┐  │          │  ┌─────────────────────┐  │
│  │   CantonBridge.sol  │  │          │  │ WayfinderBridgeConfig │  │
│  │                     │  │          │  │                     │  │
│  │ depositToCanton()   │──┼──────────┼──│ CreatePendingDeposit│  │
│  │ withdrawFromCanton()│◀─┼──────────┼──│ WithdrawalEvent     │  │
│  └─────────────────────┘  │          │  └─────────────────────┘  │
│                           │          │                           │
│  ┌─────────────────────┐  │          │  ┌─────────────────────┐  │
│  │ WrappedCantonToken  │  │          │  │    CIP56Manager     │  │
│  │                     │  │          │  │                     │  │
│  │ mint() / burn()     │  │          │  │ Mint() / Burn()     │  │
│  └─────────────────────┘  │          │  └─────────────────────┘  │
└───────────────────────────┘          └───────────────────────────┘
                │                                      │
                │         ┌─────────────────┐          │
                └────────▶│    RELAYER      │◀─────────┘
                          │                 │
                          │ - Event polling │
                          │ - TX submission │
                          │ - State tracking│
                          │                 │
                          │ :8080 (API)     │
                          │ :9090 (Metrics) │
                          └─────────────────┘
                                   │
                                   ▼
                          ┌─────────────────┐
                          │   PostgreSQL    │
                          │                 │
                          │ - Transfers     │
                          │ - Chain state   │
                          │ - Offsets       │
                          └─────────────────┘
```

---

## Support

- Check logs: `docker logs canton` / relayer stdout
- Database: `docker exec postgres psql -U postgres -d relayer`
- Metrics: http://localhost:9090/metrics
- Canton Console: `docker exec -it canton /canton/bin/canton`
