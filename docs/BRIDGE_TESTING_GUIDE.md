# Canton-Ethereum Bridge Testing Guide

This guide walks through building, deploying, and testing the Canton-Ethereum bridge from start to finish.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start](#quick-start)
3. [Step-by-Step Setup](#step-by-step-setup)
4. [Testing the Bridge](#testing-the-bridge)
5. [EVM → Canton Flow (Deposit)](#evm--canton-flow-deposit)
6. [Canton → EVM Flow (Withdrawal)](#canton--evm-flow-withdrawal)
7. [Troubleshooting](#troubleshooting)
8. [Next Steps](#next-steps)

---

## Prerequisites

- Docker & Docker Compose
- Go 1.21+
- Node.js 18+ (for Foundry/Cast)
- Foundry (forge, cast, anvil)

```bash
# Install Foundry if not already installed
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

---

## Quick Start

```bash
# 1. Start all services
docker compose up -d

# 2. Wait for Canton to be healthy (30-60 seconds)
docker compose ps  # Should show canton as "healthy"

# 3. Bootstrap the bridge contracts on Canton
go run scripts/bootstrap-bridge.go \
  -config config.yaml \
  -issuer "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e" \
  -package "6694b7794de78352c5893ded301e6cf0080db02cbdfa7fab23cfd9e8a56eb73d"

# 4. Start the relayer
go run cmd/relayer/main.go -config config.yaml

# 5. Check health
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

# Check parties exist
curl -s http://localhost:5013/v2/parties | jq '.partyDetails[].party'
# Should include BridgeIssuer party
```

### Step 4: Allocate Bridge Issuer Party (if not done)

```bash
# Check if BridgeIssuer exists
curl -s http://localhost:5013/v2/parties | jq '.partyDetails[].party' | grep BridgeIssuer

# If not, allocate it:
curl -X POST http://localhost:5013/v2/parties \
  -H 'Content-Type: application/json' \
  -d '{"partyIdHint": "BridgeIssuer"}'
```

### Step 5: Bootstrap Canton Bridge Contracts

```bash
# Run bootstrap script
go run scripts/bootstrap-bridge.go \
  -config config.yaml \
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

### Step 6: Start the Relayer

```bash
# Run relayer (foreground)
go run cmd/relayer/main.go -config config.yaml

# Or run in background
go run cmd/relayer/main.go -config config.yaml &
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

### Step 7: Verify Everything is Running

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
| Issuer Fingerprint | 122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e |

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

> **Note**: The full deposit flow requires a `FingerprintMapping` to be registered on Canton for the recipient. Without this, the relayer will detect the deposit but fail at the "get fingerprint mapping" step. This is expected behavior - see [Next Steps](#next-steps) for implementing user registration.

### Step 1: Add Token Mapping on Bridge (One-time setup)

```bash
# Add token mapping (relayer only)
# Maps ERC20 token to Canton token ID

BRIDGE="0x5FbDB2315678afecb367f032d93F642f64180aa3"
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"  # "PROM" in hex

cast send $BRIDGE "addTokenMapping(address,bytes32,bool)" \
  $TOKEN $CANTON_TOKEN_ID true \
  --rpc-url http://localhost:8545 \
  --private-key 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80

# Note: This will fail with "Token already mapped" if already done - that's OK
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
# NOTE: The full fingerprint is 33 bytes (1220... prefix), so we use the 32-byte hash portion
CANTON_RECIPIENT="0x47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"

cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
  $TOKEN $AMOUNT $CANTON_RECIPIENT \
  --rpc-url http://localhost:8545 \
  --private-key $USER_KEY
```

### Step 5: Monitor Relayer

Watch the relayer logs for:
```
INFO    Processing transfer    {"id": "0x...", "direction": "ethereum_to_canton", "amount": "100000000000000000000"}
INFO    Creating pending deposit    {"fingerprint": "47584945...", "amount": "100", "evm_tx_hash": "0x..."}
```

> **Expected**: Without a FingerprintMapping registered, you'll see:
> ```
> ERROR   Failed to submit transfer    {"error": "no FingerprintMapping found for fingerprint: 47584945..."}
> ```
> This is correct behavior - the deposit was detected but needs user registration.

### Step 6: Check Transfer Records

```bash
# Query relayer API for transfer history
curl http://localhost:8080/api/v1/transfers | jq
```

---

## Canton → EVM Flow (Withdrawal)

**Flow: Burn wrapped PROMPT on Canton → Relayer detects → Unlock PROMPT on EVM**

> **Note**: This flow requires completed deposits first (tokens minted on Canton).

### Step 1: Initiate Withdrawal on Canton

The withdrawal is initiated by the issuer on behalf of the user. This would typically happen through an API call to the relayer.

```bash
# The issuer exercises InitiateWithdrawal on WayfinderBridgeConfig
# This creates a WithdrawalRequest, processes it, and creates a WithdrawalEvent

# For now, this can be done via the Canton console or Daml Script
# The relayer will detect the WithdrawalEvent
```

### Step 2: Monitor Relayer

Watch the relayer logs for:
```
INFO    Withdrawal event detected    {"amount": "100.0", "destination": "0x7099..."}
INFO    Submitting withdrawal to EVM
INFO    Withdrawal transaction submitted    {"tx_hash": "0xabc..."}
INFO    Completing withdrawal on Canton
```

### Step 3: Verify on EVM

```bash
# Check user's token balance increased
USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
TOKEN="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"

cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url http://localhost:8545
```

### Step 4: Check Transfer Records

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
go run scripts/bootstrap-bridge.go -config config.yaml \
  -issuer "BridgeIssuer::..." -package "..."
```

### "Missing user-id" Error

This was a bug - fixed by adding `UserId` field to all Canton command submissions.

### Deposit Not Processing

1. Check relayer logs for errors
2. Verify token mapping exists on bridge contract
3. Verify user has FingerprintMapping on Canton (required!)
4. Check database for pending transfers:

```bash
docker exec postgres psql -U postgres -d relayer -c "SELECT id, status, error_message FROM transfers ORDER BY created_at DESC LIMIT 5;"
```

---

## Next Steps

### 1. Implement User Registration API

Currently, `FingerprintMapping` must be created manually. Add an API endpoint:

```go
// POST /api/v1/users/register
type RegisterUserRequest struct {
    EvmAddress string `json:"evm_address"`
}
// Returns: { "fingerprint": "1220...", "canton_party": "User::1220..." }
```

### 2. Implement Withdrawal API

Add endpoint for users to request withdrawals:

```go
// POST /api/v1/withdraw
type WithdrawRequest struct {
    Amount         string `json:"amount"`
    EvmDestination string `json:"evm_destination"`
    Fingerprint    string `json:"fingerprint"`  // or use JWT auth
}
```

### 3. Add Event Indexing

Index all bridge events for better querying:
- Deposit events from EVM
- WithdrawalEvents from Canton
- Transfer completion status

### 4. Production Considerations

- [ ] **Key Management**: Use AWS KMS or HashiCorp Vault
- [ ] **Multi-sig**: Require multiple relayers to sign withdrawals
- [ ] **Rate Limiting**: Prevent abuse
- [ ] **Monitoring**: Set up Grafana dashboards
- [ ] **Alerts**: PagerDuty for stuck transfers
- [ ] **Audit Logging**: Complete audit trail

### 5. Testing Checklist

- [x] Docker services start correctly
- [x] Ethereum contracts verified
- [x] Canton DARs uploaded
- [x] Bootstrap creates WayfinderBridgeConfig
- [x] Relayer starts and connects
- [x] Deposit event detected on EVM
- [x] Deposit creates PendingDeposit on Canton
- [ ] FingerprintMapping lookup succeeds (needs user registration)
- [ ] Full deposit flow completes
- [ ] Withdrawal flow works

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
