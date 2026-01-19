# Canton Bridge API Documentation

## API Endpoints

**Ethereum JSON-RPC (MetaMask Compatible):** `https://middleware-api-prod1.02.chainsafe.dev/eth`
**User Registration:** `https://middleware-api-prod1.02.chainsafe.dev/register`
**Health Check:** `GET /health`

*(Replace with actual mainnet URL when available)*

---

## Overview

The Canton Bridge API provides an **Ethereum-compatible JSON-RPC interface** for interacting with bridged ERC-20 tokens on the Canton Network. It enables users to:

- **Register** with their Ethereum wallet (Web3 login)
- **Query balances** of bridged tokens via standard ERC-20 methods
- **Transfer tokens** using Ethereum transactions
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

## Ethereum JSON-RPC Endpoint (`/eth`)

The `/eth` endpoint provides **MetaMask-compatible** Ethereum JSON-RPC methods. Connect to it just like you would connect to any Ethereum RPC endpoint.

### RPC Authentication

#### Transaction Submission

All transaction submissions via `eth_sendRawTransaction` require sender addresses to be whitelisted:

1. **Signature Verification**: Transaction signature is verified and sender address extracted using cryptographic recovery
2. **Whitelist Check**: Sender address is checked against the whitelist database
3. **Rejection**: Non-whitelisted transactions are rejected with a clear error message

To submit transactions, users must first:
1. Get their address whitelisted by an administrator
2. Register via the `/register` endpoint (see User Registration section)

#### Read Methods

Query methods remain **unauthenticated** for MetaMask compatibility. The following methods can be called by anyone:
- `eth_getBalance`, `eth_call`, `eth_getTransactionCount`, `eth_getLogs`
- `eth_getTransactionByHash`, `eth_getTransactionReceipt`, `eth_getBlockByNumber`
- All other read-only methods listed below

#### Error Messages

When transaction submission fails due to whitelist restrictions:

- **"sender address X is not whitelisted for transactions"** - The sender address needs to be whitelisted and registered
- **"invalid sender"** - Invalid transaction signature (signature does not match transaction data)
- **"whitelist check failed"** - Internal error during whitelist verification (check server logs)

### MetaMask Configuration

```javascript
// Add network to MetaMask
await window.ethereum.request({
  method: 'wallet_addEthereumChain',
  params: [{
    chainId: '0x7A69', // 31337 in hex (or your configured chain ID)
    chainName: 'Canton Bridge Network',
    rpcUrls: ['https://middleware-api-prod1.02.chainsafe.dev/eth'],
    nativeCurrency: {
      name: 'Synthetic ETH',
      symbol: 'sETH',
      decimals: 18
    }
  }]
});
```

### Supported Methods

The following standard Ethereum JSON-RPC methods are supported:

#### Read Methods
- `eth_chainId` - Returns the chain ID
- `eth_blockNumber` - Returns the latest block number
- `eth_gasPrice` - Returns the current gas price
- `eth_maxPriorityFeePerGas` - Returns the suggested priority fee
- `eth_estimateGas` - Estimates gas for a transaction
- `eth_getBalance` - Returns the ETH balance (synthetic for registered users)
- `eth_getTransactionCount` - Returns the nonce for an address
- `eth_getCode` - Returns the code at an address
- `eth_syncing` - Returns sync status (always false)
- `eth_call` - Executes a call without creating a transaction
- `eth_getTransactionByHash` - Returns a transaction by hash
- `eth_getTransactionReceipt` - Returns a transaction receipt
- `eth_getLogs` - Returns logs matching filter criteria
- `eth_getBlockByNumber` - Returns a block by number
- `net_version` - Returns the network ID
- `web3_clientVersion` - Returns the client version

#### Write Methods
- `eth_sendRawTransaction` - Submits a signed transaction

### ERC-20 Token Contract

The bridged ERC-20 token is available at the configured token address. You can interact with it using standard ERC-20 methods:

#### Standard ERC-20 Methods (via `eth_call`)

```javascript
// Get token balance
const balanceOf = await ethersProvider.call({
  to: tokenAddress,
  data: iface.encodeFunctionData('balanceOf', [userAddress])
});

// Get token name
const name = await ethersProvider.call({
  to: tokenAddress,
  data: iface.encodeFunctionData('name', [])
});

// Get token symbol
const symbol = await ethersProvider.call({
  to: tokenAddress,
  data: iface.encodeFunctionData('symbol', [])
});

// Get token decimals
const decimals = await ethersProvider.call({
  to: tokenAddress,
  data: iface.encodeFunctionData('decimals', [])
});

// Get total supply
const totalSupply = await ethersProvider.call({
  to: tokenAddress,
  data: iface.encodeFunctionData('totalSupply', [])
});
```

#### Transfer Tokens (via `eth_sendRawTransaction`)

```javascript
// Create and sign transaction
const tx = await wallet.signTransaction({
  to: tokenAddress,
  data: iface.encodeFunctionData('transfer', [recipientAddress, amount]),
  gasLimit: 21000,
  gasPrice: await provider.getGasPrice(),
  nonce: await provider.getTransactionCount(wallet.address),
  chainId: 31337
});

// Send transaction
const txHash = await provider.send('eth_sendRawTransaction', [tx]);

// Wait for receipt
const receipt = await provider.waitForTransaction(txHash);
```

---

## User Registration Endpoint (`/register`)

Before users can interact with bridged tokens, they must register their Ethereum address.

### Endpoint

**POST** `/register`

### Authentication

Registration requires an **EIP-191 personal signature** from the user's Ethereum wallet.

### Request Format

#### Option 1: Signature in Body

```json
{
  "signature": "0x...",
  "message": "registration:1234567890"
}
```

#### Option 2: Signature in Headers

**Headers:**
- `X-Signature`: EIP-191 signature (hex string with `0x` prefix)
- `X-Message`: The signed message

**Body:**
```json
{}
```

### Message Format

The message should be in the format: `{arbitrary_text}:{timestamp}`

Example: `registration:1234567890`

### Response

**Success (200 OK):**
```json
{
  "party": "relay::1220...",
  "fingerprint": "0x...",
  "mapping_cid": "0x..."
}
```

**Errors:**
- `401 Unauthorized` - Invalid signature or missing authentication
- `403 Forbidden` - Address not whitelisted
- `409 Conflict` - User already registered
- `500 Internal Server Error` - Registration failed

### JavaScript Example

```javascript
// Sign message
const timestamp = Math.floor(Date.now() / 1000);
const message = `registration:${timestamp}`;
const signature = await wallet.signMessage(message);

// Register user
const response = await fetch('/register', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Signature': signature,
    'X-Message': message
  },
  body: JSON.stringify({})
});

const result = await response.json();
console.log('Registered with fingerprint:', result.fingerprint);
```

### Whitelisting

Before users can register, their Ethereum address must be **whitelisted** by an administrator. This provides access control for the bridge during controlled rollout phases.

---

## Complete Integration Example

Here's a complete example using ethers.js v6:

```javascript
import { ethers } from 'ethers';

// Configuration
const RPC_URL = 'https://middleware-api-prod1.02.chainsafe.dev/eth';
const REGISTER_URL = 'https://middleware-api-prod1.02.chainsafe.dev/register';
const TOKEN_ADDRESS = '0x...'; // Your token address
const CHAIN_ID = 31337;

// ERC20 ABI (minimal)
const ERC20_ABI = [
  'function balanceOf(address) view returns (uint256)',
  'function transfer(address to, uint256 amount) returns (bool)',
  'function name() view returns (string)',
  'function symbol() view returns (string)',
  'function decimals() view returns (uint8)',
  'function totalSupply() view returns (uint256)'
];

async function main() {
  // Connect to the API
  const provider = new ethers.JsonRpcProvider(RPC_URL);

  // Connect wallet (e.g., from MetaMask or private key)
  const wallet = new ethers.Wallet('0x...', provider);

  // Step 1: Register user
  console.log('Registering user...');
  const timestamp = Math.floor(Date.now() / 1000);
  const message = `registration:${timestamp}`;
  const signature = await wallet.signMessage(message);

  const registerResponse = await fetch(REGISTER_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Signature': signature,
      'X-Message': message
    },
    body: JSON.stringify({})
  });

  if (!registerResponse.ok) {
    const error = await registerResponse.json();
    console.error('Registration failed:', error);
    return;
  }

  const registration = await registerResponse.json();
  console.log('Registered! Fingerprint:', registration.fingerprint);

  // Step 2: Get token contract
  const token = new ethers.Contract(TOKEN_ADDRESS, ERC20_ABI, wallet);

  // Step 3: Read token info
  const [name, symbol, decimals, balance] = await Promise.all([
    token.name(),
    token.symbol(),
    token.decimals(),
    token.balanceOf(wallet.address)
  ]);

  console.log(`Token: ${name} (${symbol})`);
  console.log(`Decimals: ${decimals}`);
  console.log(`Balance: ${ethers.formatUnits(balance, decimals)}`);

  // Step 4: Transfer tokens
  const recipientAddress = '0x...';
  const amount = ethers.parseUnits('10', decimals);

  console.log('Transferring tokens...');
  const tx = await token.transfer(recipientAddress, amount);
  console.log('Transaction hash:', tx.hash);

  const receipt = await tx.wait();
  console.log('Transfer confirmed in block:', receipt.blockNumber);
}

main().catch(console.error);
```

---

## Error Handling

### Registration Errors

| HTTP Status | Error | Description |
|------------|-------|-------------|
| 400 | Bad Request | Invalid JSON or missing required fields |
| 401 | Unauthorized | Invalid or missing signature |
| 403 | Forbidden | Address not whitelisted |
| 409 | Conflict | User already registered |
| 500 | Internal Server Error | Database or Canton connection error |

### Ethereum JSON-RPC Errors

Standard Ethereum JSON-RPC error codes:
- `-32700` - Parse error
- `-32600` - Invalid request
- `-32601` - Method not found
- `-32602` - Invalid params
- `-32603` - Internal error

---

## Rate Limiting

*(To be documented based on deployment requirements)*

---

## Support

For issues and questions:
- GitHub Issues: [chainsafe/canton-middleware](https://github.com/chainsafe/canton-middleware/issues)
