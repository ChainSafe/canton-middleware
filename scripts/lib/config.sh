#!/bin/bash
# =============================================================================
# Configuration loading for Canton-Ethereum bridge tests
# =============================================================================

# Default configuration (matching config.e2e-local.yaml)
USER1_KEY="${USER1_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
USER1_ADDR="${USER1_ADDR:-0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266}"

USER2_KEY="${USER2_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"
USER2_ADDR="${USER2_ADDR:-0x70997970C51812dc3A010C7d01b50e0d17dc79C8}"

TOKEN_ADDR="${TOKEN_ADDR:-0x5FbDB2315678afecb367f032d93F642f64180aa3}"
BRIDGE_ADDR="${BRIDGE_ADDR:-0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512}"

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

# Print configuration
print_config() {
    print_info "User1 Address: $USER1_ADDR"
    print_info "User2 Address: $USER2_ADDR"
    print_info "Token: $TOKEN_ADDR"
    print_info "Bridge: $BRIDGE_ADDR"
    print_info "Chain ID: $CHAIN_ID"
}
