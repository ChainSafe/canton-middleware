# Canton Bridge - Ethereum Smart Contracts

Solidity smart contracts for the Canton-Ethereum token bridge, built with Foundry.

## Contracts

- **CantonBridge.sol**: Main bridge contract handling deposits and withdrawals
- **WrappedCantonToken.sol**: ERC-20 token representing Canton tokens on Ethereum
- **IWrappedToken.sol**: Interface for wrapped tokens
- **MockERC20.sol**: Mock token for testing

## Setup

### Install Foundry

```bash
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

### Install Dependencies

```bash
forge install
```

## Development

### Build

```bash
forge build
```

### Test

```bash
forge test
```

### Test with gas reporting

```bash
forge test --gas-report
```

### Test coverage

```bash
forge coverage
```

### Format

```bash
forge fmt
```

## Deployment

### Local/Testnet

```bash
# Set environment variables
export PRIVATE_KEY=<deployer_private_key>
export RELAYER_ADDRESS=<relayer_address>
export SEPOLIA_RPC_URL=<sepolia_rpc_url>

# Deploy to Sepolia
forge script script/Deploy.s.sol:DeployScript \
    --rpc-url $SEPOLIA_RPC_URL \
    --broadcast \
    --verify
```

### Mainnet

```bash
export PRIVATE_KEY=<deployer_private_key>
export RELAYER_ADDRESS=<relayer_address>
export MAINNET_RPC_URL=<mainnet_rpc_url>
export ETHERSCAN_API_KEY=<etherscan_api_key>

forge script script/Deploy.s.sol:DeployScript \
    --rpc-url $MAINNET_RPC_URL \
    --broadcast \
    --verify
```

## Contract Verification

```bash
forge verify-contract \
    --chain-id 1 \
    --num-of-optimizations 200 \
    --constructor-args $(cast abi-encode "constructor(address,uint256,uint256)" $RELAYER_ADDRESS 1000000000000000000000 1000000000000000) \
    <CONTRACT_ADDRESS> \
    src/CantonBridge.sol:CantonBridge \
    --etherscan-api-key $ETHERSCAN_API_KEY
```

## Contract Interactions

### Add Token Mapping

```bash
cast send <BRIDGE_ADDRESS> \
    "addTokenMapping(address,bytes32,bool)" \
    <ETHEREUM_TOKEN> \
    <CANTON_TOKEN_ID> \
    false \
    --rpc-url $RPC_URL \
    --private-key $PRIVATE_KEY
```

### Deposit to Canton

```bash
# Approve bridge to spend tokens
cast send <TOKEN_ADDRESS> \
    "approve(address,uint256)" \
    <BRIDGE_ADDRESS> \
    <AMOUNT> \
    --rpc-url $RPC_URL \
    --private-key $PRIVATE_KEY

# Deposit
cast send <BRIDGE_ADDRESS> \
    "depositToCanton(address,uint256,bytes32)" \
    <TOKEN_ADDRESS> \
    <AMOUNT> \
    <CANTON_RECIPIENT> \
    --rpc-url $RPC_URL \
    --private-key $PRIVATE_KEY
```

## Security

- Pausable: Bridge can be paused by owner in emergencies
- ReentrancyGuard: Protection against reentrancy attacks
- Access Control: Only relayer can process withdrawals
- Replay Protection: Canton transaction hashes tracked to prevent double-processing

## License

MIT
