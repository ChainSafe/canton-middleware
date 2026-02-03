# Setup and Testing Guide

This guide covers setting up and testing the Canton Middleware for local development, and configuring it for devnet/testnet/production deployments.

---

## Prerequisites

### Required Tools

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.23+ | https://go.dev/dl/ |
| Docker | Latest | https://docs.docker.com/get-docker/ |
| Docker Compose | v2+ | (included with Docker Desktop) |
| Foundry | Latest | `curl -L https://foundry.paradigm.xyz \| bash && foundryup` |
| jq | Latest | `brew install jq` (macOS) or `apt install jq` (Linux) |

### Canton Docker Image

The Canton image must be available locally:

```bash
# Check if image exists
docker images | grep canton

# If not present, pull from ChainSafe registry or build locally
# Option 1: Pull (if available)
docker pull chainsafe/canton:3.4.8

# Option 2: Build from canton-docker repo
git clone https://github.com/ChainSafe/canton-docker.git
cd canton-docker && ./build_container.sh
```

---

## Local Development

### Quick Start (Recommended)

The fastest way to get a fully working local environment:

```bash
cd canton-middleware

# Single command to start everything:
# - Generates CANTON_MASTER_KEY
# - Starts Docker containers (Canton, Anvil, PostgreSQL)
# - Waits for healthy status
# - Registers test users
# - Mints DEMO tokens (500 each)
# - Deposits PROMPT tokens (100 to User 1)
./scripts/setup/bootstrap-all.sh
```

This takes ~2-3 minutes. When complete, you'll see MetaMask configuration info.

### Manual Setup (Alternative)

If you need more control:

```bash
# 1. Set environment variables
export CANTON_MASTER_KEY=$(openssl rand -base64 32)
export SKIP_CANTON_SIG_VERIFY=true

# 2. Start services
docker compose up -d

# 3. Wait for healthy status (all should show "healthy")
docker compose ps

# 4. Run bootstrap scripts individually
go run scripts/setup/bootstrap-bridge.go -config config.e2e-local.yaml
go run scripts/setup/bootstrap-demo.go -config config.e2e-local.yaml \
    -native-package-id "<package_id>" \
    -user1-fingerprint "<fingerprint>" \
    -user2-fingerprint "<fingerprint>"
```

### Verify Local Setup

```bash
# Health checks
curl http://localhost:8081/health          # API Server: {"status":"ok"}
curl http://localhost:5013/v2/version      # Canton: version info

# Check balances
./scripts/utils/query-canton-holdings.sh

# Verify consistency between Canton and database
go run scripts/utils/verify-canton-holdings.go -config config.e2e-local.yaml -compare
```

### MetaMask Configuration (Local)

| Setting | Value |
|---------|-------|
| Network Name | Canton Local |
| RPC URL | `http://localhost:8081/eth` |
| Chain ID | 31337 |
| Currency Symbol | ETH |

**Test Accounts (Anvil defaults):**

| User | Address | Private Key |
|------|---------|-------------|
| User 1 | `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266` | `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` |
| User 2 | `0x70997970C51812dc3A010C7d01b50e0d17dc79C8` | `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d` |

**Token Addresses:**

| Token | Address |
|-------|---------|
| DEMO (native) | `0xDE30000000000000000000000000000000000001` |
| PROMPT (bridged) | `0x5FbDB2315678afecb367f032d93F642f64180aa3` |

---

## Testing

### Transfer Test (MetaMask)

1. Import User 1 into MetaMask using the private key
2. Add the Canton Local network
3. Import the DEMO token
4. Send DEMO to User 2's address
5. Verify the transfer:
   ```bash
   go run scripts/utils/verify-canton-holdings.go -config config.e2e-local.yaml -compare
   ```

### Bridge Test (Deposit PROMPT)

```bash
# Using cast (Foundry)
BRIDGE=0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512
TOKEN=0x5FbDB2315678afecb367f032d93F642f64180aa3
USER1_KEY=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
AMOUNT=$(cast --to-wei 100 ether)
FINGERPRINT=$(cast keccak 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266)

# Approve bridge
cast send $TOKEN "approve(address,uint256)" $BRIDGE $AMOUNT \
    --private-key $USER1_KEY --rpc-url http://localhost:8545

# Deposit to Canton
cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" $TOKEN $AMOUNT $FINGERPRINT \
    --private-key $USER1_KEY --rpc-url http://localhost:8545

# Wait for relayer, then check balance
sleep 10
./scripts/utils/query-canton-holdings.sh
```

### View Logs

```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f api-server
docker compose logs -f relayer
docker compose logs -f canton
```

### Clean Restart

```bash
# Full reset
docker compose down -v
./scripts/setup/bootstrap-all.sh
```

---

## DevNet / Testnet / Production

### Environment Comparison

| Setting | Local | DevNet | Production |
|---------|-------|--------|------------|
| Canton | Docker container | ChainSafe DevNet | Self-hosted participant |
| Ethereum | Anvil (local) | Sepolia | Mainnet |
| Auth | Mock OAuth2 | OAuth2 (Auth0) | OAuth2 (provider TBD) |
| Database | Docker PostgreSQL | Managed PostgreSQL | Managed PostgreSQL |
| CANTON_MASTER_KEY | Generated locally | Secure secret manager | Secure secret manager |

### Configuration Files

| Environment | Config File |
|-------------|-------------|
| Local | `config.e2e-local.yaml` |
| DevNet | `config.devnet.yaml` (create from example) |
| Production | `config.mainnet.yaml` (create from example) |

### Key Configuration Changes

#### 1. Canton Connection

**Local:**
```yaml
canton:
  rpc_url: "localhost:5011"
  http_url: "http://localhost:5013"
```

**DevNet/Production:**
```yaml
canton:
  rpc_url: "canton-ledger-api-grpc.<environment>.chainsafe.dev:443"
  http_url: "https://canton-ledger-api-http.<environment>.chainsafe.dev"
  tls:
    enabled: true
    ca_cert: "/path/to/ca.crt"           # Provided by network operator
    client_cert: "/path/to/client.crt"   # Provided by network operator
    client_key: "/path/to/client.key"    # Provided by network operator
```

#### 2. OAuth2 Authentication

**Local:**
```yaml
canton:
  auth:
    enabled: true
    token_url: "http://localhost:8088/oauth/token"
    client_id: "local-test-client"
    client_secret: "local-test-secret"
```

**DevNet/Production:**
```yaml
canton:
  auth:
    enabled: true
    token_url: "https://<auth-provider>/oauth/token"  # e.g., Auth0
    client_id: "<your-client-id>"                     # From secret manager
    client_secret: "<your-client-secret>"             # From secret manager
    audience: "https://canton-ledger-api.<env>"       # Required for Auth0
```

#### 3. Ethereum Connection

**Local:**
```yaml
ethereum:
  rpc_url: "http://localhost:8545"
  ws_url: "ws://localhost:8545"
```

**DevNet (Sepolia):**
```yaml
ethereum:
  rpc_url: "https://sepolia.infura.io/v3/<api-key>"
  ws_url: "wss://sepolia.infura.io/ws/v3/<api-key>"
```

**Production (Mainnet):**
```yaml
ethereum:
  rpc_url: "https://mainnet.infura.io/v3/<api-key>"
  ws_url: "wss://mainnet.infura.io/ws/v3/<api-key>"
```

#### 4. Database

**Local:**
```yaml
database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "postgres"
  database: "erc20_api"
```

**DevNet/Production:**
```yaml
database:
  host: "<managed-postgres-host>"
  port: 5432
  user: "<db-user>"                    # From secret manager
  password: "<db-password>"            # From secret manager
  database: "erc20_api"
  ssl_mode: "require"                  # Enable SSL for production
```

#### 5. Contract Addresses

**Local (Anvil):**
```yaml
contracts:
  bridge: "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
  prompt_token: "0x5FbDB2315678afecb367f032d93F642f64180aa3"
```

**DevNet/Production:**
```yaml
contracts:
  bridge: "<deployed-bridge-address>"       # Deploy to target network
  prompt_token: "<deployed-token-address>"  # Deploy or use existing
```

### Secrets Management

**Never commit secrets to git.** Use one of these approaches:

1. **Environment Variables:**
   ```bash
   export CANTON_AUTH_CLIENT_SECRET="..."
   export DATABASE_PASSWORD="..."
   export ETHEREUM_RELAYER_PRIVATE_KEY="..."
   ```

2. **Secrets File (gitignored):**
   ```bash
   # Create secrets/env.sh (already in .gitignore)
   source secrets/env.sh
   ```

3. **Secret Manager (Production):**
   - AWS Secrets Manager
   - HashiCorp Vault
   - Google Secret Manager

### Deployment Checklist

- [ ] Canton participant node accessible
- [ ] OAuth2 credentials configured
- [ ] TLS certificates in place (if required)
- [ ] Ethereum RPC accessible
- [ ] Database provisioned and migrated
- [ ] Contract addresses updated in config
- [ ] Relayer private key secured
- [ ] CANTON_MASTER_KEY in secret manager
- [ ] Health checks passing
- [ ] Monitoring/alerting configured

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| API server "Restarting" | `CANTON_MASTER_KEY` not set | `export CANTON_MASTER_KEY=$(openssl rand -base64 32)` then restart |
| "Canton not reachable" | Canton not healthy | `docker compose up -d canton` and wait 30s |
| "No holdings found" | Bootstrap not run | Run `./scripts/setup/bootstrap-all.sh` |
| "User not whitelisted" | Address not in whitelist table | Add to whitelist via PostgreSQL |
| Transfer fails | Insufficient balance or not registered | Check `/register` endpoint first |

### Check Service Status

```bash
# All containers
docker compose ps

# Canton health
docker inspect --format='{{.State.Health.Status}}' canton

# Canton synchronizer connection
curl -s http://localhost:5013/v2/state/connected-synchronizers | jq '.connectedSynchronizers | length'
# Should return 1

# Database connection
docker compose exec postgres psql -U postgres -d erc20_api -c "SELECT COUNT(*) FROM users;"
```

### Reset Everything

```bash
# Nuclear option - removes all data
docker compose down -v
docker volume prune -f

# Start fresh
./scripts/setup/bootstrap-all.sh
```

---

## Useful Commands

| Action | Command |
|--------|---------|
| Start services | `docker compose up -d` |
| Stop services | `docker compose down` |
| View logs | `docker compose logs -f <service>` |
| Check balances | `./scripts/utils/query-canton-holdings.sh` |
| Verify consistency | `go run scripts/utils/verify-canton-holdings.go -config config.e2e-local.yaml -compare` |
| Register native user | `go run scripts/testing/register-native-user.go -config config.e2e-local.yaml` |
| Run tests | `make test` |
