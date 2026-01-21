#!/bin/bash
# =============================================================================
# Bridge operations for Canton-Ethereum bridge tests
# =============================================================================

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

# Whitelist users in database
whitelist_users() {
    print_header "Whitelist Users"

    PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
        -c "INSERT INTO whitelist (evm_address) VALUES ('$USER1_ADDR'), ('$USER2_ADDR') ON CONFLICT DO NOTHING" \
        > /dev/null 2>&1

    print_success "Whitelisted $USER1_ADDR"
    print_success "Whitelisted $USER2_ADDR"
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

# Get balance from Canton via eth_call
get_canton_balance() {
    local token_addr=$1
    local user_addr=$2

    # Use cast to call balanceOf, strip scientific notation suffix (e.g., "123 [1.23e2]" -> "123")
    local result
    result=$(cast call "$token_addr" "balanceOf(address)(uint256)" "$user_addr" --rpc-url "$ETH_RPC_URL" 2>/dev/null || echo "0")
    echo "${result%% *}"
}

# Deposit tokens to Canton
deposit_to_canton() {
    local amount=$1
    local fingerprint=$2
    local private_key=$3

    print_header "Deposit Tokens to Canton"

    # Convert amount to Wei
    local amount_wei=$(cast --to-wei "$amount" ether 2>/dev/null)

    # Approve bridge contract
    print_step "Approving bridge contract..."
    local approve_tx=$(cast send "$TOKEN_ADDR" "approve(address,uint256)" "$BRIDGE_ADDR" "$amount_wei" \
        --private-key "$private_key" --rpc-url "$ANVIL_URL" --json 2>/dev/null | jq -r '.transactionHash')
    print_info "Approval tx: $approve_tx"

    # Wait for approval
    cast receipt "$approve_tx" --rpc-url "$ANVIL_URL" > /dev/null 2>&1

    # Deposit to Canton
    print_step "Depositing to Canton..."

    # Convert fingerprint to bytes32
    local canton_recipient=$(fingerprint_to_bytes32 "$fingerprint")

    local deposit_tx=$(cast send "$BRIDGE_ADDR" "depositToCanton(address,uint256,bytes32)" \
        "$TOKEN_ADDR" "$amount_wei" "$canton_recipient" \
        --private-key "$private_key" --rpc-url "$ANVIL_URL" --json 2>/dev/null | jq -r '.transactionHash')
    print_info "Deposit tx: $deposit_tx"

    # Wait for deposit
    cast receipt "$deposit_tx" --rpc-url "$ANVIL_URL" > /dev/null 2>&1
    print_success "Deposit submitted"
}

# Wait for balance update on Canton
wait_for_balance() {
    local token_addr=$1
    local user_addr=$2
    local expected_min=${3:-0}
    local max_wait=${4:-60}

    print_step "Waiting for relayer to process deposit..."
    sleep 5

    local balance_check_interval=3
    local balance="0"

    for ((i=0; i<max_wait; i+=balance_check_interval)); do
        balance=$(get_canton_balance "$token_addr" "$user_addr")
        # Use awk for large number comparison (Wei values exceed bash integer limits)
        if [ "$balance" != "0" ] && awk "BEGIN {exit !($balance > $expected_min)}"; then
            break
        fi
        print_info "Waiting for balance... (current: $balance)"
        sleep $balance_check_interval
    done

    if [ "$balance" = "0" ]; then
        print_error "Balance not updated (timeout)"
    fi

    echo "$balance"
}

# Test ERC20 metadata endpoints
test_erc20_metadata() {
    print_header "Test ERC20 Metadata"

    local token_name=$(cast call "$TOKEN_ADDR" "name()(string)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    local token_symbol=$(cast call "$TOKEN_ADDR" "symbol()(string)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    local token_decimals=$(cast call "$TOKEN_ADDR" "decimals()(uint8)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)
    local token_supply=$(cast call "$TOKEN_ADDR" "totalSupply()(uint256)" --rpc-url "$ETH_RPC_URL" 2>/dev/null)

    print_success "Name: $token_name"
    print_success "Symbol: $token_symbol"
    print_success "Decimals: $token_decimals"
    print_success "Total Supply: $token_supply"
}

# Send ERC20 transfer on Canton
send_canton_transfer() {
    local to_addr=$1
    local amount=$2
    local private_key=$3

    local amount_wei=$(cast --to-wei "$amount" ether 2>/dev/null)

    # Use cast to send ERC20 transfer via eth_sendRawTransaction
    # Use --legacy flag to avoid EIP-1559 (eth_feeHistory) which our API doesn't support
    # Note: cast may fail on receipt parsing due to null vs [] issue, but tx still succeeds
    local transfer_output=$(cast send "$TOKEN_ADDR" "transfer(address,uint256)" "$to_addr" "$amount_wei" \
        --private-key "$private_key" --rpc-url "$ETH_RPC_URL" --legacy 2>&1 || true)

    # Extract transaction hash from output (works even if cast errors on parsing)
    local transfer_tx=$(echo "$transfer_output" | grep -oE "0x[a-f0-9]{64}" | head -1)

    if [ -z "$transfer_tx" ]; then
        print_error "Transfer failed: $transfer_output"
    fi

    echo "$transfer_tx"
}
