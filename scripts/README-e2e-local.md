# E2E Local Testing Scripts

This directory contains end-to-end testing scripts for the Canton-Ethereum bridge in a fully local Docker environment.

## Available Scripts

### 1. `e2e-local.go` - Go-based E2E Test

Full-featured E2E test written in Go with comprehensive error handling and detailed output.

```bash
go run scripts/e2e-local.go [--cleanup] [--skip-docker] [--verbose]
```

**Features:**
- Complete type-safe implementation
- Detailed error messages
- Proper Wei/decimal conversions
- Database connectivity for whitelisting
- Configurable via `config.e2e-local.yaml`

### 2. `e2e-local.sh` - Bash + Cast E2E Test

Lightweight E2E test using `cast` (Foundry) commands for all ERC20 interactions.

```bash
./scripts/e2e-local.sh [--cleanup] [--skip-docker] [--verbose]
```

**Features:**
- Uses `cast` commands for ERC20 operations
- No Go compilation needed
- Great for quick testing and CI pipelines
- Demonstrates MetaMask-compatible workflows
- Uses shared libraries in `scripts/lib/`

### 3. `metamask-test.sh` - Interactive MetaMask Testing ⭐

Interactive test script that prepares the environment and pauses for manual MetaMask testing.

```bash
./scripts/metamask-test.sh [--cleanup] [--skip-docker] [--verbose]
```

**Features:**
- Automated setup (deposits, registration, etc.)
- Pauses for manual MetaMask interaction
- Provides detailed MetaMask setup instructions
- Verifies results after manual testing
- Perfect for UX validation

**See**: [MetaMask Testing Guide](README-metamask-test.md) for detailed instructions

**Requirements:**
- [Foundry](https://getfoundry.sh/) (`cast` command)
- `jq` (JSON processing)
- `psql` (PostgreSQL client)
- Docker and Docker Compose
- MetaMask browser extension

## Shared Libraries

The bash scripts use shared libraries located in `scripts/lib/`:

- **`common.sh`** - Color definitions and print utilities
- **`config.sh`** - Configuration and environment variables
- **`services.sh`** - Docker and service health checks
- **`bridge.sh`** - Bridge operations (register, deposit, transfer, etc.)

This modular approach makes it easy to create new test scripts by reusing common functionality.

## Test Flow

Both scripts execute the same test flow:

1. **Start Docker services** (optional)
   - Anvil (local Ethereum)
   - Canton (local ledger)
   - PostgreSQL
   - Mock OAuth2 server
   - Relayer + API Server

2. **Wait for services** to be healthy

3. **Verify token balances** on Anvil

4. **Whitelist users** in database

5. **Register users** on API server
   - Creates Canton parties and fingerprint mappings

6. **Deposit tokens** from Anvil to Canton
   - Approve bridge contract using `cast`
   - Call `depositToCanton()` using `cast`
   - Wait for relayer to process

7. **Verify Canton balances**
   - Query via `eth_call` on `/eth` endpoint

8. **Transfer tokens** between users on Canton
   - Send ERC20 `transfer()` via Canton's MetaMask-compatible API

9. **Verify final balances**

10. **Test ERC20 metadata endpoints**
    - `name()`, `symbol()`, `decimals()`, `totalSupply()`

## Configuration

Both scripts use `config.e2e-local.yaml` for configuration:

```yaml
users:
  user1:
    private_key: "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
    address: "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
  user2:
    private_key: "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
    address: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

contracts:
  token_address: "0x5FbDB2315678afecb367f032d93F642f64180aa3"
  bridge_address: "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"

amounts:
  total_deposit: "100.0"
  transfer_amount: "25.0"
```

## Command Examples

### Quick test (services already running)
```bash
./scripts/e2e-local.sh --skip-docker
```

### Full test with cleanup
```bash
./scripts/e2e-local.sh --cleanup
```

### Verbose output
```bash
go run scripts/e2e-local.go --verbose
```

### Test with services startup
```bash
./scripts/e2e-local.sh
```

## Troubleshooting

### Cast commands fail with "eth_feeHistory not available"

The bash script automatically uses `--legacy` flag to avoid EIP-1559 transactions. If you're running cast commands manually, add the `--legacy` flag:

```bash
cast send $TOKEN "transfer(address,uint256)" $TO $AMOUNT \
  --private-key $KEY --rpc-url http://localhost:8081/eth --legacy
```

### "Transaction receipt parsing error"

This is expected behavior. The API server returns `logs: null` instead of `logs: []` which causes cast to fail parsing the receipt. However, **the transaction still succeeds**. The scripts handle this by extracting the transaction hash from the error output.

### Balance not updating

- Check relayer logs: `docker logs canton-bridge-relayer`
- Ensure users are whitelisted in the database
- Verify fingerprint mappings exist on Canton

### Services not healthy

Wait longer or check individual service logs:
```bash
docker logs erc20-api-server
docker logs canton-bridge-relayer
docker logs canton
```

## CI/CD Integration

The bash script is ideal for CI pipelines:

```yaml
# Example GitHub Actions
- name: Install dependencies
  run: |
    curl -L https://foundry.paradigm.xyz | bash
    foundryup

- name: Run E2E test
  run: ./scripts/e2e-local.sh --cleanup
```

## Notes

- Both scripts use Anvil's default mnemonic accounts
- User1 is the token deployer and has initial supply
- Contract addresses are deterministic based on the deployment script
- The bash script demonstrates the exact workflow a MetaMask user would follow
