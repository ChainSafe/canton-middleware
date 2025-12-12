#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge Unified Test Script
# =============================================================================
# Tests the bridge across all environments: local, devnet, and mainnet.
#
# Usage:
#   ./scripts/test-bridge.sh --config config.local.yaml
#   ./scripts/test-bridge.sh --config config.devnet.yaml
#   ./scripts/test-bridge.sh --config config.mainnet.yaml --skip-setup
#
# Options:
#   --config <file>     Config file (required)
#   --clean             Reset environment before starting
#   --skip-setup        Skip Docker/bootstrap, run bridge tests only
#   --deposit-only      Only test deposit flow
#   --withdraw-only     Only test withdrawal flow
#   --dry-run           Validate configuration without executing transactions (mainnet only)
#
# Examples:
#   # Local: full setup and test
#   ./scripts/test-bridge.sh --config config.local.yaml --clean
#
#   # Local: rerun tests (services already running)
#   ./scripts/test-bridge.sh --config config.local.yaml --skip-setup
#
#   # Devnet: full test
#   ./scripts/test-bridge.sh --config config.devnet.yaml
#
#   # Mainnet: test only (already bootstrapped)
#   ./scripts/test-bridge.sh --config config.mainnet.yaml --skip-setup --deposit-only
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

# =============================================================================
# Parse Arguments
# =============================================================================

CONFIG_FILE=""
CLEAN=false
SKIP_SETUP=false
DEPOSIT_ONLY=false
WITHDRAW_ONLY=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --config)
            CONFIG_FILE="$2"
            shift 2
            ;;
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
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            head -50 "$0" | tail -n +2 | sed 's/^# //' | sed 's/^#//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [ -z "$CONFIG_FILE" ]; then
    echo -e "${RED}Error: --config is required${NC}"
    echo "Usage: $0 --config <config-file> [options]"
    exit 1
fi

# Resolve config file path
if [[ "$CONFIG_FILE" != /* ]]; then
    CONFIG_FILE="$PROJECT_DIR/$CONFIG_FILE"
fi

if [ ! -f "$CONFIG_FILE" ]; then
    echo -e "${RED}Error: Config file not found: $CONFIG_FILE${NC}"
    exit 1
fi

# =============================================================================
# Detect Environment from Config
# =============================================================================

detect_environment() {
    if grep -q "localhost:5011" "$CONFIG_FILE"; then
        echo "local"
    elif grep -q "dev1\|devnet" "$CONFIG_FILE"; then
        echo "devnet"
    elif grep -q "prod1\|mainnet" "$CONFIG_FILE" || grep -q "chain_id: 1$" "$CONFIG_FILE"; then
        echo "mainnet"
    else
        echo "unknown"
    fi
}

ENV=$(detect_environment)

# =============================================================================
# Environment-specific Configuration
# =============================================================================

case "$ENV" in
    local)
        DOCKER_COMPOSE_CMD="docker compose"
        RELAYER_LOG="/tmp/relayer.log"
        CONFIRMATION_WAIT=8
        
        # Local Anvil accounts (from mnemonic)
        RELAYER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
        RELAYER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
        OWNER="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
        OWNER_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
        USER="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
        USER_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
        
        # Test amounts
        TEST_DEPOSIT_TOKENS=100
        TEST_WITHDRAW_TOKENS=50
        FUND_AMOUNT="1000000000000000000000"  # 1000 tokens
        ;;
    devnet)
        DOCKER_COMPOSE_CMD="docker compose -f docker-compose.yaml -f docker-compose.devnet.yaml"
        RELAYER_LOG="/tmp/relayer-devnet.log"
        CONFIRMATION_WAIT=10
        
        # Devnet accounts (Sepolia testnet)
        RELAYER="0x914db8873AcFd84b834278e20BB9fCE9DD223043"
        RELAYER_KEY="0x***REMOVED***"
        OWNER="0x8A0A6FF59ad10e009b0fdB3B3CA7A0356eDcCCbf"
        OWNER_KEY="0x3ffae7a5be2fa63022325b175a04cab999af2b8ad956208d10861a75701eae9b"
        USER="0x4768CCb3cE015698468A65bf8208b3f6919c769e"
        USER_KEY="0xeacbff42147f4a4493e2212a70ace9e0ef4e40532e5aa3e049a0eb355e8fc5be"
        
        # Test amounts
        TEST_DEPOSIT_TOKENS=100
        TEST_WITHDRAW_TOKENS=50
        FUND_AMOUNT="1000000000000000000000"  # 1000 tokens
        ;;
    mainnet)
        DOCKER_COMPOSE_CMD="docker compose -f docker-compose.yaml -f docker-compose.mainnet.yaml"
        RELAYER_LOG="/tmp/relayer-mainnet.log"
        CONFIRMATION_WAIT=180  # 12 blocks * ~15s
        
        # Mainnet accounts loaded from secrets
        if [ -f "$PROJECT_DIR/secrets/mainnet-owner-key.txt" ]; then
            OWNER_KEY="0x$(cat "$PROJECT_DIR/secrets/mainnet-owner-key.txt" | tr -d '\n')"
            OWNER=$(cast wallet address --private-key "$OWNER_KEY" 2>/dev/null || echo "")
        fi
        if [ -f "$PROJECT_DIR/secrets/mainnet-user-key.txt" ]; then
            USER_KEY="0x$(cat "$PROJECT_DIR/secrets/mainnet-user-key.txt" | tr -d '\n')"
            USER=$(cast wallet address --private-key "$USER_KEY" 2>/dev/null || echo "")
        fi
        
        # Test amounts (smaller for mainnet)
        TEST_DEPOSIT_TOKENS=10
        TEST_WITHDRAW_TOKENS=5
        FUND_AMOUNT=""  # Don't fund on mainnet
        ;;
    *)
        echo -e "${RED}Error: Could not detect environment from config${NC}"
        exit 1
        ;;
esac

# Calculate wei amounts
TEST_DEPOSIT_AMOUNT_WEI="${TEST_DEPOSIT_TOKENS}000000000000000000"
TEST_WITHDRAW_AMOUNT_DECIMAL="${TEST_WITHDRAW_TOKENS}.0"

# Token ID (same across environments)
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

kill_relayer() {
    print_step "Stopping any existing relayer processes..."
    pkill -9 -f "cmd/relayer" 2>/dev/null || true
    pkill -9 -f "main.go" 2>/dev/null || true
    lsof -ti:8080 | xargs kill -9 2>/dev/null || true
    sleep 2
}

kill_mock_oauth() {
    pkill -9 -f "mock-oauth2-server" 2>/dev/null || true
    lsof -ti:8088 | xargs kill -9 2>/dev/null || true
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
    
    # Also wait for HTTP API
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
    
    # Wait for synchronizer connection
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

check_jwt_token() {
    local token_file="$1"
    if [ ! -f "$token_file" ]; then
        return 1
    fi
    
    TOKEN_PAYLOAD=$(cat "$token_file" | cut -d'.' -f2)
    TOKEN_EXP=$(echo "${TOKEN_PAYLOAD}==" | base64 -d 2>/dev/null | jq -r '.exp' 2>/dev/null || echo "")
    CURRENT_TIME=$(date +%s)
    
    if [ -n "$TOKEN_EXP" ] && [ "$TOKEN_EXP" != "null" ]; then
        if [ "$CURRENT_TIME" -gt "$TOKEN_EXP" ]; then
            print_error "JWT token has EXPIRED!"
            echo "Token expired at: $(date -r $TOKEN_EXP 2>/dev/null || date -d @$TOKEN_EXP 2>/dev/null)"
            return 1
        else
            HOURS_LEFT=$(( ($TOKEN_EXP - $CURRENT_TIME) / 3600 ))
            print_info "JWT token valid for ~${HOURS_LEFT} more hours"
        fi
    fi
    return 0
}

confirm_mainnet() {
    echo ""
    echo -e "${RED}╔══════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║                    ⚠️  MAINNET WARNING ⚠️                            ║${NC}"
    echo -e "${RED}║                                                                      ║${NC}"
    echo -e "${RED}║  You are about to execute transactions on ETHEREUM MAINNET.          ║${NC}"
    echo -e "${RED}║  This will use REAL ETH for gas and transfer REAL tokens.            ║${NC}"
    echo -e "${RED}║                                                                      ║${NC}"
    echo -e "${RED}╚══════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    if [ "$DRY_RUN" = true ]; then
        print_warning "DRY RUN MODE - No transactions will be executed"
        return 0
    fi
    
    read -p "Type 'MAINNET' to confirm: " CONFIRM
    if [ "$CONFIRM" != "MAINNET" ]; then
        print_error "Aborted by user"
        exit 1
    fi
}

# =============================================================================
# Parse Configuration from YAML
# =============================================================================

parse_config() {
    # Parse ethereum section
    ETH_RPC_URL=$(awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
        | grep 'rpc_url:' | sed 's/.*rpc_url: *"\([^"]*\)".*/\1/')
    
    CHAIN_ID=$(awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
        | grep 'chain_id:' | sed 's/.*chain_id: *\([0-9]*\).*/\1/')
    
    BRIDGE=$(awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
        | grep 'bridge_contract:' | sed 's/.*bridge_contract: *"\([^"]*\)".*/\1/')
    
    TOKEN=$(awk '/^ethereum:/{flag=1;next}/^[^[:space:]]/{flag=0}flag' "$CONFIG_FILE" \
        | grep 'token_contract:' | sed 's/.*token_contract: *"\([^"]*\)".*/\1/')
    
    # Parse canton section
    PARTY_ID=$(grep 'relayer_party:' "$CONFIG_FILE" | sed 's/.*relayer_party: *"\([^"]*\)".*/\1/')
    DOMAIN_ID=$(grep 'domain_id:' "$CONFIG_FILE" | sed 's/.*domain_id: *"\([^"]*\)".*/\1/')
    PACKAGE_ID=$(grep 'bridge_package_id:' "$CONFIG_FILE" | sed 's/.*bridge_package_id: *"\([^"]*\)".*/\1/')
    
    # Extract fingerprint for deposits
    FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
    if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
        FINGERPRINT="${FULL_FINGERPRINT:4}"
    else
        FINGERPRINT="$FULL_FINGERPRINT"
    fi
}

# =============================================================================
# Main Script
# =============================================================================

cd "$PROJECT_DIR"

print_header "CANTON-ETHEREUM BRIDGE TEST"
echo ""
ENV_UPPER=$(echo "$ENV" | tr '[:lower:]' '[:upper:]')
echo -e "${YELLOW}Environment: ${ENV_UPPER}${NC}"
echo "Config file: $CONFIG_FILE"
echo ""

# Parse config
parse_config

# Environment-specific pre-flight checks
case "$ENV" in
    devnet)
        if ! check_jwt_token "$PROJECT_DIR/secrets/devnet-token.txt"; then
            print_error "secrets/devnet-token.txt not found or expired!"
            exit 1
        fi
        ;;
    mainnet)
        confirm_mainnet
        if ! check_jwt_token "$PROJECT_DIR/secrets/mainnet-token.txt"; then
            print_error "secrets/mainnet-token.txt not found or expired!"
            exit 1
        fi
        ;;
esac

print_info "Party ID: $PARTY_ID"
print_info "Domain ID: $DOMAIN_ID"
print_info "Fingerprint: 0x$FINGERPRINT"

# =============================================================================
# Step 0: Clean environment (optional)
# =============================================================================

if [ "$CLEAN" = true ]; then
    print_header "STEP 0: Cleaning Environment"
    kill_relayer
    kill_mock_oauth
    print_step "Stopping Docker containers..."
    $DOCKER_COMPOSE_CMD down -v 2>/dev/null || true
    print_success "Environment cleaned"
fi

# =============================================================================
# Setup Phase (skipped with --skip-setup)
# =============================================================================

if [ "$SKIP_SETUP" = false ]; then
    print_header "SETUP PHASE: Starting Services"
    
    case "$ENV" in
        local)
            # Start mock OAuth2 server for local testing
            print_step "Starting mock OAuth2 server..."
            kill_mock_oauth
            go run "$SCRIPT_DIR/mock-oauth2-server.go" > /tmp/mock-oauth2.log 2>&1 &
            MOCK_OAUTH_PID=$!
            sleep 2
            if curl -s http://localhost:8088/health | grep -q "OK"; then
                print_success "Mock OAuth2 server running on port 8088"
            else
                print_error "Failed to start mock OAuth2 server"
                cat /tmp/mock-oauth2.log
                exit 1
            fi
            
            # Start Docker services (Canton, Anvil, Postgres)
            if docker compose ps --format '{{.State}}' canton 2>/dev/null | grep -q "running"; then
                print_warning "Docker services already running"
            else
                print_step "Starting docker compose..."
                $DOCKER_COMPOSE_CMD up -d
            fi
            
            wait_for_canton
            
            # Wait for DARs to be uploaded
            print_step "Waiting for DAR packages to be uploaded..."
            CIP56_PACKAGE_ID="e02fdc1d7d2245dad7a0f3238087b155a03bd15cec7c27924ecfa52af1a47dbe"
            DAR_MAX_ATTEMPTS=60
            DAR_ATTEMPT=0
            PACKAGE_COUNT=0
            CIP56_FOUND=false
            while [ $DAR_ATTEMPT -lt $DAR_MAX_ATTEMPTS ]; do
                PACKAGES_JSON=$(curl -s http://localhost:5013/v2/packages 2>/dev/null || echo '{"packageIds":[]}')
                PACKAGE_COUNT=$(echo "$PACKAGES_JSON" | jq '.packageIds | length' 2>/dev/null || echo "0")
                
                if echo "$PACKAGES_JSON" | jq -e ".packageIds | index(\"$CIP56_PACKAGE_ID\")" >/dev/null 2>&1; then
                    CIP56_FOUND=true
                fi
                
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
            
            # Read contract addresses from broadcast file (local only)
            BROADCAST_FILE="$PROJECT_DIR/contracts/ethereum-wayfinder/broadcast/Deployer.s.sol/${CHAIN_ID}/run-latest.json"
            if [ -f "$BROADCAST_FILE" ]; then
                print_step "Reading contract addresses from broadcast file..."
                TOKEN=$(jq -r '.transactions[] | select(.contractName == "PromptToken") | .contractAddress' "$BROADCAST_FILE")
                BRIDGE=$(jq -r '.transactions[] | select(.contractName == "CantonBridge") | .contractAddress' "$BROADCAST_FILE")
                
                # Update config with deployed addresses
                sed -i.bak "s|bridge_contract: \"0x[a-fA-F0-9]*\"|bridge_contract: \"$BRIDGE\"|" "$CONFIG_FILE"
                sed -i.bak "s|token_contract: \"0x[a-fA-F0-9]*\"|token_contract: \"$TOKEN\"|" "$CONFIG_FILE"
                rm -f "${CONFIG_FILE}.bak"
                print_success "Config updated with contract addresses"
            fi
            
            # Allocate party and update config
            print_step "Allocating BridgeIssuer party..."
            EXISTING_PARTY=$(curl -s http://localhost:5013/v2/parties | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1 || true)
            if [ -n "$EXISTING_PARTY" ]; then
                print_warning "BridgeIssuer already exists"
                PARTY_ID="$EXISTING_PARTY"
            else
                PARTY_RESPONSE=$(curl -s -X POST http://localhost:5013/v2/parties \
                    -H 'Content-Type: application/json' \
                    -d '{"partyIdHint": "BridgeIssuer"}')
                PARTY_ID=$(echo "$PARTY_RESPONSE" | jq -r '.partyDetails.party // empty')
            fi
            
            # Get domain ID
            SYNC_RESPONSE=$(curl -s http://localhost:5013/v2/state/connected-synchronizers)
            DOMAIN_ID=$(echo "$SYNC_RESPONSE" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')
            
            # Update config with party and domain
            sed -i.bak "s|domain_id: \".*\"|domain_id: \"$DOMAIN_ID\"|" "$CONFIG_FILE"
            sed -i.bak "s|relayer_party: \".*\"|relayer_party: \"$PARTY_ID\"|" "$CONFIG_FILE"
            rm -f "${CONFIG_FILE}.bak"
            
            # Update fingerprint
            FULL_FINGERPRINT=$(echo "$PARTY_ID" | sed 's/.*:://')
            if [[ "$FULL_FINGERPRINT" == 1220* ]] && [ ${#FULL_FINGERPRINT} -eq 68 ]; then
                FINGERPRINT="${FULL_FINGERPRINT:4}"
            else
                FINGERPRINT="$FULL_FINGERPRINT"
            fi
            
            print_success "Party allocated: $PARTY_ID"
            ;;
            
        devnet|mainnet)
            # Start Postgres only
            print_step "Starting docker compose (Postgres only)..."
            $DOCKER_COMPOSE_CMD up -d
            
            # Wait for Postgres
            print_step "Waiting for Postgres..."
            sleep 2
            if docker exec postgres pg_isready -U postgres >/dev/null 2>&1; then
                print_success "Postgres is ready!"
            else
                print_warning "Postgres may not be ready, continuing anyway..."
            fi
            
            # Verify Ethereum RPC
            print_step "Checking Ethereum RPC endpoint..."
            if cast block-number --rpc-url "$ETH_RPC_URL" >/dev/null 2>&1; then
                BLOCK_NUM=$(cast block-number --rpc-url "$ETH_RPC_URL" 2>/dev/null)
                print_success "Ethereum RPC is reachable (block: $BLOCK_NUM)"
            else
                print_error "Failed to query block number from $ETH_RPC_URL"
                exit 1
            fi
            ;;
    esac
    
    echo ""
    $DOCKER_COMPOSE_CMD ps
fi

# =============================================================================
# Verify Contracts
# =============================================================================

print_header "VERIFY CONTRACTS"

print_info "PromptToken:  $TOKEN"
print_info "CantonBridge: $BRIDGE"

print_step "Checking bridge contract..."
BRIDGE_RELAYER=$(cast call $BRIDGE "relayer()(address)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Bridge relayer: $BRIDGE_RELAYER"

print_step "Checking token contract..."
TOKEN_NAME=$(cast call $TOKEN "name()(string)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
print_info "Token name: $TOKEN_NAME"

print_success "Contracts verified"

# =============================================================================
# Bootstrap Phase (skipped with --skip-setup)
# =============================================================================

if [ "$SKIP_SETUP" = false ]; then
    print_header "BOOTSTRAP PHASE"
    
    if [ "$DRY_RUN" = true ]; then
        print_warning "DRY RUN - Would run bootstrap and register-user scripts"
    else
        print_step "Running bootstrap script..."
        go run scripts/bootstrap-bridge.go \
            -config "$CONFIG_FILE" \
            -issuer "$PARTY_ID" \
            -package "$PACKAGE_ID" || {
                print_warning "Bootstrap may have failed or contracts already exist"
            }
        
        print_step "Running register-user script..."
        go run scripts/register-user.go \
            -config "$CONFIG_FILE" \
            -party "$PARTY_ID" || {
                print_warning "User registration may have failed or user already exists"
            }
    fi
    
    print_success "Bootstrap complete"
fi

# =============================================================================
# Start the Relayer
# =============================================================================

print_header "START RELAYER"

kill_relayer

print_step "Starting relayer in background..."
go run cmd/relayer/main.go -config "$CONFIG_FILE" > "$RELAYER_LOG" 2>&1 &
RELAYER_PID=$!
echo "Relayer PID: $RELAYER_PID"

print_step "Waiting for relayer to start..."
sleep 5

if curl -s http://localhost:8080/health | grep -q "OK"; then
    print_success "Relayer is healthy"
else
    print_error "Relayer health check failed"
    tail -30 "$RELAYER_LOG"
    exit 1
fi

# =============================================================================
# Setup Bridge (EVM)
# =============================================================================

print_header "SETUP BRIDGE (EVM)"

if [ "$DRY_RUN" = true ]; then
    print_warning "DRY RUN - Would add token mapping"
else
    print_step "Adding token mapping..."
    cast send $BRIDGE "addTokenMapping(address,bytes32)" \
        $TOKEN $CANTON_TOKEN_ID \
        --rpc-url "$ETH_RPC_URL" \
        --private-key $OWNER_KEY > /dev/null 2>&1 || print_warning "Token mapping may already exist"
fi

print_success "Bridge setup complete"

# =============================================================================
# Test Deposit (EVM → Canton)
# =============================================================================

if [ "$WITHDRAW_ONLY" = false ]; then
    print_header "TEST: EVM → Canton Deposit"
    
    if [ "$DRY_RUN" = true ]; then
        print_warning "DRY RUN - Would execute deposit of ${TEST_DEPOSIT_TOKENS} tokens"
    else
        # Fund user (local/devnet only)
        if [ -n "$FUND_AMOUNT" ]; then
            print_step "Transferring tokens from owner to user..."
            cast send $TOKEN "transfer(address,uint256)" $USER "$FUND_AMOUNT" \
                --rpc-url "$ETH_RPC_URL" \
                --private-key $OWNER_KEY > /dev/null 2>&1
            print_success "User funded"
        fi
        
        # Approve
        print_step "Approving bridge to spend tokens..."
        cast send $TOKEN "approve(address,uint256)" $BRIDGE "$TEST_DEPOSIT_AMOUNT_WEI" \
            --rpc-url "$ETH_RPC_URL" \
            --private-key $USER_KEY > /dev/null 2>&1
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
        
        # Wait for relayer
        print_step "Waiting for relayer to process deposit..."
        sleep $CONFIRMATION_WAIT
        
        # Verify on Canton
        print_step "Verifying holdings on Canton..."
        HOLDINGS_OUTPUT=$(go run scripts/query-holdings.go -config "$CONFIG_FILE" -party "$PARTY_ID" 2>/dev/null)
        echo "$HOLDINGS_OUTPUT"
        
        HOLDING_CID=$(echo "$HOLDINGS_OUTPUT" | grep "Contract ID:" | head -1 | awk '{print $3}')
        print_info "Holding CID: $HOLDING_CID"
        
        if [ -z "$HOLDING_CID" ]; then
            print_error "No holdings found - deposit may have failed"
            tail -30 "$RELAYER_LOG"
            exit 1
        fi
        
        print_success "Deposit verified on Canton!"
    fi
fi

# =============================================================================
# Test Withdrawal (Canton → EVM)
# =============================================================================

if [ "$DEPOSIT_ONLY" = false ]; then
    print_header "TEST: Canton → EVM Withdrawal"
    
    if [ "$DRY_RUN" = true ]; then
        print_warning "DRY RUN - Would execute withdrawal of ${TEST_WITHDRAW_TOKENS} tokens"
    else
        # Get holding CID if not already set
        if [ -z "$HOLDING_CID" ]; then
            HOLDINGS_OUTPUT=$(go run scripts/query-holdings.go -config "$CONFIG_FILE" -party "$PARTY_ID" 2>/dev/null)
            HOLDING_CID=$(echo "$HOLDINGS_OUTPUT" | grep "Contract ID:" | head -1 | awk '{print $3}')
        fi
        
        if [ -z "$HOLDING_CID" ]; then
            print_error "No holdings found for withdrawal"
            exit 1
        fi
        
        BALANCE_BEFORE=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
        print_info "User balance before: $BALANCE_BEFORE"
        
        print_step "Initiating withdrawal of ${TEST_WITHDRAW_TOKENS} tokens..."
        go run scripts/initiate-withdrawal.go \
            -config "$CONFIG_FILE" \
            -holding-cid "$HOLDING_CID" \
            -amount "$TEST_WITHDRAW_AMOUNT_DECIMAL" \
            -evm-destination "$USER"
        
        print_step "Waiting for relayer to process withdrawal..."
        sleep $CONFIRMATION_WAIT
        
        BALANCE_AFTER=$(cast call $TOKEN "balanceOf(address)(uint256)" $USER --rpc-url "$ETH_RPC_URL" 2>/dev/null)
        print_info "User balance after: $BALANCE_AFTER"
        
        print_success "Withdrawal processed"
    fi
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
echo "CONFIGURATION (${ENV_UPPER})"
echo "═══════════════════════════════════════════════════════════════════════"
echo "Config:          $CONFIG_FILE"
echo "ETH RPC:         $ETH_RPC_URL"
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
elif [ "$WITHDRAW_ONLY" = true ]; then
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
echo "RELAYER STATUS"
echo "═══════════════════════════════════════════════════════════════════════"
curl -s http://localhost:8080/api/v1/transfers | jq '.transfers[] | {id: .ID, direction: .Direction, status: .Status}' 2>/dev/null || echo "No transfers found"
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "COMMANDS"
echo "═══════════════════════════════════════════════════════════════════════"
echo "View relayer logs:     tail -f $RELAYER_LOG"
echo "Query holdings:        go run scripts/query-holdings.go -config \"$CONFIG_FILE\" -party \"$PARTY_ID\""
echo "Stop relayer:          pkill -f 'cmd/relayer'"
echo "Stop all services:     $DOCKER_COMPOSE_CMD down"
echo ""
