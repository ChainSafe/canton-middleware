# Wayfinder Deployment Requirements

> **Note:** This document was written for the Wayfinder showcase and may be outdated. Key changes since this was written:
> - All users are now **external parties** using the **Interactive Submission API** (no ~200 internal party limit)
> - Bootstrap flow has changed -- see [Local Interop Testing](LOCAL_INTEROP_TESTING.md) and [DevNet Interop Testing](DEVNET_INTEROP_TESTING.md)
> - Canton signing keys use **secp256k1** (not ed25519)

This document outlines what Wayfinder needs to prepare for:
1. **Single Transfer Showcase** - using ChainSafe infrastructure
2. **Full Production Deployment** - as participant node owner and issuer

---

## Table of Contents

- [Single Transfer Showcase](#single-transfer-showcase)
- [Full Production Deployment](#full-production-deployment)
  - [Infrastructure Components](#infrastructure-components)
  - [Deployment Checklist](#deployment-checklist)
  - [Operational Requirements](#operational-requirements)
  - [Security Hardening](#security-hardening)
- [Summary](#summary)

---

## Single Transfer Showcase

### Using ChainSafe Infrastructure

For the single transfer showcase, **Wayfinder needs to prepare nothing technical**.

| Component | Who Provides | What's Needed |
|-----------|--------------|---------------|
| Canton Participant Node | ChainSafe | Already set up |
| Ethereum Node (Sepolia) | ChainSafe | Already set up |
| DAML Contracts (DARs) | ChainSafe | Upload to participant |
| Solidity Contracts | ChainSafe | Deploy to Sepolia |
| Relayer Middleware | ChainSafe | Run and configure |
| **PROMPT Token on Sepolia** | Wayfinder/ChainSafe | Deploy or use existing |

### Wayfinder's Involvement for Showcase

1. **Provide the Canton recipient fingerprint** - a `bytes32` representing the Canton party where tokens should be minted
2. **Optionally observe** the transaction on both chains
3. **No operational responsibility** during showcase

---

## Full Production Deployment

When Wayfinder deploys as the **participant node owner and issuer**, the complete stack must be deployed and operated.

### Infrastructure Components

```
┌─────────────────────────────────────────────────────────────────┐
│                     WAYFINDER INFRASTRUCTURE                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────┐   ┌──────────────────┐   ┌──────────────┐ │
│  │  Canton          │   │   API Server     │   │  PostgreSQL  │ │
│  │  Participant     │   │   (Port 8081)    │   │  Database    │ │
│  │  Node            │   │                  │   │              │ │
│  │                  │   │  • /eth JSON-RPC │   │  • Users     │ │
│  │  Ports:          │   │  • /register     │   │  • Balances  │ │
│  │  • 5011 gRPC     │   │  • MetaMask      │   │  • Transfers │ │
│  │  • 5013 HTTP     │   │    compatible    │   │              │ │
│  └────────┬─────────┘   └────────┬─────────┘   └──────┬───────┘ │
│           │                      │                     │         │
│           │             ┌────────┴─────────┐          │         │
│           │             │     Relayer      │          │         │
│           │             │  (Bridge Events) │          │         │
│           │             └────────┬─────────┘          │         │
│           └──────────────────────┼────────────────────┘         │
│                                  │                               │
│  ┌──────────────────────────────────────────────────────────────┤
│  │                        ETHEREUM                               │
│  │  ┌─────────────────┐   ┌─────────────────┐                   │
│  │  │ CantonBridge    │   │ PROMPT Token    │                   │
│  │  │ Smart Contract  │   │ (existing ERC20)│                   │
│  │  └─────────────────┘   └─────────────────┘                   │
│  └──────────────────────────────────────────────────────────────┤
└─────────────────────────────────────────────────────────────────┘
```

**Two Go services:**
- **API Server** - Handles MetaMask connections, user registration, ERC-20 translation
- **Relayer** - Bridges PROMPT token between Ethereum ↔ Canton

---

### Deployment Checklist

#### A. Canton Participant Node

**Required Software:**
- Canton Distribution - Use `chainsafe/canton:3.4.8` Docker image (or newer)
- Minimum 8GB RAM, SSD storage

**Configuration File** (similar to `simple-topology.conf`):

```hocon
canton.participants.participant1 {
  storage.type = postgres  // Use postgres for production, not memory
  storage.config {
    url = "jdbc:postgresql://localhost:5432/canton"
    user = "canton"
    password = "${CANTON_DB_PASSWORD}"
  }
  
  admin-api {
    address = "0.0.0.0"
    port = 5012
  }
  
  ledger-api {
    address = "0.0.0.0"  
    port = 5011
    // Wildcard auth since Wayfinder IS the operator
    auth-services = [{ type = wildcard }]
  }
  
  http-ledger-api {
    address = "0.0.0.0"
    port = 5013
  }
}
```

**Checklist:**
- [ ] Deploy Canton participant node
- [ ] Connect to Canton Network Testnet/Mainnet Synchronizer (endpoint provided by Canton Network)
- [ ] Configure firewall: Ledger API (5011) should NOT be publicly exposed - only the relayer connects
- [ ] Set up PostgreSQL storage for production durability

---

#### B. DAML Contracts (DARs)

**Build and upload these packages:**

```bash
cd contracts/canton-erc20/daml

# Build all packages
./scripts/setup/build-dars.sh

# Upload to participant (order matters):
# 1. common
# 2. cip56-token  
# 3. bridge-core
# 4. bridge-wayfinder
```

**Key Package IDs to record:**

| Package | Purpose |
|---------|---------|
| `cip56-token` | CIP56Manager for token minting/burning |
| `bridge-wayfinder` | WayfinderBridgeConfig for bridge operations |

---

#### C. Ethereum Contracts

**Deploy to Mainnet (production) or Sepolia (testnet):**

```bash
cd contracts/ethereum

export PRIVATE_KEY="<deployer-private-key>"
export RELAYER_ADDRESS="<wayfinder-relayer-address>"
export RPC_URL="https://mainnet.infura.io/v3/<YOUR_KEY>"

forge script script/Deploy.s.sol --rpc-url $RPC_URL --broadcast --verify
```

**Record deployed addresses:**
- `CantonBridge` contract address
- Token mapping configuration (PROMPT token → Canton token ID)

**Bridge Setup Transactions:**

```bash
# Add PROMPT token mapping
cast send $BRIDGE "addTokenMapping(address,bytes32,bool)" \
    $PROMPT_TOKEN $CANTON_TOKEN_ID false \
    --rpc-url $RPC_URL --private-key $RELAYER_KEY

# Grant MINTER_ROLE to bridge (if using wrapped token pattern)
cast send $TOKEN "grantRole(bytes32,address)" $MINTER_ROLE $RELAYER \
    --rpc-url $RPC_URL --private-key $RELAYER_KEY
```

---

#### D. API Server & Relayer

**Build the Go binaries:**

```bash
make build
# Creates: bin/api-server, bin/relayer
```

**Production Configuration** (`config.production.yaml`):

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  host: "localhost"
  port: 5432
  user: "relayer"
  password: "${DB_PASSWORD}"
  database: "canton_bridge"
  ssl_mode: "require"

ethereum:
  rpc_url: "https://mainnet.infura.io/v3/${INFURA_API_KEY}"
  ws_url: "wss://mainnet.infura.io/ws/v3/${INFURA_API_KEY}"
  chain_id: 1  # Mainnet
  bridge_contract: "0x<DEPLOYED_BRIDGE>"
  token_contract: "0x28d38df637db75533bd3f71426f3410a82041544"  # PROMPT
  relayer_private_key: "${RELAYER_PRIVATE_KEY}"
  confirmation_blocks: 12  # Mainnet finality
  gas_limit: 300000
  polling_interval: "15s"

canton:
  rpc_url: "localhost:5011"  # Your participant
  domain_id: "<SYNCHRONIZER_ID>"
  application_id: "wayfinder-bridge"
  relayer_party: "<WAYFINDER_ISSUER_PARTY>"
  bridge_package_id: "<BRIDGE_WAYFINDER_PACKAGE_ID>"
  bridge_module: "Wayfinder.Bridge"
  bridge_contract: "WayfinderBridgeConfig"
  tls:
    enabled: false  # Self-hosted, no TLS needed for local connection
  auth:
    type: "oauth2"
    token_url: "https://your-oauth-provider/token"
    client_id: "${OAUTH_CLIENT_ID}"
    client_secret: "${OAUTH_CLIENT_SECRET}"
  polling_interval: "1s"

bridge:
  max_transfer_amount: "1000000000000000000000000"  # 1M tokens
  min_transfer_amount: "1000000000000000"          # 0.001 tokens
  rate_limit_per_hour: 1000

monitoring:
  enabled: true
  metrics_port: 9090
```

**Environment Variables Required:**

```bash
# Encryption key for custodial Canton keys (generate securely)
export CANTON_MASTER_KEY=$(openssl rand -base64 32)

# Database credentials
export DB_PASSWORD="<secure-password>"

# Ethereum relayer private key
export RELAYER_PRIVATE_KEY="<hex-encoded-key>"

# OAuth2 credentials for Canton authentication
export OAUTH_CLIENT_ID="<client-id>"
export OAUTH_CLIENT_SECRET="<client-secret>"
```

---

#### E. Bootstrap Process

**One-time setup after deployment:**

```bash
# 1. Allocate the Wayfinder Issuer party
curl -X POST http://localhost:5013/v2/parties \
  -H 'Content-Type: application/json' \
  -d '{"partyIdHint": "WayfinderIssuer"}'

# Record: Party ID like "WayfinderIssuer::1220abc...def"

# 2. Run bootstrap script
go run scripts/setup/bootstrap-bridge.go \
  -config config.production.yaml \
  -issuer "WayfinderIssuer::1220..." \
  -package "<BRIDGE_WAYFINDER_PACKAGE_ID>"
```

**Bootstrap creates:**
- `CIP56Manager` contract (for PROMPT token minting/burning)
- `WayfinderBridgeConfig` contract (bridge configuration)

---

### Operational Requirements

| Aspect | Requirement |
|--------|-------------|
| **Uptime** | API Server and Relayer must run 24/7 |
| **Private Keys** | Securely store: Ethereum relayer key, `CANTON_MASTER_KEY` (HSM recommended) |
| **ETH Balance** | Keep relayer address funded for gas fees |
| **Monitoring** | Prometheus metrics at `:9090/metrics` |
| **Database Backup** | Regular PostgreSQL backups (users, balances, transfer state) |
| **Logs** | Structured JSON logging, shipped to observability stack |
| **TLS Termination** | HTTPS for API Server public endpoint |

---

### Security Hardening

- [ ] **Never expose Ledger API (5011) publicly** - only relayer connects
- [ ] **Use HTTPS/TLS** for Ethereum RPC endpoints
- [ ] **Store private keys in secrets manager** (Vault, AWS Secrets, etc.)
- [ ] **Enable PostgreSQL SSL**
- [ ] **Set up alerting** for:
  - Failed transfers
  - Stream disconnections
  - Low ETH balance
  - High transaction latency
- [ ] **Rate limiting** on bridge API endpoints
- [ ] **Audit logging** for all admin operations

---

## Summary

| Scenario | What Wayfinder Provides |
|----------|------------------------|
| **Showcase (ChainSafe infra)** | Just the recipient fingerprint; observe the transfer |
| **Production Deployment** | Full infrastructure: Canton node, relayer, contracts, monitoring |

### For Single Showcase Transfer

Wayfinder needs to provide:
1. A Canton party fingerprint (the `bytes32` recipient address)
2. Optionally, the amount of PROMPT to transfer

### For Full Production

Wayfinder becomes the **participant node operator** and **bridge issuer**, requiring:

1. ✅ Canton participant node infrastructure
2. ✅ PostgreSQL database
3. ✅ API Server deployment (MetaMask compatibility layer)
4. ✅ Relayer deployment (bridge event processing)
5. ✅ Ethereum contract deployment and configuration
6. ✅ Monitoring and operational procedures
7. ✅ Private key management (`CANTON_MASTER_KEY`, relayer key)

---

## Related Documentation

- [Architecture](./ARCHITECTURE.md) - System design and data flows
- [Local Interop Testing](./LOCAL_INTEROP_TESTING.md) - Full local E2E testing
- [API Documentation](./API_DOCUMENTATION.md) - Endpoint reference
