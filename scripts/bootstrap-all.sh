#!/bin/bash
# =============================================================================
# Canton Bridge Full Bootstrap Script
# =============================================================================
# This script automates the entire local setup process:
# 1. Shuts down any existing services
# 2. Starts Docker services (Canton, Anvil, PostgreSQL, etc.)
# 3. Waits for all services to be healthy
# 4. Registers test users
# 5. Mints DEMO tokens (500 each by default)
# 6. Optionally deposits PROMPT tokens (100 to User 1 only)
# 7. Displays MetaMask configuration
#
# Final state:
#   User 1: 500 DEMO, 100 PROMPT
#   User 2: 500 DEMO, 0 PROMPT
#
# Usage:
#   ./scripts/bootstrap-all.sh                    # Full setup
#   ./scripts/bootstrap-all.sh --skip-prompt      # Skip PROMPT deposit
#   ./scripts/bootstrap-all.sh --demo-amount 1000 # Custom DEMO amount
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
DEMO_AMOUNT="500"
PROMPT_AMOUNT="100"
SKIP_PROMPT=false
SKIP_SHUTDOWN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-prompt)
            SKIP_PROMPT=true
            shift
            ;;
        --skip-shutdown)
            SKIP_SHUTDOWN=true
            shift
            ;;
        --demo-amount)
            DEMO_AMOUNT="$2"
            shift 2
            ;;
        --prompt-amount)
            PROMPT_AMOUNT="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Test user keys (Anvil defaults)
USER1_PRIVATE_KEY="ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER2_PRIVATE_KEY="59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
USER1_ADDRESS="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
USER2_ADDRESS="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

# Contract addresses (Anvil deployment)
TOKEN_ADDRESS="0x5FbDB2315678afecb367f032d93F642f64180aa3"
BRIDGE_ADDRESS="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"

# Config
NATIVE_TOKEN_PACKAGE_ID="3cc8001e5d4814175003822af1efc5bcfb7826c00b1f764d9d17d4f4ca1f0809"
CONFIG_FILE="config.e2e-local.yaml"

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

wait_for_service() {
    local name=$1
    local url=$2
    local max_attempts=${3:-30}
    
    print_step "Waiting for $name..."
    for i in $(seq 1 $max_attempts); do
        if curl -s "$url" > /dev/null 2>&1; then
            print_success "$name is ready"
            return 0
        fi
        sleep 2
    done
    print_error "$name failed to start"
    return 1
}

# =============================================================================
# Main Script
# =============================================================================

print_header "Canton Bridge Full Bootstrap"
echo "    DEMO Amount: $DEMO_AMOUNT per user"
echo "    PROMPT Amount: $PROMPT_AMOUNT per user (skip: $SKIP_PROMPT)"

# Step 1: Generate master key
print_header "Step 1: Generate Master Key"
export CANTON_MASTER_KEY=$(openssl rand -base64 32)
print_success "Master key generated"

# Step 2: Shutdown existing services
if [ "$SKIP_SHUTDOWN" = false ]; then
    print_header "Step 2: Shutting Down Existing Services"
    docker compose down -v 2>/dev/null || true
    print_success "Services stopped"
fi

# Step 3: Start Docker services
print_header "Step 3: Starting Docker Services"
docker compose up -d --build
print_success "Docker services starting"

# Step 4: Wait for services
print_header "Step 4: Waiting for Services"
sleep 5  # Give containers time to initialize

wait_for_service "Anvil" "http://localhost:8545"
wait_for_service "API Server" "http://localhost:8081/health"

# Wait for Canton (check via bootstrap logs)
print_step "Waiting for Canton bootstrap..."
for i in $(seq 1 60); do
    if docker compose logs bootstrap 2>&1 | grep -q "Bootstrap complete"; then
        print_success "Canton bootstrap complete"
        break
    fi
    if [ $i -eq 60 ]; then
        print_warning "Bootstrap timeout, continuing anyway..."
    fi
    sleep 2
done

# Step 5: Extract domain ID and relayer party
print_header "Step 5: Extracting Canton Configuration"
DOMAIN_ID=$(docker compose logs bootstrap 2>&1 | grep "domain_id:" | tail -1 | sed 's/.*domain_id: "\(.*\)"/\1/')
RELAYER_PARTY=$(docker compose logs bootstrap 2>&1 | grep "relayer_party:" | tail -1 | sed 's/.*relayer_party: "\(.*\)"/\1/')

if [ -z "$DOMAIN_ID" ]; then
    print_error "Failed to extract domain ID"
    exit 1
fi
print_success "Domain ID: ${DOMAIN_ID:0:30}..."
print_success "Relayer Party: ${RELAYER_PARTY:0:30}..."

# Update config file with current domain ID and relayer party
print_step "Updating config with Canton details..."
sed -i.bak "s|domain_id: \"local::[^\"]*\"|domain_id: \"$DOMAIN_ID\"|" "$CONFIG_FILE"
sed -i.bak "s|relayer_party: \"BridgeIssuer::[^\"]*\"|relayer_party: \"$RELAYER_PARTY\"|" "$CONFIG_FILE"
rm -f "${CONFIG_FILE}.bak"
print_success "Config updated"

# Step 6: Whitelist users
print_header "Step 6: Whitelist Test Users"
docker compose exec -T postgres psql -U postgres -d erc20_api -c \
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
        print_success "User $i registered: ${FINGERPRINT:0:20}..."
        if [ $i -eq 1 ]; then
            USER1_FINGERPRINT=$FINGERPRINT
        else
            USER2_FINGERPRINT=$FINGERPRINT
        fi
    else
        print_error "User $i registration failed: $RESPONSE"
        exit 1
    fi
done

# Step 8: Bootstrap DEMO tokens
print_header "Step 8: Bootstrap DEMO Tokens"
go run scripts/bootstrap-demo.go \
    -config "$CONFIG_FILE" \
    -native-package-id "$NATIVE_TOKEN_PACKAGE_ID" \
    -user1-fingerprint "$USER1_FINGERPRINT" \
    -user2-fingerprint "$USER2_FINGERPRINT" \
    -mint-amount "$DEMO_AMOUNT" 2>&1 | grep -E "^(>>>|✓|DEMO|User)"

print_success "DEMO tokens minted: $DEMO_AMOUNT per user"

# Step 9: Deposit PROMPT tokens (optional)
if [ "$SKIP_PROMPT" = false ]; then
    print_header "Step 9: Deposit PROMPT Tokens"
    
    ANVIL_URL="http://localhost:8545"
    
    AMOUNT_WEI=$(cast --to-wei "$PROMPT_AMOUNT" ether 2>/dev/null)
    
    # Deposit PROMPT for User 1 only (User 1 is the token deployer on Anvil)
    print_step "Depositing $PROMPT_AMOUNT PROMPT for User 1..."
    
    # Approve bridge
    cast send "$TOKEN_ADDRESS" "approve(address,uint256)" "$BRIDGE_ADDRESS" "$AMOUNT_WEI" \
        --private-key "$USER1_PRIVATE_KEY" --rpc-url "$ANVIL_URL" > /dev/null 2>&1
    
    # Deposit to Canton
    cast send "$BRIDGE_ADDRESS" "depositToCanton(address,uint256,bytes32)" \
        "$TOKEN_ADDRESS" "$AMOUNT_WEI" "$USER1_FINGERPRINT" \
        --private-key "$USER1_PRIVATE_KEY" --rpc-url "$ANVIL_URL" > /dev/null 2>&1
    
    print_success "User 1 PROMPT deposit submitted"
    
    # Wait for relayer to process deposit
    print_step "Waiting for relayer to process deposit..."
    sleep 10
    
    # Verify PROMPT balance via API
    for i in 1 2 3 4 5; do
        USER1_PROMPT=$(docker compose exec -T postgres psql -U postgres -d erc20_api -t -c \
            "SELECT prompt_balance FROM users WHERE evm_address = '$USER1_ADDRESS'" 2>/dev/null | tr -d ' ')
        if [ -n "$USER1_PROMPT" ] && [ "$USER1_PROMPT" != "0" ] && [ "$USER1_PROMPT" != "0.000000000000000000" ]; then
            break
        fi
        sleep 3
    done
    
    print_success "PROMPT deposit complete"
fi

# Step 10: Display MetaMask info
print_header "MetaMask Configuration"

echo ""
echo -e "${YELLOW}1. Add Network to MetaMask:${NC}"
echo "   Network Name:  Canton Local"
echo -e "   RPC URL:       ${GREEN}http://localhost:8081/eth${NC}"
echo -e "   Chain ID:      ${GREEN}31337${NC}"
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
echo "   PROMPT (bridged ERC-20):"
echo -e "   Address:  ${GREEN}$TOKEN_ADDRESS${NC}"
echo "   Symbol:   PROMPT"
echo "   Decimals: 18"
echo ""
echo "   DEMO (native Canton token):"
echo -e "   Address:  ${GREEN}0xDE30000000000000000000000000000000000001${NC}"
echo "   Symbol:   DEMO"
echo "   Decimals: 18"

# Show current balances
print_header "Current Balances"
echo ""

# Query all balances
USER1_PROMPT=$(docker compose exec -T postgres psql -U postgres -d erc20_api -t -c \
    "SELECT prompt_balance FROM users WHERE evm_address = '$USER1_ADDRESS'" 2>/dev/null | tr -d ' ')
USER1_DEMO=$(docker compose exec -T postgres psql -U postgres -d erc20_api -t -c \
    "SELECT demo_balance FROM users WHERE evm_address = '$USER1_ADDRESS'" 2>/dev/null | tr -d ' ')
USER2_PROMPT=$(docker compose exec -T postgres psql -U postgres -d erc20_api -t -c \
    "SELECT prompt_balance FROM users WHERE evm_address = '$USER2_ADDRESS'" 2>/dev/null | tr -d ' ')
USER2_DEMO=$(docker compose exec -T postgres psql -U postgres -d erc20_api -t -c \
    "SELECT demo_balance FROM users WHERE evm_address = '$USER2_ADDRESS'" 2>/dev/null | tr -d ' ')

# Format balances (trim to reasonable decimals)
format_balance() {
    local bal=$1
    if [ -z "$bal" ] || [ "$bal" = "0" ]; then
        echo "0"
    else
        # Show first 6 decimal places
        echo "$bal" | awk '{printf "%.6f", $1}'
    fi
}

echo -e "   User 1 (${USER1_ADDRESS:0:10}...):"
echo -e "      PROMPT: ${GREEN}$(format_balance $USER1_PROMPT)${NC}"
echo -e "      DEMO:   ${GREEN}$(format_balance $USER1_DEMO)${NC}"
echo ""
echo -e "   User 2 (${USER2_ADDRESS:0:10}...):"
echo -e "      PROMPT: ${GREEN}$(format_balance $USER2_PROMPT)${NC}"
echo -e "      DEMO:   ${GREEN}$(format_balance $USER2_DEMO)${NC}"

print_header "Ready for Testing!"
echo ""
echo "   You can now test MetaMask transfers between User 1 and User 2."
echo "   Both PROMPT and DEMO tokens should work."
echo ""
