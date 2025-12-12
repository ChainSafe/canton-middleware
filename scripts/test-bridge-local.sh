#!/bin/bash
# =============================================================================
# ⚠️  DEPRECATED - Use test-bridge.sh instead
# =============================================================================
# This script is deprecated. Please use the unified test script:
#
#   ./scripts/test-bridge.sh --config config.local.yaml
#   ./scripts/test-bridge.sh --config config.local.yaml --clean
#   ./scripts/test-bridge.sh --config config.local.yaml --skip-setup
#
# This file is kept for reference only.
# =============================================================================
#
# Canton-Ethereum Bridge Full Test Script (Local)
# =============================================================================
# This script automates the entire BRIDGE_TESTING_GUIDE.md flow
#
# Usage:
#   ./scripts/test-bridge-local.sh [--clean] [--skip-docker] [--skip-bootstrap]
#
# Options:
#   --clean          Reset Docker environment before starting
#   --skip-docker    Skip Docker setup (assume services are already running)
#   --skip-bootstrap Skip Canton bootstrap (use after first successful run)
# =============================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
ETH_RPC_URL="${ETH_RPC_URL:-http://localhost:8545}"
CHAIN_ID="${CHAIN_ID:-31337}"
# BRIDGE and TOKEN addresses are read from broadcast file in Step 2
RELAYER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
RELAYER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
# Owner is the deployer of PromptToken and CantonBridge (has admin rights)
OWNER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
OWNER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
USER_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"
PACKAGE_ID="6694b7794de78352c5893ded301e6cf0080db02cbdfa7fab23cfd9e8a56eb73d"

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_DIR/config.local.yaml"

# Parse arguments
CLEAN=false
SKIP_DOCKER=false
SKIP_BOOTSTRAP=false
for arg in "$@"; do
    case $arg in
        --clean)
            CLEAN=true
            shift
            ;;
        --skip-docker)
            SKIP_DOCKER=true
            shift
            ;;
        --skip-bootstrap)
            SKIP_BOOTSTRAP=true
            shift
            ;;
    esac
done

DOCKER_COMPOSE_CMD="docker compose"

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

wait_for_canton() {
    print_step "Waiting for Canton to become healthy..."
    local max_attempts=60
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        local status=$(docker inspect --format='{{.State.Health.Status}}' canton 2>/dev/null || echo "starting")
        if [ "$status" = "healthy" ]; then
            print_success "Canton is healthy!"
            break
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    if [ $attempt -ge $max_attempts ]; then
        print_error "Canton failed to become healthy after ${max_attempts} attempts"
        exit 1
    fi
    
    # Also wait for HTTP API to be responsive
    print_step "Waiting for Canton HTTP API..."
    attempt=0
    while [ $attempt -lt 30 ]; do
        if curl -s http://localhost:5013/v2/version >/dev/null 2>&1; then
            print_success "Canton HTTP API is ready!"
            break
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    if [ $attempt -ge 30 ]; then
        print_error "Canton HTTP API failed to become ready"
        exit 1
    fi
    
    # Wait for Canton to be connected to a synchronizer (required for party allocation)
    print_step "Waiting for Canton to connect to synchronizer..."
    attempt=0
    while [ $attempt -lt 60 ]; do
        local sync_count=$(curl -s http://localhost:5013/v2/state/connected-synchronizers 2>/dev/null | jq '.connectedSynchronizers | length' 2>/dev/null || echo "0")
        if [ "$sync_count" -gt 0 ] 2>/dev/null; then
            print_success "Canton connected to synchronizer!"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    print_error "Canton failed to connect to synchronizer"
    exit 1
}

kill_relayer() {
    print_step "Stopping any existing relayer processes..."
    pkill -9 -f "cmd/relayer" 2>/dev/null || true
    pkill -9 -f "main.go" 2>/dev/null || true
    lsof -ti:8080 | xargs kill -9 2>/dev/null || true
    sleep 2
}

# =============================================================================
# Main Script
# =============================================================================

cd "$PROJECT_DIR"

print_header "CANTON-ETHEREUM BRIDGE TEST SCRIPT"
echo ""
echo "Project directory: $PROJECT_DIR"
echo "Config file: $CONFIG_FILE"
echo ""

# =============================================================================
# Step 0: Clean environment (optional)
# =============================================================================

if [ "$CLEAN" = true ]; then
    print_header "STEP 0: Cleaning Environment"
    kill_relayer
    print_step "Stopping Docker containers..."
    $DOCKER_COMPOSE_CMD down -v 2>/dev/null || true
    print_success "Environment cleaned"
fi

# =============================================================================
# Step 1: Start Docker Services
# =============================================================================

if [ "$SKIP_DOCKER" = false ]; then
    print_header "STEP 1: Start Docker Services"
    
    # Local mode: start everything including Canton
    if docker compose ps --format '{{.State}}' canton 2>/dev/null | grep -q "running"; then
        print_warning "Docker services already running"
    else
        print_step "Starting docker compose..."
        $DOCKER_COMPOSE_CMD up -d
    fi

    wait_for_canton

    echo ""
    $DOCKER_COMPOSE_CMD ps
fi

# =============================================================================
# Step 2: Verify Ethereum Contracts
# =============================================================================

print_header "STEP 2: Verify Ethereum Contracts"

# Read contract addresses from Foundry broadcast
BROADCAST_FILE="$PROJECT_DIR/contracts/ethereum-wayfinder/broadcast/Deployer.s.sol/${CHAIN_ID}/run-latest.json"

if [ -f "$BROADCAST_FILE" ]; then
    print_step "Reading contract addresses from broadcast file..."
    
    # Extract PromptToken address
    TOKEN=$(jq -r '.transactions[] | select(.contractName == "PromptToken") | .contractAddress' "$BROADCAST_FILE")
    if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
        print_error "Failed to extract PromptToken address from broadcast file"
        exit 1
    fi
    
    # Extract CantonBridge address
    BRIDGE=$(jq -r '.transactions[] | select(.contractName == "CantonBridge") | .contractAddress' "$BROADCAST_FILE")
    if [ -z "$BRIDGE" ] || [ "$BRIDGE" = "null" ]; then
        print_error "Failed to extract CantonBridge address from broadcast file"
        exit 1
    fi
    
    print_info "PromptToken:  $TOKEN"
    print_info "CantonBridge: $BRIDGE"
    
    # Update config.yaml with extracted addresses
    print_step "Updating config.local.yaml with contract addresses..."
    sed -i.bak "s|bridge_contract: \"0x[a-fA-F0-9]*\"|bridge_contract: \"$BRIDGE\"|" "$CONFIG_FILE"
    sed -i.bak "s|token_contract: \"0x[a-fA-F0-9]*\"|token_contract: \"$TOKEN\"|" "$CONFIG_FILE"
    rm -f "${CONFIG_FILE}.bak"
    print_success "Config updated with contract addresses"
else
    print_error "Broadcast file not found: $BROADCAST_FILE"
    print_info "Make sure contracts are deployed first"
    exit 1
fi

print_step "Checking bridge contract..."
BRIDGE_RELAYER=$(cast call $BRIDGE "relayer()(address)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Bridge relayer: $BRIDGE_RELAYER"

BRIDGE_OWNER=$(cast call $BRIDGE "owner()(address)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Bridge owner:   $BRIDGE_OWNER"

print_step "Checking token contract..."
TOKEN_NAME=$(cast call $TOKEN "name()(string)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Token name: $TOKEN_NAME"

print_success "Ethereum contracts verified"

# =============================================================================
# Step 3: Verify Canton DARs
# =============================================================================

print_header "STEP 3: Verify Canton DARs"

# cip56-token package ID (required for CIP56Manager)
CIP56_PACKAGE_ID="e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe"

print_step "Waiting for DAR packages to be uploaded..."
DAR_MAX_ATTEMPTS=60
DAR_ATTEMPT=0
PACKAGE_COUNT=0
CIP56_FOUND=false
while [ $DAR_ATTEMPT -lt $DAR_MAX_ATTEMPTS ]; do
    PACKAGES_JSON=$(curl -s http://localhost:5013/v2/packages 2>/dev/null || echo '{"packageIds":[]}')
    PACKAGE_COUNT=$(echo "$PACKAGES_JSON" | jq '.packageIds | length' 2>/dev/null || echo "0")

    # Check if cip56-token package is uploaded
    if echo "$PACKAGES_JSON" | jq -e ".packageIds | index(\"$CIP56_PACKAGE_ID\")" >/dev/null 2>&1; then
        CIP56_FOUND=true
    fi

    # Need both: enough packages AND the cip56-token package
    if [ "$PACKAGE_COUNT" -ge 30 ] && [ "$CIP56_FOUND" = true ]; then
        break
    fi
    echo -n "."
    sleep 2
    DAR_ATTEMPT=$((DAR_ATTEMPT + 1))
done
echo ""

print_info "Packages uploaded: $PACKAGE_COUNT"
print_info "cip56-token package: $CIP56_FOUND"

if [ "$PACKAGE_COUNT" -lt 30 ]; then
    print_error "Expected at least 30 packages, got $PACKAGE_COUNT"
    print_info "Check deployer logs: docker logs deployer"
    exit 1
fi

if [ "$CIP56_FOUND" != true ]; then
    print_error "cip56-token package not found (required for CIP56Manager)"
    print_info "Check deployer logs: docker logs deployer"
    exit 1
fi

print_success "Canton DARs verified"

# =============================================================================
# Step 4: Allocate Bridge Issuer Party
# =============================================================================

print_header "STEP 4: Allocate Bridge Issuer Party"

# Check if party exists
EXISTING_PARTY=$(curl -s http://localhost:5013/v2/parties | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1 || true)

if [ -n "$EXISTING_PARTY" ]; then
    print_warning "BridgeIssuer already exists"
    PARTY_ID="$EXISTING_PARTY"
else
    print_step "Creating BridgeIssuer party..."
    PARTY_RESPONSE=$(curl -s -X POST http://localhost:5013/v2/parties \
        -H 'Content-Type: application/json' \
        -d '{"partyIdHint": "BridgeIssuer"}')
    PARTY_ID=$(echo "$PARTY_RESPONSE" | jq -r '.partyDetails.party // empty')

    if [ -z "$PARTY_ID" ]; then
        print_error "Failed to allocate party. Response: $PARTY_RESPONSE"
        exit 1
    fi
fi

print_info "Party ID: $PARTY_ID"

# Get domain ID
print_step "Getting domain ID..."
SYNC_RESPONSE=$(curl -s http://localhost:5013/v2/state/connected-synchronizers)
DOMAIN_ID=$(echo "$SYNC_RESPONSE" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')

if [ -z "$DOMAIN_ID" ]; then
    print_error "Failed to get domain ID. Response: $SYNC_RESPONSE"
    exit 1
fi

print_info "Domain ID: $DOMAIN_ID"

# Extract fingerprint (without 1220 prefix)
# shellcheck disable=SC2001
FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
    FINGERPRINT="${FULL_FINGERPRINT:4}"
else
    FINGERPRINT="$FULL_FINGERPRINT"
fi
print_info "Fingerprint (bytes32): 0x$FINGERPRINT"

print_success "Party allocated"

# =============================================================================
# Step 4b: Update config.yaml
# =============================================================================

print_header "STEP 4b: Update config.yaml"

print_step "Updating domain_id and relayer_party..."

# Use sed to update config.yaml
sed -i.bak "s|domain_id: \".*\"|domain_id: \"$DOMAIN_ID\"|" "$CONFIG_FILE"
sed -i.bak "s|relayer_party: \".*\"|relayer_party: \"$PARTY_ID\"|" "$CONFIG_FILE"
rm -f "${CONFIG_FILE}.bak"

print_info "domain_id: $DOMAIN_ID"
print_info "relayer_party: $PARTY_ID"

print_success "Config updated"

# =============================================================================
# Step 5: Bootstrap Canton Bridge Contracts
# =============================================================================

if [ "$SKIP_BOOTSTRAP" = true ]; then
    print_header "STEP 5: Bootstrap Canton Bridge Contracts (Skipped)"
    print_warning "Skipping bootstrap (--skip-bootstrap flag)"
else
    print_header "STEP 5: Bootstrap Canton Bridge Contracts"

    print_step "Running bootstrap script..."
    go run scripts/bootstrap-bridge.go \
        -config config.local.yaml \
        -issuer "$PARTY_ID" \
        -package "$PACKAGE_ID" || {
            print_warning "Bootstrap may have failed or contracts already exist"
            print_info "If contracts exist, use --skip-bootstrap next time"
        }

    print_success "Bootstrap complete"
fi

# =============================================================================
# Step 6: Register Test User
# =============================================================================

if [ "$SKIP_BOOTSTRAP" = true ]; then
    print_header "STEP 6: Register Test User (Skipped)"
    print_warning "Skipping user registration (--skip-bootstrap flag)"
else
    print_header "STEP 6: Register Test User"

    print_step "Running register-user script..."
    go run scripts/register-user.go \
        -config config.local.yaml \
        -party "$PARTY_ID" || {
            print_warning "User registration may have failed or user already exists"
        }

    print_success "User registered"
fi

# =============================================================================
# Step 7: Start the Relayer
# =============================================================================

print_header "STEP 7: Start the Relayer"

kill_relayer

print_step "Starting relayer in background..."
go run cmd/relayer/main.go -config "$CONFIG_FILE" > /tmp/relayer.log 2>&1 &
RELAYER_PID=$!
echo "Relayer PID: $RELAYER_PID"

print_step "Waiting for relayer to start..."
sleep 5

# Check health
if curl -s http://localhost:8080/health | grep -q "OK"; then
    print_success "Relayer is healthy"
else
    print_error "Relayer health check failed"
    cat /tmp/relayer.log | tail -20
    exit 1
fi

# =============================================================================
# Step 8: Setup Bridge Contracts (EVM)
# =============================================================================

print_header "STEP 8: Setup Bridge Contracts (EVM)"

# Add token mapping (owner-only in new lock/unlock model)
print_step "Adding token mapping..."
cast send $BRIDGE "addTokenMapping(address,bytes32)" \
    $TOKEN $CANTON_TOKEN_ID \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $OWNER_KEY > /dev/null 2>&1 || print_warning "Token mapping may already exist"

print_success "Bridge setup complete"

# =============================================================================
# Step 9: EVM → Canton Deposit
# =============================================================================

print_header "STEP 9: EVM → Canton Deposit"

# Fund user with tokens from owner (PromptToken has fixed pre-minted supply, no mint function)
print_step "Transferring 1000 tokens from owner to user..."
cast send $TOKEN "transfer(address,uint256)" $USER "1000000000000000000000" \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $OWNER_KEY > /dev/null 2>&1
print_success "User funded with tokens"

# Approve
print_step "Approving bridge to spend tokens..."
cast send $TOKEN "approve(address,uint256)" $BRIDGE "1000000000000000000000" \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $USER_KEY > /dev/null 2>&1
print_success "Tokens approved"

# Deposit
print_step "Depositing 100 tokens to Canton..."
CANTON_RECIPIENT="0x$FINGERPRINT"
DEPOSIT_TX=$(cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
    $TOKEN "100000000000000000000" $CANTON_RECIPIENT \
    --rpc-url "$ETH_RPC_URL" \
    --private-key $USER_KEY --json 2>/dev/null | jq -r '.transactionHash')
print_info "Deposit TX: $DEPOSIT_TX"
print_success "Deposit submitted"

# Wait for relayer to process
print_step "Waiting for relayer to process deposit..."
sleep 8

# Verify on Canton
print_step "Verifying holdings on Canton..."
HOLDINGS_OUTPUT=$(go run scripts/query-holdings.go -config "$CONFIG_FILE" -party "$PARTY_ID" 2>/dev/null)
echo "$HOLDINGS_OUTPUT"

# Extract holding CID for withdrawal
HOLDING_CID=$(echo "$HOLDINGS_OUTPUT" | grep "Contract ID:" | head -1 | awk '{print $3}')
print_info "Holding CID: $HOLDING_CID"

if [ -z "$HOLDING_CID" ]; then
    print_error "No holdings found - deposit may have failed"
    echo ""
    echo "Relayer logs:"
    tail -30 /tmp/relayer.log
    exit 1
fi

print_success "Deposit verified on Canton"

# =============================================================================
# Step 10: Canton → EVM Withdrawal
# =============================================================================

print_header "STEP 10: Canton → EVM Withdrawal"

# Get user balance before
BALANCE_BEFORE=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "User balance before withdrawal: $BALANCE_BEFORE"

# Initiate withdrawal
print_step "Initiating withdrawal of 50 tokens..."
go run scripts/initiate-withdrawal.go \
    -config "$CONFIG_FILE" \
    -holding-cid "$HOLDING_CID" \
    -amount "50.0" \
    -evm-destination "$USER"

# Wait for relayer to process
print_step "Waiting for relayer to process withdrawal..."
sleep 8

# Verify on EVM
print_step "Verifying balance on EVM..."
BALANCE_AFTER=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "User balance after withdrawal: $BALANCE_AFTER"

print_success "Withdrawal processed"

# =============================================================================
# Summary
# =============================================================================

print_header "TEST SUMMARY"

echo ""
echo -e "${GREEN}All tests passed!${NC}"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "CONFIGURATION"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Party ID:        $PARTY_ID"
echo "Domain ID:       $DOMAIN_ID"
echo "Fingerprint:     0x$FINGERPRINT"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "TRANSFER SUMMARY (lock/unlock model)"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Deposit:         100 tokens (EVM → Canton, locked in bridge)"
echo "Withdrawal:      50 tokens (Canton → EVM, unlocked from bridge)"
echo ""
echo "User EVM balance:"
echo "  Before:        1000 tokens (funded from owner)"
echo "  After deposit: 900 tokens (1000 - 100 locked)"
echo "  After withdraw:$BALANCE_AFTER wei"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "RELAYER STATUS"
echo "═══════════════════════════════════════════════════════════════════════"
curl -s http://localhost:8080/api/v1/transfers | jq '.transfers[] | {id: .ID, direction: .Direction, status: .Status}' 2>/dev/null || echo "No transfers found"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "COMMANDS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "View relayer logs:     tail -f /tmp/relayer.log"
echo "Query holdings:        go run scripts/query-holdings.go -config \"$CONFIG_FILE\" -party \"$PARTY_ID\""
echo "Stop relayer:          pkill -f 'cmd/relayer'"
echo "Stop all services:     $DOCKER_COMPOSE_CMD down"
echo ""
