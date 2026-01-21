#!/bin/bash
# =============================================================================
# Configuration loading for Canton-Ethereum bridge tests
# =============================================================================

# Default configuration (matching config.e2e-local.yaml)
USER1_KEY="${USER1_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
USER1_ADDR="${USER1_ADDR:-0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266}"

USER2_KEY="${USER2_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"
USER2_ADDR="${USER2_ADDR:-0x70997970C51812dc3A010C7d01b50e0d17dc79C8}"

# Get addresses from relayer logs (most accurate - this is what's actually being watched)
_get_relayer_bridge_address() {
    docker logs canton-bridge-relayer 2>&1 | grep -o '"bridge_contract": "[^"]*"' | tail -1 | grep -oE '0x[a-fA-F0-9]{40}' || echo ""
}

# Get token address from relayer - it's the token_contract in the config
_get_token_address() {
    # Token address is deterministic for first deployment, but check config
    # For now use the deployer output as it's more reliable for token
    docker logs deployer 2>&1 | grep "PromptToken deployed to:" | tail -1 | grep -oE '0x[a-fA-F0-9]{40}' || echo ""
}

# Initialize addresses - prefer relayer's actual address, fall back to deployer, then default
BRIDGE_ADDR="${BRIDGE_ADDR:-$(_get_relayer_bridge_address)}"
BRIDGE_ADDR="${BRIDGE_ADDR:-0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512}"

TOKEN_ADDR="${TOKEN_ADDR:-$(_get_token_address)}"
TOKEN_ADDR="${TOKEN_ADDR:-0x5FbDB2315678afecb367f032d93F642f64180aa3}"

ANVIL_URL="${ANVIL_URL:-http://localhost:8545}"
API_SERVER_URL="${API_SERVER_URL:-http://localhost:8081}"
RELAYER_URL="${RELAYER_URL:-http://localhost:8080}"
ETH_RPC_URL="${ETH_RPC_URL:-$API_SERVER_URL/eth}"

DEPOSIT_AMOUNT="${DEPOSIT_AMOUNT:-100}"
TRANSFER_AMOUNT="${TRANSFER_AMOUNT:-25}"

# Database config
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASS="${DB_PASS:-p@ssw0rd}"
DB_NAME="${DB_NAME:-erc20_api}"

# Chain ID
CHAIN_ID="${CHAIN_ID:-31337}"

# DEMO token address (synthetic for MetaMask)
DEMO_TOKEN_ADDR="${DEMO_TOKEN_ADDR:-0xDEMO000000000000000000000000000000000001}"

# Print configuration
print_config() {
    print_info "User1 Address: $USER1_ADDR"
    print_info "User2 Address: $USER2_ADDR"
    print_info "Token: $TOKEN_ADDR"
    print_info "Bridge: $BRIDGE_ADDR"
    print_info "Chain ID: $CHAIN_ID"
}

# Get native token package ID from DAR or config
get_native_token_package_id() {
    local DAR_PATH="contracts/canton-erc20/daml/native-token/.daml/dist/native-token-1.1.0.dar"
    
    # First check if DAR exists and extract package ID
    if [ -f "$DAR_PATH" ]; then
        # Extract package ID from DAR filename pattern
        local PKG_ID=$(daml damlc inspect-dar "$DAR_PATH" 2>/dev/null | grep "native-token-1.1.0-" | head -1 | sed 's/.*native-token-1.1.0-\([a-f0-9]*\).*/\1/' | head -1)
        if [ -n "$PKG_ID" ]; then
            echo "$PKG_ID"
            return 0
        fi
    fi
    
    # Fallback: try to get from config.yaml
    if [ -f "config.yaml" ]; then
        local PKG_ID=$(grep "native_token_package_id:" config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
        if [ -n "$PKG_ID" ] && [ "$PKG_ID" != "" ]; then
            echo "$PKG_ID"
            return 0
        fi
    fi
    
    # Not found
    return 1
}
