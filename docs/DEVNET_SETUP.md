# DevNet Setup Guide

This guide covers setting up the Canton-Ethereum bridge middleware for DevNet (Canton) and Sepolia (Ethereum), including bootstrapping tokens, registering users, and connecting MetaMask.

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Access to Canton DevNet (OAuth credentials)
- Sepolia ETH for gas (get from faucet)
- PROMPT tokens on Sepolia (for bridging)

## 1. Configuration Setup

### 1.1 Create Secrets File

Create `secrets/devnet-secrets.sh` with your credentials:

```bash
#!/bin/bash
# DevNet Secrets - DO NOT COMMIT THIS FILE

# Canton DevNet OAuth2 Credentials (Auth0)
export CANTON_AUTH_CLIENT_ID="your-client-id"
export CANTON_AUTH_CLIENT_SECRET="your-client-secret"
export CANTON_AUTH_AUDIENCE="https://canton-ledger-api-dev1.01.chainsafe.dev"
export CANTON_AUTH_TOKEN_URL="https://dev-2j3m40ajwym1zzaq.eu.auth0.com/oauth/token"

# Test User Private Keys (for MetaMask import)
export USER1_PRIVATE_KEY="your-user1-private-key"
export USER2_PRIVATE_KEY="your-user2-private-key"

# Test User Addresses (derived from private keys above)
export USER1_ADDRESS="0xYourUser1Address"
export USER2_ADDRESS="0xYourUser2Address"

# Sepolia Ethereum RPC (Infura)
export ETHEREUM_RPC_URL="https://sepolia.infura.io/v3/your-infura-key"
export ETHEREUM_WS_URL="wss://sepolia.infura.io/ws/v3/your-infura-key"

# Relayer Private Key (for signing Ethereum transactions)
export ETHEREUM_RELAYER_PRIVATE_KEY="your-relayer-private-key"
```

Make it executable:
```bash
chmod +x secrets/devnet-secrets.sh
```

### 1.2 Create Local Config Files

Copy and customize the config files:

```bash
# API Server config
cp config.api-server.devnet.yaml config.api-server.local-devnet.yaml

# Relayer config  
cp config.devnet.yaml config.local-devnet.yaml
```

Edit `config.local-devnet.yaml` to add:
- Sepolia RPC/WS URLs (from your Infura account)
- Relayer private key
- OAuth credentials (hardcoded, since Go doesn't expand env vars in YAML)

Edit `config.api-server.local-devnet.yaml` to add:
- OAuth credentials (hardcoded)
- Custom chain ID for MetaMask (e.g., `1155111101`)

> **Note:** These config files are in `.gitignore` to prevent committing secrets.

## 2. Start Infrastructure

### 2.1 Start PostgreSQL

```bash
docker run -d \
  --name postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=erc20_api \
  -p 5432:5432 \
  postgres:15

# Wait for it to start
sleep 5

# Initialize schema
docker exec -i postgres psql -U postgres -d erc20_api < pkg/db/schema.sql
```

### 2.2 Build and Start API Server

```bash
# Source secrets
source secrets/devnet-secrets.sh

# Build
go build -o bin/api-server ./cmd/api-server

# Start (runs on port 8081)
./bin/api-server -config config.api-server.local-devnet.yaml
```

### 2.3 Build and Start Relayer

In a new terminal:

```bash
# Source secrets
source secrets/devnet-secrets.sh

# Build
go build -o bin/relayer ./cmd/relayer

# Start (runs on port 8080)
./bin/relayer -config config.local-devnet.yaml
```

## 3. Register Users

Users need a `FingerprintMapping` contract on Canton to receive tokens.

### 3.1 Compute Fingerprints

Fingerprints are derived from EVM addresses (Keccak256 hash):

```bash
# User 1
go run -exec '' - "0x4768CCb3cE015698468A65bf8208b3f6919c769e" << 'EOF'
package main
import (
    "fmt"
    "os"
    "strings"
    "github.com/ethereum/go-ethereum/common"
    "golang.org/x/crypto/sha3"
)
func main() {
    addr := common.HexToAddress(os.Args[1])
    h := sha3.NewLegacyKeccak256()
    h.Write(addr.Bytes())
    fmt.Printf("%x\n", h.Sum(nil))
}
EOF
```

### 3.2 Register Users on Canton

```bash
# User 1
go run scripts/register-user.go \
  -config config.local-devnet.yaml \
  -fingerprint "USER1_FINGERPRINT" \
  -evm-address "0xUser1Address"

# User 2
go run scripts/register-user.go \
  -config config.local-devnet.yaml \
  -fingerprint "USER2_FINGERPRINT" \
  -evm-address "0xUser2Address"
```

### 3.3 Add Users to Database Whitelist

```bash
docker exec postgres psql -U postgres -d erc20_api << EOF
INSERT INTO whitelist (evm_address, added_at) VALUES
  ('0xUser1Address', NOW()),
  ('0xUser2Address', NOW())
ON CONFLICT (evm_address) DO NOTHING;

INSERT INTO users (evm_address, fingerprint, balance, demo_balance, created_at, updated_at) VALUES
  ('0xUser1Address', 'USER1_FINGERPRINT', 0, 0, NOW(), NOW()),
  ('0xUser2Address', 'USER2_FINGERPRINT', 0, 0, NOW(), NOW())
ON CONFLICT (evm_address) DO UPDATE SET
  fingerprint = EXCLUDED.fingerprint,
  updated_at = NOW();
EOF
```

## 4. Bootstrap DEMO Token (Native Canton Token)

DEMO is a native Canton token that doesn't require bridging.

```bash
# Mint 500 DEMO to User 1
go run scripts/bootstrap-demo.go \
  -config config.local-devnet.yaml \
  -fingerprint "USER1_FINGERPRINT" \
  -amount 500

# Mint 500 DEMO to User 2
go run scripts/bootstrap-demo.go \
  -config config.local-devnet.yaml \
  -fingerprint "USER2_FINGERPRINT" \
  -amount 500
```

Update database balances:
```bash
docker exec postgres psql -U postgres -d erc20_api << EOF
UPDATE users SET demo_balance = 500 WHERE evm_address = '0xUser1Address';
UPDATE users SET demo_balance = 500 WHERE evm_address = '0xUser2Address';
EOF
```

## 5. Bridge PROMPT Token (From Sepolia)

PROMPT is an ERC-20 token on Sepolia that gets bridged to Canton.

### 5.1 Prerequisites

- User needs PROMPT tokens on Sepolia
- User needs Sepolia ETH for gas

### 5.2 Bridge PROMPT

```bash
source secrets/devnet-secrets.sh

# Bridge 50 PROMPT from Sepolia to Canton for User 1
go run scripts/bridge-deposit.go
```

The script will:
1. Approve the bridge contract to spend PROMPT
2. Call `depositToCanton()` on the bridge
3. The relayer detects the deposit and mints PROMPT on Canton

Watch relayer logs to confirm:
```bash
tail -f logs/relayer.log
```

You should see:
```
Processing transfer {"direction": "ethereum_to_canton", "amount": "50000000000000000000"}
Creating pending deposit
Processing deposit and minting tokens
Transfer completed
```

## 6. Connect MetaMask

### 6.1 Add Custom Network

Add a new network in MetaMask:

| Field | Value |
|-------|-------|
| Network Name | Canton DevNet (Local) |
| RPC URL | `http://localhost:8081/eth` |
| Chain ID | `1155111101` |
| Currency Symbol | ETH |

> **Note:** Use a unique Chain ID (not 11155111) to avoid conflicts with Sepolia.

### 6.2 Import Test Accounts

Import User 1 and User 2 using their private keys from `secrets/devnet-secrets.sh`.

### 6.3 Add Token Contracts

Import tokens in MetaMask:

**PROMPT Token (bridged ERC-20):**
- Address: `0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e`
- Symbol: PROMPT
- Decimals: 18

**DEMO Token (native Canton):**
- Address: `0xDE30000000000000000000000000000000000001`
- Symbol: DEMO
- Decimals: 18

## 7. Making Transfers

### 7.1 Transfer via MetaMask

1. Select the Canton DevNet network
2. Select the sender account (User 1 or User 2)
3. Click "Send" on the token
4. Enter recipient address and amount
5. Confirm transaction

The API server intercepts the transaction and executes it on Canton.

### 7.2 Verify Balances

Check database:
```bash
docker exec postgres psql -U postgres -d erc20_api \
  -c "SELECT evm_address, balance as prompt, demo_balance as demo FROM users;"
```

Check via RPC:
```bash
# User 1 PROMPT balance
curl -s -X POST http://localhost:8081/eth \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e","data":"0x70a08231000000000000000000000000USER1_ADDRESS_NO_0x"},"latest"],"id":1}'
```

## 8. Useful Scripts

| Script | Description |
|--------|-------------|
| `scripts/setup-devnet.sh` | Automated setup (start services, register users, bootstrap tokens) |
| `scripts/metamask-info-devnet.sh` | Display MetaMask connection details |
| `scripts/bridge-deposit.go` | Bridge PROMPT from Sepolia to Canton |
| `scripts/demo-activity.go` | Query DEMO holdings and events on Canton |
| `scripts/reconcile-demo.go` | Reconcile database with Canton events |
| `scripts/register-user.go` | Register a user on Canton |
| `scripts/bootstrap-demo.go` | Mint DEMO tokens to a user |

## 9. Troubleshooting

### API Server won't start
- Check PostgreSQL is running: `docker ps | grep postgres`
- Check config file has correct OAuth credentials
- Check Canton DevNet is accessible

### Relayer won't connect to Sepolia
- Verify `ETHEREUM_RPC_URL` is correct
- Check Infura API key is valid
- Ensure relayer has Sepolia ETH for gas

### MetaMask shows 0 balance
- Verify user is registered on Canton (`scripts/register-user.go`)
- Verify user is in database whitelist
- Check API server logs for errors

### Bridge deposit not processed
- Check relayer logs for errors
- Verify user has `FingerprintMapping` with correct package version
- Ensure deposit transaction was confirmed on Sepolia

### Package version mismatch error
```
INTERPRETATION_UPGRADE_ERROR_VALIDATION_FAILED
```
This means old contracts exist from a previous package version. The user needs to be re-registered with the current package, or old contracts need to be archived.

## 10. Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐
│    MetaMask     │────▶│   API Server    │──────┐
│  (Chain 1155...)│     │   (port 8081)   │      │
└─────────────────┘     └─────────────────┘      │
                                                  ▼
┌─────────────────┐     ┌─────────────────┐  ┌──────────┐
│    Sepolia      │────▶│    Relayer      │──▶│ Canton   │
│  (Chain 11155111)     │   (port 8080)   │  │ DevNet   │
└─────────────────┘     └─────────────────┘  └──────────┘
        │                       │
        │                       ▼
        │               ┌─────────────────┐
        └──────────────▶│   PostgreSQL    │
                        │   (port 5432)   │
                        └─────────────────┘
```

- **API Server**: Emulates Ethereum RPC for MetaMask, translates to Canton operations
- **Relayer**: Bridges events between Sepolia and Canton
- **PostgreSQL**: Caches balances and tracks processed events
- **Canton DevNet**: DAML ledger with token contracts
- **Sepolia**: Ethereum testnet with bridge and PROMPT contracts
