#!/bin/bash
# =============================================================================
# Canton-Ethereum Bridge MetaMask Interactive Test
# =============================================================================
# This script prepares the test environment and then pauses for manual
# MetaMask testing. It automates setup steps 1-5, then waits for you to
# perform transfers via MetaMask, and finally verifies the results.
#
# Usage:
#   ./scripts/metamask-test.sh [--cleanup] [--skip-docker] [--verbose]
#
# Flags:
#   --cleanup      Stop and remove Docker services after test
#   --skip-docker  Skip Docker compose start (assume services are running)
#   --verbose      Enable verbose output
#
# Interactive Flow:
#   1-5. Automated setup (deposits, registration, etc.)
#   6.   PAUSE - Perform transfers manually via MetaMask
#   7-8. Automated verification (balances, metadata)
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

# Print MetaMask connection instructions
print_metamask_instructions() {
    print_header "MetaMask Setup Instructions"

    echo -e "${CYAN}1. Open MetaMask and add the local network:${RESET}"
    print_info "Network Name: Canton Local"
    print_info "RPC URL: ${GREEN}$ETH_RPC_URL${RESET}"
    print_info "Chain ID: ${GREEN}$CHAIN_ID${RESET}"
    print_info "Currency Symbol: ETH"
    echo ""

    echo -e "${CYAN}2. Import test accounts:${RESET}"
    print_info "User1 Private Key: ${GREEN}$USER1_KEY${RESET}"
    print_info "User1 Address: ${GREEN}$USER1_ADDR${RESET}"
    echo ""
    print_info "User2 Private Key: ${GREEN}$USER2_KEY${RESET}"
    print_info "User2 Address: ${GREEN}$USER2_ADDR${RESET}"
    echo ""

    echo -e "${CYAN}3. Add the token to MetaMask:${RESET}"
    print_info "Token Address: ${GREEN}$TOKEN_ADDR${RESET}"
    print_info "Token Symbol: PROMPT"
    print_info "Decimals: 18"
    echo ""

    echo -e "${CYAN}4. Current balances on Canton:${RESET}"
    print_info "User1: ${GREEN}$USER1_CANTON_BALANCE${RESET} tokens"
    print_info "User2: ${GREEN}$USER2_CANTON_BALANCE${RESET} tokens"
    echo ""
}

# Print transfer instructions
print_transfer_instructions() {
    print_header "Manual Transfer Instructions"

    echo -e "${YELLOW}Now it's time to test transfers with MetaMask!${RESET}"
    echo ""
    echo -e "${CYAN}Suggested test scenarios:${RESET}"
    echo ""

    echo -e "${GREEN}Test 1: Simple Transfer${RESET}"
    print_info "1. Switch to User1 account in MetaMask"
    print_info "2. Send ${GREEN}$TRANSFER_AMOUNT PROMPT${RESET} tokens to User2"
    print_info "3. Recipient: ${GREEN}$USER2_ADDR${RESET}"
    print_info "4. Confirm the transaction"
    echo ""

    echo -e "${GREEN}Test 2: Multiple Transfers${RESET}"
    print_info "1. Try sending different amounts"
    print_info "2. Test transfers in both directions (User1 <-> User2)"
    print_info "3. Verify balance updates in MetaMask after each transfer"
    echo ""

    echo -e "${GREEN}Test 3: Error Cases (Optional)${RESET}"
    print_info "1. Try sending more tokens than available (should fail)"
    print_info "2. Try sending to invalid address (should fail)"
    echo ""

    echo -e "${CYAN}What to observe:${RESET}"
    print_info "✓ Transaction confirmation in MetaMask"
    print_info "✓ Balance updates (may take a few seconds)"
    print_info "✓ Transaction appears in MetaMask activity"
    print_info "✓ No errors or rejections"
    echo ""
}

# Wait for user to complete testing
wait_for_user() {
    echo -e "${YELLOW}════════════════════════════════════════════════════════════${RESET}"
    echo -e "${YELLOW}  Press ENTER when you've finished testing with MetaMask${RESET}"
    echo -e "${YELLOW}════════════════════════════════════════════════════════════${RESET}"
    read -r
}

# Main test execution
main() {
    print_header "Canton-Ethereum Bridge MetaMask Interactive Test"

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
    # Step 4: Deposit Tokens to Canton
    # ==========================================================================
    deposit_to_canton "$DEPOSIT_AMOUNT" "$USER1_FINGERPRINT" "$USER1_KEY"

    # ==========================================================================
    # Step 5: Verify Canton Balance
    # ==========================================================================
    print_header "Step 5: Verify Canton Balance"

    USER1_CANTON_BALANCE=$(wait_for_balance "$TOKEN_ADDR" "$USER1_ADDR" 0 60)
    USER2_CANTON_BALANCE=$(get_canton_balance "$TOKEN_ADDR" "$USER2_ADDR")

    print_success "User1 Canton balance: $USER1_CANTON_BALANCE"
    print_success "User2 Canton balance: $USER2_CANTON_BALANCE"

    # ==========================================================================
    # Step 6: MANUAL METAMASK TESTING
    # ==========================================================================
    print_header "Step 6: Manual MetaMask Testing"

    print_metamask_instructions
    print_transfer_instructions
    wait_for_user

    # ==========================================================================
    # Step 7: Verify Final Balances
    # ==========================================================================
    print_header "Step 7: Verify Final Balances"

    print_step "Fetching current balances..."
    sleep 2 # Give a moment for any pending transactions

    USER1_FINAL=$(get_canton_balance "$TOKEN_ADDR" "$USER1_ADDR")
    USER2_FINAL=$(get_canton_balance "$TOKEN_ADDR" "$USER2_ADDR")

    print_success "User1 final balance: $USER1_FINAL"
    print_success "User2 final balance: $USER2_FINAL"

    # Calculate changes
    local user1_change=$((USER1_FINAL - USER1_CANTON_BALANCE))
    local user2_change=$((USER2_FINAL - USER2_CANTON_BALANCE))

    echo ""
    print_info "Balance changes:"
    if [ $user1_change -lt 0 ]; then
        print_info "User1: ${RED}$user1_change${RESET} (sent)"
    elif [ $user1_change -gt 0 ]; then
        print_info "User1: ${GREEN}+$user1_change${RESET} (received)"
    else
        print_info "User1: ${YELLOW}0${RESET} (no change)"
    fi

    if [ $user2_change -lt 0 ]; then
        print_info "User2: ${RED}$user2_change${RESET} (sent)"
    elif [ $user2_change -gt 0 ]; then
        print_info "User2: ${GREEN}+$user2_change${RESET} (received)"
    else
        print_info "User2: ${YELLOW}0${RESET} (no change)"
    fi
    echo ""

    # ==========================================================================
    # Step 8: Test ERC20 Metadata Endpoints
    # ==========================================================================
    test_erc20_metadata

    print_header "MetaMask Testing Completed!"

    echo -e "${GREEN}Congratulations! You've successfully tested the Canton bridge with MetaMask.${RESET}"
    echo ""
    print_info "What you tested:"
    print_info "✓ MetaMask network configuration"
    print_info "✓ Account import and management"
    print_info "✓ Token visibility in MetaMask"
    print_info "✓ ERC20 transfers via MetaMask UI"
    print_info "✓ Balance updates and synchronization"
    echo ""
}

# Run main
main
