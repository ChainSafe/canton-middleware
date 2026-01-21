#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge - Local Setup Script
# =============================================================================
# This script sets up everything needed for local development and testing.
# It handles:
# - Prerequisites checking
# - Git submodule initialization
# - DAML DAR building
# - Docker service startup
# - Automatic package ID detection from Canton
# - Running e2e tests
#
# Usage:
#   ./scripts/setup-local.sh              # Full setup + e2e test
#   ./scripts/setup-local.sh --setup-only # Setup without running tests
#   ./scripts/setup-local.sh --skip-build # Skip DAR building
#   ./scripts/setup-local.sh --clean      # Clean and rebuild everything
#
# Prerequisites:
#   - Docker and Docker Compose
#   - Go 1.23+ (for running tests)
#   - DAML SDK (optional - only if DARs need rebuilding)
#   - Foundry/Cast (for Ethereum interactions)
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
NC='\033[0m' # No Color

# Parse arguments
SETUP_ONLY=false
SKIP_BUILD=false
CLEAN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --setup-only)
            SETUP_ONLY=true
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --clean)
            CLEAN=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --setup-only    Setup environment without running tests"
            echo "  --skip-build    Skip DAR building (use existing DARs)"
            echo "  --clean         Clean everything and rebuild"
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
    echo -e "${CYAN}>>> $1${NC}"
}

# =============================================================================
# Prerequisites Check
# =============================================================================
check_prerequisites() {
    print_header "Checking Prerequisites"
    
    local missing=()
    
    # Docker
    if command -v docker &> /dev/null; then
        print_success "Docker: $(docker --version | head -1)"
    else
        missing+=("Docker")
    fi
    
    # Docker Compose
    if docker compose version &> /dev/null; then
        print_success "Docker Compose: $(docker compose version --short)"
    else
        missing+=("Docker Compose")
    fi
    
    # Go
    if command -v go &> /dev/null; then
        print_success "Go: $(go version | awk '{print $3}')"
    else
        missing+=("Go 1.23+")
    fi
    
    # Cast (Foundry)
    if command -v cast &> /dev/null; then
        print_success "Foundry/Cast: $(cast --version | head -1)"
    else
        print_warning "Foundry/Cast not found (optional - install with: curl -L https://foundry.paradigm.xyz | bash)"
    fi
    
    # DAML (optional)
    if command -v daml &> /dev/null; then
        print_success "DAML SDK: $(daml version | head -1)"
    else
        print_warning "DAML SDK not found (optional - only needed to rebuild DARs)"
    fi
    
    if [ ${#missing[@]} -gt 0 ]; then
        print_error "Missing required tools: ${missing[*]}"
        echo ""
        echo "Please install the missing prerequisites and try again."
        echo ""
        echo "Installation guides:"
        echo "  Docker:  https://docs.docker.com/get-docker/"
        echo "  Go:      https://golang.org/doc/install"
        echo "  DAML:    curl -sSL https://get.daml.com/ | sh"
        echo "  Foundry: curl -L https://foundry.paradigm.xyz | bash"
        exit 1
    fi
    
    print_success "All required prerequisites met"
}

# =============================================================================
# Git Submodules
# =============================================================================
init_submodules() {
    print_header "Initializing Git Submodules"
    
    cd "$PROJECT_ROOT"
    
    # Check if canton-erc20 submodule exists (the main one we need)
    if [ ! -d "contracts/canton-erc20/daml" ]; then
        print_info "Initializing submodules..."
        git submodule update --init --recursive 2>/dev/null || {
            print_warning "Some submodules failed to update - trying individual init..."
            # Initialize just canton-erc20 which is required
            git submodule update --init contracts/canton-erc20 || {
                print_error "Failed to initialize canton-erc20 submodule"
                exit 1
            }
        }
        print_success "Submodules initialized"
    else
        print_info "Submodules already present"
        # Try to update but don't fail if some submodules have issues
        git submodule update --recursive 2>/dev/null || {
            print_warning "Some submodules could not be updated (this is usually fine)"
        }
        print_success "Submodules checked"
    fi
    
    # Verify canton-erc20 is present
    if [ ! -d "contracts/canton-erc20/daml" ]; then
        print_error "canton-erc20 submodule not found"
        echo "Try: git submodule update --init contracts/canton-erc20"
        exit 1
    fi
}

# =============================================================================
# Build DARs
# =============================================================================
build_dars() {
    print_header "Building DAML DARs"
    
    local daml_dir="$PROJECT_ROOT/contracts/canton-erc20/daml"
    
    # Check if DARs already exist
    local dar_count=$(find "$daml_dir" -name "*.dar" 2>/dev/null | wc -l | tr -d ' ')
    
    if [ "$dar_count" -gt 0 ] && [ "$CLEAN" = false ]; then
        print_info "Found $dar_count existing DAR files"
        
        # Check if key DARs exist
        local required_dars=("cip56-token" "bridge-wayfinder" "bridge-core" "native-token" "common")
        local all_present=true
        
        for dar in "${required_dars[@]}"; do
            if ! find "$daml_dir/$dar" -name "*.dar" -print -quit 2>/dev/null | grep -q .; then
                print_warning "Missing DAR: $dar"
                all_present=false
            fi
        done
        
        if [ "$all_present" = true ]; then
            print_success "All required DARs present"
            return 0
        fi
    fi
    
    # Check if DAML SDK is available
    if ! command -v daml &> /dev/null; then
        print_error "DAML SDK not found - cannot build DARs"
        echo "Install with: curl -sSL https://get.daml.com/ | sh"
        echo ""
        echo "Alternatively, ensure pre-built DARs exist in:"
        echo "  $daml_dir/*/\\.daml/dist/*.dar"
        exit 1
    fi
    
    print_info "Building DARs (this may take a few minutes)..."
    
    cd "$daml_dir"
    
    # Use multi-package build if available
    if [ -f "multi-package.yaml" ]; then
        daml build --all
    else
        # Build each package individually
        for pkg in common cip56-token bridge-core bridge-wayfinder native-token; do
            if [ -d "$pkg" ]; then
                print_info "Building $pkg..."
                cd "$pkg"
                daml build
                cd "$daml_dir"
            fi
        done
    fi
    
    print_success "DARs built successfully"
}

# =============================================================================
# Docker Services
# =============================================================================
start_docker_services() {
    print_header "Starting Docker Services"
    
    cd "$PROJECT_ROOT"
    
    if [ "$CLEAN" = true ]; then
        print_info "Cleaning existing containers and volumes..."
        docker compose down -v --remove-orphans 2>/dev/null || true
        print_success "Cleaned"
    fi
    
    print_info "Starting services..."
    docker compose up -d --build
    
    print_success "Docker services starting"
}

# =============================================================================
# Wait for Services
# =============================================================================
wait_for_services() {
    print_header "Waiting for Services to be Ready"
    
    local max_wait=180
    local waited=0
    
    # Wait for Anvil
    print_info "Waiting for Anvil..."
    while ! curl -s http://localhost:8545 -X POST -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' > /dev/null 2>&1; do
        sleep 2
        waited=$((waited + 2))
        if [ $waited -gt $max_wait ]; then
            print_error "Timeout waiting for Anvil"
            exit 1
        fi
    done
    print_success "Anvil ready"
    
    # Wait for Canton
    print_info "Waiting for Canton..."
    while ! docker exec canton grpcurl --plaintext localhost:5011 \
        com.digitalasset.canton.health.admin.v0.StatusService.Status 2>/dev/null | \
        grep -q '"active": true'; do
        sleep 5
        waited=$((waited + 5))
        if [ $waited -gt $max_wait ]; then
            print_error "Timeout waiting for Canton"
            docker logs canton --tail 50
            exit 1
        fi
    done
    print_success "Canton ready"
    
    # Wait for API Server
    print_info "Waiting for API Server..."
    while ! curl -s http://localhost:8081/health > /dev/null 2>&1; do
        sleep 2
        waited=$((waited + 2))
        if [ $waited -gt $max_wait ]; then
            print_error "Timeout waiting for API Server"
            exit 1
        fi
    done
    print_success "API Server ready"
    
    # Wait for Relayer
    print_info "Waiting for Relayer..."
    while ! curl -s http://localhost:8080/health > /dev/null 2>&1; do
        sleep 2
        waited=$((waited + 2))
        if [ $waited -gt $max_wait ]; then
            print_error "Timeout waiting for Relayer"
            exit 1
        fi
    done
    print_success "Relayer ready"
    
    print_success "All services are ready!"
}

# =============================================================================
# Extract Package IDs
# =============================================================================
extract_package_ids() {
    print_header "Detecting Package IDs from Canton"
    
    # Get JWT token
    local jwt_token=$(curl -s http://localhost:8088/oauth/token \
        -d "grant_type=client_credentials&client_id=local-test-client&client_secret=local-test-secret&audience=http://canton:5011" \
        | jq -r '.access_token')
    
    if [ -z "$jwt_token" ] || [ "$jwt_token" = "null" ]; then
        print_warning "Could not get JWT token - using existing config"
        return 0
    fi
    
    # List packages from Canton
    local packages=$(curl -s http://localhost:5013/v2/packages \
        -H "Authorization: Bearer $jwt_token" \
        -H "Content-Type: application/json" 2>/dev/null)
    
    if [ -z "$packages" ]; then
        print_warning "Could not list packages from Canton"
        return 0
    fi
    
    # Extract package IDs by examining DAR metadata
    # The package IDs are in the DAR files themselves
    local daml_dir="$PROJECT_ROOT/contracts/canton-erc20/daml"
    
    print_info "Extracting package IDs from DARs..."
    
    # These can be extracted from daml.yaml or the DAR hash
    # For now, we rely on the IDs being stable across builds of the same source
    
    # Get package IDs from Canton's package list
    local package_ids=$(echo "$packages" | jq -r '.package_ids[]' 2>/dev/null)
    
    if [ -n "$package_ids" ]; then
        echo "    Found packages in Canton:"
        echo "$package_ids" | while read -r pkg; do
            echo "      - ${pkg:0:16}..."
        done
    fi
    
    print_success "Package IDs detected"
    echo ""
    echo "    Note: Package IDs are configured in config.e2e-local.yaml"
    echo "    If you rebuilt DARs, update the package IDs in the config."
}

# =============================================================================
# Run E2E Test
# =============================================================================
run_e2e_test() {
    print_header "Running E2E Test"
    
    cd "$PROJECT_ROOT"
    
    ./scripts/e2e-local.sh --skip-docker
}

# =============================================================================
# Print Summary
# =============================================================================
print_summary() {
    print_header "Setup Complete!"
    
    echo ""
    echo "  Services running:"
    echo "    - Anvil (Ethereum):     http://localhost:8545"
    echo "    - Canton Ledger API:    localhost:5011 (gRPC)"
    echo "    - Canton HTTP API:      http://localhost:5013"
    echo "    - API Server:           http://localhost:8081"
    echo "    - Relayer:              http://localhost:8080"
    echo "    - PostgreSQL:           localhost:5432"
    echo "    - Mock OAuth2:          http://localhost:8088"
    echo ""
    echo "  Test accounts (Anvil default mnemonic):"
    echo "    User 1: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
    echo "    User 2: 0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
    echo ""
    echo "  Token addresses:"
    echo "    PROMPT: 0x5FbDB2315678afecb367f032d93F642f64180aa3"
    echo "    DEMO:   0xDE30000000000000000000000000000000000001"
    echo ""
    echo "  Next steps:"
    echo "    1. Run MetaMask test:  ./scripts/metamask-test.sh"
    echo "    2. Run E2E test:       ./scripts/e2e-local.sh"
    echo "    3. View logs:          docker compose logs -f"
    echo "    4. Stop services:      docker compose down"
    echo ""
}

# =============================================================================
# Main
# =============================================================================
main() {
    print_header "Canton-Ethereum Bridge Local Setup"
    
    check_prerequisites
    init_submodules
    
    if [ "$SKIP_BUILD" = false ]; then
        build_dars
    fi
    
    start_docker_services
    wait_for_services
    extract_package_ids
    
    if [ "$SETUP_ONLY" = true ]; then
        print_summary
    else
        run_e2e_test
        print_summary
    fi
}

main
