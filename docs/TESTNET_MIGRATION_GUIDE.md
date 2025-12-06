# Canton Testnet Migration Guide

This guide walks through migrating the Canton-Ethereum Bridge from local Docker testing to Canton Network testnet.

## Overview

| Environment | Canton | Ethereum | Auth | Status |
|-------------|--------|----------|------|--------|
| Local | Docker (Canton 3.4.8) | Anvil | Wildcard | ✅ Working |
| Testnet (Self-Hosted) | Own participant + Testnet Sync | Sepolia/Holesky | Wildcard | 🎯 Recommended |
| Testnet (Shared) | Canton Network Infra | Sepolia/Holesky | JWT | Alternative |
| Production | Self-hosted participant | Ethereum Mainnet | Wildcard | Future |

---

## Prerequisites

Before starting, ensure you have:
- ✅ Local testing working (`./scripts/test-bridge.sh --clean` passes)
- ✅ Understanding of the bridge architecture
- Access to Canton Network documentation

---

## Authorization Strategy

### When to Use Each Auth Type

| Deployment Model | Auth Type | Why |
|------------------|-----------|-----|
| Local Docker dev | Wildcard | You control everything, API not exposed |
| Self-hosted participant (testnet/prod) | Wildcard | You operate the participant, only relayer connects |
| Canton Network shared infrastructure | JWT | Required by the network operator |

### Issuer Model Context

In the **issuer model**, the bridge relayer is the only application that interacts with the Canton Ledger API:
- Users interact with Canton **through the relayer**, not directly
- The relayer holds the issuer party and acts on behalf of users
- No multi-tenant access to the participant node is needed

**Key insight**: If you operate your own participant node (self-hosted), **wildcard auth is appropriate** because:
1. The participant operator (you) **is** the application operator
2. Only your relayer connects to the Ledger API
3. The Ledger API should be firewalled from external access

**JWT is required when**:
- Connecting to Canton Network's shared testnet/mainnet infrastructure
- The network operator provides JWT credentials for authentication

### Go Client JWT Support

The middleware already supports JWT authentication via `pkg/canton/client.go`:
- `GetAuthContext()` adds JWT bearer token to gRPC metadata
- `loadToken()` reads token from file specified in `auth.token_file` config
- No code changes needed - just configure the token file path

---

## Phase 1: Canton Testnet Access

There are two approaches to Canton testnet access:

### Option A: Self-Hosted Participant on Testnet (Recommended)

Run your own participant node and connect it to Canton Network's testnet synchronizer:
- **Auth**: Wildcard (same as local dev)
- **You control**: Participant node, party allocation, DAR uploads
- **Network provides**: Synchronizer endpoint to connect to

This is the recommended approach for the bridge relayer since production will also be self-hosted.

### Option B: Canton Network Shared Infrastructure

Use a participant node operated by Canton Network:
- **Auth**: JWT (credentials provided by network)
- **Network provides**: Participant endpoint, JWT token, TLS certs
- **You control**: Only your allocated party

---

### 1.1 Apply for Testnet Access

**For Option A (Self-Hosted)**:
1. Request access to connect your participant to the testnet synchronizer
2. You'll receive: Synchronizer endpoint URL, network configuration

**For Option B (Shared Infrastructure)**:
1. **Visit Canton Network**: https://www.canton.network/
2. **Request testnet access** through the developer portal
3. **Receive credentials**:
   - Participant node endpoint (gRPC URL)
   - TLS certificates (CA cert, client cert, client key)
   - JWT token or credentials
   - Party allocation instructions

### 1.2 Understand Your Participant Node (Option B)

You'll receive information about your participant node:

```
Participant Node: participant.testnet.canton.network:4001
Admin API: admin.testnet.canton.network:4002
HTTP/JSON API: https://json.testnet.canton.network:7575
```

### 1.3 Verify Connectivity

```bash
# Test gRPC connectivity (with TLS)
grpcurl -cacert ca.pem -cert client.pem -key client-key.pem \
  participant.testnet.canton.network:4001 list

# Or use the Canton HTTP API
curl -H "Authorization: Bearer $JWT_TOKEN" \
  https://json.testnet.canton.network:7575/v2/version
```

---

## Phase 2: DAML Contract Deployment

### 2.1 Build DAR Files

The DAML contracts need to be built and deployed to testnet:

```bash
cd contracts/canton-erc20/daml

# Build all packages
./scripts/build-all.sh

# This creates DAR files:
# - bridge-wayfinder-*.dar
# - bridge-core-*.dar
# - cip56-token-*.dar
# - common-*.dar
```

### 2.2 Deploy DARs to Testnet

**Option A: Via Canton Console**
```scala
// Connect to participant admin API
participant.dars.upload("path/to/bridge-wayfinder.dar")
participant.dars.upload("path/to/bridge-core.dar")
participant.dars.upload("path/to/cip56-token.dar")
participant.dars.upload("path/to/common.dar")
```

**Option B: Via HTTP API**
```bash
# Upload DAR via HTTP API
curl -X POST \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @bridge-wayfinder.dar \
  https://json.testnet.canton.network:7575/v2/packages
```

### 2.3 Record Package IDs

After deployment, get the package IDs:

```bash
curl -H "Authorization: Bearer $JWT_TOKEN" \
  https://json.testnet.canton.network:7575/v2/packages | jq '.packageIds'
```

Update `config.yaml` with the new package IDs:
```yaml
canton:
  bridge_package_id: "<testnet-bridge-wayfinder-package-id>"
  core_package_id: "<testnet-bridge-core-package-id>"
```

---

## Phase 3: Ethereum Testnet Setup

### 3.1 Choose a Testnet

| Testnet | Faucet | Block Explorer |
|---------|--------|----------------|
| Sepolia | https://sepoliafaucet.com/ | https://sepolia.etherscan.io/ |
| Holesky | https://holesky-faucet.pk910.de/ | https://holesky.etherscan.io/ |

### 3.2 Get Testnet ETH

```bash
# Fund your deployer/relayer address with testnet ETH
# Use the faucet links above
```

### 3.3 Deploy Bridge Contracts

```bash
cd contracts/ethereum

# Set environment variables
export PRIVATE_KEY="your-deployer-private-key"
export RPC_URL="https://sepolia.infura.io/v3/YOUR_API_KEY"

# Deploy contracts
forge script script/Deployer.s.sol --rpc-url $RPC_URL --broadcast --verify

# Record deployed addresses:
# - CantonBridge: 0x...
# - WrappedCantonToken: 0x...
```

### 3.4 Verify Contracts

```bash
# Verify on Etherscan
forge verify-contract <CONTRACT_ADDRESS> src/CantonBridge.sol:CantonBridge \
  --etherscan-api-key $ETHERSCAN_API_KEY \
  --chain sepolia
```

---

## Phase 4: Configuration Updates

### 4.1 Create Testnet Config

Create `config.testnet.yaml`:

```yaml
server:
  port: 8080
  metrics_port: 9090

database:
  host: "localhost"  # Or your DB host
  port: 5432
  name: "relayer_testnet"
  user: "postgres"
  password: "${DB_PASSWORD}"  # Use env var
  ssl_mode: "require"

ethereum:
  rpc_url: "https://sepolia.infura.io/v3/${INFURA_API_KEY}"
  chain_id: 11155111  # Sepolia
  bridge_address: "0x<YOUR_DEPLOYED_BRIDGE_ADDRESS>"
  token_address: "0x<YOUR_DEPLOYED_TOKEN_ADDRESS>"
  relayer_address: "0x<YOUR_RELAYER_ADDRESS>"
  relayer_private_key: "${RELAYER_PRIVATE_KEY}"  # Use env var!
  confirmations: 3  # Higher for testnet safety
  poll_interval: "15s"

canton:
  # For Option A (self-hosted): Use your participant's Ledger API
  # For Option B (shared): Use the endpoint provided by Canton Network
  rpc_url: "localhost:5011"  # or "participant.testnet.canton.network:4001"
  ledger_id: "canton-testnet"
  domain_id: "<testnet-domain-id>"
  application_id: "canton-eth-bridge-testnet"
  relayer_party: "<your-allocated-party-id>"
  bridge_package_id: "<testnet-bridge-wayfinder-package-id>"
  core_package_id: "<testnet-bridge-core-package-id>"
  
  # TLS Configuration
  # Option A (self-hosted): Can be disabled if running locally
  # Option B (shared): REQUIRED - certs provided by Canton Network
  tls:
    enabled: false  # Set to true for Option B
    cert_file: "/path/to/client.pem"
    key_file: "/path/to/client-key.pem"
    ca_file: "/path/to/ca.pem"
  
  # JWT Authentication  
  # Option A (self-hosted): Not needed - use wildcard auth
  # Option B (shared): REQUIRED - token provided by Canton Network
  auth:
    token_file: ""  # Set to "/path/to/jwt-token.txt" for Option B
```

### 4.2 Environment Variables

Create `.env.testnet`:

```bash
# Database
DB_PASSWORD=your-secure-password

# Ethereum
INFURA_API_KEY=your-infura-key
RELAYER_PRIVATE_KEY=your-relayer-private-key
ETHERSCAN_API_KEY=your-etherscan-key

# Canton
CANTON_JWT_TOKEN=your-jwt-token

# Don't commit this file!
```

### 4.3 TLS/JWT Support (Already Implemented)

The Go client in `pkg/canton/client.go` already supports both TLS and JWT authentication:

**TLS Support** (lines 37-46):
```go
if config.TLS.Enabled {
    tlsConfig, err := loadTLSConfig(&config.TLS)
    // ... loads cert, key, and CA from configured paths
    creds := credentials.NewTLS(tlsConfig)
    opts = append(opts, grpc.WithTransportCredentials(creds))
}
```

**JWT Support** (lines 78-93):
```go
func (c *Client) GetAuthContext(ctx context.Context) context.Context {
    token, err := c.loadToken()  // Reads from auth.token_file
    if token != "" {
        md := metadata.Pairs("authorization", "Bearer "+token)
        return metadata.NewOutgoingContext(ctx, md)
    }
    return ctx
}
```

**No code changes needed** - just configure the paths in your testnet config:
- Set `canton.tls.enabled: true` and provide cert paths
- Set `canton.auth.token_file` to point to your JWT token file

---

## Phase 5: Party Setup on Canton

### 5.1 Allocate Bridge Party

On testnet, party allocation may require admin privileges or be pre-allocated:

```bash
# Via HTTP API (if allowed)
curl -X POST \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"partyIdHint": "BridgeIssuer"}' \
  https://json.testnet.canton.network:7575/v2/parties

# Or via Canton console with admin access
```

### 5.2 Bootstrap Bridge Contracts

```bash
# Update the bootstrap script to use testnet config
go run scripts/bootstrap-bridge.go \
  -config config.testnet.yaml \
  -issuer "$RELAYER_PARTY" \
  -package "$BRIDGE_PACKAGE_ID"
```

### 5.3 Register Test Users

```bash
go run scripts/register-user.go \
  -config config.testnet.yaml \
  -party "$TEST_USER_PARTY"
```

---

## Phase 6: Testing on Testnet

### 6.1 Start the Relayer

```bash
# With testnet config
go run cmd/relayer/main.go -config config.testnet.yaml

# Or build and run
go build -o bin/relayer ./cmd/relayer
./bin/relayer -config config.testnet.yaml
```

### 6.2 Test Deposit Flow (EVM → Canton)

```bash
# 1. Get test tokens (mint or from faucet)
cast send $TOKEN "mint(address,uint256)" $USER "1000000000000000000000" \
  --rpc-url $RPC_URL --private-key $DEPLOYER_KEY

# 2. Approve bridge
cast send $TOKEN "approve(address,uint256)" $BRIDGE "1000000000000000000000" \
  --rpc-url $RPC_URL --private-key $USER_KEY

# 3. Deposit to Canton
cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
  $TOKEN "100000000000000000000" $CANTON_RECIPIENT \
  --rpc-url $RPC_URL --private-key $USER_KEY

# 4. Monitor relayer logs
# 5. Verify on Canton (query holdings)
```

### 6.3 Test Withdrawal Flow (Canton → EVM)

```bash
# 1. Query holdings on Canton
go run scripts/query-holdings.go -config config.testnet.yaml -party "$USER_PARTY"

# 2. Initiate withdrawal
go run scripts/initiate-withdrawal.go \
  -config config.testnet.yaml \
  -holding-cid "$HOLDING_CID" \
  -amount "50.0" \
  -evm-destination "$USER_EVM_ADDRESS"

# 3. Monitor relayer logs
# 4. Verify on Ethereum (check balance)
```

---

## Phase 7: Monitoring & Operations

### 7.1 Set Up Monitoring

```bash
# Prometheus metrics available at :9090/metrics
# Key metrics to watch:
# - relayer_transfers_total
# - relayer_transfer_latency_seconds
# - canton_stream_lag_seconds
# - ethereum_block_height
```

### 7.2 Logging

```bash
# Enable structured logging
export LOG_LEVEL=debug
export LOG_FORMAT=json
```

### 7.3 Alerts

Set up alerts for:
- Transfer failures
- Stream disconnections
- High latency
- Low ETH balance (for gas)

---

## Checklist

### Canton Testnet (Option A: Self-Hosted)
- [ ] Requested testnet synchronizer access
- [ ] Configured participant to connect to testnet synchronizer
- [ ] Verified participant is connected and synced
- [ ] Uploaded DAR packages to your participant
- [ ] Recorded package IDs
- [ ] Allocated bridge party
- [ ] Bootstrapped bridge contracts

### Canton Testnet (Option B: Shared Infrastructure)
- [ ] Applied for testnet access via Canton Network portal
- [ ] Received participant node credentials (endpoint, JWT, certs)
- [ ] Verified gRPC connectivity with TLS/JWT
- [ ] Uploaded DAR packages via HTTP API
- [ ] Recorded package IDs
- [ ] Received allocated party ID
- [ ] Bootstrapped bridge contracts

### Ethereum Testnet
- [ ] Chose testnet (Sepolia/Holesky)
- [ ] Funded deployer with testnet ETH
- [ ] Deployed bridge contracts
- [ ] Verified contracts on Etherscan
- [ ] Recorded contract addresses
- [ ] Funded relayer with testnet ETH

### Configuration
- [ ] Created `config.testnet.yaml`
- [ ] Set up environment variables
- [ ] (Option B only) Configured TLS certificates
- [ ] (Option B only) Configured JWT token file

### Testing
- [ ] Relayer starts and connects
- [ ] Deposit flow works (EVM → Canton)
- [ ] Withdrawal flow works (Canton → EVM)
- [ ] Monitoring is set up

---

## Troubleshooting

### "TLS handshake failed" (Option B only)
- Verify certificate paths are correct in config
- Check certificate expiration dates
- Ensure CA cert matches the participant node's certificate chain
- Verify `canton.tls.enabled: true` in config

### "JWT token expired" (Option B only)
- Request a new token from Canton Network
- Check token expiration time in the JWT payload
- Ensure token file path is correct in `canton.auth.token_file`

### "Party not found"
- Verify party was allocated on testnet
- Check party ID format matches testnet

### "Package not found"
- Verify DARs were uploaded successfully
- Check package IDs match deployed versions

### "Insufficient gas"
- Fund relayer address with more testnet ETH
- Check gas price settings

---

## Resources

- [Canton Network Documentation](https://www.canton.network/developer-resources)
- [Canton Testnet Portal](https://testnet.canton.network/) (if available)
- [Sepolia Faucet](https://sepoliafaucet.com/)
- [Foundry Documentation](https://book.getfoundry.sh/)

---

## Next Steps After Testnet

Once testnet testing is successful:

1. **Security Audit** - Conduct thorough security review
2. **Load Testing** - Test with higher transaction volumes
3. **Mainnet Preparation** - Plan mainnet deployment
4. **Documentation** - Update all docs for production
5. **Runbooks** - Create operational runbooks

