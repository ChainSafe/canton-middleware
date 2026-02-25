#!/bin/bash
# =============================================================================
# Canton Bridge Remote Bootstrap Script (DevNet / Mainnet)
# =============================================================================
# This script bootstraps the API server for remote Canton (ChainSafe DevNet/Mainnet).
# Only uses API Server + PostgreSQL - no local Canton, Anvil, or Relayer.
#
# Usage:
#   ./scripts/remote/bootstrap-remote.sh --devnet   # Uses config.api-server.devnet.yaml
#   ./scripts/remote/bootstrap-remote.sh --mainnet  # Uses config.api-server.mainnet.yaml
#
# What it does:
#   1. Generates CANTON_MASTER_KEY
#   2. Starts PostgreSQL and API Server containers
#   3. Waits for services to be healthy
#   4. Whitelists and registers test users
#   5. Bootstraps DEMO token (native Canton token)
#   6. Displays MetaMask configuration
# =============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Defaults
NETWORK=""
CONFIG_FILE=""
DEMO_AMOUNT="500"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --devnet)
            NETWORK="devnet"
            CONFIG_FILE="config.api-server.devnet.yaml"
            shift
            ;;
        --mainnet)
            NETWORK="mainnet"
            CONFIG_FILE="config.api-server.mainnet.yaml"
            shift
            ;;
        --demo-amount)
            DEMO_AMOUNT="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 --devnet|--mainnet [--demo-amount N]"
            exit 1
            ;;
    esac
done

if [ -z "$NETWORK" ]; then
    echo -e "${RED}Error: Must specify --devnet or --mainnet${NC}"
    echo "Usage: $0 --devnet|--mainnet [--demo-amount N]"
    exit 1
fi

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

# =============================================================================
# Main Script
# =============================================================================

print_header "Canton Bridge Remote Bootstrap ($NETWORK)"
echo "    Config: $CONFIG_FILE"
echo "    DEMO Amount: $DEMO_AMOUNT per user"

# Step 1: Check config exists
print_header "Step 1: Validate Configuration"

if [ ! -f "$CONFIG_FILE" ]; then
    print_error "Config file not found: $CONFIG_FILE"
    exit 1
fi
print_success "Config file exists: $CONFIG_FILE"

# Step 2: Generate master key
print_header "Step 2: Generate Master Key"
export CANTON_MASTER_KEY=$(openssl rand -base64 32)
export CONFIG_FILE
export SKIP_CANTON_SIG_VERIFY=true
print_success "Master key generated"

# Step 3: Stop existing services
print_header "Step 3: Stop Existing Services"
# Stop local stack first (may hold conflicting ports 5432, 8081)
docker compose down -v 2>/dev/null || true
docker compose -f docker-compose.remote.yaml down -v 2>/dev/null || true
print_success "Services stopped"

# Step 4: Start services
print_header "Step 4: Start Docker Services"
docker compose -f docker-compose.remote.yaml up -d --build
print_success "Docker services starting"

# Step 5: Wait for services
print_header "Step 5: Wait for Services"
sleep 5

# Wait for API Server
print_step "Waiting for API Server..."
for i in $(seq 1 60); do
    if curl -s "http://localhost:8081/health" > /dev/null 2>&1; then
        print_success "API Server is ready"
        break
    fi
    if [ $i -eq 60 ]; then
        print_error "API Server failed to start"
        echo "Check logs with: docker compose -f docker-compose.remote.yaml logs api-server"
        exit 1
    fi
    sleep 2
done

# Step 6: Whitelist test users
print_header "Step 6: Whitelist Test Users"

# Test user addresses (from Anvil defaults - same keys work for signing)
USER1_ADDRESS="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
USER2_ADDRESS="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
USER1_PRIVATE_KEY="ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER2_PRIVATE_KEY="59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

docker compose -f docker-compose.remote.yaml exec -T postgres psql -U postgres -d erc20_api -c \
    "INSERT INTO whitelist (evm_address, note) VALUES 
     ('$USER1_ADDRESS', 'Test User 1'),
     ('$USER2_ADDRESS', 'Test User 2')
     ON CONFLICT DO NOTHING;" > /dev/null
print_success "Users whitelisted"

# Step 7: Register users
print_header "Step 7: Register Users"

for i in 1 2; do
    if [ $i -eq 1 ]; then
        PRIVATE_KEY=$USER1_PRIVATE_KEY
        ADDRESS=$USER1_ADDRESS
    else
        PRIVATE_KEY=$USER2_PRIVATE_KEY
        ADDRESS=$USER2_ADDRESS
    fi
    
    MESSAGE="Register for Canton Bridge"
    SIGNATURE=$(cast wallet sign --private-key "$PRIVATE_KEY" "$MESSAGE" 2>/dev/null)
    
    RESPONSE=$(curl -s -X POST http://localhost:8081/register \
        -H "Content-Type: application/json" \
        -d "{\"signature\": \"$SIGNATURE\", \"message\": \"$MESSAGE\"}")
    
    if echo "$RESPONSE" | grep -q "fingerprint"; then
        FINGERPRINT=$(echo "$RESPONSE" | jq -r '.fingerprint')
        CANTON_PARTY=$(echo "$RESPONSE" | jq -r '.party // empty')
        print_success "User $i registered: ${FINGERPRINT:0:20}..."
        if [ -n "$CANTON_PARTY" ]; then
            echo "    Canton Party: ${CANTON_PARTY:0:30}..."
        fi
        if [ $i -eq 1 ]; then
            USER1_FINGERPRINT=$FINGERPRINT
            USER1_CANTON_PARTY=$CANTON_PARTY
        else
            USER2_FINGERPRINT=$FINGERPRINT
            USER2_CANTON_PARTY=$CANTON_PARTY
        fi
    else
        print_error "User $i registration failed: $RESPONSE"
        echo "Response: $RESPONSE"
        echo ""
        echo "Check API server logs: docker compose -f docker-compose.remote.yaml logs api-server"
        exit 1
    fi
done

# Step 8: Bootstrap DEMO tokens
print_header "Step 8: Bootstrap DEMO Tokens"

print_step "Minting DEMO tokens on ChainSafe $NETWORK..."
DATABASE_HOST=localhost go run scripts/setup/bootstrap-demo.go \
    -config "$CONFIG_FILE" \
    -user1-fingerprint "$USER1_FINGERPRINT" \
    -user2-fingerprint "$USER2_FINGERPRINT" \
    -user1-party "$USER1_CANTON_PARTY" \
    -user2-party "$USER2_CANTON_PARTY" \
    -mint-amount "$DEMO_AMOUNT" 2>&1 | grep -E "^(>>>|✓|DEMO|User|Error|\[)" || {
        print_warning "DEMO bootstrap output above - check for errors"
    }

print_success "DEMO bootstrap attempted"

# Step 9: Display MetaMask info
print_header "MetaMask Configuration"

echo ""
echo -e "${YELLOW}1. Add Network to MetaMask:${NC}"
echo "   Network Name:  Canton $NETWORK"
echo -e "   RPC URL:       ${GREEN}http://localhost:8081/eth${NC}"
echo -e "   Chain ID:      ${GREEN}1337${NC}"
echo "   Currency:      ETH"
echo ""
echo -e "${YELLOW}2. Import Test Accounts:${NC}"
echo ""
echo "   User 1:"
echo -e "   Address:     ${GREEN}$USER1_ADDRESS${NC}"
echo -e "   Private Key: ${GREEN}$USER1_PRIVATE_KEY${NC}"
echo ""
echo "   User 2:"
echo -e "   Address:     ${GREEN}$USER2_ADDRESS${NC}"
echo -e "   Private Key: ${GREEN}$USER2_PRIVATE_KEY${NC}"
echo ""
echo -e "${YELLOW}3. Import Tokens:${NC}"
echo ""
echo "   DEMO (native Canton token):"
echo -e "   Address:  ${GREEN}0xDE30000000000000000000000000000000000001${NC}"
echo "   Symbol:   DEMO"
echo "   Decimals: 18"

# Show current balances
print_header "Current Balances"
echo ""

USER1_DEMO=$(docker compose -f docker-compose.remote.yaml exec -T postgres psql -U postgres -d erc20_api -t -c \
    "SELECT demo_balance FROM users WHERE evm_address = '$USER1_ADDRESS'" 2>/dev/null | tr -d ' ')
USER2_DEMO=$(docker compose -f docker-compose.remote.yaml exec -T postgres psql -U postgres -d erc20_api -t -c \
    "SELECT demo_balance FROM users WHERE evm_address = '$USER2_ADDRESS'" 2>/dev/null | tr -d ' ')

format_balance() {
    local bal=$1
    if [ -z "$bal" ] || [ "$bal" = "0" ]; then
        echo "0"
    else
        echo "$bal" | awk '{printf "%.6f", $1}'
    fi
}

echo -e "   User 1 (${USER1_ADDRESS:0:10}...):"
echo -e "      DEMO:   ${GREEN}$(format_balance $USER1_DEMO)${NC}"
echo ""
echo -e "   User 2 (${USER2_ADDRESS:0:10}...):"
echo -e "      DEMO:   ${GREEN}$(format_balance $USER2_DEMO)${NC}"

print_header "Ready for Testing!"
echo ""
echo "   You can now test MetaMask transfers between User 1 and User 2."
echo "   DEMO token transfers are native Canton transactions on $NETWORK."
echo ""
echo "   Logs: docker compose -f docker-compose.remote.yaml logs -f api-server"
echo ""
