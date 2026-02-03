#!/bin/bash
# Quick diagnostic script to check Anvil state

echo "=== Checking Anvil State ==="
echo ""

# User addresses
USER1="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
USER2="0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

# Contract addresses from config
TOKEN="0x5FbDB2315678afecb367f032d93F642f64180aa3"
BRIDGE="0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"

ANVIL_URL="http://localhost:8545"

echo "1. Check User1 ETH balance (should be ~10000 ETH):"
cast balance $USER1 --rpc-url $ANVIL_URL

echo ""
echo "2. Check User2 ETH balance (should be ~10000 ETH):"
cast balance $USER2 --rpc-url $ANVIL_URL

echo ""
echo "3. Check if Token contract exists at $TOKEN:"
cast code $TOKEN --rpc-url $ANVIL_URL | head -c 50
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo "... (contract exists)"
else
    echo "ERROR: Contract not found!"
fi

echo ""
echo "4. Check if Bridge contract exists at $BRIDGE:"
cast code $BRIDGE --rpc-url $ANVIL_URL | head -c 50
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo "... (contract exists)"
else
    echo "ERROR: Contract not found!"
fi

echo ""
echo "5. Check User1 token balance:"
cast call $TOKEN "balanceOf(address)(uint256)" $USER1 --rpc-url $ANVIL_URL 2>/dev/null
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to get token balance!"
fi

echo ""
echo "6. Check token name:"
cast call $TOKEN "name()(string)" --rpc-url $ANVIL_URL 2>/dev/null

echo ""
echo "7. Check token total supply:"
cast call $TOKEN "totalSupply()(uint256)" --rpc-url $ANVIL_URL 2>/dev/null

echo ""
echo "=== Checking deployer logs ==="
docker logs deployer 2>&1 | tail -20
