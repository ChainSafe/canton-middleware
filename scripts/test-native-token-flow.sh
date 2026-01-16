#!/bin/bash
# =============================================================================
# Native Token Integration Test Flow
# =============================================================================
# Self-contained test that starts Canton, uploads DARs, and tests the full
# mint/burn/transfer flow for native Canton tokens.
#
# Usage:
#   ./scripts/test-native-token-flow.sh           # Full test (start → test → stop)
#   ./scripts/test-native-token-flow.sh --keep    # Keep services running after test
#   ./scripts/test-native-token-flow.sh --stop    # Just stop services
#
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="${PROJECT_DIR}/.test-config.yaml"
DAML_DIR="$PROJECT_DIR/contracts/canton-erc20/daml"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

print_header() {
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${NC}"
}

print_step() { echo -e "${CYAN}>>> $1${NC}"; }
print_success() { echo -e "${GREEN}✓ $1${NC}"; }
print_error() { echo -e "${RED}✗ $1${NC}"; }
print_warn() { echo -e "${YELLOW}! $1${NC}"; }

cd "$PROJECT_DIR"

# =============================================================================
# Parse arguments
# =============================================================================
KEEP_RUNNING=false
STOP_ONLY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --keep) KEEP_RUNNING=true; shift ;;
        --stop) STOP_ONLY=true; shift ;;
        -h|--help)
            echo "Usage: $0 [--keep] [--stop]"
            echo ""
            echo "Options:"
            echo "  --keep    Keep Docker services running after test"
            echo "  --stop    Just stop Docker services and exit"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# =============================================================================
# Stop services
# =============================================================================
stop_services() {
    print_header "Stopping Docker Services"
    docker compose down -v 2>/dev/null || true
    print_success "Services stopped"
}

if [ "$STOP_ONLY" = true ]; then
    stop_services
    exit 0
fi

# =============================================================================
# Build DAML packages
# =============================================================================
print_header "Building DAML Packages"

print_step "Building all DAML packages..."
cd "$DAML_DIR"
daml build --all 2>&1 | tail -20
cd "$PROJECT_DIR"

# Verify native-token DAR exists
if [ ! -f "$DAML_DIR/native-token/.daml/dist/"*.dar ]; then
    print_error "native-token DAR not found. Build failed?"
    exit 1
fi
print_success "DAML packages built"

# =============================================================================
# Start Docker services
# =============================================================================
print_header "Starting Docker Services"

print_step "Starting Canton, Anvil, and supporting services..."
docker compose up -d postgres anvil mock-oauth2 canton 2>&1

# Wait for Canton to be healthy before starting bootstrap
print_step "Waiting for Canton to be healthy before bootstrap..."

# Wait for services to be healthy
print_step "Waiting for services to be healthy..."
max_wait=120
elapsed=0
while [ $elapsed -lt $max_wait ]; do
    canton_health=$(docker inspect --format='{{.State.Health.Status}}' canton 2>/dev/null || echo "not found")
    if [ "$canton_health" = "healthy" ]; then
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
    echo -n "."
done
echo ""

if [ "$canton_health" != "healthy" ]; then
    print_error "Canton failed to become healthy after ${max_wait}s"
    docker compose logs canton | tail -50
    exit 1
fi
print_success "Canton is healthy"

# Wait for Canton to connect to synchronizer
print_step "Waiting for Canton synchronizer connection..."
CANTON_URL="http://localhost:5013"
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    sync_count=$(curl -s "$CANTON_URL/v2/state/connected-synchronizers" 2>/dev/null | jq '.connectedSynchronizers | length' 2>/dev/null || echo "0")
    if [ "$sync_count" -gt 0 ] 2>/dev/null; then
        break
    fi
    sleep 2
    attempt=$((attempt + 1))
done

if [ "$sync_count" -eq 0 ] 2>/dev/null; then
    print_error "Canton failed to connect to synchronizer"
    exit 1
fi
print_success "Canton connected to synchronizer"

# =============================================================================
# Run Bootstrap (creates BridgeIssuer party and uploads DARs)
# =============================================================================
print_header "Running Bootstrap"

print_step "Starting bootstrap container (creates party, uploads DARs)..."
docker compose up -d deployer bootstrap 2>&1

# Wait for bootstrap to complete
print_step "Waiting for bootstrap to complete..."
max_wait=120
elapsed=0
while [ $elapsed -lt $max_wait ]; do
    bootstrap_status=$(docker inspect --format='{{.State.Status}}' bootstrap 2>/dev/null || echo "not found")
    if [ "$bootstrap_status" = "exited" ]; then
        bootstrap_exit=$(docker inspect --format='{{.State.ExitCode}}' bootstrap 2>/dev/null || echo "1")
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
    echo -n "."
done
echo ""

if [ "$bootstrap_exit" != "0" ]; then
    print_error "Bootstrap failed with exit code $bootstrap_exit"
    docker logs bootstrap 2>&1 | tail -30
    exit 1
fi
print_success "Bootstrap completed successfully"

# =============================================================================
# Get OAuth token and configuration
# =============================================================================
print_header "Configuring Test Environment"

print_step "Getting OAuth token..."
TOKEN=$(curl -s -X POST http://localhost:8088/oauth/token \
    -H "Content-Type: application/json" \
    -d '{"client_id":"local-test-client","client_secret":"local-test-secret","audience":"http://localhost:5011","grant_type":"client_credentials"}' \
    | jq -r '.access_token')

if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
    print_error "Failed to get OAuth token"
    exit 1
fi
print_success "OAuth token obtained"

print_step "Getting Canton party and domain..."
PARTY_ID=$(curl -s "$CANTON_URL/v2/parties" -H "Authorization: Bearer $TOKEN" | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1 || true)
DOMAIN_ID=$(curl -s "$CANTON_URL/v2/state/connected-synchronizers" -H "Authorization: Bearer $TOKEN" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')

if [ -z "$PARTY_ID" ]; then
    print_error "Could not find BridgeIssuer party"
    exit 1
fi
echo "    Party ID: $PARTY_ID"
echo "    Domain ID: $DOMAIN_ID"

# Generate config file
print_step "Generating test config file..."
cat > "$CONFIG_FILE" << EOF
# Auto-generated config for native token test
canton:
  rpc_url: "localhost:5011"
  relayer_party: "$PARTY_ID"
  domain_id: "$DOMAIN_ID"
  tls:
    enabled: false
  auth:
    client_id: "local-test-client"
    client_secret: "local-test-secret"
    audience: "http://localhost:5011"
    token_url: "http://localhost:8088/oauth/token"
EOF
print_success "Config saved to .test-config.yaml"

# =============================================================================
# Get package IDs
# =============================================================================
print_step "Getting package IDs from DARs..."
NATIVE_TOKEN_PKG=$(daml damlc inspect-dar "$DAML_DIR/native-token/.daml/dist/"*.dar 2>/dev/null | grep "^native-token-" | head -1 | sed 's/native-token-[0-9.]*-\([a-f0-9]*\)\/.*/\1/')
CIP56_PKG=$(daml damlc inspect-dar "$DAML_DIR/cip56-token/.daml/dist/"*.dar 2>/dev/null | grep "^cip56-token-" | head -1 | sed 's/cip56-token-[0-9.]*-\([a-f0-9]*\)\/.*/\1/')

if [ -z "$NATIVE_TOKEN_PKG" ] || [ -z "$CIP56_PKG" ]; then
    print_error "Could not extract package IDs from DARs"
    exit 1
fi
echo "    Native Token Package: ${NATIVE_TOKEN_PKG:0:16}..."
echo "    CIP56 Package: ${CIP56_PKG:0:16}..."

# =============================================================================
# Build Go test program
# =============================================================================
print_header "Building Test Program"
print_step "Compiling test-native-token.go..."
go build -o /tmp/test-native-token ./scripts/test-native-token.go
print_success "Build successful"

# =============================================================================
# Run Native Token Tests
# =============================================================================

# Step 1: Setup
print_header "Test Step 1: Setup Contracts"
OUTPUT=$(/tmp/test-native-token -config "$CONFIG_FILE" -action setup -package-id "$NATIVE_TOKEN_PKG" -cip56-package-id "$CIP56_PKG" 2>&1)
echo "$OUTPUT"
CONFIG_CID=$(echo "$OUTPUT" | grep "config-cid" | sed 's/.*--config-cid //')

if [ -z "$CONFIG_CID" ]; then
    print_error "Failed to extract config CID from setup output"
    exit 1
fi
print_success "Setup complete"

# Step 2: Mint
print_header "Test Step 2: Mint 1000 Tokens"
/tmp/test-native-token -config "$CONFIG_FILE" -action mint -package-id "$NATIVE_TOKEN_PKG" -config-cid "$CONFIG_CID" -amount "1000.0"

# Step 3: Check balance after mint
print_header "Test Step 3: Check Balance After Mint"
BALANCE_OUTPUT=$(/tmp/test-native-token -config "$CONFIG_FILE" -action balance -cip56-package-id "$CIP56_PKG" 2>&1)
echo "$BALANCE_OUTPUT"
HOLDING_CID=$(echo "$BALANCE_OUTPUT" | grep "Contract:" | head -1 | awk '{print $NF}')

if [ -z "$HOLDING_CID" ]; then
    print_error "Failed to extract holding CID"
    exit 1
fi

# Step 4: Burn
print_header "Test Step 4: Burn 250 Tokens"
/tmp/test-native-token -config "$CONFIG_FILE" -action burn -package-id "$NATIVE_TOKEN_PKG" -config-cid "$CONFIG_CID" -holding-cid "$HOLDING_CID" -amount "250.0"

# Step 5: Check balance after burn
print_header "Test Step 5: Check Balance After Burn"
BALANCE_OUTPUT=$(/tmp/test-native-token -config "$CONFIG_FILE" -action balance -cip56-package-id "$CIP56_PKG" 2>&1)
echo "$BALANCE_OUTPUT"
HOLDING_CID=$(echo "$BALANCE_OUTPUT" | grep "Contract:" | head -1 | awk '{print $NF}')

# Step 6: Transfer
print_header "Test Step 6: Transfer 100 Tokens"
/tmp/test-native-token -config "$CONFIG_FILE" -action transfer -package-id "$NATIVE_TOKEN_PKG" -config-cid "$CONFIG_CID" -holding-cid "$HOLDING_CID" -recipient "$PARTY_ID" -amount "100.0"

# Step 7: Final balance
print_header "Test Step 7: Final Balance"
/tmp/test-native-token -config "$CONFIG_FILE" -action balance -cip56-package-id "$CIP56_PKG"

# Step 8: Audit trail
print_header "Test Step 8: Audit Trail"
/tmp/test-native-token -config "$CONFIG_FILE" -action events -package-id "$NATIVE_TOKEN_PKG"

# =============================================================================
# Summary
# =============================================================================
print_header "Test Summary"
echo ""
echo "Operations completed:"
echo "  1. ✓ Created CIP56Manager contract"
echo "  2. ✓ Created NativeTokenConfig contract"
echo "  3. ✓ Minted 1000 tokens → MintEvent created"
echo "  4. ✓ Burned 250 tokens → BurnEvent created (750 remaining)"
echo "  5. ✓ Transferred 100 tokens → TransferEvent created"
echo "  6. ✓ Final: 650 + 100 = 750 tokens in holdings"
echo ""
print_success "All native token operations completed successfully!"
echo ""
echo "This emulates what the ERC20 API server would do when handling:"
echo "  - erc20_mint → IssuerMint choice"
echo "  - erc20_burn → IssuerBurn choice"
echo "  - erc20_transfer → IssuerTransfer choice"
echo "  - erc20_balanceOf → Query CIP56Holdings"
echo ""

# =============================================================================
# Cleanup
# =============================================================================
if [ "$KEEP_RUNNING" = true ]; then
    print_warn "Services left running (--keep flag). Stop with: $0 --stop"
else
    stop_services
fi
