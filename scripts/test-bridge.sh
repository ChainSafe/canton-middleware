#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge Manager
# =============================================================================
# Manages the Canton-Ethereum bridge test environment with Docker containers.
#
# Usage:
#   ./scripts/test-bridge.sh              # Interactive menu (default)
#   ./scripts/test-bridge.sh --start-only # Start services without tests
#   ./scripts/test-bridge.sh --full-test  # Run complete test flow
#   ./scripts/test-bridge.sh --deposit 50 # Deposit 50 tokens
#   ./scripts/test-bridge.sh --withdraw 25 # Withdraw 25 tokens
#
# Options:
#   --start-only        Start services without running tests
#   --stop              Stop all containers (keep data)
#   --clean             Reset environment (docker compose down -v)
#   --full-test         Run complete test (100 deposit + 50 withdraw)
#   --deposit <amount>  Deposit specified token amount
#   --withdraw <amount> Withdraw specified token amount
#   --status            Show container and transfer status
#   -i, --interactive   Force interactive menu
#   --skip-setup        Skip container startup (assume running)
#   --logs              Show relayer logs
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

# =============================================================================
# Colors and Constants
# =============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DOCKER_COMPOSE_CMD="docker compose"

# Docker-exposed ports
ANVIL_URL="http://localhost:8545"
CANTON_URL="http://localhost:5013"
RELAYER_URL="http://localhost:8080"

# Anvil default accounts
OWNER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
OWNER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
USER_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

# Token ID
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"
FUND_AMOUNT="1000000000000000000000"

# Paths
DAML_DIR="$PROJECT_DIR/contracts/canton-erc20/daml"
CONFIG_DOCKER="$PROJECT_DIR/config.docker.yaml"
HOST_CONFIG="$PROJECT_DIR/.test-config.yaml"

# Global state (set by load_configuration)
TOKEN=""
BRIDGE=""
PARTY_ID=""
DOMAIN_ID=""
FINGERPRINT=""
CIP56_PACKAGE_ID=""
BRIDGE_WAYFINDER_PACKAGE_ID=""
BRIDGE_CORE_PACKAGE_ID=""

# =============================================================================
# Output Helpers
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

# =============================================================================
# Wait Helpers
# =============================================================================

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
        local status=$(docker inspect --format='{{.State.Status}}' canton-bridge-relayer 2>/dev/null || echo "not_found")
        
        if [ "$status" = "running" ]; then
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

wait_for_deposit_confirmation() {
    local tx_hash="$1"
    local max_attempts=30
    local attempt=0
    
    local initial_transfers=$(curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null || echo '{"transfers":[]}')
    local initial_count=$(echo "$initial_transfers" | jq '[.transfers[] | select(.Direction == "ethereum_to_canton" or .Direction == "deposit")] | length' 2>/dev/null || echo "0")
    
    print_step "Waiting for deposit confirmation..."
    while [ $attempt -lt $max_attempts ]; do
        local transfers=$(curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null || echo '{"transfers":[]}')
        local completed=$(echo "$transfers" | jq '[.transfers[] | select((.Direction == "ethereum_to_canton" or .Direction == "deposit") and .Status == "completed")] | length' 2>/dev/null || echo "0")
        local current_count=$(echo "$transfers" | jq '[.transfers[] | select(.Direction == "ethereum_to_canton" or .Direction == "deposit")] | length' 2>/dev/null || echo "0")
        
        if [ "$completed" -gt 0 ] && [ "$current_count" -gt "$initial_count" ]; then
            echo ""
            print_success "Deposit confirmed!"
            return 0
        fi
        
        if [ "$current_count" -gt "$initial_count" ]; then
            local latest_status=$(echo "$transfers" | jq -r '[.transfers[] | select(.Direction == "ethereum_to_canton" or .Direction == "deposit")][-1].Status' 2>/dev/null)
            if [ "$latest_status" = "completed" ]; then
                echo ""
                print_success "Deposit confirmed!"
                return 0
            fi
        fi
        
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_warning "Deposit confirmation timed out after $((max_attempts * 2))s"
    return 1
}

wait_for_withdrawal_confirmation() {
    local balance_before="$1"
    local max_attempts=30
    local attempt=0
    
    local before_num=$(echo "$balance_before" | awk '{print $1}')
    
    print_step "Waiting for withdrawal confirmation..."
    while [ $attempt -lt $max_attempts ]; do
        local balance_after=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null)
        local after_num=$(echo "$balance_after" | awk '{print $1}')
        
        if echo "$after_num > $before_num" | bc -l 2>/dev/null | grep -q "1"; then
            echo ""
            print_info "User EVM balance after: $balance_after"
            print_success "Withdrawal confirmed - EVM balance updated!"
            return 0
        fi
        
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_warning "Withdrawal confirmation timed out after $((max_attempts * 2))s"
    return 1
}

# =============================================================================
# Core Functions
# =============================================================================

get_package_id() {
    local pkg_name="$1"
    local dar_file=$(ls "$DAML_DIR/$pkg_name/.daml/dist/"*.dar 2>/dev/null | head -1)
    if [ -n "$dar_file" ] && [ -f "$dar_file" ]; then
        daml damlc inspect-dar "$dar_file" 2>/dev/null | grep "^${pkg_name}-[0-9]" | tail -1 | grep -oE '"[a-f0-9]{64}"' | tr -d '"' || echo ""
    fi
}

preflight_checks() {
    print_header "PRE-FLIGHT: Checking DAR Files"
    
    if ! command -v daml &> /dev/null; then
        print_error "daml CLI not found. Please install the Daml SDK:"
        print_info "curl -sSL https://get.daml.com/ | sh"
        return 1
    fi
    
    print_step "Building DAR files..."
    if [ -f "$SCRIPT_DIR/build-dars.sh" ]; then
        if ! "$SCRIPT_DIR/build-dars.sh"; then
            print_error "Failed to build DAR files"
            return 1
        fi
    else
        print_error "build-dars.sh not found"
        return 1
    fi
    print_success "DAR files built"
    
    print_step "Extracting package IDs from DAR files..."
    CIP56_PACKAGE_ID=$(get_package_id "cip56-token")
    BRIDGE_WAYFINDER_PACKAGE_ID=$(get_package_id "bridge-wayfinder")
    BRIDGE_CORE_PACKAGE_ID=$(get_package_id "bridge-core")
    
    if [ -z "$CIP56_PACKAGE_ID" ] || [ -z "$BRIDGE_WAYFINDER_PACKAGE_ID" ] || [ -z "$BRIDGE_CORE_PACKAGE_ID" ]; then
        print_error "Failed to extract package IDs from DARs"
        print_info "cip56-token: ${CIP56_PACKAGE_ID:-NOT FOUND}"
        print_info "bridge-wayfinder: ${BRIDGE_WAYFINDER_PACKAGE_ID:-NOT FOUND}"
        print_info "bridge-core: ${BRIDGE_CORE_PACKAGE_ID:-NOT FOUND}"
        return 1
    fi
    
    print_info "cip56-token:      $CIP56_PACKAGE_ID"
    print_info "bridge-wayfinder: $BRIDGE_WAYFINDER_PACKAGE_ID"
    print_info "bridge-core:      $BRIDGE_CORE_PACKAGE_ID"
    
    print_step "Updating config.docker.yaml with package IDs..."
    if [ -f "$CONFIG_DOCKER" ]; then
        sed -i.bak "s/bridge_package_id: \"[a-f0-9]*\"/bridge_package_id: \"$BRIDGE_WAYFINDER_PACKAGE_ID\"/" "$CONFIG_DOCKER"
        sed -i.bak "s/core_package_id: \"[a-f0-9]*\"/core_package_id: \"$BRIDGE_CORE_PACKAGE_ID\"/" "$CONFIG_DOCKER"
        sed -i.bak "s/cip56_package_id: \"[a-f0-9]*\"/cip56_package_id: \"$CIP56_PACKAGE_ID\"/" "$CONFIG_DOCKER"
        rm -f "${CONFIG_DOCKER}.bak"
        print_success "Config updated with current package IDs"
    else
        print_error "config.docker.yaml not found"
        return 1
    fi
    
    return 0
}

clean_environment() {
    print_header "Cleaning Environment"
    print_step "Stopping and removing all containers and volumes..."
    $DOCKER_COMPOSE_CMD down -v 2>/dev/null || true
    docker volume rm canton-middleware_config_state 2>/dev/null || true
    print_success "Environment cleaned"
}

stop_services() {
    print_header "Stopping Services"
    print_step "Stopping all containers..."
    $DOCKER_COMPOSE_CMD down 2>/dev/null || true
    print_success "Services stopped"
}

start_services() {
    print_header "Starting Docker Services"
    
    print_step "Starting docker compose..."
    $DOCKER_COMPOSE_CMD up --build -d
    
    echo ""
    print_step "Container status:"
    $DOCKER_COMPOSE_CMD ps
    echo ""
    
    wait_for_service "Anvil (Ethereum)" "$ANVIL_URL" 30 || return 1
    wait_for_service "Canton HTTP API" "$CANTON_URL/v2/version" 60 || return 1
    wait_for_canton_sync || return 1
    
    print_step "Waiting for bootstrap container to complete..."
    local bootstrap_status=""
    local bootstrap_attempts=0
    while [ $bootstrap_attempts -lt 120 ]; do
        bootstrap_status=$(docker inspect --format='{{.State.Status}}' bootstrap 2>/dev/null || echo "not_found")
        if [ "$bootstrap_status" = "exited" ]; then
            local exit_code=$(docker inspect --format='{{.State.ExitCode}}' bootstrap 2>/dev/null || echo "1")
            if [ "$exit_code" = "0" ]; then
                print_success "Bootstrap completed successfully!"
                break
            else
                print_error "Bootstrap failed with exit code $exit_code"
                docker logs bootstrap 2>&1 | tail -30
                return 1
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
    
    wait_for_relayer || return 1
    
    print_success "All services are ready!"
    return 0
}

load_configuration() {
    print_header "Loading Configuration"
    
    print_step "Getting contract addresses..."
    local BROADCAST_FILE="$PROJECT_DIR/contracts/ethereum-wayfinder/broadcast/Deployer.s.sol/31337/run-latest.json"
    if [ -f "$BROADCAST_FILE" ]; then
        TOKEN=$(jq -r '.transactions[] | select(.contractName == "PromptToken") | .contractAddress' "$BROADCAST_FILE")
        BRIDGE=$(jq -r '.transactions[] | select(.contractName == "CantonBridge") | .contractAddress' "$BROADCAST_FILE")
    else
        TOKEN="0x5fbdb2315678afecb367f032d93f642f64180aa3"
        BRIDGE="0xe7f1725e7734ce288f8367e1bb143e90bb3f0512"
    fi
    print_info "Token contract: $TOKEN"
    print_info "Bridge contract: $BRIDGE"
    
    print_step "Getting Canton party and domain..."
    PARTY_ID=$(curl -s "$CANTON_URL/v2/parties" | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1 || true)
    DOMAIN_ID=$(curl -s "$CANTON_URL/v2/state/connected-synchronizers" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')
    
    if [ -z "$PARTY_ID" ]; then
        print_error "BridgeIssuer party not found - bootstrap may have failed"
        return 1
    fi
    
    print_info "Party ID: $PARTY_ID"
    print_info "Domain ID: $DOMAIN_ID"
    
    local FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
    if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
        FINGERPRINT="${FULL_FINGERPRINT:4}"
    else
        FINGERPRINT="$FULL_FINGERPRINT"
    fi
    print_info "Fingerprint: 0x$FINGERPRINT"
    
    # Extract package IDs if not already set (needed when script restarts with services running)
    if [ -z "$BRIDGE_WAYFINDER_PACKAGE_ID" ]; then
        # Find DAR files dynamically (version-agnostic)
        local WAYFINDER_DAR=$(ls "$PROJECT_DIR/contracts/canton-erc20/daml/bridge-wayfinder/.daml/dist/"*.dar 2>/dev/null | head -1)
        local CORE_DAR=$(ls "$PROJECT_DIR/contracts/canton-erc20/daml/bridge-core/.daml/dist/"*.dar 2>/dev/null | head -1)
        local CIP56_DAR=$(ls "$PROJECT_DIR/contracts/canton-erc20/daml/cip56-token/.daml/dist/"*.dar 2>/dev/null | head -1)
        
        if [ -n "$WAYFINDER_DAR" ] && [ -n "$CORE_DAR" ] && [ -n "$CIP56_DAR" ]; then
            BRIDGE_WAYFINDER_PACKAGE_ID=$(daml damlc inspect-dar "$WAYFINDER_DAR" 2>/dev/null | grep "^package_id:" | awk '{print $2}' | head -1)
            BRIDGE_CORE_PACKAGE_ID=$(daml damlc inspect-dar "$CORE_DAR" 2>/dev/null | grep "^package_id:" | awk '{print $2}' | head -1)
            CIP56_PACKAGE_ID=$(daml damlc inspect-dar "$CIP56_DAR" 2>/dev/null | grep "^package_id:" | awk '{print $2}' | head -1)
            print_info "Package IDs loaded from DAR files"
        else
            print_warning "DAR files not found - package IDs will be empty"
        fi
    fi
    
    # Generate host config
    print_step "Generating host config file..."
    cat > "$HOST_CONFIG" << EOF
# Auto-generated config for host access to Docker services
# Generated by test-bridge.sh - DO NOT COMMIT

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
    return 0
}

setup_bridge() {
    print_header "Setup Bridge (EVM)"
    
    print_step "Checking bridge contract..."
    local BRIDGE_RELAYER=$(cast call $BRIDGE "relayer()(address)" --rpc-url "$ANVIL_URL" 2>/dev/null || echo "unknown")
    print_info "Bridge relayer: $BRIDGE_RELAYER"
    
    print_step "Adding token mapping..."
    cast send $BRIDGE "addTokenMapping(address,bytes32)" \
        $TOKEN $CANTON_TOKEN_ID \
        --rpc-url "$ANVIL_URL" \
        --private-key $OWNER_KEY > /dev/null 2>&1 || print_warning "Token mapping may already exist"
    
    print_success "Bridge setup complete"
}

run_deposit() {
    local amount="$1"
    
    if [ -z "$amount" ] || [ "$amount" -le 0 ] 2>/dev/null; then
        print_error "Invalid deposit amount: $amount"
        return 1
    fi
    
    local amount_wei="${amount}000000000000000000"
    
    print_header "Deposit: $amount tokens (EVM → Canton)"
    
    # Fund user if needed
    print_step "Checking user balance..."
    local user_balance=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null | awk '{print $1}')
    if [ -z "$user_balance" ] || [ "$user_balance" = "0" ]; then
        print_step "Funding user account..."
        cast send $TOKEN "transfer(address,uint256)" $USER "$FUND_AMOUNT" \
            --rpc-url "$ANVIL_URL" \
            --private-key $OWNER_KEY > /dev/null 2>&1
        print_success "User funded"
    fi
    
    user_balance=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null)
    print_info "User token balance: $user_balance"
    
    # Approve
    print_step "Approving bridge to spend tokens..."
    cast send $TOKEN "approve(address,uint256)" $BRIDGE "$amount_wei" \
        --rpc-url "$ANVIL_URL" \
        --private-key $USER_KEY > /dev/null 2>&1
    print_success "Approved"
    
    # Deposit
    print_step "Depositing $amount tokens to Canton..."
    local canton_recipient="0x$FINGERPRINT"
    local deposit_tx=$(cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
        $TOKEN "$amount_wei" $canton_recipient \
        --rpc-url "$ANVIL_URL" \
        --private-key $USER_KEY --json 2>/dev/null | jq -r '.transactionHash')
    print_info "Deposit TX: $deposit_tx"
    print_success "Deposit submitted"
    
    wait_for_deposit_confirmation "$deposit_tx"
}

run_withdrawal() {
    local amount="$1"
    
    if [ -z "$amount" ] || [ "$amount" -le 0 ] 2>/dev/null; then
        print_error "Invalid withdrawal amount: $amount"
        return 1
    fi
    
    local amount_decimal="${amount}.0"
    
    print_header "Withdrawal: $amount tokens (Canton → EVM)"
    
    local balance_before=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ANVIL_URL" 2>/dev/null)
    print_info "User EVM balance before: $balance_before"
    
    print_step "Finding CIP56Holding on Canton..."
    local holding_info=$(go run scripts/get-holding-cid.go -config "$HOST_CONFIG" -with-balance 2>/dev/null)
    
    if [ -z "$holding_info" ]; then
        print_error "No CIP56Holding found - you may need to deposit first"
        return 1
    fi
    
    local holding_cid=$(echo "$holding_info" | awk '{print $1}')
    local holding_balance=$(echo "$holding_info" | awk '{print $2}' | cut -d'.' -f1)
    
    print_info "Found holding CID: ${holding_cid:0:40}..."
    print_info "Available balance: $holding_balance tokens"
    
    # Check if sufficient balance
    if [ "$amount" -gt "$holding_balance" ] 2>/dev/null; then
        print_error "Insufficient balance: requested $amount but only $holding_balance available"
        return 1
    fi
    
    print_step "Initiating withdrawal of $amount tokens to $USER..."
    local withdraw_output=$(go run scripts/initiate-withdrawal.go \
        -config "$HOST_CONFIG" \
        -holding-cid "$holding_cid" \
        -amount "$amount_decimal" \
        -evm-destination "$USER" 2>&1)
    
    local withdraw_exit=$?
    
    if [ $withdraw_exit -eq 0 ]; then
        print_success "Withdrawal initiated"
        echo "$withdraw_output" | grep -E "(Request CID|WithdrawalRequest)" | head -2 | while read line; do
            print_info "$line"
        done
        wait_for_withdrawal_confirmation "$balance_before"
    else
        print_error "Withdrawal failed"
        echo "$withdraw_output" | tail -5
        return 1
    fi
}

show_status() {
    print_header "Status"
    
    echo ""
    echo -e "${BOLD}DOCKER SERVICES${NC}"
    echo "═══════════════════════════════════════════════════════════════════════"
    $DOCKER_COMPOSE_CMD ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "No containers running"
    echo ""
    
    # Check if services are running
    local relayer_status=$(docker inspect --format='{{.State.Status}}' canton-bridge-relayer 2>/dev/null || echo "not_running")
    if [ "$relayer_status" != "running" ]; then
        print_warning "Services are not running. Use option 7 to start."
        return
    fi
    
    echo -e "${BOLD}ENDPOINTS${NC}"
    echo "═══════════════════════════════════════════════════════════════════════"
    echo "Anvil RPC:       $ANVIL_URL"
    echo "Canton HTTP:     $CANTON_URL"
    echo "Relayer:         $RELAYER_URL"
    echo ""
    
    if [ -n "$TOKEN" ]; then
        echo -e "${BOLD}CONTRACTS${NC}"
        echo "═══════════════════════════════════════════════════════════════════════"
        echo "PromptToken:     $TOKEN"
        echo "CantonBridge:    $BRIDGE"
        echo ""
    fi
    
    if [ -n "$PARTY_ID" ]; then
        echo -e "${BOLD}CANTON${NC}"
        echo "═══════════════════════════════════════════════════════════════════════"
        echo "Party ID:        $PARTY_ID"
        echo "Domain ID:       $DOMAIN_ID"
        echo "Fingerprint:     0x$FINGERPRINT"
        echo ""
    fi
    
    echo -e "${BOLD}RELAYER TRANSFERS${NC}"
    echo "═══════════════════════════════════════════════════════════════════════"
    curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null | jq '.transfers[] | {id: .ID, direction: .Direction, status: .Status}' 2>/dev/null || echo "No transfers found or relayer not reachable"
    echo ""
}

show_logs() {
    print_header "Relayer Logs (last 50 lines)"
    docker logs canton-bridge-relayer 2>&1 | tail -50
}

run_full_test() {
    print_header "Full Test: 100 deposit + 50 withdraw"
    
    setup_bridge
    run_deposit 100
    run_withdrawal 50
    
    print_header "Test Complete"
    show_status
}

# =============================================================================
# Interactive Menu
# =============================================================================

read_amount() {
    local prompt="$1"
    local default="$2"
    local amount
    
    # Print prompt to stderr so it's not captured by command substitution
    echo -ne "${CYAN}$prompt [$default]: ${NC}" >&2
    read amount
    
    if [ -z "$amount" ]; then
        amount="$default"
    fi
    
    # Validate number
    if ! [[ "$amount" =~ ^[0-9]+$ ]] || [ "$amount" -le 0 ]; then
        print_error "Invalid amount. Please enter a positive number." >&2
        return 1
    fi
    
    echo "$amount"
}

show_menu() {
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  CANTON BRIDGE MANAGER${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "  1) Full test (100 deposit + 50 withdraw)"
    echo "  2) Deposit tokens"
    echo "  3) Withdraw tokens"
    echo "  4) View status"
    echo "  5) View relayer logs"
    echo "  6) Start services"
    echo "  7) Stop services"
    echo "  8) Clean & restart"
    echo "  0) Exit"
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -ne "${CYAN}Select option: ${NC}"
}

ensure_services_running() {
    local relayer_status=$(docker inspect --format='{{.State.Status}}' canton-bridge-relayer 2>/dev/null || echo "not_running")
    if [ "$relayer_status" != "running" ]; then
        print_warning "Services not running. Starting..."
        preflight_checks || return 1
        start_services || return 1
        load_configuration || return 1
        setup_bridge
    elif [ -z "$TOKEN" ]; then
        # Services running but config not loaded
        load_configuration || return 1
    fi
    return 0
}

interactive_menu() {
    while true; do
        show_menu
        read choice
        
        case $choice in
            1)
                if ensure_services_running; then
                    run_full_test || true
                fi
                ;;
            2)
                if ensure_services_running; then
                    amount=$(read_amount "Enter deposit amount" "100")
                    if [ $? -eq 0 ] && [ -n "$amount" ]; then
                        run_deposit "$amount" || true
                    fi
                fi
                ;;
            3)
                if ensure_services_running; then
                    amount=$(read_amount "Enter withdrawal amount" "50")
                    if [ $? -eq 0 ] && [ -n "$amount" ]; then
                        run_withdrawal "$amount" || true
                    fi
                fi
                ;;
            4)
                show_status
                ;;
            5)
                show_logs
                ;;
            6)
                preflight_checks || continue
                start_services || continue
                load_configuration || continue
                setup_bridge
                print_success "Services started and ready!"
                ;;
            7)
                stop_services
                TOKEN=""
                BRIDGE=""
                PARTY_ID=""
                ;;
            8)
                clean_environment
                preflight_checks || continue
                start_services || continue
                load_configuration || continue
                setup_bridge
                ;;
            0|q|Q)
                echo ""
                print_success "Goodbye!"
                exit 0
                ;;
            *)
                print_error "Invalid option: $choice"
                ;;
        esac
        
        echo ""
        echo -ne "${YELLOW}Press Enter to continue...${NC}"
        read
    done
}

# =============================================================================
# Argument Parsing
# =============================================================================

CLEAN=false
SKIP_SETUP=false
SHOW_LOGS=false
START_ONLY=false
STOP=false
FULL_TEST=false
INTERACTIVE=false
STATUS=false
DEPOSIT_AMOUNT=""
WITHDRAW_AMOUNT=""
HAS_ACTION=false

show_help() {
    head -35 "$0" | tail -n +2 | sed 's/^# //' | sed 's/^#//'
    exit 0
}

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
        --logs)
            SHOW_LOGS=true
            shift
            ;;
        --start-only)
            START_ONLY=true
            HAS_ACTION=true
            shift
            ;;
        --stop)
            STOP=true
            HAS_ACTION=true
            shift
            ;;
        --full-test)
            FULL_TEST=true
            HAS_ACTION=true
            shift
            ;;
        --status)
            STATUS=true
            HAS_ACTION=true
            shift
            ;;
        -i|--interactive)
            INTERACTIVE=true
            shift
            ;;
        --deposit)
            DEPOSIT_AMOUNT="$2"
            HAS_ACTION=true
            shift 2
            ;;
        --withdraw)
            WITHDRAW_AMOUNT="$2"
            HAS_ACTION=true
            shift 2
            ;;
        -h|--help)
            show_help
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# =============================================================================
# Main Execution
# =============================================================================

cd "$PROJECT_DIR"

# Handle --stop first (no setup needed)
if [ "$STOP" = true ]; then
    stop_services
    exit 0
fi

# Handle --status (no setup needed)
if [ "$STATUS" = true ] && [ "$HAS_ACTION" = true ] && [ -z "$DEPOSIT_AMOUNT" ] && [ -z "$WITHDRAW_AMOUNT" ] && [ "$FULL_TEST" = false ] && [ "$START_ONLY" = false ]; then
    show_status
    exit 0
fi

# Handle --clean
if [ "$CLEAN" = true ]; then
    clean_environment
fi

# If no action flags provided, go to interactive mode
if [ "$HAS_ACTION" = false ] || [ "$INTERACTIVE" = true ]; then
    interactive_menu
    exit 0
fi

# Non-interactive mode: run actions

# Preflight checks
if [ "$SKIP_SETUP" = false ]; then
    preflight_checks || exit 1
fi

# Start services if needed
if [ "$SKIP_SETUP" = false ]; then
    start_services || exit 1
fi

# Load configuration
load_configuration || exit 1

# Handle --start-only
if [ "$START_ONLY" = true ]; then
    print_header "Services Started"
    show_status
    echo ""
    echo "Services are running. Use --stop to stop them."
    echo "Or run again without --start-only to run tests."
    exit 0
fi

# Setup bridge
setup_bridge

# Handle --full-test
if [ "$FULL_TEST" = true ]; then
    run_deposit 100
    run_withdrawal 50
fi

# Handle --deposit
if [ -n "$DEPOSIT_AMOUNT" ]; then
    run_deposit "$DEPOSIT_AMOUNT"
fi

# Handle --withdraw
if [ -n "$WITHDRAW_AMOUNT" ]; then
    run_withdrawal "$WITHDRAW_AMOUNT"
fi

# Show summary
print_header "Summary"
show_status

# Show logs if requested
if [ "$SHOW_LOGS" = true ]; then
    show_logs
fi
