#!/bin/bash
# =============================================================================
# ⚠️  DEPRECATED - Use test-bridge.sh instead
# =============================================================================
# This script is deprecated. Please use the unified test script:
#
#   ./scripts/test-bridge.sh --config config.devnet.yaml
#   ./scripts/test-bridge.sh --config config.devnet.yaml --clean
#   ./scripts/test-bridge.sh --config config.devnet.yaml --skip-setup
#
# This file is kept for reference only.
# =============================================================================
#
# Canton-Ethereum Bridge DevNet Test Script
# =============================================================================
# Tests the bridge using:
# - Canton: 5North DevNet (hosted Ledger API)
# - Ethereum: Sepolia testnet (via Infura/Alchemy)
#
# NOTE: This script uses Sepolia testnet ETH and tokens.
# - Transactions consume testnet ETH for gas
# - Tokens are test assets, but contracts are shared across the team
#
# CONFIGURATION:
#   All values are read from config.devnet.yaml
#   Environment variables can override config values:
#     ETH_RPC_URL, ETH_BRIDGE_CONTRACT, ETH_TOKEN_CONTRACT
#     RELAYER_PRIVATE_KEY, OWNER_PRIVATE_KEY, USER_PRIVATE_KEY
#
# BEFORE RUNNING:
#   1. Fill in config.devnet.yaml if you need overrides (defaults are pre-configured)
#   2. Verify JWT token is valid: cat secrets/devnet-token.txt
#   3. Check wallet balances for Sepolia testnet gas
#   4. Review contract addresses carefully
#
# On devnet, the relayer waits for ethereum.confirmation_blocks (currently 1)
# before processing events. This script only waits long enough for that to happen.
#
# =============================================================================
# Usage:
#   ./scripts/test-bridge-devnet-2.sh [--clean] [--skip-bootstrap] [--dry-run]
#
# Options:
#   --clean          Reset Docker environment (Postgres only)
#   --skip-bootstrap Skip Canton bootstrap (use after first successful run)
#   --dry-run        Validate configuration without executing transactions
#
# =============================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_DIR/config.devnet.yaml"
DOCKER_COMPOSE_CMD="docker compose -f docker-compose.yaml -f docker-compose.devnet.yaml"

# =============================================================================
# Test Amounts (assuming 18-decimal ERC-20)
# =============================================================================
# DevNet bridge limits (from config.devnet.yaml):
# - max_transfer_amount: 1000 tokens
# - min_transfer_amount: 0.001 tokens
# The test amounts below are safely within those bounds.

# Human-readable token amounts
TEST_FUND_USER_TOKENS=20       # total tokens to fund test user
TEST_DEPOSIT_TOKENS=20          # portion deposited to Canton
TEST_WITHDRAW_TOKENS=10          # portion withdrawn back to EVM

# On-chain integer amounts (wei-style, 18 decimals)
TEST_FUND_USER_AMOUNT_WEI="${TEST_FUND_USER_TOKENS}000000000000000000"
TEST_DEPOSIT_AMOUNT_WEI="${TEST_DEPOSIT_TOKENS}000000000000000000"

# Decimal string used by Canton withdrawal script
TEST_WITHDRAW_AMOUNT_DECIMAL="${TEST_WITHDRAW_TOKENS}.0"

# Parse arguments
CLEAN=false
SKIP_BOOTSTRAP=false
DRY_RUN=false
for arg in "$@"; do
    case $arg in
        --clean)
            CLEAN=true
            shift
            ;;
        --skip-bootstrap)
            SKIP_BOOTSTRAP=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
    esac
done

# =============================================================================
# Helper Functions
# =============================================================================

print_header() {
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
}

print_step() {
    echo -e "${CYAN}>>> $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "    $1"
}

kill_relayer() {
    print_step "Stopping any existing relayer processes..."
    pkill -9 -f "cmd/relayer" 2>/dev/null || true
    pkill -9 -f "main.go" 2>/dev/null || true
    lsof -ti:8080 | xargs kill -9 2>/dev/null || true
    sleep 2
}

confirm_devnet() {
    echo ""
    echo -e "${YELLOW}╔══════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║                      DevNet / Sepolia Testnet                        ║${NC}"
    echo -e "${YELLOW}║                                                                      ║${NC}"
    echo -e "${YELLOW}║  You are about to execute transactions on Sepolia testnet.           ║${NC}"
    echo -e "${YELLOW}║  These use testnet ETH but interact with shared devnet contracts.    ║${NC}"
    echo -e "${YELLOW}║                                                                      ║${NC}"
    echo -e "${YELLOW}╚══════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    if [ "$DRY_RUN" = true ]; then
        print_warning "DRY RUN MODE - No transactions will be executed"
        return 0
    fi
}

# =============================================================================
# Pre-flight Checks
# =============================================================================

cd "$PROJECT_DIR"

print_header "CANTON-ETHEREUM BRIDGE DEVNET TEST"

# Check config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    print_error "config.devnet.yaml not found!"
    echo "Create config.devnet.yaml first with devnet configuration."
    exit 1
fi

# =============================================================================
# Parse Configuration from config.devnet.yaml
# =============================================================================

print_step "Parsing configuration from config.devnet.yaml..."

# Parse Ethereum section
ETH_RPC_CONFIG=$(
  awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
    | grep 'rpc_url:' \
    | sed 's/.*rpc_url: *"\([^"]*\)".*/\1/'
)

ETH_BRIDGE_CONFIG=$(
  awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
    | grep 'bridge_contract:' \
    | sed 's/.*bridge_contract: *"\([^"]*\)".*/\1/'
)

ETH_TOKEN_CONFIG=$(
  awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
    | grep 'token_contract:' \
    | sed 's/.*token_contract: *"\([^"]*\)".*/\1/'
)

RELAYER_KEY_CONFIG=$(
  awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
    | grep 'relayer_private_key:' \
    | sed 's/.*relayer_private_key: *"\([^"]*\)".*/\1/'
)

# Owner and User keys added manually

OWNER_KEY="0x3ffae7a5be2fa63022325b175a04cab999af2b8ad956208d10861a75701eae9b"
OWNER=$(cast wallet address --private-key "0x$OWNER_KEY" 2>/dev/null || cast wallet address --private-key "$OWNER_KEY" 2>/dev/null)
USER_KEY="0xeacbff42147f4a4493e2212a70ace9e0ef4e40532e5aa3e049a0eb355e8fc5be"
USER=$(cast wallet address --private-key "0x$USER_KEY" 2>/dev/null || cast wallet address --private-key "$USER_KEY" 2>/dev/null)

# Parse Canton section
CANTON_RPC_CONFIG=$(grep 'rpc_url:' "$CONFIG_FILE" | grep -v '#' | head -2 | tail -1 | sed 's/.*rpc_url: *"\([^"]*\)".*/\1/')
PARTY_ID_CONFIG=$(grep 'relayer_party:' "$CONFIG_FILE" | sed 's/.*relayer_party: *"\([^"]*\)".*/\1/')
DOMAIN_ID_CONFIG=$(grep 'domain_id:' "$CONFIG_FILE" | sed 's/.*domain_id: *"\([^"]*\)".*/\1/')
PACKAGE_ID_CONFIG=$(grep 'bridge_package_id:' "$CONFIG_FILE" | sed 's/.*bridge_package_id: *"\([^"]*\)".*/\1/')
CORE_PACKAGE_ID_CONFIG=$(grep 'core_package_id:' "$CONFIG_FILE" | sed 's/.*core_package_id: *"\([^"]*\)".*/\1/')

# Environment overrides (preferred), fallback to config
ETH_RPC_URL="${ETH_RPC_URL:-$ETH_RPC_CONFIG}"
BRIDGE="${ETH_BRIDGE_CONTRACT:-$ETH_BRIDGE_CONFIG}"
TOKEN="${ETH_TOKEN_CONTRACT:-$ETH_TOKEN_CONFIG}"
RELAYER_KEY="${RELAYER_PRIVATE_KEY:-$RELAYER_KEY_CONFIG}"
CANTON_RPC_URL="${CANTON_RPC_URL:-$CANTON_RPC_CONFIG}"
PARTY_ID="${CANTON_RELAYER_PARTY:-$PARTY_ID_CONFIG}"
DOMAIN_ID="${CANTON_DOMAIN_ID:-$DOMAIN_ID_CONFIG}"
PACKAGE_ID="${CANTON_BRIDGE_PACKAGE_ID:-$PACKAGE_ID_CONFIG}"

# Validate required values
if [ -z "$ETH_RPC_URL" ]; then
    print_error "ETH RPC URL not set in config or ETH_RPC_URL env var"
    exit 1
fi

if [ -z "$BRIDGE" ]; then
    print_error "Bridge contract not set in config or ETH_BRIDGE_CONTRACT env var"
    exit 1
fi

if [ -z "$TOKEN" ]; then
    print_error "Token contract not set in config or ETH_TOKEN_CONTRACT env var"
    exit 1
fi

if [ -z "$RELAYER_KEY" ]; then
    print_error "Relayer private key not set in config or RELAYER_PRIVATE_KEY env var"
    exit 1
fi

if [ -z "$PARTY_ID" ]; then
    print_error "Relayer party not set in config or CANTON_RELAYER_PARTY env var"
    exit 1
fi

if [ -z "$DOMAIN_ID" ]; then
    print_error "Domain ID not set in config or CANTON_DOMAIN_ID env var"
    exit 1
fi

if [ -z "$PACKAGE_ID" ]; then
    print_error "Bridge package ID not set in config or CANTON_BRIDGE_PACKAGE_ID env var"
    exit 1
fi

# Verify chain ID is Sepolia (11155111)
CHAIN_ID=$(cast chain-id --rpc-url "$ETH_RPC_URL" 2>/dev/null)
if [ "$CHAIN_ID" != "11155111" ]; then
    print_error "Expected Sepolia testnet (chain ID 11155111), got chain ID $CHAIN_ID"
    exit 1
fi
print_success "Confirmed Sepolia testnet (chain ID: 11155111)"


# Derive relayer address from private key
RELAYER=$(cast wallet address --private-key "0x$RELAYER_KEY" 2>/dev/null || cast wallet address --private-key "$RELAYER_KEY" 2>/dev/null)
if [ -z "$RELAYER" ]; then
    print_error "Failed to derive relayer address from private key"
    exit 1
fi

CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"

echo ""
echo -e "${YELLOW}Mode: DEVNET (Canton 5North) + Sepolia Testnet${NC}"
echo -e "${YELLOW}ETH RPC URL: ${ETH_RPC_URL}${NC}"
echo ""
echo "Project directory: $PROJECT_DIR"
echo "Config file: $CONFIG_FILE"
echo ""

# Show config sources
print_step "Configuration sources:"
if [ -n "$ETH_RPC_URL" ] && [ "$ETH_RPC_URL" != "$ETH_RPC_CONFIG" ]; then
    print_info "ETH RPC URL (from env):        $ETH_RPC_URL"
else
    print_info "ETH RPC URL (from config):     $ETH_RPC_URL"
fi
if [ -n "${ETH_BRIDGE_CONTRACT:-}" ]; then
    print_info "Bridge contract (from env):    $BRIDGE"
else
    print_info "Bridge contract (from config): $BRIDGE"
fi
if [ -n "${ETH_TOKEN_CONTRACT:-}" ]; then
    print_info "Token contract (from env):     $TOKEN"
else
    print_info "Token contract (from config):  $TOKEN"
fi
print_info "Canton RPC URL:                $CANTON_RPC_URL"
print_info "Party ID:                      $PARTY_ID"
print_info "Domain ID:                     $DOMAIN_ID"
print_info "Package ID:                    $PACKAGE_ID"

# Confirm devnet execution
confirm_devnet

# Check JWT token exists
if [ ! -f "$PROJECT_DIR/secrets/devnet-token.txt" ]; then
    print_error "secrets/devnet-token.txt not found!"
    echo "Get JWT token for devnet and save to secrets/devnet-token.txt"
    exit 1
fi

# Check JWT token not expired
TOKEN_PAYLOAD=$(cat "$PROJECT_DIR/secrets/devnet-token.txt" | cut -d'.' -f2)
TOKEN_EXP=$(echo "${TOKEN_PAYLOAD}==" | base64 -d 2>/dev/null | jq -r '.exp' 2>/dev/null || echo "")
CURRENT_TIME=$(date +%s)
if [ -n "$TOKEN_EXP" ] && [ "$TOKEN_EXP" != "null" ]; then
    if [ "$CURRENT_TIME" -gt "$TOKEN_EXP" ]; then
        print_error "JWT token has EXPIRED!"
        echo "Token expired at: $(date -r $TOKEN_EXP 2>/dev/null || date -d @$TOKEN_EXP 2>/dev/null)"
        exit 1
    else
        HOURS_LEFT=$(( ($TOKEN_EXP - $CURRENT_TIME) / 3600 ))
        print_info "JWT token valid for ~${HOURS_LEFT} more hours"
    fi
fi

# Extract fingerprint for deposits
FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
    FINGERPRINT="${FULL_FINGERPRINT:4}"
else
    FINGERPRINT="$FULL_FINGERPRINT"
fi
print_info "Fingerprint (bytes32): 0x$FINGERPRINT"

# Validate addresses
print_step "Validating wallet addresses..."
print_info "Relayer: $RELAYER"
if [ -n "$OWNER" ]; then
    print_info "Owner:   $OWNER"
else
    print_warning "Owner key not set (OWNER_PRIVATE_KEY) - some operations will be skipped"
fi
if [ -n "$USER" ]; then
    print_info "User:    $USER"
else
    print_warning "User key not set (USER_PRIVATE_KEY) - deposit/withdrawal tests will be skipped"
fi

# =============================================================================
# Step 0: Clean environment (optional)
# =============================================================================

if [ "$CLEAN" = true ]; then
    print_header "STEP 0: Cleaning Environment"
    kill_relayer
    print_step "Stopping Docker containers..."
    $DOCKER_COMPOSE_CMD down 2>/dev/null || true
    print_success "Environment cleaned"
fi

# =============================================================================
# Step 1: Start Local Services (Postgres only)
# =============================================================================

print_header "STEP 1: Start Local Services (Postgres only)"

print_step "Starting docker compose..."
$DOCKER_COMPOSE_CMD up -d

# Check Ethereum RPC
print_step "Checking Ethereum RPC endpoint..."
if cast block-number --rpc-url "$ETH_RPC_URL" >/dev/null 2>&1; then
    BLOCK_NUM=$(cast block-number --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    print_success "Ethereum RPC is reachable (block: $BLOCK_NUM)"
else
    print_error "Failed to query block number from $ETH_RPC_URL"
    exit 1
fi

# Wait for Postgres
print_step "Waiting for Postgres..."
sleep 2
if docker exec postgres pg_isready -U postgres >/dev/null 2>&1; then
    print_success "Postgres is ready!"
else
    print_warning "Postgres may not be ready, continuing anyway..."
fi

echo ""
$DOCKER_COMPOSE_CMD ps

# =============================================================================
# Step 2: Verify Ethereum Contracts (DevNet / Sepolia)
# =============================================================================

print_header "STEP 2: Verify Ethereum Contracts (Sepolia)"

print_info "CantonBridge: $BRIDGE"
print_info "PromptToken:  $TOKEN"

# Verify contracts
print_step "Verifying contracts on Sepolia..."
BRIDGE_RELAYER=$(cast call $BRIDGE "relayer()(address)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Bridge relayer: $BRIDGE_RELAYER"

if [ "$BRIDGE_RELAYER" != "$RELAYER" ]; then
    print_warning "Bridge relayer ($BRIDGE_RELAYER) does not match configured relayer ($RELAYER)"
fi

TOKEN_NAME=$(cast call $TOKEN "name()(string)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Token name: $TOKEN_NAME"

print_success "Ethereum contracts verified"

# =============================================================================
# Step 3: Bootstrap Canton (if needed)
# =============================================================================

if [ "$SKIP_BOOTSTRAP" = true ]; then
    print_header "STEP 3: Canton Bootstrap (Skipped)"
    print_warning "Skipping bootstrap (--skip-bootstrap flag)"
else
    print_header "STEP 3: Bootstrap Canton Bridge Contracts"
    
    if [ "$DRY_RUN" = true ]; then
        print_warning "DRY RUN - Would run bootstrap script"
    else
        print_step "Running bootstrap script against DevNet..."
        go run scripts/bootstrap-bridge.go \
            -config "$CONFIG_FILE" \
            -issuer "$PARTY_ID" \
            -package "$PACKAGE_ID" || {
                print_warning "Bootstrap may have failed or contracts already exist"
                print_info "If contracts exist, use --skip-bootstrap next time"
            }
    fi
    
    print_success "Bootstrap complete"
fi

# =============================================================================
# Step 4: Register Test User
# =============================================================================

print_header "STEP 4: Register Test User"

if [ "$DRY_RUN" = true ]; then
    print_warning "DRY RUN - Would run register-user script"
else
    print_step "Running register-user script..."
    go run scripts/register-user.go \
        -config "$CONFIG_FILE" \
        -party "$PARTY_ID" || {
            print_warning "User registration may have failed or user already exists"
        }
fi

print_success "User registration complete"

# =============================================================================
# Step 5: Start the Relayer
# =============================================================================

print_header "STEP 5: Start the Relayer"

kill_relayer

print_step "Starting relayer in background..."
go run cmd/relayer/main.go -config "$CONFIG_FILE" > /tmp/relayer-devnet.log 2>&1 &
RELAYER_PID=$!
echo "Relayer PID: $RELAYER_PID"

print_step "Waiting for relayer to start..."
sleep 10

# Check health
if curl -s http://localhost:8080/health | grep -q "OK"; then
    print_success "Relayer is healthy"
else
    print_error "Relayer health check failed"
    echo ""
    echo "Relayer logs:"
    tail -30 /tmp/relayer-devnet.log
    exit 1
fi

# =============================================================================
# Step 6: Setup Bridge (EVM)
# =============================================================================

print_header "STEP 6: Setup Bridge Contracts (EVM)"

if [ "$DRY_RUN" = true ]; then
    print_warning "DRY RUN - Would add token mapping (if not exists)"
else
    print_step "Checking existing token mapping..."
    EXISTING_MAPPING=$(cast call $BRIDGE "ethereumToCantonToken(address)(bytes32)" $TOKEN --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    
    if [ "$EXISTING_MAPPING" = "0x0000000000000000000000000000000000000000000000000000000000000000" ]; then
        print_step "Adding token mapping..."
        cast send $BRIDGE "addTokenMapping(address,bytes32)" \
            $TOKEN $CANTON_TOKEN_ID \
            --rpc-url "$ETH_RPC_URL" \
            --private-key $OWNER_KEY > /dev/null 2>&1
        print_success "Token mapping added"
    elif [ "$EXISTING_MAPPING" = "$CANTON_TOKEN_ID" ]; then
        print_success "Token mapping already exists with correct Canton token ID"
    else
        print_error "Token already mapped to different Canton token ID!"
        print_info "Existing: $EXISTING_MAPPING"
        print_info "Expected: $CANTON_TOKEN_ID"
        exit 1
    fi
    print_success "Bridge setup complete"
fi

# =============================================================================
# Step 7: Test Deposit (EVM → Canton)
# =============================================================================

print_header "STEP 7: EVM → Canton Deposit"

if [ "$DRY_RUN" = true ]; then
    print_warning "DRY RUN - Would execute deposit"
    print_info "Would transfer ${TEST_FUND_USER_TOKENS} tokens from owner to user"
    print_info "Would approve bridge for ${TEST_FUND_USER_TOKENS} tokens"
    print_info "Would deposit ${TEST_DEPOSIT_TOKENS} tokens to Canton"
else
    # Fund user
    print_step "Transferring ${TEST_FUND_USER_TOKENS} tokens from owner to user..."
    cast erc20 transfer $TOKEN $USER "$TEST_FUND_USER_AMOUNT_WEI" \
        --rpc-url "$ETH_RPC_URL" \
        --private-key $OWNER_KEY 2>/dev/null >&1
    print_success "User funded"

    # Approve
    print_step "Approving bridge..."
    cast erc20 approve $TOKEN $BRIDGE "$TEST_FUND_USER_AMOUNT_WEI" \
        --rpc-url "$ETH_RPC_URL" \
        --private-key $USER_KEY 2>/dev/null >&1
    print_success "Approved"

    # Deposit
    print_step "Depositing ${TEST_DEPOSIT_TOKENS} tokens to Canton..."
    CANTON_RECIPIENT="0x$FINGERPRINT"
    DEPOSIT_TX=$(cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
        $TOKEN "$TEST_DEPOSIT_AMOUNT_WEI" $CANTON_RECIPIENT \
        --rpc-url "$ETH_RPC_URL" \
        --private-key $USER_KEY --json 2>/dev/null | jq -r '.transactionHash')
    print_info "Deposit TX: $DEPOSIT_TX"
    print_success "Deposit submitted"

    # Wait for relayer (shorter for devnet, 1 confirmation)
    print_step "Waiting for relayer to process deposit (devnet, 1 confirmation)..."
    sleep 30  # 1 block + relayer processing margin on Sepolia

    # Verify on Canton
    print_step "Verifying holdings on Canton..."
    HOLDINGS_OUTPUT=$(go run scripts/query-holdings.go -config "$CONFIG_FILE" -party "$PARTY_ID" 2>/dev/null)
    echo "$HOLDINGS_OUTPUT"

    HOLDING_CID=$(echo "$HOLDINGS_OUTPUT" | grep "Contract ID:" | head -1 | awk '{print $3}')
    print_info "Holding CID: $HOLDING_CID"

    if [ -z "$HOLDING_CID" ]; then
        print_error "No holdings found - deposit may have failed"
        echo ""
        echo "Relayer logs:"
        tail -50 /tmp/relayer-devnet.log
        exit 1
    fi

    print_success "Deposit verified on Canton!"
fi

# =============================================================================
# Step 8: Test Withdrawal (Canton → EVM)
# =============================================================================

print_header "STEP 8: Canton → EVM Withdrawal"

if [ "$DRY_RUN" = true ]; then
    print_warning "DRY RUN - Would execute withdrawal"
    print_info "Would withdraw ${TEST_WITHDRAW_TOKENS} tokens to user address"
else
    BALANCE_BEFORE=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    print_info "User balance before: $BALANCE_BEFORE"

    print_step "Initiating withdrawal of ${TEST_WITHDRAW_TOKENS} tokens..."
    go run scripts/initiate-withdrawal.go \
        -config "$CONFIG_FILE" \
        -holding-cid "$HOLDING_CID" \
        -amount "$TEST_WITHDRAW_AMOUNT_DECIMAL" \
        -evm-destination "$USER"

    print_step "Waiting for relayer to process withdrawal (devnet)..."
    sleep 20

    BALANCE_AFTER=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    print_info "User balance after: $BALANCE_AFTER"

    print_success "Withdrawal processed"
fi

# =============================================================================
# Summary
# =============================================================================

print_header "TEST SUMMARY"

echo ""
if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}DRY RUN completed - no transactions were executed${NC}"
else
    echo -e "${GREEN}All tests passed!${NC}"
fi
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "CONFIGURATION (DevNet)"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Canton:          5North DevNet (${CANTON_RPC_URL})"
echo "Ethereum:        Sepolia testnet ($ETH_RPC_URL)"
echo "Bridge:          $BRIDGE"
echo "Token:           $TOKEN"
echo "Party ID:        $PARTY_ID"
echo "Domain ID:       $DOMAIN_ID"
echo "Fingerprint:     0x$FINGERPRINT"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "TRANSFER SUMMARY"
echo "═══════════════════════════════════════════════════════════════════════"
if [ "$DRY_RUN" = true ]; then
    echo "Deposit:         (dry run - not executed)"
    echo "Withdrawal:      (dry run - not executed)"
elif [ -z "$USER_KEY" ]; then
    echo "Deposit:         (skipped - no user key)"
    echo "Withdrawal:      (skipped - no user key)"
else
    echo "Deposit:         ${TEST_DEPOSIT_TOKENS} tokens (EVM → Canton)"
    echo "Withdrawal:      ${TEST_WITHDRAW_TOKENS} tokens (Canton → EVM)"
fi
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "COMMANDS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "View relayer logs:     tail -f /tmp/relayer-devnet.log"
echo "Query holdings:        go run scripts/query-holdings.go -config config.devnet.yaml -party \"$PARTY_ID\""
echo "Stop relayer:          pkill -f 'cmd/relayer'"
echo "Stop local services:   $DOCKER_COMPOSE_CMD down"
echo ""
