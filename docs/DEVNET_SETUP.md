# 5North DevNet Setup Guide

This guide explains how to connect the Canton-Ethereum Bridge to ChainSafe's 5North DevNet instead of a local Docker Canton instance.

---

## Quick Start (Pre-Configured Setup)

If the DevNet is already configured (JWT, DARs, party, user rights are shared), just:

```bash
# 1. Check JWT token is not expired (add == padding for base64url)
TOKEN=$(cat secrets/devnet-token.txt | cut -d'.' -f2)
EXP=$(echo "${TOKEN}==" | base64 -d | jq -r '.exp')
echo "Expires: $(date -r $EXP)"   # macOS
# echo "Expires: $(date -d @$EXP)" # Linux

# 2. Run the test (first time)
./scripts/test-bridge-devnet.sh --clean

# 3. For subsequent runs (skip bootstrap)
./scripts/test-bridge-devnet.sh --skip-bootstrap
```

**Pre-configured values:**
- Party: `daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c`
- JWT Subject: `RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients`
- Endpoint: `canton-ledger-api-grpc-dev1.chainsafe.dev:80` (plaintext gRPC)

If you need to set up from scratch, continue reading below.

---

## Overview

| Component | Local Docker | 5North DevNet |
|-----------|--------------|---------------|
| Canton Ledger API | `localhost:5011` | `canton-ledger-api-grpc-dev1.chainsafe.dev:80` |
| Authentication | Wildcard (no auth) | JWT (Auth0) |
| TLS | Disabled | Disabled (plaintext gRPC port 80) |
| Party Allocation | Via HTTP API | Via 5N Dashboard |
| DAR Upload | Via deployer container | Via 5N Dashboard |
| Domain/Synchronizer | Auto-configured | Query via API |

## Prerequisites

### 1. Tools Required
- Go 1.21+
- Docker & Docker Compose
- Foundry (`cast`, `forge`)
- `grpcurl` (for debugging)
- `jq` (for JSON parsing)

### 2. Access Requirements
- JWT token from ChainSafe infrastructure team
- Access to 5N Dashboard (for new setups)

---

## Step-by-Step Setup

### Step 1: Store JWT Token

Once you receive the JWT token from the infrastructure team, save it to:

```bash
# Save token to secrets directory (this file is gitignored)
echo "eyJhbGciOiJSUzI1NiI..." > secrets/devnet-token.txt
```

**Check token expiration:**
```bash
TOKEN=$(cat secrets/devnet-token.txt | cut -d'.' -f2)
EXP=$(echo "${TOKEN}==" | base64 -d | jq -r '.exp')
echo "Expires: $(date -r $EXP)"   # macOS
# echo "Expires: $(date -d @$EXP)" # Linux
```

### Step 2: Upload DARs to 5North

Build the DAR files locally:
```bash
cd contracts/canton-erc20/daml
./scripts/build-all.sh
```

Upload via 5N Dashboard or admin API:
- `bridge-wayfinder-*.dar`
- `bridge-core-*.dar`
- `cip56-token-*.dar`
- `common-*.dar`

**Note the package IDs** after upload (you'll need `bridge_package_id` for config).

### Step 3: Allocate Party

Via 5N Dashboard, create a party (e.g., "daml-autopilot" or "BridgeIssuer").

Note the full party ID format:
```
hint::1220fingerprint...
```

Example:
```
daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c
```

### Step 4: Grant User Rights (CRITICAL!)

**This step is often missed and causes "PermissionDenied" errors.**

The JWT's `sub` claim must be mapped to a Canton user with `canActAs` and `canReadAs` rights.

```bash
# Set variables
TOKEN=$(cat secrets/devnet-token.txt)
JWT_SUB="RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients"  # From JWT 'sub' claim
PARTY_ID="daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c"

# Grant rights
grpcurl -plaintext -H "Authorization: Bearer $TOKEN" -d "{
  \"user_id\": \"$JWT_SUB\",
  \"rights\": [
    {\"can_act_as\": {\"party\": \"$PARTY_ID\"}},
    {\"can_read_as\": {\"party\": \"$PARTY_ID\"}},
    {\"participant_admin\": {}}
  ]
}" canton-ledger-api-grpc-dev1.chainsafe.dev:80 \
  com.daml.ledger.api.v2.admin.UserManagementService/GrantUserRights
```

**Verify rights were granted:**
```bash
grpcurl -plaintext -H "Authorization: Bearer $TOKEN" -d "{
  \"user_id\": \"$JWT_SUB\"
}" canton-ledger-api-grpc-dev1.chainsafe.dev:80 \
  com.daml.ledger.api.v2.admin.UserManagementService/ListUserRights
```

Expected output:
```json
{
  "rights": [
    {"participant_admin": {}},
    {"can_act_as": {"party": "daml-autopilot::1220..."}},
    {"can_read_as": {"party": "daml-autopilot::1220..."}}
  ]
}
```

### Step 5: Get Domain/Synchronizer ID

```bash
grpcurl -plaintext -H "Authorization: Bearer $TOKEN" -d "{
  \"party\": \"$PARTY_ID\"
}" canton-ledger-api-grpc-dev1.chainsafe.dev:80 \
  com.daml.ledger.api.v2.StateService/GetConnectedSynchronizers
```

Example output:
```json
{
  "connected_synchronizers": [{
    "synchronizer_alias": "global",
    "synchronizer_id": "global-domain::1220be58c29e65de40bf273be1dc2b266d43a9a002ea5b18955aeef7aac881bb471a"
  }]
}
```

### Step 6: Update config.devnet.yaml

```yaml
canton:
  # 5North DevNet gRPC endpoint (plaintext)
  rpc_url: "canton-ledger-api-grpc-dev1.chainsafe.dev:80"
  ledger_id: ""
  domain_id: "global-domain::1220be58c29e65de40bf273be1dc2b266d43a9a002ea5b18955aeef7aac881bb471a"
  application_id: "canton-middleware"
  relayer_party: "daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c"
  bridge_package_id: "6694b7794de78352c5893ded301e6cf0080db02cbdfa7fab23cfd9e8a56eb73d"
  core_package_id: "60c0e065bc4bb98d0ef9507e28666e18c8af0f68c56fc224d4a7f423a20909bc"
  
  # TLS disabled (using plaintext port 80)
  tls:
    enabled: false
  
  # JWT Authentication
  auth:
    token_file: "secrets/devnet-token.txt"
```

---

## Running the Tests

### Quick Test (after setup is complete)

```bash
# First run - bootstraps Canton contracts
./scripts/test-bridge-devnet.sh --clean

# Subsequent runs - skip bootstrap
./scripts/test-bridge-devnet.sh --skip-bootstrap
```

### Manual Testing

```bash
# 1. Start local services (Anvil + Postgres)
docker compose -f docker-compose.yaml -f docker-compose.devnet.yaml up -d

# 2. Bootstrap Canton contracts (only once)
go run scripts/bootstrap-bridge.go -config config.devnet.yaml -issuer "$PARTY_ID"

# 3. Register user for deposits
go run scripts/register-user.go -config config.devnet.yaml -party "$PARTY_ID"

# 4. Start relayer
go run cmd/relayer/main.go -config config.devnet.yaml

# 5. Query holdings
go run scripts/query-holdings.go -config config.devnet.yaml -party "$PARTY_ID"
```

---

## Troubleshooting

### "Unauthenticated" Error

**Cause:** JWT token is missing, invalid, or expired.

**Fix:**
1. Check token exists: `cat secrets/devnet-token.txt`
2. Verify token isn't expired (see Step 1 for how to check expiration)
3. If expired, request a new token from the infrastructure team

### "PermissionDenied" Error

**Cause:** JWT user doesn't have `canActAs`/`canReadAs` rights.

**Fix:** Run the `GrantUserRights` command from Step 4 above.

### "INVALID_PRESCRIBED_SYNCHRONIZER_ID" Error

**Cause:** Wrong `domain_id` in config.

**Fix:**
1. Run `GetConnectedSynchronizers` to get the current domain ID
2. Update `domain_id` in `config.devnet.yaml`

### "Transport: authentication handshake failed" (ALPN Error)

**Cause:** gRPC-go 1.67+ TLS ALPN incompatibility.

**Fix:** Use plaintext port 80 instead of TLS port 443:
```yaml
canton:
  rpc_url: "canton-ledger-api-grpc-dev1.chainsafe.dev:80"
  tls:
    enabled: false
```

### Connection Timeout

**Cause:** Wrong endpoint or port not accessible.

**Fix:**
1. Test connectivity: `nc -zv canton-ledger-api-grpc-dev1.chainsafe.dev 80`
2. Verify endpoint with health check:
   ```bash
   grpcurl -plaintext canton-ledger-api-grpc-dev1.chainsafe.dev:80 grpc.health.v1.Health/Check
   ```

---

## Code Changes for DevNet

The following files have hardcoded user IDs that must match the JWT `sub` claim:

### scripts/bootstrap-bridge.go
```go
// Line ~355, ~415
UserId: "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients", // JWT subject
```

### scripts/register-user.go
```go
// Line ~335
UserId: "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients",
```

### scripts/initiate-withdrawal.go
```go
// Line ~399, ~449
UserId: "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients",
```

### pkg/canton/client.go
```go
// Multiple locations
UserId: "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients",
```

**TODO:** Make `UserId` configurable via `config.yaml` instead of hardcoded.

---

## Endpoints Reference

| Service | Endpoint |
|---------|----------|
| gRPC (plaintext) | `canton-ledger-api-grpc-dev1.chainsafe.dev:80` |
| gRPC (TLS) | `canton-ledger-api-grpc-dev1.chainsafe.dev:443` (may have ALPN issues) |

---

## Next Steps

After DevNet testing is successful:

1. **Sepolia Testing:** Update Ethereum config to use Sepolia testnet instead of Anvil
2. **Production Preparation:** 
   - Get production JWT credentials from infrastructure team
   - Connect to Canton mainnet
   - Deploy Ethereum contracts to mainnet

