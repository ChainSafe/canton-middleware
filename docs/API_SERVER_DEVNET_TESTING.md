# ERC-20 API Server DevNet Testing Guide

This guide explains how to test the ERC-20 API server against ChainSafe's 5North DevNet with minimal token usage.

---

## Prerequisites

### Tools Required

```bash
# Go 1.21+
go version

# Docker (for local PostgreSQL)
docker --version

# Foundry (cast, forge)
cast --version

# jq (JSON parsing)
jq --version
```

### Tokens Required

| Token | Amount | Purpose |
|-------|--------|---------|
| Sepolia ETH | ~0.05 | Gas for EVM transactions |
| PROMPT (Sepolia) | 5 | Test deposits |

### Contract Addresses (Sepolia)

| Contract | Address |
|----------|---------|
| PROMPT Token | `0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e` |
| Canton Bridge | `0x363Dd0b55bf74D5b494B064AA8E8c2Ef5eD58d75` |

---

## Part 1: Cast Fundamentals

`cast` is Foundry's command-line tool for interacting with EVM chains. Here are the key commands for testing:

### Wallet Operations

```bash
# Generate a new wallet (for testing)
cast wallet new

# Get address from private key
cast wallet address --private-key <PRIVATE_KEY>

# Sign an EIP-191 message (used for API authentication)
cast wallet sign --private-key <PRIVATE_KEY> "message to sign"
```

### Reading Contract State

```bash
# Check ETH balance
cast balance <ADDRESS> --rpc-url https://sepolia.infura.io/v3/<API_KEY>

# Check ERC-20 token balance
cast call <TOKEN_ADDRESS> "balanceOf(address)(uint256)" <ADDRESS> --rpc-url <RPC_URL>

# Check allowance
cast call <TOKEN_ADDRESS> "allowance(address,address)(uint256)" <OWNER> <SPENDER> --rpc-url <RPC_URL>
```

### Writing Transactions

```bash
# Approve token spending
cast send <TOKEN_ADDRESS> "approve(address,uint256)" <SPENDER> <AMOUNT> \
  --private-key <PRIVATE_KEY> \
  --rpc-url <RPC_URL>

# Deposit to Canton bridge
cast send <BRIDGE_ADDRESS> "depositToCanton(bytes32,uint256)" <FINGERPRINT> <AMOUNT> \
  --private-key <PRIVATE_KEY> \
  --rpc-url <RPC_URL>
```

### Unit Conversion

```bash
# Convert to wei (18 decimals)
cast --to-wei 5        # 5000000000000000000

# Convert from wei
cast --from-wei 5000000000000000000    # 5

# Parse units with decimals
cast --to-unit 5000000000000000000 ether    # 5
```

### Hashing

```bash
# Keccak256 hash
cast keccak "hello"

# ABI encode
cast abi-encode "transfer(address,uint256)" 0x123... 1000000000000000000
```

---

## Part 2: Setup

### Step 1: Start Local PostgreSQL

```bash
# Create and start postgres container
docker run -d --name postgres-devnet \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=p@ssw0rd \
  -e POSTGRES_DB=erc20_api \
  -p 5432:5432 \
  postgres:15-alpine

# Wait for postgres to be ready
sleep 5

# Initialize the schema
docker exec -i postgres-devnet psql -U postgres -d erc20_api < deployments/api-server/schema.sql
```

### Step 2: Verify PostgreSQL

```bash
# Check tables exist
docker exec postgres-devnet psql -U postgres -d erc20_api -c "\dt"

# Expected output:
#              List of relations
#  Schema |     Name      | Type  |  Owner
# --------+---------------+-------+----------
#  public | token_metrics | table | postgres
#  public | users         | table | postgres
#  public | whitelist     | table | postgres
```

### Step 3: Start the API Server

```bash
# From project root
go run cmd/api-server/main.go -config config.api-server.devnet.yaml
```

### Step 4: Verify API Server Health

```bash
curl http://localhost:8081/health
# Expected: {"status":"ok"}
```

---

## Part 3: Testing the API

### Environment Variables

Set these for convenience:

```bash
# Sepolia RPC
export RPC_URL="https://sepolia.infura.io/v3/<YOUR_API_KEY>"

# Contract addresses
export TOKEN="0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e"
export BRIDGE="0x363Dd0b55bf74D5b494B064AA8E8c2Ef5eD58d75"

# Test wallets (use your own keys with Sepolia ETH + PROMPT)
export USER1_KEY="<PRIVATE_KEY_1>"
export USER2_KEY="<PRIVATE_KEY_2>"
export USER1_ADDRESS=$(cast wallet address --private-key $USER1_KEY)
export USER2_ADDRESS=$(cast wallet address --private-key $USER2_KEY)

# API endpoint
export API_URL="http://localhost:8081"
```

### Helper Function: Authenticated RPC Call

```bash
# Function to make authenticated RPC calls
rpc_call() {
  local method=$1
  local params=$2
  local private_key=$3
  
  # Create message to sign (method + timestamp)
  local timestamp=$(date +%s)
  local message="${method}:${timestamp}"
  
  # Sign the message
  local signature=$(cast wallet sign --private-key $private_key "$message" 2>/dev/null)
  local address=$(cast wallet address --private-key $private_key)
  
  # Make the RPC call
  curl -s -X POST $API_URL \
    -H "Content-Type: application/json" \
    -H "X-EVM-Address: $address" \
    -H "X-EVM-Signature: $signature" \
    -H "X-EVM-Message: $message" \
    -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}"
}
```

---

## Phase 1: Read-Only Tests (No Tokens)

### Test Token Metadata

```bash
# Get token name
curl -s -X POST $API_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"erc20_name","params":{},"id":1}' | jq

# Expected: {"jsonrpc":"2.0","result":{"name":"PROMPT"},"id":1}

# Get token symbol
curl -s -X POST $API_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"erc20_symbol","params":{},"id":1}' | jq

# Expected: {"jsonrpc":"2.0","result":{"symbol":"PROMPT"},"id":1}

# Get decimals
curl -s -X POST $API_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"erc20_decimals","params":{},"id":1}' | jq

# Expected: {"jsonrpc":"2.0","result":{"decimals":18},"id":1}

# Get total supply
curl -s -X POST $API_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"erc20_totalSupply","params":{},"id":1}' | jq

# Expected: {"jsonrpc":"2.0","result":{"total_supply":"0"},"id":1}
```

---

## Phase 2: User Registration (No Tokens, Just Gas)

### Register User 1

```bash
# Add to whitelist first (direct DB insert for testing)
docker exec postgres-devnet psql -U postgres -d erc20_api -c \
  "INSERT INTO whitelist (evm_address, note) VALUES ('$USER1_ADDRESS', 'Test user 1') ON CONFLICT DO NOTHING;"

# Register via API
rpc_call "user_register" "{}" $USER1_KEY | jq

# Expected response includes:
# - canton_party: The allocated Canton party ID
# - fingerprint: The EVM-Canton fingerprint mapping
# - mapping_cid: The Canton contract ID for the mapping
```

### Register User 2

```bash
# Whitelist
docker exec postgres-devnet psql -U postgres -d erc20_api -c \
  "INSERT INTO whitelist (evm_address, note) VALUES ('$USER2_ADDRESS', 'Test user 2') ON CONFLICT DO NOTHING;"

# Register
rpc_call "user_register" "{}" $USER2_KEY | jq
```

### Verify Registrations

```bash
# Check database
docker exec postgres-devnet psql -U postgres -d erc20_api -c \
  "SELECT evm_address, canton_party, fingerprint, balance FROM users;"
```

---

## Phase 3: Deposits (Uses 5 PROMPT)

### Check Current Balances

```bash
# Check PROMPT balance on Sepolia
cast call $TOKEN "balanceOf(address)(uint256)" $USER1_ADDRESS --rpc-url $RPC_URL
cast call $TOKEN "balanceOf(address)(uint256)" $USER2_ADDRESS --rpc-url $RPC_URL
```

### Get Fingerprints

```bash
# Get User1's fingerprint from database
USER1_FINGERPRINT=$(docker exec postgres-devnet psql -U postgres -d erc20_api -t -A -c \
  "SELECT fingerprint FROM users WHERE evm_address = '$USER1_ADDRESS';")

USER2_FINGERPRINT=$(docker exec postgres-devnet psql -U postgres -d erc20_api -t -A -c \
  "SELECT fingerprint FROM users WHERE evm_address = '$USER2_ADDRESS';")

echo "User1 fingerprint: $USER1_FINGERPRINT"
echo "User2 fingerprint: $USER2_FINGERPRINT"
```

### Approve Bridge Contract

```bash
# Approve bridge to spend 5 PROMPT (5 * 10^18 wei)
AMOUNT=$(cast --to-wei 5)

cast send $TOKEN "approve(address,uint256)" $BRIDGE $AMOUNT \
  --private-key $USER1_KEY \
  --rpc-url $RPC_URL

# Verify approval
cast call $TOKEN "allowance(address,address)(uint256)" $USER1_ADDRESS $BRIDGE --rpc-url $RPC_URL
```

### Deposit to Canton

```bash
# Deposit 3 PROMPT to User1
DEPOSIT1=$(cast --to-wei 3)

# Pad fingerprint to bytes32 (remove 0x, pad to 64 chars)
FP1_PADDED=$(echo $USER1_FINGERPRINT | sed 's/0x//' | xargs printf '%064s' | tr ' ' '0')

cast send $BRIDGE "depositToCanton(bytes32,uint256)" "0x$FP1_PADDED" $DEPOSIT1 \
  --private-key $USER1_KEY \
  --rpc-url $RPC_URL

# Deposit 2 PROMPT to User2 (from User1's wallet for simplicity)
DEPOSIT2=$(cast --to-wei 2)
FP2_PADDED=$(echo $USER2_FINGERPRINT | sed 's/0x//' | xargs printf '%064s' | tr ' ' '0')

cast send $BRIDGE "depositToCanton(bytes32,uint256)" "0x$FP2_PADDED" $DEPOSIT2 \
  --private-key $USER1_KEY \
  --rpc-url $RPC_URL
```

### Wait for Relayer Processing

The relayer needs to detect the deposit events and process them on Canton. This typically takes 30-60 seconds.

```bash
# Wait for processing
echo "Waiting 60 seconds for relayer to process deposits..."
sleep 60
```

### Verify Canton Balances

```bash
# Check User1 balance via API
rpc_call "erc20_balanceOf" "{}" $USER1_KEY | jq
# Expected: {"result":{"balance":"3.0","address":"0x..."}}

# Check User2 balance via API
rpc_call "erc20_balanceOf" "{}" $USER2_KEY | jq
# Expected: {"result":{"balance":"2.0","address":"0x..."}}

# Check total supply
curl -s -X POST $API_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"erc20_totalSupply","params":{},"id":1}' | jq
# Expected: {"result":{"total_supply":"5.0"}}
```

---

## Phase 4: Transfer (No Additional Tokens)

### Transfer 1 PROMPT from User1 to User2

```bash
# Get User2's address for transfer
rpc_call "erc20_transfer" "{\"to\":\"$USER2_ADDRESS\",\"amount\":\"1.0\"}" $USER1_KEY | jq

# Expected: {"result":{"tx_hash":"...","from":"0x...","to":"0x...","amount":"1.0"}}
```

### Verify Updated Balances

```bash
# User1 should now have 2 PROMPT
rpc_call "erc20_balanceOf" "{}" $USER1_KEY | jq

# User2 should now have 3 PROMPT
rpc_call "erc20_balanceOf" "{}" $USER2_KEY | jq
```

---

## Phase 5: Withdrawal (Returns Tokens)

Withdrawals are initiated on Canton and processed by the relayer to release tokens on EVM.

### Check Current Holdings

```bash
# Use the bridge-activity script to see holdings
go run scripts/bridge-activity.go -config config.devnet.yaml
```

### Initiate Withdrawal

```bash
# Get holding CID for User1
USER1_PARTY=$(docker exec postgres-devnet psql -U postgres -d erc20_api -t -A -c \
  "SELECT canton_party FROM users WHERE evm_address = '$USER1_ADDRESS';")

# Find holding and withdraw 1 PROMPT
go run scripts/initiate-withdrawal.go \
  -config config.devnet.yaml \
  -party "$USER1_PARTY" \
  -amount "1.0" \
  -evm-destination "$USER1_ADDRESS"
```

### Verify Withdrawal

```bash
# Wait for relayer to process
sleep 60

# Check EVM balance increased
cast call $TOKEN "balanceOf(address)(uint256)" $USER1_ADDRESS --rpc-url $RPC_URL

# Check Canton balance decreased
rpc_call "erc20_balanceOf" "{}" $USER1_KEY | jq
```

---

## Cleanup

```bash
# Stop and remove postgres container
docker stop postgres-devnet
docker rm postgres-devnet

# Stop API server (Ctrl+C in terminal running it)
```

---

## Troubleshooting

### "user not whitelisted"

Add the address to the whitelist:
```bash
docker exec postgres-devnet psql -U postgres -d erc20_api -c \
  "INSERT INTO whitelist (evm_address) VALUES ('<ADDRESS>');"
```

### "invalid signature"

Ensure you're signing the correct message format: `{method}:{timestamp}`

The timestamp must be within 5 minutes of server time.

### Deposits not appearing

1. Check relayer is running and connected to devnet
2. Check relayer logs for errors
3. Verify the fingerprint padding is correct (64 hex chars)

### "Unauthenticated" from Canton

JWT token may be expired. Check expiration:
```bash
TOKEN=$(cat secrets/devnet-token.txt | cut -d'.' -f2)
echo "${TOKEN}==" | base64 -d | jq -r '.exp' | xargs -I{} date -r {}
```

---

## Token Budget Summary

| Action | PROMPT Used | Sepolia ETH |
|--------|-------------|-------------|
| Register 2 users | 0 | ~0.01 |
| Deposit 5 PROMPT | 5 | ~0.02 |
| Transfer 1 PROMPT | 0 | 0 |
| Withdraw 1 PROMPT | 0 | ~0.02 |
| **Total** | **5** | **~0.05** |

After testing, you'll have:
- 1 PROMPT back on Sepolia (from withdrawal)
- 4 PROMPT on Canton (1 User1, 3 User2)

