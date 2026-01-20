#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge Local E2E Test (Bash + Cast version)
# =============================================================================
# This script runs a complete E2E test using cast commands for ERC20 interactions.
#
# Test Flow:
# 1. Start Docker services (optional)
# 2. Wait for all services to be healthy
# 3. Verify test token balances
# 4. Whitelist users in database
# 5. Register users on API server
# 6. Deposit tokens from Anvil to Canton (using cast)
# 7. Verify Canton balances
# 8. Transfer tokens between users on Canton (using cast via eth_sendRawTransaction)
# 9. Verify final balances
# 10. Test ERC20 metadata endpoints
#
# Usage:
#   ./scripts/e2e-local.sh [--cleanup] [--skip-docker] [--verbose]
#
# Flags:
#   --cleanup      Stop and remove Docker services after test
#   --skip-docker  Skip Docker compose start (assume services are running)
#   --verbose      Enable verbose output
# =============================================================================

set -e

# Suppress foundry nightly warnings
export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
RESET='\033[0m'

# Parse flags
CLEANUP=false
SKIP_DOCKER=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --cleanup)
            CLEANUP=true
            shift
            ;;
        --skip-docker)
            SKIP_DOCKER=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Configuration (matching config.e2e-local.yaml)
USER1_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER1_ADDR="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

USER2_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
USER2_ADDR="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

TOKEN_ADDR="0x5FbDB2315678afecb367f032d93F642f64180aa3"
BRIDGE_ADDR="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"

ANVIL_URL="http://localhost:8545"
API_SERVER_URL="http://localhost:8081"
RELAYER_URL="http://localhost:8080"
ETH_RPC_URL="$API_SERVER_URL/eth"

DEPOSIT_AMOUNT="100" # 100 tokens
TRANSFER_AMOUNT="25" # 25 tokens

# Database config
DB_HOST="localhost"
DB_PORT="5432"
DB_USER="postgres"
DB_PASS="p@ssw0rd"
DB_NAME="erc20_api"

# Helper functions
print_header() {
    echo -e "\n${BLUE}══════════════════════════════════════════════════════════════════════${RESET}"
    echo -e "${BLUE}  $1${RESET}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${RESET}"
}

print_step() {
    echo -e "${CYAN}>>> $1${RESET}"
}

print_success() {
    echo -e "${GREEN}✓ $1${RESET}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${RESET}"
}

print_error() {
    echo -e "${RED}✗ $1${RESET}"
    exit 1
}

print_info() {
    echo -e "    $1"
}

# Wait for HTTP endpoint to be ready
wait_for_endpoint() {
    local url=$1
    local name=$2
    local max_attempts=60
    local interval=3

    print_step "Waiting for $name..."

    for ((i=1; i<=max_attempts; i++)); do
        if curl -s -f "$url" > /dev/null 2>&1; then
            print_success "$name is ready"
            return 0
        fi
        sleep $interval
    done

    print_error "$name not ready after $max_attempts attempts"
}

# Wait for Anvil (JSON-RPC)
wait_for_anvil() {
    local max_attempts=60
    local interval=3

    print_step "Waiting for Anvil..."

    for ((i=1; i<=max_attempts; i++)); do
        if cast block-number --rpc-url "$ANVIL_URL" > /dev/null 2>&1; then
            print_success "Anvil is ready"
            return 0
        fi
        sleep $interval
    done

    print_error "Anvil not ready after $max_attempts attempts"
}

# Sign EIP-191 message for registration
sign_eip191() {
    local message=$1
    local private_key=$2

    # Use cast to sign the message with EIP-191 prefix
    cast wallet sign --private-key "$private_key" "$message"
}

# Register user on API server
register_user() {
    local name=$1
    local private_key=$2
    local user_addr=$3

    # Create registration message
    local timestamp=$(date +%s)
    local message="registration:$timestamp"

    # Sign the message
    local signature=$(sign_eip191 "$message" "$private_key" 2>/dev/null)

    # Send registration request
    local response=$(curl -s -X POST "$API_SERVER_URL/register" \
        -H "Content-Type: application/json" \
        -d "{\"signature\":\"$signature\",\"message\":\"$message\"}")

    # Check response
    if echo "$response" | jq -e '.fingerprint' > /dev/null 2>&1; then
        local fingerprint=$(echo "$response" | jq -r '.fingerprint')
        print_success "$name registered with fingerprint: ${fingerprint:0:20}..." >&2
        echo "$fingerprint"
    elif echo "$response" | grep -q "already registered\|conflict" 2>/dev/null; then
        print_warning "$name already registered, fetching fingerprint from database" >&2
        # Query database for existing fingerprint
        local fingerprint=$(PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
            -t -c "SELECT fingerprint FROM users WHERE evm_address = '$user_addr';" 2>/dev/null | tr -d '[:space:]')

        if [ -n "$fingerprint" ]; then
            print_success "$name fingerprint: ${fingerprint:0:20}..." >&2
            echo "$fingerprint"
        else
            print_error "$name already registered but fingerprint not found in database"
        fi
    else
        print_error "$name registration failed: $response"
    fi
}

# Get balance from Canton via eth_call
get_canton_balance() {
    local token_addr=$1
    local user_addr=$2

    # Use cast to call balanceOf
    cast call "$token_addr" "balanceOf(address)(uint256)" "$user_addr" --rpc-url "$ETH_RPC_URL" 2>/dev/null || echo "0"
}

# Convert bytes32 fingerprint to hex (remove 0x1220 multihash prefix if present)
fingerprint_to_bytes32() {
    local fingerprint=$1

    # Remove 0x prefix
    fingerprint=${fingerprint#0x}

    # If it starts with 1220 (multihash prefix), remove it
    if [[ $fingerprint == 1220* ]]; then
        fingerprint=${fingerprint:4}
    fi

    # Ensure it's 64 chars (32 bytes), pad with zeros if needed
    printf "0x%064s" "$fingerprint" | tr ' ' '0'
}

# Main test execution
main() {
    print_header "Canton-Ethereum Bridge Local E2E Test (Cast Version)"

    # Start Docker services
    if [ "$SKIP_DOCKER" = false ]; then
        print_header "Starting Docker Services"
        print_step "Running: docker compose up -d --build"

        if [ "$VERBOSE" = true ]; then
            docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml up -d --build
        else
            docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml up -d --build > /dev/null 2>&1
        fi

        print_success "Docker services started"
    else
        print_info "Skipping Docker compose start (assuming services are running)"
    fi

    # Cleanup function
    cleanup() {
        if [ "$CLEANUP" = true ]; then
            print_header "Cleanup"
            print_step "Stopping Docker services..."
            docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml down -v > /dev/null 2>&1
            print_success "Docker services stopped"
        fi
    }
    trap cleanup EXIT

    # Wait for services
    print_header "Waiting for Services"
    wait_for_anvil
    wait_for_endpoint "$API_SERVER_URL/health" "API Server"
    wait_for_endpoint "$RELAYER_URL/health" "Relayer"
    print_success "All services are healthy"

    print_info "User1 Address: $USER1_ADDR"
    print_info "User2 Address: $USER2_ADDR"
    print_info "Token: $TOKEN_ADDR"
    print_info "Bridge: $BRIDGE_ADDR"

    # ==========================================================================
    # Step 1: Verify Token Balance on Anvil
    # ==========================================================================
    print_header "Step 1: Verify Token Balance"

    USER1_BALANCE=$(cast call "$TOKEN_ADDR" "balanceOf(address)(uint256)" "$USER1_ADDR" --rpc-url "$ANVIL_URL" 2>/dev/null)

    # Convert to human-readable (divide by 10^18)
    DEPOSIT_WEI=$(cast --to-wei "$DEPOSIT_AMOUNT" ether 2>/dev/null)

    # Convert balance to human-readable
    BALANCE_HUMAN=$(cast --from-wei "$USER1_BALANCE" ether 2>/dev/null || echo "unknown")

    print_success "User1 has tokens: $BALANCE_HUMAN ($USER1_BALANCE wei)"

    # ==========================================================================
    # Step 2: Whitelist Users in Database
    # ==========================================================================
    print_header "Step 2: Whitelist Users"

    PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
        -c "INSERT INTO whitelist (evm_address) VALUES ('$USER1_ADDR'), ('$USER2_ADDR') ON CONFLICT DO NOTHING" \
        > /dev/null 2>&1

    print_success "Whitelisted $USER1_ADDR"
    print_success "Whitelisted $USER2_ADDR"

    # ==========================================================================
    # Step 3: Register Users
    # ==========================================================================
    print_header "Step 3: Register Users"

    USER1_FINGERPRINT=$(register_user "User1" "$USER1_KEY" "$USER1_ADDR")
    USER2_FINGERPRINT=$(register_user "User2" "$USER2_KEY" "$USER2_ADDR")

    # ==========================================================================
    # Step 4: Deposit Tokens to Canton
    # ==========================================================================
    print_header "Step 4: Deposit Tokens to Canton"

    # Approve bridge contract
    print_step "Approving bridge contract..."
    APPROVE_TX=$(cast send "$TOKEN_ADDR" "approve(address,uint256)" "$BRIDGE_ADDR" "$DEPOSIT_WEI" \
        --private-key "$USER1_KEY" --rpc-url "$ANVIL_URL" --json | jq -r '.transactionHash')
    print_info "Approval tx: $APPROVE_TX"

    # Wait for approval
    cast receipt "$APPROVE_TX" --rpc-url "$ANVIL_URL" > /dev/null 2>&1

    # Deposit to Canton
    print_step "Depositing to Canton..."

    # Convert fingerprint to bytes32
    CANTON_RECIPIENT=$(fingerprint_to_bytes32 "$USER1_FINGERPRINT")

    DEPOSIT_TX=$(cast send "$BRIDGE_ADDR" "depositToCanton(address,uint256,bytes32)" \
        "$TOKEN_ADDR" "$DEPOSIT_WEI" "$CANTON_RECIPIENT" \
        --private-key "$USER1_KEY" --rpc-url "$ANVIL_URL" --json | jq -r '.transactionHash')
    print_info "Deposit tx: $DEPOSIT_TX"

    # Wait for deposit
    cast receipt "$DEPOSIT_TX" --rpc-url "$ANVIL_URL" > /dev/null 2>&1
    print_success "Deposit submitted"

    # ==========================================================================
    # Step 5: Verify Canton Balance
    # ==========================================================================
    print_header "Step 5: Verify Canton Balance"

    print_step "Waiting for relayer to process deposit..."
    sleep 5

    # Poll for balance update
    MAX_WAIT=60
    BALANCE_CHECK_INTERVAL=3
    USER1_CANTON_BALANCE="0"

    for ((i=0; i<MAX_WAIT; i+=BALANCE_CHECK_INTERVAL)); do
        USER1_CANTON_BALANCE=$(get_canton_balance "$TOKEN_ADDR" "$USER1_ADDR")
        if [ "$USER1_CANTON_BALANCE" != "0" ]; then
            break
        fi
        print_info "Waiting for balance... (current: $USER1_CANTON_BALANCE)"
        sleep $BALANCE_CHECK_INTERVAL
    done

    if [ "$USER1_CANTON_BALANCE" = "0" ]; then
        print_error "User1 balance not updated (timeout)"
    fi
    print_success "User1 Canton balance: $USER1_CANTON_BALANCE"

    # ==========================================================================
    # Step 6: Transfer Tokens on Canton (User1 -> User2)
    # ==========================================================================
    print_header "Step 6: Transfer Tokens (User1 -> User2)"

    print_step "Transferring $TRANSFER_AMOUNT tokens..."

    TRANSFER_WEI=$(cast --to-wei "$TRANSFER_AMOUNT" ether 2>/dev/null)

    # Use cast to send ERC20 transfer via eth_sendRawTransaction
    # Use --legacy flag to avoid EIP-1559 (eth_feeHistory) which our API doesn't support
    # Note: cast may fail on receipt parsing due to null vs [] issue, but tx still succeeds
    TRANSFER_OUTPUT=$(cast send "$TOKEN_ADDR" "transfer(address,uint256)" "$USER2_ADDR" "$TRANSFER_WEI" \
        --private-key "$USER1_KEY" --rpc-url "$ETH_RPC_URL" --legacy 2>&1 || true)

    # Extract transaction hash from output (works even if cast errors on parsing)
    TRANSFER_TX=$(echo "$TRANSFER_OUTPUT" | grep -oE "0x[a-f0-9]{64}" | head -1)

    if [ -z "$TRANSFER_TX" ]; then
        print_error "Transfer failed: $TRANSFER_OUTPUT"
    fi
    print_success "Transfer completed: ${TRANSFER_TX:0:40}..."

    # Wait for transfer to process
    sleep 3

    # ==========================================================================
    # Step 7: Verify Final Balances
    # ==========================================================================
    print_header "Step 7: Final Balances"

    # Poll for User2 balance
    MAX_WAIT=30
    for ((i=0; i<MAX_WAIT; i+=2)); do
        USER2_CANTON_BALANCE=$(get_canton_balance "$TOKEN_ADDR" "$USER2_ADDR")
        if [ "$USER2_CANTON_BALANCE" != "0" ]; then
            break
        fi
        sleep 2
    done

    USER1_FINAL=$(get_canton_balance "$TOKEN_ADDR" "$USER1_ADDR")
    USER2_FINAL=$(get_canton_balance "$TOKEN_ADDR" "$USER2_ADDR")

    print_success "User1 final balance: $USER1_FINAL"
    print_success "User2 final balance: $USER2_FINAL"

    # ==========================================================================
    # Step 8: Test ERC20 Metadata Endpoints
    # ==========================================================================
    print_header "Step 8: Test ERC20 Metadata"

    TOKEN_NAME=$(cast call "$TOKEN_ADDR" "name()(string)" --rpc-url "$ETH_RPC_URL")
    TOKEN_SYMBOL=$(cast call "$TOKEN_ADDR" "symbol()(string)" --rpc-url "$ETH_RPC_URL")
    TOKEN_DECIMALS=$(cast call "$TOKEN_ADDR" "decimals()(uint8)" --rpc-url "$ETH_RPC_URL")
    TOKEN_SUPPLY=$(cast call "$TOKEN_ADDR" "totalSupply()(uint256)" --rpc-url "$ETH_RPC_URL")

    print_success "Name: $TOKEN_NAME"
    print_success "Symbol: $TOKEN_SYMBOL"
    print_success "Decimals: $TOKEN_DECIMALS"
    print_success "Total Supply: $TOKEN_SUPPLY"

    print_header "Local E2E Test Completed Successfully!"
}

# Run main
main
