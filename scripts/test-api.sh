#!/bin/bash
# =============================================================================
# ERC-20 API Server Manager
# =============================================================================
# Manages the ERC-20 JSON-RPC API server test environment with Docker containers.
#
# Usage:
#   ./scripts/test-api.sh              # Interactive menu (default)
#   ./scripts/test-api.sh --start-only # Start services without tests
#   ./scripts/test-api.sh --full-test  # Run complete test flow
#   ./scripts/test-api.sh --setup      # Setup test users only
#   ./scripts/test-api.sh --test       # Run ERC-20 method tests
#   ./scripts/test-api.sh --transfer   # Transfer 10 PROMPT User1 -> User2
#   ./scripts/test-api.sh --withdraw   # Withdraw 30 PROMPT each to EVM
#
# Options:
#   --start-only    Start services without running tests
#   --stop          Stop all containers (keep data)
#   --clean         Reset environment (docker compose down -v)
#   --full-test     Run complete test (setup + methods + transfer + withdraw)
#   --setup         Setup test users (whitelist, register, fund 50 PROMPT)
#   --test          Test all ERC-20 methods
#   --transfer      Transfer 10 PROMPT from User1 to User2
#   --withdraw      Withdraw 30 PROMPT each from Canton to EVM
#   --status        Show container and API status
#   -i, --interactive   Force interactive menu
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

# Endpoints
API_URL="http://localhost:8081/rpc"
ANVIL_URL="http://localhost:8545"
CANTON_URL="http://localhost:5013"
RELAYER_URL="http://localhost:8080"

# Anvil default accounts
# User 1 - Account 1
USER1_ADDRESS="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
USER1_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

# User 2 - Account 2
USER2_ADDRESS="0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC"
USER2_KEY="0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"

# Owner - Account 0 (token owner for funding)
OWNER_ADDRESS="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
OWNER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

# Token amounts (in wei)
FUND_AMOUNT="100000000000000000000"   # 100 tokens
DEPOSIT_AMOUNT="50000000000000000000" # 50 tokens
TRANSFER_AMOUNT="10"                   # 10 tokens (decimal for RPC)

# Canton token ID for bridge mapping (bytes32 - "PROM" padded)
CANTON_TOKEN_ID="0x0000000000000000000000000000000000000000000000000000000050524f4d"

# Global state
TOKEN=""
BRIDGE=""
USER1_FINGERPRINT=""
USER2_FINGERPRINT=""

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

wait_for_api_server() {
    print_step "Waiting for API server..."
    local max_attempts=60
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        if curl -s "http://localhost:8081/health" 2>/dev/null | grep -q "ok"; then
            print_success "API server is healthy!"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    echo ""
    print_error "API server failed to become healthy"
    return 1
}

wait_for_deposit_confirmation() {
    local max_attempts=45
    local attempt=0
    
    print_step "Waiting for deposits to be processed by relayer..."
    
    # Get initial transfer count
    local initial_completed=$(curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null | \
        jq '[.transfers[] | select(.Status == "completed" and (.Direction == "ethereum_to_canton" or .Direction == "deposit"))] | length' 2>/dev/null || echo "0")
    
    while [ $attempt -lt $max_attempts ]; do
        # Check relayer for completed deposits
        local transfers=$(curl -s "$RELAYER_URL/api/v1/transfers" 2>/dev/null)
        local completed=$(echo "$transfers" | \
            jq '[.transfers[] | select(.Status == "completed" and (.Direction == "ethereum_to_canton" or .Direction == "deposit"))] | length' 2>/dev/null || echo "0")
        
        # We expect 2 new deposits (User1 and User2)
        local new_deposits=$((completed - initial_completed))
        if [ "$new_deposits" -ge 2 ] 2>/dev/null; then
            echo ""
            print_success "Both deposits confirmed! ($new_deposits new deposits)"
            return 0
        fi
        
        if [ "$new_deposits" -ge 1 ] 2>/dev/null; then
            echo -n "+"
        else
            echo -n "."
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_warning "Deposit confirmation timed out - deposits may still be processing"
    print_info "View holdings: go run scripts/bridge-activity.go -config .test-config.yaml"
}

# =============================================================================
# RPC Helpers
# =============================================================================

# Make an RPC call without authentication
rpc_call() {
    local method=$1
    local params=$2
    
    curl -s -X POST "$API_URL" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}"
}

# Make an authenticated RPC call with EVM signature
rpc_call_auth() {
    local method=$1
    local params=$2
    local private_key=$3
    
    local timestamp=$(date +%s)
    local message="${method}:${timestamp}"
    local signature=$(cast wallet sign --private-key "$private_key" "$message" 2>/dev/null)
    
    curl -s -X POST "$API_URL" \
        -H "Content-Type: application/json" \
        -H "X-Signature: $signature" \
        -H "X-Message: $message" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}"
}

# =============================================================================
# Core Functions
# =============================================================================

load_contracts() {
    print_step "Loading contract addresses..."
    local BROADCAST_FILE="$PROJECT_DIR/contracts/ethereum-wayfinder/broadcast/Deployer.s.sol/31337/run-latest.json"
    
    if [ -f "$BROADCAST_FILE" ]; then
        TOKEN=$(jq -r '.transactions[] | select(.contractName == "PromptToken") | .contractAddress' "$BROADCAST_FILE")
        BRIDGE=$(jq -r '.transactions[] | select(.contractName == "CantonBridge") | .contractAddress' "$BROADCAST_FILE")
    else
        TOKEN="0x5fbdb2315678afecb367f032d93f642f64180aa3"
        BRIDGE="0xe7f1725e7734ce288f8367e1bb143e90bb3f0512"
    fi
    
    print_info "Token: $TOKEN"
    print_info "Bridge: $BRIDGE"
}

generate_host_config() {
    print_step "Generating host config file..."
    
    # Get party and domain from Canton API
    local PARTY_ID=$(curl -s "$CANTON_URL/v2/parties" | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1)
    local DOMAIN_ID=$(curl -s "$CANTON_URL/v2/state/connected-synchronizers" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')
    
    if [ -z "$PARTY_ID" ]; then
        print_warning "BridgeIssuer party not found - config may be incomplete"
        return 1
    fi
    
    # Get package IDs from DAR files
    local WAYFINDER_DAR=$(ls "$PROJECT_DIR/contracts/canton-erc20/daml/bridge-wayfinder/.daml/dist/"*.dar 2>/dev/null | head -1)
    local CORE_DAR=$(ls "$PROJECT_DIR/contracts/canton-erc20/daml/bridge-core/.daml/dist/"*.dar 2>/dev/null | head -1)
    local CIP56_DAR=$(ls "$PROJECT_DIR/contracts/canton-erc20/daml/cip56-token/.daml/dist/"*.dar 2>/dev/null | head -1)
    
    local BRIDGE_WAYFINDER_PACKAGE_ID=""
    local BRIDGE_CORE_PACKAGE_ID=""
    local CIP56_PACKAGE_ID=""
    
    if [ -n "$WAYFINDER_DAR" ]; then
        # Extract package ID from DAR listing (format: package-name-version-HASH/...)
        BRIDGE_WAYFINDER_PACKAGE_ID=$(daml damlc inspect-dar "$WAYFINDER_DAR" 2>/dev/null | grep -m1 "^bridge-wayfinder-" | sed 's/.*-\([a-f0-9]\{64\}\)\/.*/\1/')
        BRIDGE_CORE_PACKAGE_ID=$(daml damlc inspect-dar "$CORE_DAR" 2>/dev/null | grep -m1 "^bridge-core-" | sed 's/.*-\([a-f0-9]\{64\}\)\/.*/\1/')
        CIP56_PACKAGE_ID=$(daml damlc inspect-dar "$CIP56_DAR" 2>/dev/null | grep -m1 "^cip56-token-" | sed 's/.*-\([a-f0-9]\{64\}\)\/.*/\1/')
    fi
    
    # Use contract addresses if already loaded, otherwise use defaults
    local TOKEN_ADDR="${TOKEN:-0x5fbdb2315678afecb367f032d93f642f64180aa3}"
    local BRIDGE_ADDR="${BRIDGE:-0xe7f1725e7734ce288f8367e1bb143e90bb3f0512}"
    
    # Write config file (full config matching test-bridge.sh format)
    cat > "$PROJECT_DIR/.test-config.yaml" << EOF
# Auto-generated config for host access to Docker services
# Generated by test-api.sh - DO NOT COMMIT

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
  bridge_contract: "$BRIDGE_ADDR"
  token_contract: "$TOKEN_ADDR"
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
    print_info "Party: ${PARTY_ID:0:50}..."
}

clean_environment() {
    print_header "Cleaning Environment"
    print_step "Stopping and removing all containers and volumes..."
    cd "$PROJECT_DIR"
    $DOCKER_COMPOSE_CMD down -v 2>/dev/null || true
    docker volume rm canton-middleware_config_state 2>/dev/null || true
    print_success "Environment cleaned"
}

stop_services() {
    print_header "Stopping Services"
    print_step "Stopping all containers..."
    cd "$PROJECT_DIR"
    $DOCKER_COMPOSE_CMD down 2>/dev/null || true
    print_success "Services stopped"
}

start_services() {
    print_header "Starting Docker Services"
    cd "$PROJECT_DIR"
    
    print_step "Starting docker compose..."
    $DOCKER_COMPOSE_CMD up --build -d
    
    echo ""
    print_step "Container status:"
    $DOCKER_COMPOSE_CMD ps
    echo ""
    
    wait_for_service "Anvil (Ethereum)" "$ANVIL_URL" 30 || return 1
    wait_for_service "Canton HTTP API" "$CANTON_URL/v2/version" 60 || return 1
    
    print_step "Waiting for bootstrap to complete..."
    local max_attempts=120
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        local status=$(docker inspect --format='{{.State.Status}}' bootstrap 2>/dev/null || echo "not_found")
        if [ "$status" = "exited" ]; then
            local exit_code=$(docker inspect --format='{{.State.ExitCode}}' bootstrap 2>/dev/null || echo "1")
            if [ "$exit_code" = "0" ]; then
                print_success "Bootstrap completed!"
                break
            else
                print_error "Bootstrap failed"
                docker logs bootstrap 2>&1 | tail -20
                return 1
            fi
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    echo ""
    
    wait_for_api_server || return 1
    wait_for_service "Relayer" "$RELAYER_URL/health" 30 || return 1
    
    load_contracts
    print_success "All services are ready!"
    
    # Generate/update host config for bridge-activity.go
    generate_host_config
}

# =============================================================================
# Test User Setup
# =============================================================================

add_to_whitelist() {
    local address=$1
    print_step "Adding $address to whitelist..."
    
    docker exec postgres psql -U postgres -d erc20_api -c \
        "INSERT INTO whitelist (evm_address) VALUES ('$address') ON CONFLICT DO NOTHING;" > /dev/null 2>&1
    
    print_success "Whitelisted $address"
}

register_user() {
    local address=$1
    local private_key=$2
    local name=$3
    
    print_step "Registering $name ($address)..." >&2
    
    local result=$(rpc_call_auth "user_register" "{}" "$private_key")
    
    if echo "$result" | grep -q '"party"'; then
        local party=$(echo "$result" | jq -r '.result.party // empty')
        local fingerprint=$(echo "$result" | jq -r '.result.fingerprint // empty')
        print_success "Registered $name: ${party:0:40}..." >&2
        print_info "Fingerprint: ${fingerprint:0:16}..." >&2
        # Return fingerprint to caller via stdout
        echo "$fingerprint"
        return 0
    elif echo "$result" | grep -q "already registered"; then
        print_warning "$name already registered - fetching existing fingerprint..." >&2
        # User exists, we need to get their fingerprint from the database
        local fingerprint=$(docker exec postgres psql -U postgres -d erc20_api -t -c \
            "SELECT fingerprint FROM users WHERE evm_address = '$address';" 2>/dev/null | tr -d ' \n')
        if [ -n "$fingerprint" ]; then
            print_info "Existing fingerprint: ${fingerprint:0:16}..." >&2
            echo "$fingerprint"
        fi
        return 0
    else
        print_error "Failed to register $name" >&2
        print_info "Response: $result" >&2
        return 1
    fi
}

setup_bridge() {
    print_step "Setting up bridge token mapping..."
    
    # Check if mapping already exists
    local existing_mapping=$(cast call $BRIDGE "ethereumToCantonToken(address)(bytes32)" "$TOKEN" --rpc-url "$ANVIL_URL" 2>/dev/null)
    
    if [ "$existing_mapping" != "0x0000000000000000000000000000000000000000000000000000000000000000" ] && [ -n "$existing_mapping" ]; then
        print_info "Token mapping already exists"
        return 0
    fi
    
    # Add token mapping (owner-only function)
    local result=$(cast send $BRIDGE "addTokenMapping(address,bytes32)" \
        $TOKEN $CANTON_TOKEN_ID \
        --rpc-url "$ANVIL_URL" \
        --private-key $OWNER_KEY 2>&1)
    
    if echo "$result" | grep -q "transactionHash"; then
        print_success "Token mapping added to bridge"
    else
        print_error "Failed to add token mapping: $result"
        return 1
    fi
}

fund_evm_wallet() {
    local address=$1
    local name=$2
    
    print_step "Funding $name EVM wallet..."
    
    # Check current balance
    local balance=$(cast call $TOKEN "balanceOf(address)(uint256)" "$address" --rpc-url "$ANVIL_URL" 2>/dev/null | awk '{print $1}')
    
    if [ -z "$balance" ] || [ "$balance" = "0" ]; then
        cast send $TOKEN "transfer(address,uint256)" "$address" "$FUND_AMOUNT" \
            --rpc-url "$ANVIL_URL" \
            --private-key $OWNER_KEY > /dev/null 2>&1
        print_success "Funded $name with 100 PROMPT"
    else
        print_info "$name already has tokens"
    fi
}

deposit_to_canton() {
    local address=$1
    local private_key=$2
    local fingerprint=$3
    local name=$4
    
    print_step "Depositing 50 PROMPT to Canton for $name..."
    
    # Fingerprint from server already has 0x prefix, pad to 64 hex chars (bytes32)
    local clean_fp=$(echo "$fingerprint" | sed 's/^0x//')
    # Pad fingerprint to 64 characters (bytes32)
    while [ ${#clean_fp} -lt 64 ]; do
        clean_fp="${clean_fp}0"
    done
    local canton_recipient="0x${clean_fp}"
    
    print_info "Deposit fingerprint (bytes32): $canton_recipient"
    
    # Approve bridge
    local approve_result=$(cast send $TOKEN "approve(address,uint256)" "$BRIDGE" "$DEPOSIT_AMOUNT" \
        --rpc-url "$ANVIL_URL" \
        --private-key "$private_key" 2>&1)
    
    if ! echo "$approve_result" | grep -q "transactionHash"; then
        print_error "Approve failed for $name: $approve_result"
        return 1
    fi
    
    # Deposit to Canton
    local result=$(cast send $BRIDGE "depositToCanton(address,uint256,bytes32)" \
        $TOKEN "$DEPOSIT_AMOUNT" "$canton_recipient" \
        --rpc-url "$ANVIL_URL" \
        --private-key "$private_key" 2>&1)
    
    if echo "$result" | grep -q "transactionHash"; then
        local tx_hash=$(echo "$result" | grep "transactionHash" | awk '{print $2}')
        print_success "Deposit submitted for $name (tx: ${tx_hash:0:20}...)"
    else
        print_error "Deposit failed for $name"
        print_info "Error: $result"
        return 1
    fi
}

setup_test_users() {
    print_header "Setting Up Test Users"
    
    if [ -z "$TOKEN" ]; then
        load_contracts
    fi
    
    echo ""
    echo -e "${BOLD}Test Users:${NC}"
    echo "  User1: $USER1_ADDRESS"
    echo "  User2: $USER2_ADDRESS"
    echo ""
    
    # Whitelist both users
    add_to_whitelist "$USER1_ADDRESS"
    add_to_whitelist "$USER2_ADDRESS"
    echo ""
    
    # Register both users and capture fingerprints
    USER1_FINGERPRINT=$(register_user "$USER1_ADDRESS" "$USER1_KEY" "User1")
    if [ -z "$USER1_FINGERPRINT" ]; then
        print_error "Failed to get User1 fingerprint"
        return 1
    fi
    
    USER2_FINGERPRINT=$(register_user "$USER2_ADDRESS" "$USER2_KEY" "User2")
    if [ -z "$USER2_FINGERPRINT" ]; then
        print_error "Failed to get User2 fingerprint"
        return 1
    fi
    echo ""
    
    # Fund EVM wallets
    fund_evm_wallet "$USER1_ADDRESS" "User1"
    fund_evm_wallet "$USER2_ADDRESS" "User2"
    echo ""
    
    # Setup bridge token mapping (required before deposits)
    setup_bridge
    echo ""
    
    # Deposit to Canton using fingerprints from registration
    print_info "Using User1 fingerprint: ${USER1_FINGERPRINT:0:16}..."
    print_info "Using User2 fingerprint: ${USER2_FINGERPRINT:0:16}..."
    echo ""
    
    deposit_to_canton "$USER1_ADDRESS" "$USER1_KEY" "$USER1_FINGERPRINT" "User1"
    deposit_to_canton "$USER2_ADDRESS" "$USER2_KEY" "$USER2_FINGERPRINT" "User2"
    echo ""
    
    # Wait for deposits to be processed by relayer
    wait_for_deposit_confirmation
    
    print_header "Setup Complete"
    print_info "User1 and User2 are registered and funded with 50 PROMPT each"
}

# =============================================================================
# ERC-20 Method Tests
# =============================================================================

test_erc20_methods() {
    print_header "Testing ERC-20 Methods"
    
    local all_passed=true
    
    # Public methods (no auth)
    echo ""
    echo -e "${BOLD}Public Methods (No Auth Required)${NC}"
    echo "────────────────────────────────────────────────────────────"
    
    # erc20_name
    local result=$(rpc_call "erc20_name" "{}")
    if echo "$result" | grep -q "PROMPT"; then
        print_success "erc20_name: PROMPT"
    else
        print_error "erc20_name failed"
        print_info "$result"
        all_passed=false
    fi
    
    # erc20_symbol
    result=$(rpc_call "erc20_symbol" "{}")
    if echo "$result" | grep -q "PROMPT"; then
        print_success "erc20_symbol: PROMPT"
    else
        print_error "erc20_symbol failed"
        all_passed=false
    fi
    
    # erc20_decimals
    result=$(rpc_call "erc20_decimals" "{}")
    if echo "$result" | grep -q "18"; then
        print_success "erc20_decimals: 18"
    else
        print_error "erc20_decimals failed"
        all_passed=false
    fi
    
    # erc20_totalSupply
    result=$(rpc_call "erc20_totalSupply" "{}")
    if echo "$result" | grep -q "totalSupply"; then
        local supply=$(echo "$result" | jq -r '.result.totalSupply // "0"')
        print_success "erc20_totalSupply: $supply"
    else
        print_error "erc20_totalSupply failed"
        print_info "$result"
        all_passed=false
    fi
    
    # Authenticated methods
    echo ""
    echo -e "${BOLD}Authenticated Methods (EVM Signature Required)${NC}"
    echo "────────────────────────────────────────────────────────────"
    
    # erc20_balanceOf for User1
    result=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER1_KEY")
    if echo "$result" | grep -q "balance"; then
        local balance=$(echo "$result" | jq -r '.result.balance // "0"')
        print_success "erc20_balanceOf (User1): $balance"
    elif echo "$result" | grep -q "not registered"; then
        print_warning "erc20_balanceOf: User1 not registered yet"
    else
        print_error "erc20_balanceOf (User1) failed"
        print_info "$result"
        all_passed=false
    fi
    
    # erc20_balanceOf for User2
    result=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER2_KEY")
    if echo "$result" | grep -q "balance"; then
        local balance=$(echo "$result" | jq -r '.result.balance // "0"')
        print_success "erc20_balanceOf (User2): $balance"
    elif echo "$result" | grep -q "not registered"; then
        print_warning "erc20_balanceOf: User2 not registered yet"
    else
        print_error "erc20_balanceOf (User2) failed"
        all_passed=false
    fi
    
    echo ""
    if [ "$all_passed" = true ]; then
        print_success "All ERC-20 method tests passed!"
    else
        print_warning "Some tests failed - see details above"
    fi
}

# =============================================================================
# Transfer Test
# =============================================================================

run_transfer() {
    print_header "Transfer: 10 PROMPT (User1 -> User2)"
    
    # Get balances before
    print_step "Getting balances before transfer..."
    local user1_before=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER1_KEY" | jq -r '.result.balance // "0"')
    local user2_before=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER2_KEY" | jq -r '.result.balance // "0"')
    
    print_info "User1 balance before: $user1_before"
    print_info "User2 balance before: $user2_before"
    
    # Execute transfer
    print_step "Executing transfer of $TRANSFER_AMOUNT PROMPT..."
    local result=$(rpc_call_auth "erc20_transfer" "{\"to\":\"$USER2_ADDRESS\",\"amount\":\"$TRANSFER_AMOUNT\"}" "$USER1_KEY")
    
    if echo "$result" | grep -q '"success":true'; then
        print_success "Transfer successful!"
        
        # Get balances after
        sleep 2
        print_step "Getting balances after transfer..."
        local user1_after=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER1_KEY" | jq -r '.result.balance // "0"')
        local user2_after=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER2_KEY" | jq -r '.result.balance // "0"')
        
        print_info "User1 balance after: $user1_after"
        print_info "User2 balance after: $user2_after"
        
    elif echo "$result" | grep -q "insufficient\|balance"; then
        print_warning "Transfer skipped - insufficient balance"
        print_info "Make sure to run 'Setup test users' first"
    elif echo "$result" | grep -q "not registered"; then
        print_warning "Transfer skipped - users not registered"
        print_info "Make sure to run 'Setup test users' first"
    else
        print_error "Transfer failed"
        print_info "$result"
        return 1
    fi
}

# =============================================================================
# Status
# =============================================================================

show_status() {
    print_header "Status"
    
    echo ""
    echo -e "${BOLD}DOCKER SERVICES${NC}"
    echo "═══════════════════════════════════════════════════════════════════════"
    cd "$PROJECT_DIR"
    $DOCKER_COMPOSE_CMD ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "No containers running"
    echo ""
    
    # Check if API server is running
    local api_status=$(curl -s "http://localhost:8081/health" 2>/dev/null)
    if echo "$api_status" | grep -q "ok"; then
        echo -e "${BOLD}API SERVER${NC}"
        echo "═══════════════════════════════════════════════════════════════════════"
        echo "Status:    ${GREEN}Healthy${NC}"
        echo "Endpoint:  $API_URL"
        echo ""
        
        # Token info
        local name=$(rpc_call "erc20_name" "{}" | jq -r '.result // "?"')
        local symbol=$(rpc_call "erc20_symbol" "{}" | jq -r '.result // "?"')
        local supply=$(rpc_call "erc20_totalSupply" "{}" | jq -r '.result.totalSupply // "?"')
        
        echo -e "${BOLD}TOKEN INFO${NC}"
        echo "═══════════════════════════════════════════════════════════════════════"
        echo "Name:         $name"
        echo "Symbol:       $symbol"
        echo "Total Supply: $supply"
        echo ""
        
        # User balances (if registered)
        echo -e "${BOLD}USER BALANCES${NC}"
        echo "═══════════════════════════════════════════════════════════════════════"
        
        local user1_result=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER1_KEY" 2>/dev/null)
        if echo "$user1_result" | grep -q "balance"; then
            local user1_balance=$(echo "$user1_result" | jq -r '.result.balance // "0"')
            echo "User1 ($USER1_ADDRESS): $user1_balance"
        else
            echo "User1: Not registered"
        fi
        
        local user2_result=$(rpc_call_auth "erc20_balanceOf" "{}" "$USER2_KEY" 2>/dev/null)
        if echo "$user2_result" | grep -q "balance"; then
            local user2_balance=$(echo "$user2_result" | jq -r '.result.balance // "0"')
            echo "User2 ($USER2_ADDRESS): $user2_balance"
        else
            echo "User2: Not registered"
        fi
        echo ""
    else
        print_warning "API server not running"
    fi
}

# =============================================================================
# Withdrawal
# =============================================================================

wait_for_withdrawal_confirmation() {
    local user_address="$1"
    local balance_before="$2"
    local max_attempts=30
    local attempt=0
    
    print_step "Waiting for withdrawal to complete on EVM..."
    
    while [ $attempt -lt $max_attempts ]; do
        local balance_after=$(cast call $TOKEN "balanceOf(address)(uint256)" "$user_address" --rpc-url "$ANVIL_URL" 2>/dev/null | awk '{print $1}')
        
        # Check if EVM balance increased
        if [ -n "$balance_after" ] && [ "$balance_after" != "$balance_before" ]; then
            echo ""
            print_success "Withdrawal confirmed on EVM!"
            print_info "EVM balance: $balance_after"
            return 0
        fi
        
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo ""
    print_warning "Withdrawal confirmation timed out"
    return 1
}

withdraw_user() {
    local address=$1
    local private_key=$2
    local name=$3
    local amount=$4
    
    print_step "Withdrawing $amount PROMPT for $name..."
    
    # Get current Canton balance via RPC
    local balance_result=$(rpc_call_auth "erc20_balanceOf" "{}" "$private_key")
    local balance=$(echo "$balance_result" | jq -r '.result.balance // "0"')
    
    # Remove any decimal places for comparison (balance might be like "40.000000000000000000")
    local balance_int=$(echo "$balance" | cut -d'.' -f1)
    
    if [ -z "$balance_int" ] || [ "$balance_int" = "0" ] || [ "$balance_int" = "null" ]; then
        print_info "$name has no Canton balance to withdraw"
        return 0
    fi
    
    print_info "$name Canton balance: $balance"
    
    # Check if requested amount exceeds balance
    if [ "$amount" -gt "$balance_int" ] 2>/dev/null; then
        print_warning "Requested $amount but only $balance_int available, withdrawing $balance_int"
        amount="$balance_int"
    fi
    
    # Get EVM balance before withdrawal
    local evm_balance_before=$(cast call $TOKEN "balanceOf(address)(uint256)" "$address" --rpc-url "$ANVIL_URL" 2>/dev/null | awk '{print $1}')
    print_info "$name EVM balance before: $evm_balance_before"
    
    # Find holding CID for this user
    local user_party=$(docker exec postgres psql -U postgres -d erc20_api -t -A -c \
        "SELECT canton_party FROM users WHERE evm_address = '$address';" 2>/dev/null)
    
    if [ -z "$user_party" ]; then
        print_error "Could not find Canton party for $name"
        return 1
    fi
    
    print_info "Canton party: ${user_party:0:40}..."
    
    # Use get-holding-cid.go with party filter
    local holding_info=$(go run scripts/get-holding-cid.go -config "$PROJECT_DIR/.test-config.yaml" -party "$user_party" -with-balance 2>/dev/null)
    
    if [ -z "$holding_info" ]; then
        print_warning "No holding found for $name - may have already withdrawn"
        return 0
    fi
    
    local holding_cid=$(echo "$holding_info" | awk '{print $1}')
    local holding_balance=$(echo "$holding_info" | awk '{print $2}')
    
    print_info "Found holding: ${holding_cid:0:30}... (balance: $holding_balance)"
    
    # Format amount with decimal for Canton
    local amount_decimal="${amount}.0"
    
    # Initiate withdrawal using the existing script
    print_step "Initiating withdrawal of $amount to $address..."
    
    local withdraw_result=$(go run scripts/initiate-withdrawal.go \
        -config "$PROJECT_DIR/.test-config.yaml" \
        -holding-cid "$holding_cid" \
        -amount "$amount_decimal" \
        -evm-destination "$address" 2>&1)
    
    if echo "$withdraw_result" | grep -q -i "error\|failed"; then
        print_error "Withdrawal failed for $name"
        print_info "$withdraw_result"
        return 1
    fi
    
    print_success "Withdrawal initiated for $name"
    
    # Wait for withdrawal to complete
    wait_for_withdrawal_confirmation "$address" "$evm_balance_before"
}

run_withdrawals() {
    local amount="${1:-30}"
    
    print_header "Withdrawing $amount PROMPT Each (Canton -> EVM)"
    
    withdraw_user "$USER1_ADDRESS" "$USER1_KEY" "User1" "$amount"
    echo ""
    withdraw_user "$USER2_ADDRESS" "$USER2_KEY" "User2" "$amount"
    
    print_header "Withdrawals Complete"
}

# =============================================================================
# Full Test
# =============================================================================

run_full_test() {
    print_header "Full Test: Setup + Methods + Transfer + Withdraw"
    
    setup_test_users || return 1
    test_erc20_methods
    run_transfer
    
    # Withdraw 30 PROMPT from each user
    echo ""
    run_withdrawals 30
    
    # Wait for cache updates to propagate
    print_step "Waiting for cache updates..."
    sleep 3
    
    print_header "Full Test Complete"
    show_status
}

# =============================================================================
# Interactive Menu
# =============================================================================

view_users() {
    print_header "Registered Users"
    
    # Query the database for all users including cached balance
    local users=$(docker exec postgres psql -U postgres -d erc20_api -t -A -F '|' -c \
        "SELECT id, evm_address, canton_party, fingerprint, mapping_cid, COALESCE(balance::text, '0') as balance, balance_updated_at, created_at FROM users ORDER BY id;" 2>/dev/null)
    
    if [ -z "$users" ]; then
        print_warning "No users registered yet"
        return
    fi
    
    # Get total supply from metrics
    local total_supply=$(docker exec postgres psql -U postgres -d erc20_api -t -A -c \
        "SELECT COALESCE(total_supply::text, '0') FROM token_metrics WHERE id = 1;" 2>/dev/null)
    
    echo ""
    echo -e "${CYAN}┌────┬────────────────────────────────────────────┬──────────────────────────┬─────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│ ID │ EVM Address                                │ Cached Balance           │ Canton Party                                │${NC}"
    echo -e "${CYAN}├────┼────────────────────────────────────────────┼──────────────────────────┼─────────────────────────────────────────────┤${NC}"
    
    echo "$users" | while IFS='|' read -r id evm_addr party fingerprint mapping_cid balance balance_updated created_at; do
        printf "${CYAN}│${NC} %-2s ${CYAN}│${NC} %-42s ${CYAN}│${NC} %-24s ${CYAN}│${NC} %-43s ${CYAN}│${NC}\n" "$id" "$evm_addr" "$balance" "${party:0:43}"
    done
    
    echo -e "${CYAN}└────┴────────────────────────────────────────────┴──────────────────────────┴─────────────────────────────────────────────┘${NC}"
    
    echo ""
    echo -e "${GREEN}Total Supply (cached): ${total_supply:-0}${NC}"
    echo ""
    print_info "Detailed User Information:"
    echo ""
    
    echo "$users" | while IFS='|' read -r id evm_addr party fingerprint mapping_cid balance balance_updated created_at; do
        echo -e "${GREEN}User $id:${NC}"
        echo "  EVM Address:     $evm_addr"
        echo "  Cached Balance:  $balance"
        echo "  Balance Updated: ${balance_updated:-Never}"
        echo "  Canton Party:    $party"
        echo "  Fingerprint:     $fingerprint"
        echo "  Mapping CID:     ${mapping_cid:-N/A}"
        echo "  Registered:      $created_at"
        echo ""
    done
}

show_menu() {
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  ERC-20 API SERVER MANAGER${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "  1) Full test (setup + methods + transfer + withdraw)"
    echo "  2) Setup test users (whitelist, register, fund 50 PROMPT)"
    echo "  3) Test ERC-20 methods"
    echo "  4) Transfer 10 PROMPT (User1 -> User2)"
    echo "  5) Withdraw 30 PROMPT each (Canton -> EVM)"
    echo "  6) View status"
    echo "  7) View registered users"
    echo "  8) Start services"
    echo "  9) Stop services"
    echo "  c) Clean & restart"
    echo "  0) Exit"
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -ne "${CYAN}Select option: ${NC}"
}

ensure_services_running() {
    local api_status=$(curl -s "http://localhost:8081/health" 2>/dev/null || echo "")
    if ! echo "$api_status" | grep -q "ok"; then
        print_warning "Services not running. Starting..."
        start_services || return 1
    elif [ -z "$TOKEN" ]; then
        load_contracts
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
                    setup_test_users || true
                fi
                ;;
            3)
                if ensure_services_running; then
                    test_erc20_methods || true
                fi
                ;;
            4)
                if ensure_services_running; then
                    run_transfer || true
                fi
                ;;
            5)
                if ensure_services_running; then
                    run_withdrawals 30 || true
                fi
                ;;
            6)
                show_status
                ;;
            7)
                view_users
                ;;
            8)
                start_services || true
                ;;
            9)
                stop_services
                TOKEN=""
                BRIDGE=""
                ;;
            c|C)
                clean_environment
                start_services || true
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
START_ONLY=false
STOP=false
FULL_TEST=false
SETUP=false
TEST=false
TRANSFER=false
WITHDRAW=false
STATUS=false
INTERACTIVE=false
HAS_ACTION=false

show_help() {
    head -30 "$0" | tail -n +2 | sed 's/^# //' | sed 's/^#//'
    exit 0
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --clean)
            CLEAN=true
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
        --setup)
            SETUP=true
            HAS_ACTION=true
            shift
            ;;
        --test)
            TEST=true
            HAS_ACTION=true
            shift
            ;;
        --transfer)
            TRANSFER=true
            HAS_ACTION=true
            shift
            ;;
        --withdraw)
            WITHDRAW=true
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

# Handle --stop first
if [ "$STOP" = true ]; then
    stop_services
    exit 0
fi

# Handle --status
if [ "$STATUS" = true ] && [ "$SETUP" = false ] && [ "$TEST" = false ] && [ "$TRANSFER" = false ] && [ "$FULL_TEST" = false ]; then
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

# Non-interactive mode: start services if needed
if [ "$START_ONLY" = false ]; then
    ensure_services_running || exit 1
fi

# Handle --start-only
if [ "$START_ONLY" = true ]; then
    start_services || exit 1
    print_header "Services Started"
    show_status
    exit 0
fi

# Handle --full-test
if [ "$FULL_TEST" = true ]; then
    run_full_test
    exit 0
fi

# Handle --setup
if [ "$SETUP" = true ]; then
    setup_test_users
fi

# Handle --test
if [ "$TEST" = true ]; then
    test_erc20_methods
fi

# Handle --transfer
if [ "$TRANSFER" = true ]; then
    run_transfer
fi

# Handle --withdraw
if [ "$WITHDRAW" = true ]; then
    run_withdrawals 30
fi

# Show summary
print_header "Summary"
show_status
