#!/bin/bash

# Test balance query via eth_call
# User1: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
# Token: 0x5FbDB2315678afecb367f032d93F642f64180aa3

# balanceOf(address) selector: 0x70a08231
# Encode address as 32 bytes (pad left with zeros)
ADDRESS="000000000000000000000000f39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
DATA="0x70a08231${ADDRESS}"

echo "Testing eth_call for balanceOf..."
echo "Address: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
echo "Token: 0x5FbDB2315678afecb367f032d93F642f64180aa3"
echo "Data: $DATA"
echo

curl -s -X POST http://localhost:8081/eth \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_call",
    "params": [{
      "to": "0x5FbDB2315678afecb367f032d93F642f64180aa3",
      "data": "'$DATA'"
    }],
    "id": 1
  }' | jq '.'
