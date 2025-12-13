# Local Testing Guide

This guide walks through testing the Canton-Ethereum bridge locally using Docker containers.

## Prerequisites

- Docker and Docker Compose
- Go 1.23+
- Daml SDK (`curl -sSL https://get.daml.com/ | sh`)
- Foundry (`curl -L https://foundry.paradigm.xyz | bash`)

## Quick Start

```bash
# Start interactive menu
./scripts/test-bridge.sh

# Or run full test directly
./scripts/test-bridge.sh --full-test
```

---

## Interactive Menu Walkthrough

Run the script without arguments to enter interactive mode:

```bash
./scripts/test-bridge.sh
```

You'll see:

```
══════════════════════════════════════════════════════════════════════
  CANTON BRIDGE MANAGER
══════════════════════════════════════════════════════════════════════

  1) Full test (100 deposit + 50 withdraw)
  2) Deposit tokens
  3) Withdraw tokens
  4) View status
  5) View relayer logs
  6) Start services
  7) Stop services
  8) Clean & restart
  0) Exit

══════════════════════════════════════════════════════════════════════
Select option:
```

### Option 6: Start Services

If services aren't running, select **6** to start them:

```
Select option: 6

══════════════════════════════════════════════════════════════════════
  PRE-FLIGHT: Checking DAR Files
══════════════════════════════════════════════════════════════════════
>>> Building DAR files...
✓ DAR files built
>>> Extracting package IDs from DAR files...
    cip56-token:      76ef5131f7e0f51b...
    bridge-wayfinder: 6cdc1057afc883ae...
    bridge-core:      39bbb569bc40c817...

══════════════════════════════════════════════════════════════════════
  Starting Docker Services
══════════════════════════════════════════════════════════════════════
>>> Starting docker compose...
✓ All services are ready!
```

### Option 2: Deposit Tokens

Select **2** and enter an amount:

```
Select option: 2
Enter deposit amount [100]: 50

══════════════════════════════════════════════════════════════════════
  Deposit: 50 tokens (EVM → Canton)
══════════════════════════════════════════════════════════════════════
>>> Checking user balance...
    User token balance: 1000000000000000000000
>>> Approving bridge to spend tokens...
✓ Approved
>>> Depositing 50 tokens to Canton...
    Deposit TX: 0x1234...
✓ Deposit submitted
>>> Waiting for deposit confirmation...
...
✓ Deposit confirmed!
```

### Option 3: Withdraw Tokens

Select **3** and enter an amount:

```
Select option: 3
Enter withdrawal amount [50]: 25

══════════════════════════════════════════════════════════════════════
  Withdrawal: 25 tokens (Canton → EVM)
══════════════════════════════════════════════════════════════════════
    User EVM balance before: 950000000000000000000
>>> Finding CIP56Holding on Canton...
    Found holding CID: 00b2b6f47e7a007d...
    Available balance: 100 tokens
>>> Initiating withdrawal of 25 tokens...
✓ Withdrawal initiated
>>> Waiting for withdrawal confirmation...
✓ Withdrawal confirmed - EVM balance updated!
```

> **Note:** Each deposit creates a separate `CIP56Holding` contract on Canton. When withdrawing, the script automatically selects the holding with the **largest balance**. This ensures you can withdraw the maximum available from any single holding.

### Option 4: View Status

Select **4** to see current state:

```
Select option: 4

══════════════════════════════════════════════════════════════════════
  Status
══════════════════════════════════════════════════════════════════════

DOCKER SERVICES
═══════════════════════════════════════════════════════════════════════
NAME                    STATUS              PORTS
anvil                   Up 5 minutes        0.0.0.0:8545->8545/tcp
canton                  Up 5 minutes        0.0.0.0:5011-5013->5011-5013/tcp
canton-bridge-relayer   Up 4 minutes        0.0.0.0:8080->8080/tcp
mock-oauth2             Up 5 minutes        0.0.0.0:8088->8088/tcp
postgres                Up 5 minutes        0.0.0.0:5432->5432/tcp

RELAYER TRANSFERS
═══════════════════════════════════════════════════════════════════════
{
  "id": "0x1234...-1",
  "direction": "ethereum_to_canton",
  "status": "completed"
}
```

### Option 1: Full Test

Select **1** to run the complete test flow (100 deposit + 50 withdraw):

```
Select option: 1

══════════════════════════════════════════════════════════════════════
  Full Test: 100 deposit + 50 withdraw
══════════════════════════════════════════════════════════════════════
... (runs deposit then withdrawal automatically)
```

### Option 8: Clean & Restart

Select **8** to wipe everything and start fresh:

```
Select option: 8

══════════════════════════════════════════════════════════════════════
  Cleaning Environment
══════════════════════════════════════════════════════════════════════
>>> Stopping and removing all containers and volumes...
✓ Environment cleaned

══════════════════════════════════════════════════════════════════════
  Starting Docker Services
══════════════════════════════════════════════════════════════════════
... (fresh start)
```

---

## Two-Terminal Workflow

For the best testing experience, use two terminals side-by-side:

```
┌─────────────────────────────────┬─────────────────────────────────┐
│         TERMINAL 1              │         TERMINAL 2              │
│    (Interactive Control)        │    (Canton Monitoring)          │
├─────────────────────────────────┼─────────────────────────────────┤
│ ./scripts/test-bridge.sh        │ go run scripts/bridge-activity  │
│                                 │   -config .test-config.yaml     │
└─────────────────────────────────┴─────────────────────────────────┘
```

### Step-by-Step

#### Terminal 1: Start Services

```bash
cd canton-middleware
./scripts/test-bridge.sh
```

Select **6** (Start services) and wait for completion.

#### Terminal 2: Start Monitoring

Once services are running, `.test-config.yaml` is created. Start the activity monitor:

```bash
cd canton-middleware
go run scripts/bridge-activity.go -config .test-config.yaml
```

You'll see:

```
======================================================================
CANTON BRIDGE ACTIVITY REPORT
======================================================================
Network: localhost:5011
Party:   BridgeIssuer::122066a5661b97...
Time:    2025-12-13T12:00:00Z
Ledger:  Offset 50

--- RECENT DEPOSITS (EVM → Canton) -----------------------------------
No deposits found in the lookback range.

--- RECENT WITHDRAWALS (Canton → EVM) --------------------------------
No withdrawals found in the lookback range.

--- CURRENT HOLDINGS -------------------------------------------------
No holdings found.

======================================================================
Summary: 0 holding(s)
======================================================================
```

#### Terminal 1: Execute Deposit

Select **2** and deposit 100 tokens:

```
Select option: 2
Enter deposit amount [100]: 100
```

#### Terminal 2: Verify Deposit

Run the activity script again:

```bash
go run scripts/bridge-activity.go -config .test-config.yaml
```

Now you'll see:

```
--- RECENT DEPOSITS (EVM → Canton) -----------------------------------
[1] Offset: 55 | 2025-12-13T12:01:00Z
    Amount:    100.0000000000 tokens
    Recipient: 66a5661b972d0d9b75b3...
    EVM TX:    0xe1d8fcbbf4f01b64...

--- CURRENT HOLDINGS -------------------------------------------------
[1] Owner: BridgeIssuer::122066a5661b97...
    Balance:  100.0000000000 PROMPT
    Token ID: Wayfinder PROMPT
    CID:      00b2b6f47e7a007d...

======================================================================
Summary: 1 holding(s)
======================================================================
```

#### Terminal 1: Execute Withdrawal

Select **3** and withdraw 50 tokens:

```
Select option: 3
Enter withdrawal amount [50]: 50
```

#### Terminal 2: Verify Withdrawal

```bash
go run scripts/bridge-activity.go -config .test-config.yaml
```

```
--- RECENT WITHDRAWALS (Canton → EVM) --------------------------------
[1] Offset: 60 | 2025-12-13T12:02:00Z
    Amount:    50.0000000000 tokens
    Recipient: 0x70997970C51812dc3A01...
    EVM TX:    0x0a3076c17b700226...

--- CURRENT HOLDINGS -------------------------------------------------
[1] Owner: BridgeIssuer::122066a5661b97...
    Balance:  50.0000000000 PROMPT
    Token ID: Wayfinder PROMPT
    CID:      003b74f22703befe...

======================================================================
Summary: 1 holding(s)
======================================================================
```

---

## Command-Line Examples

For scripting or CI/CD, use command-line flags:

```bash
# Start services only (no tests)
./scripts/test-bridge.sh --start-only

# Run full test (100 deposit + 50 withdraw)
./scripts/test-bridge.sh --full-test

# Deposit specific amount
./scripts/test-bridge.sh --skip-setup --deposit 75

# Withdraw specific amount
./scripts/test-bridge.sh --skip-setup --withdraw 30

# View status
./scripts/test-bridge.sh --status

# Stop services
./scripts/test-bridge.sh --stop

# Clean and run full test
./scripts/test-bridge.sh --clean --full-test
```

### Bridge Activity Script Options

```bash
# Local Docker (after test-bridge.sh creates .test-config.yaml)
go run scripts/bridge-activity.go -config .test-config.yaml

# With more history (increase lookback)
go run scripts/bridge-activity.go -config .test-config.yaml -lookback 5000

# Limit results
go run scripts/bridge-activity.go -config .test-config.yaml -limit 5

# Debug mode (shows all contracts)
go run scripts/bridge-activity.go -config .test-config.yaml -debug

# For DevNet
go run scripts/bridge-activity.go -config config.devnet.yaml

# For Mainnet
go run scripts/bridge-activity.go -config config.mainnet.yaml
```

---

## Troubleshooting

### Services Won't Start

```bash
# Clean everything and retry
./scripts/test-bridge.sh --clean
./scripts/test-bridge.sh --start-only
```

### "No CIP56Holding found" on Withdrawal

You need to deposit first before you can withdraw:

```bash
./scripts/test-bridge.sh --skip-setup --deposit 100
./scripts/test-bridge.sh --skip-setup --withdraw 50
```

### Multiple Holdings / "Insufficient balance"

Each deposit creates a **separate** `CIP56Holding` contract. If you see "Insufficient balance" even though you deposited enough total:

```
>>> Finding CIP56Holding on Canton...
    Found holding CID: 00b2b6f47e7a007d...
    Available balance: 5 tokens
✗ Insufficient balance: requested 12 but only 5 available
```

The script selects the holding with the **largest balance** for withdrawals. If you have multiple smaller holdings, you may need to withdraw from each individually.

To see all your holdings:
```bash
go run scripts/bridge-activity.go -config .test-config.yaml
```

Look for the "CURRENT HOLDINGS" section to see all holdings and their balances.

### "BridgeIssuer party not found"

Bootstrap may have failed. Check logs:

```bash
docker logs bootstrap
```

Then clean and restart:

```bash
./scripts/test-bridge.sh --clean --start-only
```

### Deposit Confirmation Timed Out

If you see:
```
>>> Waiting for deposit confirmation...
..............................
⚠ Deposit confirmation timed out after 60s
```

This usually means the relayer's database is out of sync with Anvil. This happens when:
- Anvil was restarted (resets to block 0)
- But the relayer database still has old block numbers

The relayer polls from its last known block, which may be higher than Anvil's current block.

**Fix:** Clean restart to reset the database:

```bash
# From interactive menu: Select option 8 (Clean & restart)
./scripts/test-bridge.sh

# Or from command line:
./scripts/test-bridge.sh --clean --start-only
```

**To verify the issue**, check if relayer's block > Anvil's block:
```bash
# Check Anvil's current block
cast block-number --rpc-url http://localhost:8545

# Check relayer's last block (look for "from_block" or "last_block")
docker logs canton-bridge-relayer 2>&1 | grep -E "from_block|last_block"
```

If relayer's block is higher than Anvil's block, clean restart is needed.

### Bridge Activity Shows "No deposits found"

The script only looks back 1000 offsets by default. Increase lookback:

```bash
go run scripts/bridge-activity.go -config .test-config.yaml -lookback 10000
```

### Docker Build Downloads Every Time

Make sure `.dockerignore` exists (excludes `.git/`, build artifacts, etc.)

---

## Service Endpoints

When running locally, these endpoints are available:

| Service | URL | Description |
|---------|-----|-------------|
| Anvil RPC | http://localhost:8545 | Ethereum JSON-RPC |
| Canton HTTP | http://localhost:5013 | Canton HTTP JSON API |
| Canton gRPC | localhost:5011 | Canton Ledger API (gRPC) |
| Relayer API | http://localhost:8080 | Bridge relayer REST API |
| Relayer Health | http://localhost:8080/health | Health check |
| Metrics | http://localhost:9090/metrics | Prometheus metrics |
| Mock OAuth2 | http://localhost:8088 | OAuth2 token server |
| PostgreSQL | localhost:5432 | Database (user: postgres, pass: p@ssw0rd) |

