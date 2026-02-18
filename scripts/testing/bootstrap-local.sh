#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge - Local Bootstrap (One-Command Setup)
# =============================================================================
# This script sets up the complete local environment for interop testing.
# After this completes, run interop-demo.go for the full test.
#
# What it does:
#   1. Starts Docker services (Canton, Anvil, PostgreSQL, API Server, Relayer)
#   2. Waits for all services to be healthy
#   3. Extracts config from bootstrap container (domain ID, relayer party, etc.)
#   4. Whitelists test users in the database
#   5. Registers test users via the API server (EIP-191 signatures)
#   6. Bootstraps DEMO tokens (500 DEMO per user)
#
# Usage:
#   ./scripts/testing/bootstrap-local.sh              # Full setup
#   ./scripts/testing/bootstrap-local.sh --clean       # Clean slate (remove volumes)
#   ./scripts/testing/bootstrap-local.sh --skip-docker # Assume Docker is running
#
# Prerequisites:
#   - Docker and Docker Compose
#   - Go 1.23+
#   - Foundry/Cast (for EIP-191 signing)
#
# After bootstrap:
#   GOMODCACHE="$HOME/go/pkg/mod" go run scripts/testing/interop-demo.go
# =============================================================================

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/../.." && pwd )"

# Ensure Go module cache is accessible (override sandbox temp paths)
export GOMODCACHE="$HOME/go/pkg/mod"

# Source shared libs
source "$SCRIPT_DIR/../lib/common.sh"

# ─── Parse flags ──────────────────────────────────────────────────────────────
CLEAN=false
SKIP_DOCKER=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --clean)      CLEAN=true; shift ;;
        --skip-docker) SKIP_DOCKER=true; shift ;;
        --verbose)    VERBOSE=true; shift ;;
        -h|--help)
            echo "Usage: $0 [--clean] [--skip-docker] [--verbose]"
            exit 0 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# ─── Constants ────────────────────────────────────────────────────────────────
USER1_KEY="ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
USER1_ADDR="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
USER2_KEY="59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
USER2_ADDR="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

API_URL="http://localhost:8081"
ANVIL_URL="http://localhost:8545"
RELAYER_URL="http://localhost:8080"
DB_USER="postgres"
DB_NAME="erc20_api"

# ─── Step 1: Docker ──────────────────────────────────────────────────────────
start_docker() {
    print_header "Step 1: Start Docker Services"

    cd "$PROJECT_ROOT"

    if [ "$CLEAN" = true ]; then
        print_step "Cleaning previous state..."
        docker compose down -v --remove-orphans 2>/dev/null || true
        print_success "Cleaned"
    fi

    export CANTON_MASTER_KEY=$(openssl rand -base64 32)
    export SKIP_CANTON_SIG_VERIFY=true

    print_step "Starting services (docker compose up --build)..."
    if [ "$VERBOSE" = true ]; then
        docker compose up -d --build
    else
        docker compose up -d --build 2>&1 | tail -5
    fi
    print_success "Docker services started"
}

# ─── Step 2: Wait for health ─────────────────────────────────────────────────
wait_for_services() {
    print_header "Step 2: Wait for Services"

    local max_wait=180
    local waited=0

    # Anvil
    print_step "Waiting for Anvil..."
    while ! cast block-number --rpc-url "$ANVIL_URL" > /dev/null 2>&1; do
        sleep 2; waited=$((waited+2))
        [ $waited -gt $max_wait ] && { print_error "Anvil timeout"; exit 1; }
    done
    print_success "Anvil ready"

    # Canton
    print_step "Waiting for Canton..."
    while ! docker ps --format "{{.Names}} {{.Status}}" | grep -q "canton.*healthy"; do
        sleep 3; waited=$((waited+3))
        [ $waited -gt $max_wait ] && { print_error "Canton timeout"; exit 1; }
    done
    print_success "Canton ready"

    # Bootstrap container must finish
    print_step "Waiting for bootstrap to complete..."
    while docker ps --format "{{.Names}}" | grep -q "^bootstrap$"; do
        sleep 3; waited=$((waited+3))
        [ $waited -gt $max_wait ] && { print_error "Bootstrap timeout"; exit 1; }
    done
    # Check exit code
    local bs_exit=$(docker inspect bootstrap --format='{{.State.ExitCode}}')
    if [ "$bs_exit" != "0" ]; then
        print_error "Bootstrap container failed (exit $bs_exit). Check: docker logs bootstrap"
        exit 1
    fi
    print_success "Bootstrap completed"

    # API Server
    print_step "Waiting for API Server..."
    while ! curl -s "$API_URL/health" > /dev/null 2>&1; do
        sleep 2; waited=$((waited+2))
        [ $waited -gt $max_wait ] && { print_error "API Server timeout"; exit 1; }
    done
    print_success "API Server ready"

    # Relayer
    print_step "Waiting for Relayer..."
    while ! curl -s "$RELAYER_URL/health" > /dev/null 2>&1; do
        sleep 2; waited=$((waited+2))
        [ $waited -gt $max_wait ] && { print_error "Relayer timeout"; exit 1; }
    done
    print_success "Relayer ready"

    print_success "All services healthy!"
}

# ─── Step 3: Extract config ──────────────────────────────────────────────────
extract_config() {
    print_header "Step 3: Extract Config from Bootstrap"

    RELAYER_PARTY=$(docker logs bootstrap 2>&1 | grep -o 'relayer_party: "[^"]*"' | head -1 | sed 's/relayer_party: "//;s/"$//')
    DOMAIN_ID=$(docker logs bootstrap 2>&1 | grep -o 'domain_id: "[^"]*"' | head -1 | sed 's/domain_id: "//;s/"$//')
    BRIDGE_PKG=$(docker logs bootstrap 2>&1 | grep -o 'bridge_package_id: "[^"]*"' | head -1 | sed 's/bridge_package_id: "//;s/"$//')

    if [ -z "$RELAYER_PARTY" ] || [ -z "$DOMAIN_ID" ]; then
        print_error "Could not extract relayer_party or domain_id from bootstrap logs"
        echo "Run: docker logs bootstrap"
        exit 1
    fi

    print_success "Relayer Party: ${RELAYER_PARTY:0:40}..."
    print_success "Domain ID:     ${DOMAIN_ID:0:40}..."
    print_success "Bridge Pkg:    ${BRIDGE_PKG:0:16}..."

    # Get contract addresses from deployer (last deployment)
    TOKEN_ADDR=$(docker logs deployer 2>&1 | grep "PromptToken deployed to:" | tail -1 | grep -oE '0x[a-fA-F0-9]{40}')
    BRIDGE_ADDR=$(docker logs deployer 2>&1 | grep "CantonBridge deployed to:" | tail -1 | grep -oE '0x[a-fA-F0-9]{40}')
    TOKEN_ADDR="${TOKEN_ADDR:-0x5FbDB2315678afecb367f032d93F642f64180aa3}"
    BRIDGE_ADDR="${BRIDGE_ADDR:-0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512}"
    print_success "Token:         $TOKEN_ADDR"
    print_success "Bridge:        $BRIDGE_ADDR"

    # Auto-update config.e2e-local.yaml with current Canton/Ethereum values
    local cfg="$PROJECT_ROOT/config.e2e-local.yaml"
    if [ -f "$cfg" ]; then
        print_step "Updating $cfg with current values..."
        sed -i.bak "s|domain_id:.*#|domain_id: \"$DOMAIN_ID\"  #|" "$cfg"
        sed -i.bak "s|relayer_party:.*#|relayer_party: \"$RELAYER_PARTY\"  #|" "$cfg"
        # Update contract addresses in ethereum section
        sed -i.bak "s|bridge_contract:.*|bridge_contract: \"$BRIDGE_ADDR\"|" "$cfg"
        sed -i.bak "s|token_contract:.*|token_contract: \"$TOKEN_ADDR\"|" "$cfg"
        # Update contract addresses in contracts section
        sed -i.bak "s|token_address:.*|token_address: \"$TOKEN_ADDR\"|" "$cfg"
        sed -i.bak "s|bridge_address:.*|bridge_address: \"$BRIDGE_ADDR\"|" "$cfg"
        rm -f "${cfg}.bak"
        print_success "Config updated"
    fi
}

# ─── Helper: run SQL via Docker postgres container ───────────────────────────
run_sql() {
    docker exec postgres psql -U "$DB_USER" -d "$DB_NAME" -q "$@"
}

run_sql_raw() {
    docker exec postgres psql -U "$DB_USER" -d "$DB_NAME" -t "$@"
}

# ─── Step 4: Whitelist ───────────────────────────────────────────────────────
whitelist_users() {
    print_header "Step 4: Whitelist Test Users"

    run_sql -c "INSERT INTO whitelist (evm_address, note) VALUES
        ('$USER1_ADDR', 'Test User 1'),
        ('$USER2_ADDR', 'Test User 2')
        ON CONFLICT DO NOTHING;"

    if [ $? -ne 0 ]; then
        print_error "Failed to whitelist users. Is the postgres container running?"
        exit 1
    fi

    print_success "Whitelisted $USER1_ADDR"
    print_success "Whitelisted $USER2_ADDR"
}

# ─── Step 5: Register users ──────────────────────────────────────────────────
register_users() {
    print_header "Step 5: Register Users via API"

    local message="Register for Canton Bridge"

    # User 1
    print_step "Registering User 1..."
    local sig1=$(cast wallet sign --private-key "$USER1_KEY" "$message")
    if [ -z "$sig1" ]; then
        print_error "Failed to sign message for User 1. Is 'cast' (Foundry) installed?"
        exit 1
    fi
    local resp1=$(curl -s -X POST "$API_URL/register" \
        -H "Content-Type: application/json" \
        -d "{\"signature\":\"$sig1\",\"message\":\"$message\"}")

    USER1_FINGERPRINT=$(echo "$resp1" | jq -r '.fingerprint // empty')
    USER1_PARTY=$(echo "$resp1" | jq -r '.party // empty')
    if [ -z "$USER1_FINGERPRINT" ]; then
        if echo "$resp1" | grep -q "already registered"; then
            print_warning "User 1 already registered, fetching from database"
            USER1_FINGERPRINT=$(run_sql_raw -c "SELECT fingerprint FROM users WHERE evm_address = '$USER1_ADDR';" | tr -d '[:space:]')
        fi
        if [ -z "$USER1_FINGERPRINT" ]; then
            print_error "User 1 registration failed: $resp1"
            exit 1
        fi
    fi
    print_success "User 1: ${USER1_FINGERPRINT:0:20}..."

    # User 2
    print_step "Registering User 2..."
    local sig2=$(cast wallet sign --private-key "$USER2_KEY" "$message")
    if [ -z "$sig2" ]; then
        print_error "Failed to sign message for User 2. Is 'cast' (Foundry) installed?"
        exit 1
    fi
    local resp2=$(curl -s -X POST "$API_URL/register" \
        -H "Content-Type: application/json" \
        -d "{\"signature\":\"$sig2\",\"message\":\"$message\"}")

    USER2_FINGERPRINT=$(echo "$resp2" | jq -r '.fingerprint // empty')
    USER2_PARTY=$(echo "$resp2" | jq -r '.party // empty')
    if [ -z "$USER2_FINGERPRINT" ]; then
        if echo "$resp2" | grep -q "already registered"; then
            print_warning "User 2 already registered, fetching from database"
            USER2_FINGERPRINT=$(run_sql_raw -c "SELECT fingerprint FROM users WHERE evm_address = '$USER2_ADDR';" | tr -d '[:space:]')
        fi
        if [ -z "$USER2_FINGERPRINT" ]; then
            print_error "User 2 registration failed: $resp2"
            exit 1
        fi
    fi
    print_success "User 2: ${USER2_FINGERPRINT:0:20}..."
}

# ─── Step 6: Bootstrap DEMO tokens ──────────────────────────────────────────
bootstrap_demo_tokens() {
    print_header "Step 6: Bootstrap DEMO Tokens"

    # CIP56 package ID from config.e2e-local.yaml
    local cip56_pkg=$(grep "cip56_package_id:" "$PROJECT_ROOT/config.e2e-local.yaml" || true)
    cip56_pkg=$(echo "$cip56_pkg" | awk '{print $2}' | tr -d '"')

    if [ -z "$cip56_pkg" ]; then
        print_warning "cip56_package_id not in config, skipping DEMO bootstrap"
        return 0
    fi

    print_step "Minting 500 DEMO to each user..."

    cd "$PROJECT_ROOT"
    # Ensure Go modules are downloaded
    if ! go mod download; then
        print_warning "go mod download had errors (may be okay if modules are cached)"
    fi
    go run scripts/setup/bootstrap-demo.go \
        -config config.e2e-local.yaml \
        -cip56-package-id "$cip56_pkg" \
        -issuer "$RELAYER_PARTY" \
        -domain "$DOMAIN_ID" \
        -user1-fingerprint "$USER1_FINGERPRINT" \
        -user2-fingerprint "$USER2_FINGERPRINT" \
        -mint-amount "500.0"

    if [ $? -eq 0 ]; then
        print_success "500 DEMO minted to each user"
    else
        print_error "DEMO token bootstrap failed"
        exit 1
    fi
}

# ─── Summary ─────────────────────────────────────────────────────────────────
print_summary() {
    print_header "Bootstrap Complete!"

    echo ""
    echo "  Services:"
    echo "    Anvil (Ethereum):   $ANVIL_URL"
    echo "    Canton Ledger API:  localhost:5011 (gRPC)"
    echo "    API Server:         $API_URL"
    echo "    Relayer:            $RELAYER_URL"
    echo "    PostgreSQL:         postgres (Docker container)"
    echo ""
    echo "  Users:"
    echo "    User 1: $USER1_ADDR (500 DEMO)"
    echo "    User 2: $USER2_ADDR (500 DEMO)"
    echo ""
    echo "  Token addresses:"
    echo "    PROMPT (ERC-20):  0x5FbDB2315678afecb367f032d93F642f64180aa3"
    echo "    Bridge:           0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
    echo "    DEMO (native):    0xDE30000000000000000000000000000000000001"
    echo ""
    echo "  Next: run the interop test"
    echo "    go run scripts/testing/interop-demo.go"
    echo ""
}

# ─── Main ────────────────────────────────────────────────────────────────────
main() {
    print_header "Canton-Ethereum Bridge - Local Bootstrap"

    cd "$PROJECT_ROOT"

    if [ "$SKIP_DOCKER" = false ]; then
        start_docker
    else
        print_step "Skipping Docker start (--skip-docker)"
    fi

    wait_for_services
    extract_config
    whitelist_users
    register_users
    bootstrap_demo_tokens
    print_summary
}

main
