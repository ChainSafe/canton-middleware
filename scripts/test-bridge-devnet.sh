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
# - Canton: 5North DevNet (remote gRPC at canton-ledger-api-grpc-dev1.chainsafe.dev:80)
# - Ethereum: Sepolia via Infura (pre-deployed contracts)
#
# PRE-CONFIGURED FOR CHAINSAFE DEVNET:
# - JWT Token: secrets/devnet-token.txt (shared Auth0 credentials)
# - Party: daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c
# - DARs: Already uploaded to 5North
# - User Rights: Already granted for JWT subject
# - Domain: global-domain::1220be58c29e65de...
# - Ethereum: Contracts pre-deployed to Sepolia (addresses in broadcast file)
#
# BEFORE RUNNING - Check JWT token is not expired:
#   TOKEN=$(cat secrets/devnet-token.txt | cut -d'.' -f2)
#   EXP=$(echo "${TOKEN}==" | base64 -d | jq -r '.exp')
#   date -r $EXP  # Shows expiration date (macOS)
#
# =============================================================================
# Usage:
#   ./scripts/test-bridge-devnet.sh [--clean] [--skip-bootstrap]
#
# Options:
#   --clean          Reset Docker environment (Postgres only)
#   --skip-bootstrap Skip Canton bootstrap (use after first successful run)
#
# First run:  ./scripts/test-bridge-devnet.sh --clean
# Re-run:     ./scripts/test-bridge-devnet.sh --skip-bootstrap
#
# See docs/DEVNET_SETUP.md for full setup details (if reconfiguring)
# =============================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration (Sepolia via Infura)
ETH_RPC_URL="${ETH_RPC_URL:-https://sepolia.infura.io/v3/dd3f23220d4348e2a3b192100fc71ba1}"
CHAIN_ID="${CHAIN_ID:-11155111}"
RELAYER="0x914db8873AcFd84b834278e20BB9fCE9DD223043"
RELAYER_KEY="0xf9aac8ca8ca8fa4ff170921eca83aed78a3eb156dd3cba80a9cda033ae637066"
# Owner is the deployer of PromptToken and CantonBridge (has admin rights)
OWNER="0x8A0A6FF59ad10e009b0fdB3B3CA7A0356eDcCCbf"
OWNER_KEY="0x3ffae7a5be2fa63022325b175a04cab999af2b8ad956208d10861a75701eae9b"
USER="0x4768CCb3cE015698468A65bf8208b3f6919c769e"
USER_KEY="0xeacbff42147f4a4493e2212a70ace9e0ef4e40532e5aa3e049a0eb355e8fc5be"
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_DIR/config.devnet.yaml"
DOCKER_COMPOSE_CMD="docker compose -f docker-compose.yaml -f docker-compose.devnet.yaml"

# Parse arguments
CLEAN=false
SKIP_BOOTSTRAP=false
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

# =============================================================================
# Pre-flight Checks
# =============================================================================

cd "$PROJECT_DIR"

print_header "CANTON-ETHEREUM BRIDGE DEVNET TEST"
echo ""
echo -e "${YELLOW}Mode: 5North DevNet (remote Canton) + Ethereum Sepolia (Infura)${NC}"
echo -e "${YELLOW}ETH RPC URL: ${ETH_RPC_URL}${NC}"
echo ""
echo "Project directory: $PROJECT_DIR"
echo "Config file: $CONFIG_FILE"
echo ""

# Check config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    print_error "config.devnet.yaml not found!"
    echo "Create config.devnet.yaml first. See docs/WAYFINDER_DEPLOYMENT_REQUIREMENTS.md"
    exit 1
fi

# Check JWT token exists
if [ ! -f "$PROJECT_DIR/secrets/devnet-token.txt" ]; then
    print_error "secrets/devnet-token.txt not found!"
    echo "Get JWT token from Auth0 and save to secrets/devnet-token.txt"
    exit 1
fi

# Check JWT token not expired (JWT uses base64url, needs padding)
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

# Read party and domain from config
PARTY_ID=$(grep 'relayer_party:' "$CONFIG_FILE" | sed 's/.*relayer_party: *"\([^"]*\)".*/\1/')
DOMAIN_ID=$(grep 'domain_id:' "$CONFIG_FILE" | sed 's/.*domain_id: *"\([^"]*\)".*/\1/')
PACKAGE_ID=$(grep 'bridge_package_id:' "$CONFIG_FILE" | sed 's/.*bridge_package_id: *"\([^"]*\)".*/\1/')

if [ -z "$PARTY_ID" ] || [ "$PARTY_ID" = "" ]; then
    print_error "relayer_party not set in config.devnet.yaml"
    exit 1
fi

if [ -z "$DOMAIN_ID" ] || [ "$DOMAIN_ID" = "" ]; then
    print_error "domain_id not set in config.devnet.yaml"
    exit 1
fi

print_info "Party ID: $PARTY_ID"
print_info "Domain ID: $DOMAIN_ID"
print_info "Package ID: $PACKAGE_ID"

# Extract fingerprint for deposits
FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
    FINGERPRINT="${FULL_FINGERPRINT:4}"
else
    FINGERPRINT="$FULL_FINGERPRINT"
fi
print_info "Fingerprint (bytes32): 0x$FINGERPRINT"

# =============================================================================
# Step 0: Clean environment (optional)
# =============================================================================

if [ "$CLEAN" = true ]; then
    print_header "STEP 0: Cleaning Environment"
    kill_relayer
    print_step "Stopping Docker containers..."
    # Note: Don't use -v flag to preserve Postgres data (offsets, transfer records)
    # This prevents replaying historical events and wasting gas on duplicates
    $DOCKER_COMPOSE_CMD down 2>/dev/null || true
    print_success "Environment cleaned"
fi

# =============================================================================
# Step 1: Start Local Services (Postgres only)
# =============================================================================

print_header "STEP 1: Start Local Services (Postgres only)"

print_step "Starting docker compose (DevNet mode - Postgres only)..."
$DOCKER_COMPOSE_CMD up -d

# Check Ethereum RPC (Infura / Sepolia)
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
# Step 2: Verify Ethereum Contracts (Sepolia)
# =============================================================================

print_header "STEP 2: Verify Ethereum Contracts (Sepolia)"

print_step "Resolving Ethereum contract addresses..."

# Extract from config.devnet.yaml (ethereum section only)
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

# Environment overrides (preferred), fallback to config
BRIDGE="${ETH_BRIDGE_CONTRACT:-$ETH_BRIDGE_CONFIG}"
TOKEN="${ETH_TOKEN_CONTRACT:-$ETH_TOKEN_CONFIG}"

if [ -z "$BRIDGE" ] || [ -z "$TOKEN" ]; then
    print_error "Bridge or token contract address not found."
    print_info "Set ETH_BRIDGE_CONTRACT / ETH_TOKEN_CONTRACT or update ethereum.bridge_contract/token_contract in config.devnet.yaml."
    exit 1
fi

if [ -n "$ETH_BRIDGE_CONTRACT" ]; then
    print_info "CantonBridge (from ETH_BRIDGE_CONTRACT): $BRIDGE"
else
    print_info "CantonBridge (from config.devnet.yaml):  $BRIDGE"
fi

if [ -n "$ETH_TOKEN_CONTRACT" ]; then
    print_info "PromptToken (from ETH_TOKEN_CONTRACT):   $TOKEN"
else
    print_info "PromptToken (from config.devnet.yaml):   $TOKEN"
fi

# Verify contracts
print_step "Verifying contracts on Sepolia..."
BRIDGE_RELAYER=$(cast call $BRIDGE "relayer()(address)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Bridge relayer: $BRIDGE_RELAYER"

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
    
    print_step "Running bootstrap script against DevNet..."
    go run scripts/bootstrap-bridge.go \
        -config "$CONFIG_FILE" \
        -issuer "$PARTY_ID" \
        -package "$PACKAGE_ID" || {
            print_warning "Bootstrap may have failed or contracts already exist"
            print_info "If contracts exist, use --skip-bootstrap next time"
        }
    
    print_success "Bootstrap complete"
fi

# =============================================================================
# Step 4: Register Test User
# =============================================================================

print_header "STEP 4: Register Test User"

print_step "Running register-user script..."
go run scripts/register-user.go \
    -config "$CONFIG_FILE" \
    -party "$PARTY_ID" || {
        print_warning "User registration may have failed or user already exists"
    }

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
sleep 8

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

print_step "Adding token mapping..."
cast send $BRIDGE "addTokenMapping(address,bytes32)" \
    $TOKEN $CANTON_TOKEN_ID \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $OWNER_KEY > /dev/null 2>&1 || print_warning "Token mapping may already exist"

print_success "Bridge setup complete"

# =============================================================================
# Step 7: Test Deposit (EVM → Canton)
# =============================================================================

print_header "STEP 7: EVM → Canton Deposit"

# Fund user
print_step "Transferring 1000 tokens from owner to user..."
cast send $TOKEN "transfer(address,uint256)" $USER "1000000000000000000000" \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $OWNER_KEY > /dev/null 2>&1
print_success "User funded"

# Approve
print_step "Approving bridge..."
cast send $TOKEN "approve(address,uint256)" $BRIDGE "1000000000000000000000" \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $USER_KEY > /dev/null 2>&1
print_success "Approved"

# Deposit
print_step "Depositing 100 tokens to Canton..."
CANTON_RECIPIENT="0x$FINGERPRINT"
DEPOSIT_TX=$(cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
    $TOKEN "100000000000000000000" $CANTON_RECIPIENT \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $USER_KEY --json 2>/dev/null | jq -r '.transactionHash')
print_info "Deposit TX: $DEPOSIT_TX"
print_success "Deposit submitted"

# Wait for relayer
print_step "Waiting for relayer to process deposit..."
sleep 10

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

# =============================================================================
# Step 8: Test Withdrawal (Canton → EVM)
# =============================================================================

print_header "STEP 8: Canton → EVM Withdrawal"

BALANCE_BEFORE=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "User balance before: $BALANCE_BEFORE"

print_step "Initiating withdrawal of 50 tokens..."
go run scripts/initiate-withdrawal.go \
    -config "$CONFIG_FILE" \
    -holding-cid "$HOLDING_CID" \
    -amount "50.0" \
    -evm-destination "$USER"

print_step "Waiting for relayer to process withdrawal..."
sleep 10

BALANCE_AFTER=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "User balance after: $BALANCE_AFTER"

print_success "Withdrawal processed"

# =============================================================================
# Summary
# =============================================================================

print_header "TEST SUMMARY"

echo ""
echo -e "${GREEN}All tests passed!${NC}"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "CONFIGURATION (DevNet)"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Canton:          5North DevNet (canton-ledger-api-grpc-dev1.chainsafe.dev:80)"
echo "Ethereum:        Sepolia via Infura ($ETH_RPC_URL)"
echo "Party ID:        $PARTY_ID"
echo "Domain ID:       $DOMAIN_ID"
echo "Fingerprint:     0x$FINGERPRINT"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "TRANSFER SUMMARY"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Deposit:         100 tokens (EVM → Canton)"
echo "Withdrawal:      50 tokens (Canton → EVM)"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "COMMANDS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "View relayer logs:     tail -f /tmp/relayer-devnet.log"
echo "Query holdings:        go run scripts/query-holdings.go -config config.devnet.yaml -party \"$PARTY_ID\""
echo "Stop relayer:          pkill -f 'cmd/relayer'"
echo "Stop local services:   $DOCKER_COMPOSE_CMD down"
echo ""

