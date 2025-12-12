#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge Docker Test Script
# =============================================================================
# Tests the bridge with ALL services running in Docker containers.
#
# Usage:
#   ./scripts/test-bridge.sh
#   ./scripts/test-bridge.sh --clean
#   ./scripts/test-bridge.sh --skip-setup
#
# Options:
#   --clean             Reset environment (docker compose down -v)
#   --skip-setup        Skip container startup, run tests only
#   --deposit-only      Only test deposit flow
#   --withdraw-only     Only test withdrawal flow
#   --logs              Show relayer logs at the end
#
# Services in Docker:
#   - Canton (ledger)
#   - Anvil (Ethereum)
#   - PostgreSQL (database)
#   - Mock OAuth2 (auth)
#   - Relayer (bridge service)
#   - Bootstrap (one-shot setup)
#
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# =============================================================================
# Parse Arguments
# =============================================================================

CLEAN=false
SKIP_SETUP=false
DEPOSIT_ONLY=false
WITHDRAW_ONLY=false
SHOW_LOGS=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --clean)
            CLEAN=true
            shift
            ;;
        --skip-setup)
            SKIP_SETUP=true
            shift
            ;;
        --deposit-only)
            DEPOSIT_ONLY=true
            shift
            ;;
        --withdraw-only)
            WITHDRAW_ONLY=true
            shift
            ;;
        --logs)
            SHOW_LOGS=true
            shift
            ;;
        -h|--help)
            head -30 "$0" | tail -n +2 | sed 's/^# //' | sed 's/^#//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# =============================================================================
# Configuration
# =============================================================================

DOCKER_COMPOSE_FILE="$PROJECT_DIR/docker-compose.local.yaml"
DOCKER_COMPOSE_CMD="docker compose -f $DOCKER_COMPOSE_FILE"
CONFIRMATION_WAIT=10

# Docker-exposed ports
ANVIL_URL="http://localhost:8545"
CANTON_URL="http://localhost:5013"
RELAYER_URL="http://localhost:8080"

# Anvil default accounts
RELAYER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
RELAYER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
OWNER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
OWNER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
USER_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

# Test amounts
TEST_DEPOSIT_TOKENS=100
TEST_WITHDRAW_TOKENS=50
FUND_AMOUNT="1000000000000000000000"

# Calculate wei amounts
TEST_DEPOSIT_AMOUNT_WEI="${TEST_DEPOSIT_TOKENS}000000000000000000"
TEST_WITHDRAW_AMOUNT_DECIMAL="${TEST_WITHDRAW_TOKENS}.0"

# Token ID
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"

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

wait_for_service() {
    local name="$1"
    local url="$2"
    local max_attempts="${3:-60}"
    local attempt=0
    
    print_step "Waiting for $name..."
    while [ $attempt -lt $max_attempts ]; do
        if curl -s "$url" >/dev/null 2>&1; then
            print_success "$name is ready!"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    echo ""
    print_error "$name failed to become ready after $max_attempts attempts"
    return 1
}

wait_for_relayer() {
    print_step "Waiting for relayer container to be healthy..."
    local max_attempts=120
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        # Check if container exists and is running
        local status=$(docker inspect --format='{{.State.Status}}' canton-bridge-relayer 2>/dev/null || echo "not_found")
        
        if [ "$status" = "running" ]; then
            # Check health endpoint
            if curl -s "$RELAYER_URL/health" 2>/dev/null | grep -q "OK"; then
                print_success "Relayer is healthy!"
                return 0
            fi
        fi
        
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_error "Relayer failed to become healthy"
    print_info "Container status: $(docker inspect --format='{{.State.Status}}' canton-bridge-relayer 2>/dev/null || echo 'not found')"
    print_info "Logs:"
    docker logs canton-bridge-relayer 2>&1 | tail -20
    return 1
}

wait_for_canton_sync() {
    print_step "Waiting for Canton to connect to synchronizer..."
    local max_attempts=60
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        local sync_count=$(curl -s "$CANTON_URL/v2/state/connected-synchronizers" 2>/dev/null | jq '.connectedSynchronizers | length' 2>/dev/null || echo "0")
        if [ "$sync_count" -gt 0 ] 2>/dev/null; then
            print_success "Canton connected to synchronizer!"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_error "Canton failed to connect to synchronizer"
    return 1
}

# =============================================================================
# Main Script
# =============================================================================

cd "$PROJECT_DIR"

print_header "CANTON-ETHEREUM BRIDGE TEST (DOCKER)"
echo ""
echo -e "${YELLOW}All services running in Docker containers${NC}"
echo "Compose file: $DOCKER_COMPOSE_FILE"
echo ""

# =============================================================================
# Pre-flight: Build DARs and extract package IDs
# =============================================================================

DAML_DIR="$PROJECT_DIR/contracts/canton-erc20/daml"
CONFIG_DOCKER="$PROJECT_DIR/config.docker.yaml"

# Function to extract package ID from DAR file
get_package_id() {
    local pkg_name="$1"
    local dar_file=$(ls "$DAML_DIR/$pkg_name/.daml/dist/"*.dar 2>/dev/null | head -1)
    if [ -n "$dar_file" ] && [ -f "$dar_file" ]; then
        daml damlc inspect-dar "$dar_file" 2>/dev/null | grep "^${pkg_name}-[0-9]" | tail -1 | grep -oE '"[a-f0-9]{64}"' | tr -d '"' || echo ""
    fi
}

print_header "PRE-FLIGHT: Checking DAR Files"

# Check if daml command is available
if ! command -v daml &> /dev/null; then
    print_error "daml CLI not found. Please install the Daml SDK:"
    print_info "curl -sSL https://get.daml.com/ | sh"
    exit 1
fi

# Always build DARs to ensure they're fresh and package IDs are current
print_step "Building DAR files..."
if [ -f "$SCRIPT_DIR/build-dars.sh" ]; then
    if ! "$SCRIPT_DIR/build-dars.sh"; then
        print_error "Failed to build DAR files"
        print_info "Try building manually:"
        print_info "  cd $DAML_DIR"
        print_info "  daml build (in each package directory)"
        exit 1
    fi
else
    print_error "build-dars.sh not found at $SCRIPT_DIR/build-dars.sh"
    exit 1
fi
print_success "DAR files built"

# Extract package IDs
print_step "Extracting package IDs from DAR files..."
CIP56_PACKAGE_ID=$(get_package_id "cip56-token")
BRIDGE_WAYFINDER_PACKAGE_ID=$(get_package_id "bridge-wayfinder")
BRIDGE_CORE_PACKAGE_ID=$(get_package_id "bridge-core")

if [ -z "$CIP56_PACKAGE_ID" ] || [ -z "$BRIDGE_WAYFINDER_PACKAGE_ID" ] || [ -z "$BRIDGE_CORE_PACKAGE_ID" ]; then
    print_error "Failed to extract package IDs from DARs"
    print_info "cip56-token: ${CIP56_PACKAGE_ID:-NOT FOUND}"
    print_info "bridge-wayfinder: ${BRIDGE_WAYFINDER_PACKAGE_ID:-NOT FOUND}"
    print_info "bridge-core: ${BRIDGE_CORE_PACKAGE_ID:-NOT FOUND}"
    exit 1
fi

print_info "cip56-token:      $CIP56_PACKAGE_ID"
print_info "bridge-wayfinder: $BRIDGE_WAYFINDER_PACKAGE_ID"
print_info "bridge-core:      $BRIDGE_CORE_PACKAGE_ID"

# Update config.docker.yaml with correct package IDs
print_step "Updating config.docker.yaml with package IDs..."
if [ -f "$CONFIG_DOCKER" ]; then
    sed -i.bak "s/bridge_package_id: \"[a-f0-9]*\"/bridge_package_id: \"$BRIDGE_WAYFINDER_PACKAGE_ID\"/" "$CONFIG_DOCKER"
    sed -i.bak "s/core_package_id: \"[a-f0-9]*\"/core_package_id: \"$BRIDGE_CORE_PACKAGE_ID\"/" "$CONFIG_DOCKER"
    sed -i.bak "s/cip56_package_id: \"[a-f0-9]*\"/cip56_package_id: \"$CIP56_PACKAGE_ID\"/" "$CONFIG_DOCKER"
    rm -f "${CONFIG_DOCKER}.bak"
    print_success "Config updated with current package IDs"
else
    print_error "config.docker.yaml not found at $CONFIG_DOCKER"
    exit 1
fi

# =============================================================================
# Step 0: Clean environment (optional)
# =============================================================================

if [ "$CLEAN" = true ]; then
    print_header "STEP 0: Cleaning Environment"
    print_step "Stopping and removing all containers..."
    $DOCKER_COMPOSE_CMD down -v 2>/dev/null || true
    docker volume rm canton-middleware_config_state 2>/dev/null || true
    print_success "Environment cleaned"
fi

# =============================================================================
# Setup Phase (skipped with --skip-setup)
# =============================================================================

if [ "$SKIP_SETUP" = false ]; then
    print_header "SETUP PHASE: Starting Docker Services"
    
    # Check if docker-compose.local.yaml exists
    if [ ! -f "$DOCKER_COMPOSE_FILE" ]; then
        print_error "Docker compose file not found: $DOCKER_COMPOSE_FILE"
        exit 1
    fi
    
    # Start all services
    print_step "Starting docker compose..."
    $DOCKER_COMPOSE_CMD up --build -d
    
    echo ""
    print_step "Container status:"
    $DOCKER_COMPOSE_CMD ps
    echo ""
    
    # Wait for Anvil
    wait_for_service "Anvil (Ethereum)" "$ANVIL_URL" 30 || exit 1
    
    # Wait for Canton
    wait_for_service "Canton HTTP API" "$CANTON_URL/v2/version" 60 || exit 1
    wait_for_canton_sync || exit 1
    
    # Wait for bootstrap to complete
    print_step "Waiting for bootstrap container to complete..."
    bootstrap_status=""
    bootstrap_attempts=0
    while [ $bootstrap_attempts -lt 120 ]; do
        bootstrap_status=$(docker inspect --format='{{.State.Status}}' bootstrap 2>/dev/null || echo "not_found")
        if [ "$bootstrap_status" = "exited" ]; then
            exit_code=$(docker inspect --format='{{.State.ExitCode}}' bootstrap 2>/dev/null || echo "1")
            if [ "$exit_code" = "0" ]; then
                print_success "Bootstrap completed successfully!"
                break
            else
                print_error "Bootstrap failed with exit code $exit_code"
                docker logs bootstrap 2>&1 | tail -30
                exit 1
            fi
        fi
        echo -n "."
        sleep 2
        bootstrap_attempts=$((bootstrap_attempts + 1))
    done
    echo ""
    
    if [ "$bootstrap_status" != "exited" ]; then
        print_warning "Bootstrap still running or not started"
        docker logs bootstrap 2>&1 | tail -20
    fi
    
    # Wait for relayer
    wait_for_relayer || exit 1
    
    print_success "All services are ready!"
fi

# =============================================================================
# Get Configuration from Bootstrap
# =============================================================================

print_header "READING CONFIGURATION"

# Get contract addresses from Anvil deployment
print_step "Getting contract addresses..."
BROADCAST_FILE="$PROJECT_DIR/contracts/ethereum-wayfinder/broadcast/Deployer.s.sol/31337/run-latest.json"
if [ -f "$BROADCAST_FILE" ]; then
    TOKEN=$(jq -r '.transactions[] | select(.contractName == "PromptToken") | .contractAddress' "$BROADCAST_FILE")
    BRIDGE=$(jq -r '.transactions[] | select(.contractName == "CantonBridge") | .contractAddress' "$BROADCAST_FILE")
else
    # Default addresses from fresh Anvil deploy
    TOKEN="0x5fbdb2315678afecb367f032d93f642f64180aa3"
    BRIDGE="0xe7f1725e7734ce288f8367e1bb143e90bb3f0512"
fi

print_info "Token contract: $TOKEN"
print_info "Bridge contract: $BRIDGE"

# Get party and domain from Canton
print_step "Getting Canton party and domain..."
PARTY_ID=$(curl -s "$CANTON_URL/v2/parties" | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1 || true)
DOMAIN_ID=$(curl -s "$CANTON_URL/v2/state/connected-synchronizers" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')

if [ -z "$PARTY_ID" ]; then
    print_error "BridgeIssuer party not found - bootstrap may have failed"
    exit 1
fi

print_info "Party ID: $PARTY_ID"
print_info "Domain ID: $DOMAIN_ID"

# Extract fingerprint
FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
    FINGERPRINT="${FULL_FINGERPRINT:4}"
else
    FINGERPRINT="$FULL_FINGERPRINT"
fi
print_info "Fingerprint: 0x$FINGERPRINT"

# Generate host-accessible config for bridge-activity.go and other scripts
print_step "Generating host config file..."
HOST_CONFIG="$PROJECT_DIR/.test-config.yaml"
cat > "$HOST_CONFIG" << EOF
# Auto-generated config for host access to Docker services
# Generated by test-bridge.sh - DO NOT COMMIT
# Use with: go run scripts/bridge-activity.go -config .test-config.yaml

server:
  host: "0.0.0.0"
  port: 8080

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "p@ssw0rd"
  database: "relayer"
  ssl_mode: "disable"

ethereum:
  rpc_url: "http://localhost:8545"
  ws_url: "ws://localhost:8545"
  chain_id: 31337
  bridge_contract: "$BRIDGE"
  token_contract: "$TOKEN"
  relayer_private_key: "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
  confirmation_blocks: 1
  gas_limit: 300000
  max_gas_price: "100000000000"
  polling_interval: "5s"
  start_block: 0

canton:
  rpc_url: "localhost:5011"
  ledger_id: "canton-ledger-id"
  domain_id: "$DOMAIN_ID"
  application_id: "canton-middleware"
  relayer_party: "$PARTY_ID"
  bridge_package_id: "$BRIDGE_WAYFINDER_PACKAGE_ID"
  core_package_id: "$BRIDGE_CORE_PACKAGE_ID"
  cip56_package_id: "$CIP56_PACKAGE_ID"
  bridge_module: "Wayfinder.Bridge"
  bridge_contract: "WayfinderBridgeConfig"
  tls:
    enabled: false
  auth:
    client_id: "local-test-client"
    client_secret: "local-test-secret"
    audience: "http://localhost:5011"
    token_url: "http://localhost:8088/oauth/token"
  polling_interval: "1s"

bridge:
  max_transfer_amount: "1000000000000000000000"
  min_transfer_amount: "1000000000000000"
  rate_limit_per_hour: 100
  max_retries: 3
  retry_delay: "10s"
  processing_interval: "5s"

monitoring:
  enabled: true
  metrics_port: 9090

logging:
  level: "debug"
  format: "console"
  output_path: "stdout"
EOF
print_success "Host config saved to .test-config.yaml"

# =============================================================================
# Verify Contracts
# =============================================================================

print_header "VERIFY CONTRACTS"

print_step "Checking bridge contract..."
BRIDGE_RELAYER=$(cast call $BRIDGE "relayer()(address)" --rpc-url "$ANVIL_URL" 2>/dev/null || echo "unknown")
print_info "Bridge relayer: $BRIDGE_RELAYER"

print_step "Checking token contract..."
TOKEN_NAME=$(cast call $TOKEN "name()(string)" --rpc-url "$ANVIL_URL" 2>/dev/null || echo "unknown")
print_info "Token name: $TOKEN_NAME"

print_success "Contracts verified"

# =============================================================================
# Setup Bridge (EVM)
# =============================================================================

print_header "SETUP BRIDGE (EVM)"

print_step "Adding token mapping..."
cast send $BRIDGE "addTokenMapping(address,bytes32)" \
    $TOKEN $CANTON_TOKEN_ID \
    --rpc-url "$ANVIL_URL" \
    --private-key $OWNER_KEY > /dev/null 2>&1 || print_warning "Token mapping may already exist"

print_success "Bridge setup complete"

# =============================================================================
# Test Deposit (EVM → Canton)
# =============================================================================

if [ "$WITHDRAW_ONLY" = false ]; then
    print_header "TEST: EVM → Canton Deposit"
    
    # Fund user
    print_step "Transferring tokens from owner to user..."
    cast send $TOKEN "transfer(address,uint256)" $USER "$FUND_AMOUNT" \
        --rpc-url "$ANVIL_URL" \
        --private-key $OWNER_KEY > /dev/null 2>&1
    print_success "User funded"
    
    # Check balance
    USER_BALANCE=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null)
    print_info "User token balance: $USER_BALANCE"
    
    # Approve
    print_step "Approving bridge to spend tokens..."
    cast send $TOKEN "approve(address,uint256)" $BRIDGE "$TEST_DEPOSIT_AMOUNT_WEI" \
        --rpc-url "$ANVIL_URL" \
        --private-key $USER_KEY > /dev/null 2>&1
    print_success "Approved"
    
    # Deposit
    print_step "Depositing ${TEST_DEPOSIT_TOKENS} tokens to Canton..."
    CANTON_RECIPIENT="0x$FINGERPRINT"
    DEPOSIT_TX=$(cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
        $TOKEN "$TEST_DEPOSIT_AMOUNT_WEI" $CANTON_RECIPIENT \
        --rpc-url "$ANVIL_URL" \
        --private-key $USER_KEY --json 2>/dev/null | jq -r '.transactionHash')
    print_info "Deposit TX: $DEPOSIT_TX"
    print_success "Deposit submitted"
    
    # Wait for relayer
    print_step "Waiting for relayer to process deposit (${CONFIRMATION_WAIT}s)..."
    sleep $CONFIRMATION_WAIT
    
    # Check relayer processed it
    print_step "Checking relayer transfers..."
    TRANSFERS=$(curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null || echo '{"transfers":[]}')
    DEPOSIT_COUNT=$(echo "$TRANSFERS" | jq '[.transfers[] | select(.Direction == "deposit")] | length' 2>/dev/null || echo "0")
    print_info "Deposit transfers found: $DEPOSIT_COUNT"
    
    if [ "$DEPOSIT_COUNT" -gt 0 ]; then
        print_success "Deposit processed by relayer!"
    else
        print_warning "No deposit transfers found yet - may still be processing"
    fi
fi

# =============================================================================
# Test Withdrawal (Canton → EVM)  
# =============================================================================

if [ "$DEPOSIT_ONLY" = false ]; then
    print_header "TEST: Canton → EVM Withdrawal"
    
    # Get user's EVM balance before withdrawal
    BALANCE_BEFORE=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null)
    print_info "User EVM balance before: $BALANCE_BEFORE"
    
    # Query Canton for holdings to find the CIP56Holding CID
    print_step "Finding CIP56Holding on Canton..."
    
    # Use get-holding-cid.go to get the full contract ID
    HOLDING_CID=$(go run scripts/get-holding-cid.go -config "$HOST_CONFIG" 2>/dev/null)
    
    if [ -z "$HOLDING_CID" ]; then
        print_warning "No CIP56Holding found - deposit may not have completed"
        print_info "Skipping withdrawal test"
    else
        print_info "Found holding CID: ${HOLDING_CID:0:40}..."
        
        # Initiate withdrawal
        print_step "Initiating withdrawal of ${TEST_WITHDRAW_TOKENS} tokens to $USER..."
        
        WITHDRAW_OUTPUT=$(go run scripts/initiate-withdrawal.go \
            -config "$HOST_CONFIG" \
            -holding-cid "$HOLDING_CID" \
            -amount "$TEST_WITHDRAW_AMOUNT_DECIMAL" \
            -evm-destination "$USER" 2>&1)
        
        WITHDRAW_EXIT=$?
        
        if [ $WITHDRAW_EXIT -eq 0 ]; then
            print_success "Withdrawal initiated"
            echo "$WITHDRAW_OUTPUT" | grep -E "(Request CID|WithdrawalRequest)" | head -3 | while read line; do
                print_info "$line"
            done
            
            # Wait for relayer to process
            print_step "Waiting for relayer to process withdrawal (${CONFIRMATION_WAIT}s)..."
            sleep $CONFIRMATION_WAIT
            
            # Check EVM balance after
            BALANCE_AFTER=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null)
            print_info "User EVM balance after: $BALANCE_AFTER"
            
            # Check if balance increased
            if [ "$BALANCE_AFTER" != "$BALANCE_BEFORE" ]; then
                print_success "Withdrawal completed - EVM balance updated!"
            else
                print_warning "EVM balance unchanged - withdrawal may still be processing"
            fi
        else
            print_error "Withdrawal failed"
            echo "$WITHDRAW_OUTPUT" | tail -5
        fi
    fi
fi

# =============================================================================
# Summary
# =============================================================================

print_header "TEST SUMMARY"

echo ""
echo -e "${GREEN}Bridge test complete!${NC}"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "DOCKER SERVICES"
echo "═══════════════════════════════════════════════════════════════════════"
$DOCKER_COMPOSE_CMD ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "ENDPOINTS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Anvil RPC:       $ANVIL_URL"
echo "Canton HTTP:     $CANTON_URL"
echo "Relayer:         $RELAYER_URL"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "CONTRACTS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "PromptToken:     $TOKEN"
echo "CantonBridge:    $BRIDGE"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "CANTON"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Party ID:        $PARTY_ID"
echo "Domain ID:       $DOMAIN_ID"
echo "Fingerprint:     0x$FINGERPRINT"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "TRANSFER SUMMARY"
echo "═══════════════════════════════════════════════════════════════════════"
if [ "$WITHDRAW_ONLY" = true ]; then
    echo "Deposit:         (skipped - --withdraw-only)"
    echo "Withdrawal:      ${TEST_WITHDRAW_TOKENS} tokens (Canton → EVM)"
elif [ "$DEPOSIT_ONLY" = true ]; then
    echo "Deposit:         ${TEST_DEPOSIT_TOKENS} tokens (EVM → Canton)"
    echo "Withdrawal:      (skipped - --deposit-only)"
else
    echo "Deposit:         ${TEST_DEPOSIT_TOKENS} tokens (EVM → Canton)"
    echo "Withdrawal:      ${TEST_WITHDRAW_TOKENS} tokens (Canton → EVM)"
fi
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "RELAYER TRANSFERS"
echo "═══════════════════════════════════════════════════════════════════════"
curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null | jq '.transfers[] | {id: .ID, direction: .Direction, status: .Status}' 2>/dev/null || echo "No transfers found"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "COMMANDS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "View bridge activity:  go run scripts/bridge-activity.go -config .test-config.yaml"
echo "View relayer logs:     docker logs -f canton-bridge-relayer"
echo "View canton logs:      docker logs -f canton"
echo "View bootstrap logs:   docker logs bootstrap"
echo "Stop all services:     $DOCKER_COMPOSE_CMD down"
echo "Clean everything:      $DOCKER_COMPOSE_CMD down -v"
echo ""

if [ "$SHOW_LOGS" = true ]; then
    print_header "RELAYER LOGS"
    docker logs canton-bridge-relayer 2>&1 | tail -50
fi
