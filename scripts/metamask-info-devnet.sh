#!/bin/bash
# =============================================================================
# MetaMask Connection Info - DevNet/Sepolia
# =============================================================================
# Prints the information needed to connect MetaMask to the Canton bridge
# running against 5North DevNet + Sepolia testnet.
#
# Run this after setup-devnet.sh has completed.
#
# Usage: ./scripts/metamask-info-devnet.sh
# =============================================================================

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
CONFIG_FILE="$PROJECT_ROOT/config.devnet.yaml"

# Colors
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Extract values from config
BRIDGE_CONTRACT=$(grep "bridge_contract:" "$CONFIG_FILE" | grep -v "#" | head -1 | awk '{print $2}' | tr -d '"')
TOKEN_CONTRACT=$(grep "token_contract:" "$CONFIG_FILE" | grep -v "#" | head -1 | awk '{print $2}' | tr -d '"')
SEPOLIA_RPC=$(grep "rpc_url:" "$CONFIG_FILE" | grep -v "#" | head -1 | awk '{print $2}' | tr -d '"')

echo ""
echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  MetaMask Setup for Canton Bridge (DevNet + Sepolia)${NC}"
echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${YELLOW}Connect via Local API Server (ERC-20 RPC proxy)${NC}"
echo "   This routes through your local API server for Canton balance visibility."
echo ""
echo "   Network Name:  Canton DevNet (Local)"
echo -e "   RPC URL:       ${GREEN}http://localhost:8081/eth${NC}"
echo -e "   Chain ID:      ${GREEN}1155111101${NC}"
echo "   Currency:      ETH"
echo "   Explorer:      (none - synthetic network)"
echo ""

echo -e "${YELLOW}Token Contracts:${NC}"
echo ""
echo "   PROMPT Token (bridged ERC-20):"
echo -e "   Address:  ${GREEN}${TOKEN_CONTRACT}${NC}"
echo "   Symbol:   PROMPT"
echo "   Decimals: 18"
echo ""
echo "   DEMO Token (native Canton):"
echo -e "   Address:  ${GREEN}0xDE30000000000000000000000000000000000001${NC}"
echo "   Symbol:   DEMO"
echo "   Decimals: 18"
echo ""
echo "   Bridge Contract (Sepolia):"
echo -e "   Address:  ${GREEN}${BRIDGE_CONTRACT}${NC}"
echo ""

echo -e "${YELLOW}Getting Sepolia ETH (for gas):${NC}"
echo "   - Alchemy Faucet: https://sepoliafaucet.com"
echo "   - Infura Faucet:  https://www.infura.io/faucet/sepolia"
echo "   - PoW Faucet:     https://sepolia-faucet.pk910.de"
echo ""

echo -e "${YELLOW}Getting PROMPT Tokens:${NC}"
echo "   Option 1: If you have testnet PROMPT, deposit via the bridge"
echo "   Option 2: Use the token contract's mint function (if available)"
echo ""

echo -e "${YELLOW}User Registration:${NC}"
echo "   Before you can receive Canton balances, register your EVM address:"
echo ""
echo -e "   ${GREEN}go run scripts/register-user.go -config config.devnet.yaml -evm-address YOUR_ADDRESS${NC}"
echo ""

# Check if services are running
echo -e "${YELLOW}Service Status:${NC}"

if curl -s http://localhost:8081/health > /dev/null 2>&1; then
    echo -e "   ${GREEN}✓ API Server running${NC} (http://localhost:8081)"
else
    echo -e "   ${RED}✗ API Server not running${NC}"
fi

if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "   ${GREEN}✓ Relayer running${NC} (http://localhost:8080)"
else
    echo -e "   ${RED}✗ Relayer not running${NC}"
fi

# Test Sepolia connectivity
BLOCK=$(curl -s "$SEPOLIA_RPC" -X POST -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
    | jq -r '.result' 2>/dev/null)
if [ -n "$BLOCK" ] && [ "$BLOCK" != "null" ]; then
    echo -e "   ${GREEN}✓ Sepolia connected${NC} (block: $BLOCK)"
else
    echo -e "   ${RED}✗ Sepolia not connected${NC}"
fi

echo ""
echo -e "${YELLOW}Useful Commands:${NC}"
echo "   Check services:   ./scripts/setup-devnet.sh --status"
echo "   View logs:        docker compose -f docker-compose.yaml -f docker-compose.devnet.yaml logs -f"
echo "   Register user:    go run scripts/register-user.go -config config.devnet.yaml -evm-address 0x..."
echo ""
