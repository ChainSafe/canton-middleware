#!/bin/bash
# =============================================================================
# MetaMask Connection Info
# =============================================================================
# Prints the information needed to connect MetaMask to the local Canton bridge.
# Run this after setup-local.sh has completed.
#
# Usage: ./scripts/utils/metamask-info.sh
# =============================================================================

# Colors
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  MetaMask Setup for Canton Bridge${NC}"
echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${YELLOW}1. Add Network to MetaMask:${NC}"
echo "   Network Name:  Canton Local"
echo -e "   RPC URL:       ${GREEN}http://localhost:8081/eth${NC}"
echo -e "   Chain ID:      ${GREEN}31337${NC}"
echo "   Currency:      ETH"
echo ""

echo -e "${YELLOW}2. Import Test Accounts:${NC}"
echo ""
echo "   User 1:"
echo -e "   Address:     ${GREEN}0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266${NC}"
echo -e "   Private Key: ${GREEN}ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80${NC}"
echo ""
echo "   User 2:"
echo -e "   Address:     ${GREEN}0x70997970C51812dc3A010C7d01b50e0d17dc79C8${NC}"
echo -e "   Private Key: ${GREEN}59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d${NC}"
echo ""

echo -e "${YELLOW}3. Import Tokens:${NC}"
echo ""
echo "   PROMPT (bridged ERC-20):"
echo -e "   Address:  ${GREEN}0x5FbDB2315678afecb367f032d93F642f64180aa3${NC}"
echo "   Symbol:   PROMPT"
echo "   Decimals: 18"
echo ""
echo "   DEMO (native Canton token):"
echo -e "   Address:  ${GREEN}0xDE30000000000000000000000000000000000001${NC}"
echo "   Symbol:   DEMO"
echo "   Decimals: 18"
echo ""

echo -e "${YELLOW}4. Test Transfers:${NC}"
echo "   - Send PROMPT between User 1 and User 2"
echo "   - Send DEMO between User 1 and User 2"
echo "   - Both should show as 'Confirmed' (not stuck on 'Pending')"
echo ""

# Check if services are running
if curl -s http://localhost:8081/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ API Server is running${NC}"
    
    # Show current balances
    echo ""
    echo -e "${YELLOW}Current Balances:${NC}"
    
    # Query balances from database
    USER1_PROMPT=$(docker exec postgres psql -U postgres -d erc20_api -t -c "SELECT balance FROM users WHERE evm_address = '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266'" 2>/dev/null | tr -d ' ')
    USER1_DEMO=$(docker exec postgres psql -U postgres -d erc20_api -t -c "SELECT demo_balance FROM users WHERE evm_address = '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266'" 2>/dev/null | tr -d ' ')
    USER2_PROMPT=$(docker exec postgres psql -U postgres -d erc20_api -t -c "SELECT balance FROM users WHERE evm_address = '0x70997970C51812dc3A010C7d01b50e0d17dc79C8'" 2>/dev/null | tr -d ' ')
    USER2_DEMO=$(docker exec postgres psql -U postgres -d erc20_api -t -c "SELECT demo_balance FROM users WHERE evm_address = '0x70997970C51812dc3A010C7d01b50e0d17dc79C8'" 2>/dev/null | tr -d ' ')
    
    echo "   User 1: ${USER1_PROMPT:-?} PROMPT, ${USER1_DEMO:-?} DEMO"
    echo "   User 2: ${USER2_PROMPT:-?} PROMPT, ${USER2_DEMO:-?} DEMO"
else
    echo -e "${YELLOW}⚠ Services not running. Run ./scripts/setup/setup-local.sh first${NC}"
fi

echo ""
