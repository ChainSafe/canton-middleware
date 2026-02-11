#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge - DevNet + Sepolia Setup Script
# =============================================================================
# This script sets up local services to connect to DevNet Canton + Sepolia.
#
# What runs locally (native Go processes):
#   - PostgreSQL (Docker container for state)
#   - Relayer (connects to DevNet Canton + Sepolia)
#   - API Server (connects to DevNet Canton, serves MetaMask RPC)
#
# What runs remotely:
#   - Canton participant (5North DevNet)
#   - Ethereum network (Sepolia testnet)
#
# Usage:
#   ./scripts/setup/setup-devnet.sh              # Full setup with users + tokens
#   ./scripts/setup/setup-devnet.sh --setup-only # Start services only
#   ./scripts/setup/setup-devnet.sh --clean      # Clean database and restart
#   ./scripts/setup/setup-devnet.sh --status     # Check service status
#   ./scripts/setup/setup-devnet.sh --stop       # Stop all services
#
# =============================================================================

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Parse arguments
SETUP_ONLY=false
CLEAN=false
STATUS_ONLY=false
STOP_ONLY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --setup-only)
            SETUP_ONLY=true
            shift
            ;;
        --clean)
            CLEAN=true
            shift
            ;;
        --status)
            STATUS_ONLY=true
            shift
            ;;
        --stop)
            STOP_ONLY=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --setup-only    Start services without user/token setup"
            echo "  --clean         Clean database and restart"
            echo "  --status        Check status of services"
            echo "  --stop          Stop all services"
            echo "  -h, --help      Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

print_header() {
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
}

print_success() { echo -e "${GREEN}✓ $1${NC}"; }
print_warning() { echo -e "${YELLOW}⚠ $1${NC}"; }
print_error() { echo -e "${RED}✗ $1${NC}"; }
print_info() { echo -e "${CYAN}>>> $1${NC}"; }

# =============================================================================
# Config
# =============================================================================
RELAYER_CONFIG="$PROJECT_ROOT/config.local-devnet.yaml"
API_SERVER_CONFIG="$PROJECT_ROOT/config.api-server.local-devnet.yaml"
SECRETS_FILE="$PROJECT_ROOT/secrets/devnet-secrets.sh"

# DevNet Canton endpoint
CANTON_GRPC="canton-ledger-api-grpc-dev1.chainsafe.dev:80"

# Load secrets from file (keeps credentials out of this script)
if [ -f "$SECRETS_FILE" ]; then
    source "$SECRETS_FILE"
else
    print_error "Secrets file not found: $SECRETS_FILE"
    echo "  Please create it with OAuth credentials and user keys."
    echo "  See secrets/devnet-secrets.sh.example for template."
    exit 1
fi

# Verify required secrets are set
if [ -z "$CANTON_AUTH_CLIENT_ID" ] || [ -z "$CANTON_AUTH_CLIENT_SECRET" ]; then
    print_error "Missing OAuth credentials in $SECRETS_FILE"
    exit 1
fi

# Use secrets for auth
AUTH_AUDIENCE="${CANTON_AUTH_AUDIENCE:-https://canton-ledger-api-dev1.01.chainsafe.dev}"
AUTH_TOKEN_URL="${CANTON_AUTH_TOKEN_URL:-https://dev-2j3m40ajwym1zzaq.eu.auth0.com/oauth/token}"

# PID files
PID_DIR="$PROJECT_ROOT/.pids"
RELAYER_PID_FILE="$PID_DIR/relayer.pid"
API_SERVER_PID_FILE="$PID_DIR/api-server.pid"

# =============================================================================
# Get OAuth2 Token
# =============================================================================
get_oauth_token() {
    curl -s -X POST "$AUTH_TOKEN_URL" \
        -H "Content-Type: application/json" \
        -d "{
            \"client_id\": \"$CANTON_AUTH_CLIENT_ID\",
            \"client_secret\": \"$CANTON_AUTH_CLIENT_SECRET\",
            \"audience\": \"$AUTH_AUDIENCE\",
            \"grant_type\": \"client_credentials\"
        }" | jq -r '.access_token'
}

# =============================================================================
# Stop Services
# =============================================================================
stop_services() {
    print_header "Stopping Services"
    
    # Kill relayer
    if [ -f "$RELAYER_PID_FILE" ]; then
        local pid=$(cat "$RELAYER_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            print_info "Stopping relayer (PID: $pid)..."
            kill "$pid" 2>/dev/null || true
            sleep 1
            kill -9 "$pid" 2>/dev/null || true
        fi
        rm -f "$RELAYER_PID_FILE"
    fi
    
    # Kill api-server
    if [ -f "$API_SERVER_PID_FILE" ]; then
        local pid=$(cat "$API_SERVER_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            print_info "Stopping API server (PID: $pid)..."
            kill "$pid" 2>/dev/null || true
            sleep 1
            kill -9 "$pid" 2>/dev/null || true
        fi
        rm -f "$API_SERVER_PID_FILE"
    fi
    
    # Kill any remaining processes on ports
    lsof -ti :8080 | xargs kill -9 2>/dev/null || true
    lsof -ti :8081 | xargs kill -9 2>/dev/null || true
    
    print_success "Services stopped"
}

# =============================================================================
# Check Prerequisites
# =============================================================================
check_prerequisites() {
    print_header "Checking Prerequisites"
    
    local missing=()
    
    command -v docker &> /dev/null && print_success "Docker: $(docker --version | head -1)" || missing+=("Docker")
    command -v go &> /dev/null && print_success "Go: $(go version | awk '{print $3}')" || missing+=("Go")
    command -v grpcurl &> /dev/null && print_success "grpcurl: installed" || print_warning "grpcurl not found (optional)"
    command -v jq &> /dev/null && print_success "jq: $(jq --version)" || missing+=("jq")
    
    if [ ${#missing[@]} -gt 0 ]; then
        print_error "Missing required tools: ${missing[*]}"
        exit 1
    fi
    
    [ -f "$RELAYER_CONFIG" ] && print_success "Relayer config: $RELAYER_CONFIG" || { print_error "Missing $RELAYER_CONFIG"; exit 1; }
    [ -f "$API_SERVER_CONFIG" ] && print_success "API Server config: $API_SERVER_CONFIG" || { print_error "Missing $API_SERVER_CONFIG"; exit 1; }
    
    print_success "All prerequisites met"
}

# =============================================================================
# Check DevNet Connectivity
# =============================================================================
check_devnet_connectivity() {
    print_header "Checking DevNet Connectivity"
    
    print_info "Getting OAuth2 token..."
    local token=$(get_oauth_token)
    
    if [ -z "$token" ] || [ "$token" = "null" ]; then
        print_error "Failed to get OAuth2 token"
        exit 1
    fi
    print_success "OAuth2 token obtained"
    
    print_info "Testing Canton Ledger API..."
    local health=$(grpcurl -H "Authorization: Bearer $token" -plaintext "$CANTON_GRPC" grpc.health.v1.Health/Check 2>&1)
    
    if echo "$health" | grep -q "SERVING"; then
        print_success "DevNet Canton: $CANTON_GRPC"
    else
        print_error "Cannot connect to DevNet Canton: $health"
        exit 1
    fi
    
    # Check Sepolia
    local sepolia_rpc=$(grep "rpc_url:" "$RELAYER_CONFIG" | grep -v "#" | head -1 | awk '{print $2}' | tr -d '"')
    if [ -n "$sepolia_rpc" ] && [ "$sepolia_rpc" != '${SEPOLIA_RPC_URL}' ]; then
        print_info "Testing Sepolia..."
        local block=$(curl -s "$sepolia_rpc" -X POST -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | jq -r '.result' 2>/dev/null)
        if [ -n "$block" ] && [ "$block" != "null" ]; then
            print_success "Sepolia RPC: connected (block: $block)"
        else
            print_warning "Sepolia RPC not responding (relayer may fail)"
        fi
    else
        print_warning "Sepolia RPC URL not configured (set SEPOLIA_RPC_URL env var)"
    fi
}

# =============================================================================
# Start PostgreSQL
# =============================================================================
start_postgres() {
    print_header "Starting PostgreSQL"
    
    cd "$PROJECT_ROOT"
    
    if [ "$CLEAN" = true ]; then
        print_info "Cleaning existing database..."
        docker compose down -v postgres 2>/dev/null || true
    fi
    
    print_info "Starting PostgreSQL container..."
    docker compose up -d postgres
    
    # Wait for it
    local max_wait=30
    local waited=0
    while ! docker exec postgres pg_isready -U postgres > /dev/null 2>&1; do
        sleep 1
        waited=$((waited + 1))
        if [ $waited -gt $max_wait ]; then
            print_error "Timeout waiting for PostgreSQL"
            exit 1
        fi
    done
    
    print_success "PostgreSQL ready (localhost:5432)"
    
    # Create API server database if not exists
    print_info "Ensuring erc20_api database exists..."
    docker exec postgres psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'erc20_api'" | grep -q 1 || \
        docker exec postgres psql -U postgres -c "CREATE DATABASE erc20_api"
    
    # Apply schema
    if [ -f "$PROJECT_ROOT/pkg/db/schema.sql" ]; then
        docker exec -i postgres psql -U postgres -d erc20_api < "$PROJECT_ROOT/pkg/db/schema.sql" 2>/dev/null || true
    fi
    
    print_success "Database ready"
}

# =============================================================================
# Start API Server
# =============================================================================
start_api_server() {
    print_header "Starting API Server"
    
    mkdir -p "$PID_DIR"
    cd "$PROJECT_ROOT"
    
    print_info "Building API server..."
    go build -o "$PROJECT_ROOT/bin/api-server" ./cmd/api-server
    
    print_info "Starting API server (DevNet: $CANTON_GRPC)..."
    
    # Start in background
    nohup "$PROJECT_ROOT/bin/api-server" -config "$API_SERVER_CONFIG" > "$PROJECT_ROOT/logs/api-server.log" 2>&1 &
    local pid=$!
    echo $pid > "$API_SERVER_PID_FILE"
    
    # Wait for health check
    local max_wait=30
    local waited=0
    while ! curl -s http://localhost:8081/health > /dev/null 2>&1; do
        sleep 1
        waited=$((waited + 1))
        if [ $waited -gt $max_wait ]; then
            print_error "API server failed to start. Check logs/api-server.log"
            cat "$PROJECT_ROOT/logs/api-server.log" | tail -20
            exit 1
        fi
    done
    
    print_success "API Server running (PID: $pid, http://localhost:8081)"
}

# =============================================================================
# Start Relayer (optional - only if Sepolia configured)
# =============================================================================
start_relayer() {
    print_header "Starting Relayer"
    
    local sepolia_rpc=$(grep "rpc_url:" "$RELAYER_CONFIG" | grep -v "#" | head -1 | awk '{print $2}' | tr -d '"')
    if [ -z "$sepolia_rpc" ] || [ "$sepolia_rpc" = '${SEPOLIA_RPC_URL}' ]; then
        print_warning "Skipping relayer - SEPOLIA_RPC_URL not set"
        print_info "To enable relayer: export SEPOLIA_RPC_URL=https://sepolia.infura.io/v3/YOUR_KEY"
        return 0
    fi
    
    mkdir -p "$PID_DIR"
    cd "$PROJECT_ROOT"
    
    print_info "Building relayer..."
    go build -o "$PROJECT_ROOT/bin/relayer" ./cmd/relayer
    
    print_info "Starting relayer..."
    nohup "$PROJECT_ROOT/bin/relayer" -config "$RELAYER_CONFIG" > "$PROJECT_ROOT/logs/relayer.log" 2>&1 &
    local pid=$!
    echo $pid > "$RELAYER_PID_FILE"
    
    # Wait for health check
    local max_wait=30
    local waited=0
    while ! curl -s http://localhost:8080/health > /dev/null 2>&1; do
        sleep 1
        waited=$((waited + 1))
        if [ $waited -gt $max_wait ]; then
            print_warning "Relayer slow to start. Check logs/relayer.log"
            break
        fi
    done
    
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        print_success "Relayer running (PID: $pid, http://localhost:8080)"
    fi
}

# =============================================================================
# Compute fingerprint from EVM address (keccak256 of 20-byte address)
# =============================================================================
compute_fingerprint() {
    local evm_addr="$1"
    # Use Go to compute the correct keccak256 hash of the address bytes
    go run -exec '' - "$evm_addr" 2>/dev/null << 'GOCODE'
package main
import (
    "fmt"
    "os"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/crypto"
)
func main() {
    addr := common.HexToAddress(os.Args[1])
    fmt.Print(crypto.Keccak256Hash(addr.Bytes()).Hex()[2:])
}
GOCODE
}

# =============================================================================
# Register Users and Setup Tokens
# =============================================================================
setup_users_and_tokens() {
    print_header "Setting Up Users and Tokens"
    
    cd "$PROJECT_ROOT"
    
    # Get issuer party from config
    local issuer_party=$(grep "relayer_party:" "$RELAYER_CONFIG" | awk '{print $2}' | tr -d '"')
    local issuer_fingerprint=$(echo "$issuer_party" | sed 's/.*::1220//')
    
    # Compute unique fingerprints for each user from their EVM addresses
    print_info "Computing user fingerprints from EVM addresses..."
    USER1_FINGERPRINT=$(compute_fingerprint "$USER1_ADDRESS")
    USER2_FINGERPRINT=$(compute_fingerprint "$USER2_ADDRESS")
    
    print_info "User 1 fingerprint: ${USER1_FINGERPRINT:0:16}..."
    print_info "User 2 fingerprint: ${USER2_FINGERPRINT:0:16}..."
    
    # Register User 1 with their unique fingerprint
    print_info "Registering User 1: $USER1_ADDRESS"
    go run scripts/testing/register-user.go -config config.devnet.yaml \
        -fingerprint "$USER1_FINGERPRINT" \
        -evm-address "$USER1_ADDRESS" 2>&1 | tail -5
    
    # Register User 2 with their unique fingerprint
    print_info "Registering User 2: $USER2_ADDRESS"
    go run scripts/testing/register-user.go -config config.devnet.yaml \
        -fingerprint "$USER2_FINGERPRINT" \
        -evm-address "$USER2_ADDRESS" 2>&1 | tail -5
    
    # Check if TokenConfig exists (DEMO token - now unified in CIP56.Config)
    print_info "Checking for existing DEMO token..."
    local token=$(get_oauth_token)
    local offset=$(grpcurl -H "Authorization: Bearer $token" -plaintext "$CANTON_GRPC" \
        com.daml.ledger.api.v2.StateService/GetLedgerEnd 2>/dev/null | jq -r '.offset // "0"')
    
    local cip56_pkg=$(grep "cip56_package_id:" config.devnet.yaml | awk '{print $2}' | tr -d '"')
    local demo_exists=$(grpcurl -H "Authorization: Bearer $token" -plaintext "$CANTON_GRPC" \
        -d "{
            \"active_at_offset\": $offset,
            \"event_format\": {
                \"filters_by_party\": {
                    \"$issuer_party\": {
                        \"cumulative\": [{
                            \"template_filter\": {
                                \"template_id\": {
                                    \"package_id\": \"$cip56_pkg\",
                                    \"module_name\": \"CIP56.Config\",
                                    \"entity_name\": \"TokenConfig\"
                                }
                            }
                        }]
                    }
                }
            }
        }" com.daml.ledger.api.v2.StateService/GetActiveContracts 2>&1)
    
    if echo "$demo_exists" | grep -q "contractId"; then
        print_success "DEMO token already exists on DevNet"
    else
        print_info "Bootstrapping DEMO token with unique user fingerprints..."
        go run scripts/setup/bootstrap-demo.go -config config.devnet.yaml \
            -user1-fingerprint "$USER1_FINGERPRINT" \
            -user2-fingerprint "$USER2_FINGERPRINT" 2>&1 | tail -10
    fi
    
    # Sync database with Canton holdings using correct fingerprints
    print_info "Syncing database with Canton..."
    sync_database_with_canton
    
    print_success "Users and tokens setup complete"
}

# =============================================================================
# Sync Database with Canton Holdings
# =============================================================================
sync_database_with_canton() {
    local token=$(get_oauth_token)
    local issuer_party=$(grep "relayer_party:" "$RELAYER_CONFIG" | awk '{print $2}' | tr -d '"')
    
    # Use the computed fingerprints from setup_users_and_tokens (if set)
    # Otherwise compute them fresh
    if [ -z "$USER1_FINGERPRINT" ]; then
        USER1_FINGERPRINT=$(compute_fingerprint "$USER1_ADDRESS")
    fi
    if [ -z "$USER2_FINGERPRINT" ]; then
        USER2_FINGERPRINT=$(compute_fingerprint "$USER2_ADDRESS")
    fi
    
    # Get ledger offset
    local offset=$(grpcurl -H "Authorization: Bearer $token" -plaintext "$CANTON_GRPC" \
        com.daml.ledger.api.v2.StateService/GetLedgerEnd 2>/dev/null | jq -r '.offset // "0"')
    
    # Query CIP56Holdings (DEMO token)
    local cip56_pkg=$(grep "cip56_package_id:" config.devnet.yaml | awk '{print $2}' | tr -d '"')
    
    print_info "Querying DEMO holdings from Canton..."
    local holdings=$(grpcurl -H "Authorization: Bearer $token" -plaintext "$CANTON_GRPC" \
        -d "{
            \"active_at_offset\": $offset,
            \"event_format\": {
                \"filters_by_party\": {
                    \"$issuer_party\": {
                        \"cumulative\": [{
                            \"template_filter\": {
                                \"template_id\": {
                                    \"package_id\": \"$cip56_pkg\",
                                    \"module_name\": \"CIP56.Token\",
                                    \"entity_name\": \"CIP56Holding\"
                                }
                            }
                        }]
                    }
                }
            }
        }" com.daml.ledger.api.v2.StateService/GetActiveContracts 2>&1)
    
    # Count holdings
    local holding_count=$(echo "$holdings" | grep -c "contractId" || echo "0")
    print_info "Found $holding_count DEMO holdings on Canton"
    
    # Insert/update users in database with their UNIQUE fingerprints
    print_info "Updating database with unique user fingerprints..."
    
    # User 1 - with unique fingerprint derived from EVM address
    docker exec postgres psql -U postgres -d erc20_api -c "
        INSERT INTO users (evm_address, canton_party, fingerprint, balance, demo_balance)
        VALUES ('$USER1_ADDRESS', '$issuer_party', '$USER1_FINGERPRINT', 0, 500)
        ON CONFLICT (evm_address) DO UPDATE SET 
            canton_party = EXCLUDED.canton_party,
            fingerprint = EXCLUDED.fingerprint,
            demo_balance = EXCLUDED.demo_balance;
    " 2>/dev/null
    
    # User 2 - with unique fingerprint derived from EVM address
    docker exec postgres psql -U postgres -d erc20_api -c "
        INSERT INTO users (evm_address, canton_party, fingerprint, balance, demo_balance)
        VALUES ('$USER2_ADDRESS', '$issuer_party', '$USER2_FINGERPRINT', 0, 500)
        ON CONFLICT (evm_address) DO UPDATE SET 
            canton_party = EXCLUDED.canton_party,
            fingerprint = EXCLUDED.fingerprint,
            demo_balance = EXCLUDED.demo_balance;
    " 2>/dev/null
    
    # Whitelist both users for transactions
    print_info "Adding users to whitelist..."
    docker exec postgres psql -U postgres -d erc20_api -c "
        INSERT INTO whitelist (evm_address, note)
        VALUES 
            ('$USER1_ADDRESS', 'Test user 1'),
            ('$USER2_ADDRESS', 'Test user 2')
        ON CONFLICT (evm_address) DO NOTHING;
    " 2>/dev/null
    
    print_success "Database synced and users whitelisted"
    
    # Show current state
    print_info "Current user balances:"
    docker exec postgres psql -U postgres -d erc20_api -c \
        "SELECT evm_address, balance as prompt, demo_balance as demo FROM users;" 2>/dev/null
}

# =============================================================================
# Print Status
# =============================================================================
print_status() {
    print_header "Service Status"
    
    echo ""
    echo "  Local Services:"
    
    # PostgreSQL
    if docker exec postgres pg_isready -U postgres > /dev/null 2>&1; then
        echo -e "    PostgreSQL:     ${GREEN}running${NC} (localhost:5432)"
    else
        echo -e "    PostgreSQL:     ${RED}not running${NC}"
    fi
    
    # API Server
    if curl -s http://localhost:8081/health > /dev/null 2>&1; then
        local api_pid=$(cat "$API_SERVER_PID_FILE" 2>/dev/null || echo "?")
        echo -e "    API Server:     ${GREEN}running${NC} (PID: $api_pid, http://localhost:8081)"
    else
        echo -e "    API Server:     ${RED}not running${NC}"
    fi
    
    # Relayer
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        local rel_pid=$(cat "$RELAYER_PID_FILE" 2>/dev/null || echo "?")
        echo -e "    Relayer:        ${GREEN}running${NC} (PID: $rel_pid, http://localhost:8080)"
    else
        echo -e "    Relayer:        ${YELLOW}not running${NC}"
    fi
    
    echo ""
    echo "  Remote Services:"
    
    # DevNet Canton
    local token=$(get_oauth_token 2>/dev/null)
    if [ -n "$token" ] && [ "$token" != "null" ]; then
        local health=$(grpcurl -H "Authorization: Bearer $token" -plaintext "$CANTON_GRPC" grpc.health.v1.Health/Check 2>&1)
        if echo "$health" | grep -q "SERVING"; then
            echo -e "    DevNet Canton:  ${GREEN}connected${NC} ($CANTON_GRPC)"
        else
            echo -e "    DevNet Canton:  ${RED}not connected${NC}"
        fi
    else
        echo -e "    DevNet Canton:  ${RED}auth failed${NC}"
    fi
    
    echo ""
    echo "  Database Users:"
    docker exec postgres psql -U postgres -d erc20_api -c \
        "SELECT evm_address, demo_balance FROM users;" 2>/dev/null || echo "    (database not available)"
    
    echo ""
    echo "  MetaMask Config:"
    echo "    Network Name:  Canton DevNet (Local)"
    echo "    RPC URL:       http://localhost:8081/eth"
    echo "    Chain ID:      1155111101"
    echo "    DEMO Token:    0xDE30000000000000000000000000000000000001"
    echo ""
}

# =============================================================================
# Print Summary
# =============================================================================
print_summary() {
    print_header "Setup Complete!"
    
    echo ""
    echo "  Services running:"
    echo "    - PostgreSQL:    localhost:5432"
    echo "    - API Server:    http://localhost:8081"
    if [ -f "$RELAYER_PID_FILE" ]; then
        echo "    - Relayer:       http://localhost:8080"
    fi
    echo ""
    echo "  Connected to:"
    echo "    - DevNet Canton: $CANTON_GRPC"
    echo ""
    echo "  MetaMask Setup:"
    echo "    Network Name:    Canton DevNet (Local)"
    echo "    RPC URL:         http://localhost:8081/eth"
    echo "    Chain ID:        1155111101"
    echo ""
    echo "  Tokens:"
    echo "    DEMO Token:      0xDE30000000000000000000000000000000000001"
    echo "    PROMPT Token:    0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e"
    echo ""
    echo "  Test Users (500 DEMO each):"
    echo "    User 1: $USER1_ADDRESS"
    echo "    User 2: $USER2_ADDRESS"
    echo ""
    echo "  Private Keys (for MetaMask import):"
    echo "    User 1: $USER1_PRIVATE_KEY"
    echo "    User 2: $USER2_PRIVATE_KEY"
    echo ""
    echo "  Commands:"
    echo "    Check status:    ./scripts/setup/setup-devnet.sh --status"
    echo "    Stop services:   ./scripts/setup/setup-devnet.sh --stop"
    echo "    View API logs:   tail -f logs/api-server.log"
    echo "    MetaMask info:   ./scripts/utils/metamask-info-devnet.sh"
    echo ""
}

# =============================================================================
# Main
# =============================================================================
main() {
    print_header "Canton-Ethereum Bridge - DevNet Setup"
    
    # Create logs directory
    mkdir -p "$PROJECT_ROOT/logs"
    
    # Stop only mode
    if [ "$STOP_ONLY" = true ]; then
        stop_services
        docker compose stop postgres 2>/dev/null || true
        exit 0
    fi
    
    # Status only mode
    if [ "$STATUS_ONLY" = true ]; then
        print_status
        exit 0
    fi
    
    # Stop any existing services
    stop_services
    
    check_prerequisites
    check_devnet_connectivity
    start_postgres
    start_api_server
    start_relayer
    
    if [ "$SETUP_ONLY" = false ]; then
        setup_users_and_tokens
    fi
    
    print_summary
}

main
