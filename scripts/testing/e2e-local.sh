#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge Local E2E Test (Bash + Cast version - Refactored)
# =============================================================================
# This script runs a complete E2E test using cast commands for ERC20 interactions.
#
# Usage:
#   ./scripts/testing/e2e-local.sh [--cleanup] [--skip-docker] [--verbose]
#
# Flags:
#   --cleanup      Stop and remove Docker services after test
#   --skip-docker  Skip Docker compose start (assume services are running)
#   --verbose      Enable verbose output
# =============================================================================

set -e

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Load shared libraries
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/config.sh"
source "$SCRIPT_DIR/lib/services.sh"
source "$SCRIPT_DIR/lib/bridge.sh"

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

# Main test execution
main() {
    print_header "Canton-Ethereum Bridge Local E2E Test (Cast Version)"

    # Start Docker services
    if [ "$SKIP_DOCKER" = false ]; then
        start_docker_services "$VERBOSE"
    else
        print_info "Skipping Docker compose start (assuming services are running)"
    fi

    # Cleanup function
    cleanup() {
        if [ "$CLEANUP" = true ]; then
            stop_docker_services
        fi
    }
    trap cleanup EXIT

    # Wait for services
    wait_for_all_services
    print_config

    # ==========================================================================
    # Step 1: Verify Token Balance on Anvil
    # ==========================================================================
    print_header "Step 1: Verify Token Balance"

    USER1_BALANCE=$(cast call "$TOKEN_ADDR" "balanceOf(address)(uint256)" "$USER1_ADDR" --rpc-url "$ANVIL_URL" 2>/dev/null)
    BALANCE_HUMAN=$(cast --from-wei "$USER1_BALANCE" ether 2>/dev/null || echo "unknown")
    print_success "User1 has tokens: $BALANCE_HUMAN ($USER1_BALANCE wei)"

    # ==========================================================================
    # Step 2: Whitelist Users in Database
    # ==========================================================================
    whitelist_users

    # ==========================================================================
    # Step 3: Register Users
    # ==========================================================================
    print_header "Step 3: Register Users"

    USER1_FINGERPRINT=$(register_user "User1" "$USER1_KEY" "$USER1_ADDR")
    USER2_FINGERPRINT=$(register_user "User2" "$USER2_KEY" "$USER2_ADDR")

    # ==========================================================================
    # Step 3.5: Bootstrap DEMO Token (Native Canton Token)
    # ==========================================================================
    print_header "Step 3.5: Bootstrap DEMO Token"
    
    # Get the native token package ID from config or DAR
    NATIVE_PKG_ID=$(get_native_token_package_id)
    
    if [ -n "$NATIVE_PKG_ID" ]; then
        # Extract relayer_party and domain_id from bootstrap container logs
        RELAYER_PARTY=$(docker logs bootstrap 2>&1 | grep -o 'relayer_party: "[^"]*"' | head -1 | sed 's/relayer_party: "//;s/"$//')
        DOMAIN_ID=$(docker logs bootstrap 2>&1 | grep -o 'domain_id: "[^"]*"' | head -1 | sed 's/domain_id: "//;s/"$//')
        
        if [ -z "$RELAYER_PARTY" ] || [ -z "$DOMAIN_ID" ]; then
            print_warning "Could not extract relayer_party or domain_id from bootstrap logs, skipping DEMO token"
        else
            print_step "Bootstrapping DEMO token with native-package-id: ${NATIVE_PKG_ID:0:16}..."
            print_info "Issuer: ${RELAYER_PARTY:0:40}..."
            print_info "Domain: ${DOMAIN_ID:0:40}..."
            
            go run "$SCRIPT_DIR/bootstrap-demo.go" \
                -config config.e2e-local.yaml \
                -native-package-id "$NATIVE_PKG_ID" \
                -issuer "$RELAYER_PARTY" \
                -domain "$DOMAIN_ID" \
                -user1-fingerprint "$USER1_FINGERPRINT" \
                -user2-fingerprint "$USER2_FINGERPRINT" \
                -mint-amount "500.0"
            
            if [ $? -eq 0 ]; then
                print_success "DEMO token bootstrapped: 500 DEMO to each user"
            else
                print_warning "DEMO token bootstrap failed (continuing with PROMPT test)"
            fi
        fi
    else
        print_warning "native_token_package_id not configured, skipping DEMO token"
    fi

    # ==========================================================================
    # Step 4: Deposit Tokens to Canton
    # ==========================================================================
    deposit_to_canton "$DEPOSIT_AMOUNT" "$USER1_FINGERPRINT" "$USER1_KEY"

    # ==========================================================================
    # Step 5: Verify Canton Balance
    # ==========================================================================
    print_header "Step 5: Verify Canton Balance"

    USER1_CANTON_BALANCE=$(wait_for_balance "$TOKEN_ADDR" "$USER1_ADDR" 0 60)
    print_success "User1 Canton balance: $USER1_CANTON_BALANCE"

    # ==========================================================================
    # Step 6: Transfer Tokens on Canton (User1 -> User2)
    # ==========================================================================
    print_header "Step 6: Transfer Tokens (User1 -> User2)"

    print_step "Transferring $TRANSFER_AMOUNT tokens..."
    TRANSFER_TX=$(send_canton_transfer "$USER2_ADDR" "$TRANSFER_AMOUNT" "$USER1_KEY")
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
    test_erc20_metadata

    print_header "Local E2E Test Completed Successfully!"
}

# Run main
main
