# Canton Bridge API Documentation

## API Endpoint

**Mainnet URL:** `https://middleware-api-prod1.02.chainsafe.dev/rpc`  
*(Replace with actual mainnet URL when available)*

**Health Check:** `GET /health`

---

## Overview

The Canton Bridge API provides a JSON-RPC 2.0 interface for interacting with bridged ERC-20 tokens on the Canton Network. It enables users to:

- **Register** with their Ethereum wallet (Web3 login)
- **Query balances** of bridged tokens
- **Transfer tokens** between users on Canton
- **Access token metadata** (name, symbol, decimals, total supply)

### Architecture: Issuer-Centric Model

The bridge uses an **issuer-centric model** where:

1. **Single Issuer Party**: All bridged tokens are managed by a single issuer (the bridge relayer party) on Canton
2. **Fingerprint Mapping**: Users are identified by a cryptographic fingerprint derived from their Ethereum address (`keccak256(address)`)
3. **CIP-56 Tokens**: Tokens follow the CIP-56 standard on Canton, enabling compliant asset management
4. **No Canton Keys Required**: Users authenticate with their existing Ethereum wallets—no Canton-specific keys needed

### How Bridging Works

```
┌─────────────┐      Deposit       ┌─────────────┐      Mint        ┌─────────────┐
│  Ethereum   │ ──────────────────▶│ Middleware  │ ───────────────▶ │   Canton    │
│   (ERC-20)  │                    │   Relayer   │                  │  (CIP-56)   │
└─────────────┘                    └─────────────┘                  └─────────────┘
       │                                  │                                │
       │  1. User deposits tokens         │  2. Relayer detects event      │
       │     to bridge contract           │     and mints on Canton        │
       │                                  │                                │
       │◀─────────────────────────────────│◀────────────────────────────────│
       │         Withdrawal               │          Burn                  │
       │  4. Relayer releases tokens      │  3. User initiates withdrawal  │
```

---

## Authentication

### Web3 Wallet Authentication (EIP-191)

All authenticated endpoints require an **EIP-191 personal signature** from the user's Ethereum wallet.

#### Required Headers

| Header | Description |
|--------|-------------|
| `X-Signature` | EIP-191 signature of the message (hex string with `0x` prefix) |
| `X-Message` | The signed message in format: `{method}:{timestamp}` |

#### Signing Example

```javascript
// Message format: "method_name:unix_timestamp"
const message = `erc20_balanceOf:${Math.floor(Date.now() / 1000)}`;

// Sign with wallet (MetaMask, ethers.js, etc.)
const signature = await wallet.signMessage(message);

// Send with request
fetch('/rpc', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Signature': signature,
    'X-Message': message
  },
  body: JSON.stringify({
    jsonrpc: '2.0',
    method: 'erc20_balanceOf',
    params: {},
    id: 1
  })
});
```

### Whitelisting

Before users can register, their Ethereum address must be **whitelisted** by an administrator. This provides access control for the bridge during controlled rollout phases.

---

## JSON-RPC 2.0 Methods

### Public Methods (No Authentication)

#### `erc20_name`

Returns the token name.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "erc20_name",
  "params": {},
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": "PROMPT",
  "id": 1
}
```

**Curl Example:**
```bash
curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "erc20_name", "params": {}, "id": 1}'
```

---

#### `erc20_symbol`

Returns the token symbol.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "erc20_symbol",
  "params": {},
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": "PROMPT",
  "id": 1
}
```

**Curl Example:**
```bash
curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "erc20_symbol", "params": {}, "id": 1}'
```

---

#### `erc20_decimals`

Returns the token decimals.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "erc20_decimals",
  "params": {},
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": 18,
  "id": 1
}
```

**Curl Example:**
```bash
curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "erc20_decimals", "params": {}, "id": 1}'
```

---

#### `erc20_totalSupply`

Returns the total supply of bridged tokens on Canton.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "erc20_totalSupply",
  "params": {},
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "totalSupply": "1000000.000000000000000000"
  },
  "id": 1
}
```

**Curl Example:**
```bash
curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "erc20_totalSupply", "params": {}, "id": 1}'
```

---

### Authenticated Methods

#### `user_register`

Registers a new user with their Ethereum wallet. Requires the user's address to be whitelisted.

**Authentication:** Required (EIP-191 signature)

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "user_register",
  "params": {},
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "party": "daml-autopilot::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b",
    "fingerprint": "0x97226fafd8c54f2ba4556c78d1e7d235b08918b3fbd8e84d023bc2375cd46609",
    "mappingCid": "00abc123..."
  },
  "id": 1
}
```

**Errors:**
- `-32004` - Address not whitelisted
- `-32005` - User already registered

**Curl Example:**
```bash
MESSAGE="user_register:$(date +%s)"
SIGNATURE=$(cast wallet sign --private-key $PRIVATE_KEY "$MESSAGE")

curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -H "X-Signature: $SIGNATURE" \
  -H "X-Message: $MESSAGE" \
  -d '{"jsonrpc": "2.0", "method": "user_register", "params": {}, "id": 1}'
```

---

#### `erc20_balanceOf`

Returns the token balance for the authenticated user.

**Authentication:** Required (EIP-191 signature)

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "erc20_balanceOf",
  "params": {},
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "balance": "100.000000000000000000",
    "address": "0x4768CCb3cE015698468A65bf8208b3f6919c769e"
  },
  "id": 1
}
```

**Curl Example:**
```bash
MESSAGE="erc20_balanceOf:$(date +%s)"
SIGNATURE=$(cast wallet sign --private-key $PRIVATE_KEY "$MESSAGE")

curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -H "X-Signature: $SIGNATURE" \
  -H "X-Message: $MESSAGE" \
  -d '{"jsonrpc": "2.0", "method": "erc20_balanceOf", "params": {}, "id": 1}'
```

---

#### `erc20_transfer`

Transfers tokens from the authenticated user to another registered user.

**Authentication:** Required (EIP-191 signature)

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `to` | string | Yes | Recipient's Ethereum address |
| `amount` | string | Yes | Amount to transfer (with decimals, e.g., "10.5") |

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "erc20_transfer",
  "params": {
    "to": "0x79B3ff7ca5D5eeeF4d60bcEcD5C1294e0F328431",
    "amount": "10.0"
  },
  "id": 1
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "success": true,
    "txId": "1220abc123..."
  },
  "id": 1
}
```

**Errors:**
- `-32001` - Unauthorized (user not registered)
- `-32002` - Recipient not found/registered
- `-32003` - Insufficient funds
- `-32602` - Invalid parameters

**Curl Example:**
```bash
MESSAGE="erc20_transfer:$(date +%s)"
SIGNATURE=$(cast wallet sign --private-key $PRIVATE_KEY "$MESSAGE")

curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -H "X-Signature: $SIGNATURE" \
  -H "X-Message: $MESSAGE" \
  -d '{
    "jsonrpc": "2.0",
    "method": "erc20_transfer",
    "params": {"to": "0x79B3ff7ca5D5eeeF4d60bcEcD5C1294e0F328431", "amount": "10.0"},
    "id": 1
  }'
```

---

## Error Codes

| Code | Message | Description |
|------|---------|-------------|
| `-32700` | Parse error | Invalid JSON |
| `-32600` | Invalid Request | Invalid JSON-RPC request |
| `-32601` | Method not found | Unknown method |
| `-32602` | Invalid params | Invalid method parameters |
| `-32603` | Internal error | Server error |
| `-32001` | Unauthorized | Authentication failed or user not registered |
| `-32002` | Not found | Resource not found |
| `-32003` | Insufficient funds | Not enough balance for transfer |
| `-32004` | Address not whitelisted | User address not on whitelist |
| `-32005` | User already registered | Cannot register twice |

---

## OpenAPI Specification

```yaml
openapi: 3.0.3
info:
  title: Canton Bridge API
  description: JSON-RPC 2.0 API for Canton-Ethereum token bridge
  version: 1.0.0
  contact:
    name: ChainSafe Systems
    url: https://chainsafe.io

servers:
  - url: https://middleware-api-prod1.02.chainsafe.dev/
    description: Mainnet API

paths:
  /health:
    get:
      summary: Health check
      responses:
        '200':
          description: Service is healthy
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
                    example: "ok"

  /rpc:
    post:
      summary: JSON-RPC 2.0 endpoint
      description: |
        All API methods are accessed via this single endpoint using JSON-RPC 2.0 protocol.
        
        **Public methods:** erc20_name, erc20_symbol, erc20_decimals, erc20_totalSupply
        
        **Authenticated methods:** user_register, erc20_balanceOf, erc20_transfer
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/JsonRpcRequest'
            examples:
              getBalance:
                summary: Get balance
                value:
                  jsonrpc: "2.0"
                  method: "erc20_balanceOf"
                  params: {}
                  id: 1
              transfer:
                summary: Transfer tokens
                value:
                  jsonrpc: "2.0"
                  method: "erc20_transfer"
                  params:
                    to: "0x79B3ff7ca5D5eeeF4d60bcEcD5C1294e0F328431"
                    amount: "10.0"
                  id: 1
      parameters:
        - name: X-Signature
          in: header
          description: EIP-191 signature (required for authenticated methods)
          schema:
            type: string
            example: "0x1234567890abcdef..."
        - name: X-Message
          in: header
          description: Signed message in format "method:timestamp"
          schema:
            type: string
            example: "erc20_balanceOf:1703001234"
      responses:
        '200':
          description: JSON-RPC response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/JsonRpcResponse'

components:
  schemas:
    JsonRpcRequest:
      type: object
      required:
        - jsonrpc
        - method
        - id
      properties:
        jsonrpc:
          type: string
          enum: ["2.0"]
        method:
          type: string
          enum:
            - erc20_name
            - erc20_symbol
            - erc20_decimals
            - erc20_totalSupply
            - erc20_balanceOf
            - erc20_transfer
            - user_register
        params:
          type: object
        id:
          oneOf:
            - type: string
            - type: integer

    JsonRpcResponse:
      type: object
      properties:
        jsonrpc:
          type: string
          enum: ["2.0"]
        result:
          description: Result on success
        error:
          $ref: '#/components/schemas/JsonRpcError'
        id:
          oneOf:
            - type: string
            - type: integer

    JsonRpcError:
      type: object
      properties:
        code:
          type: integer
        message:
          type: string
        data:
          description: Additional error data

    BalanceResult:
      type: object
      properties:
        balance:
          type: string
          example: "100.000000000000000000"
        address:
          type: string
          example: "0x4768CCb3cE015698468A65bf8208b3f6919c769e"

    TransferResult:
      type: object
      properties:
        success:
          type: boolean
        txId:
          type: string

    RegisterResult:
      type: object
      properties:
        party:
          type: string
        fingerprint:
          type: string
        mappingCid:
          type: string

    SupplyResult:
      type: object
      properties:
        totalSupply:
          type: string
          example: "1000000.000000000000000000"
```

---

## Example: Complete Flow

### 1. Check Whitelist Status & Register

```bash
# Sign message with your wallet
MESSAGE="user_register:$(date +%s)"
SIGNATURE=$(cast wallet sign --private-key $PRIVATE_KEY "$MESSAGE")

# Register
curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -H "X-Signature: $SIGNATURE" \
  -H "X-Message: $MESSAGE" \
  -d '{
    "jsonrpc": "2.0",
    "method": "user_register",
    "params": {},
    "id": 1
  }'
```

### 2. Check Balance

```bash
MESSAGE="erc20_balanceOf:$(date +%s)"
SIGNATURE=$(cast wallet sign --private-key $PRIVATE_KEY "$MESSAGE")

curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -H "X-Signature: $SIGNATURE" \
  -H "X-Message: $MESSAGE" \
  -d '{
    "jsonrpc": "2.0",
    "method": "erc20_balanceOf",
    "params": {},
    "id": 1
  }'
```

### 3. Transfer Tokens

```bash
MESSAGE="erc20_transfer:$(date +%s)"
SIGNATURE=$(cast wallet sign --private-key $PRIVATE_KEY "$MESSAGE")

curl -X POST https://middleware-api-prod1.02.chainsafe.dev/rpc \
  -H "Content-Type: application/json" \
  -H "X-Signature: $SIGNATURE" \
  -H "X-Message: $MESSAGE" \
  -d '{
    "jsonrpc": "2.0",
    "method": "erc20_transfer",
    "params": {
      "to": "0x79B3ff7ca5D5eeeF4d60bcEcD5C1294e0F328431",
      "amount": "25.5"
    },
    "id": 1
  }'
```

---

## Rate Limits

| Endpoint | Limit |
|----------|-------|
| Public methods | 100 requests/minute |
| Authenticated methods | 50 requests/minute per user |

---

## Support

For issues or questions, please contact:
- **GitHub:** [ChainSafe/canton-middleware](https://github.com/ChainSafe/canton-middleware)
- **Email:** support@chainsafe.io

