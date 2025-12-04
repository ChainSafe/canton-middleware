// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/CantonBridge.sol";
import "../src/WrappedCantonToken.sol";

contract DeployScript is Script {
    // Helper function to safely read address from env, returns address(0) if not set
    function tryEnvAddress(string memory name) internal view returns (address) {
        try vm.envAddress(name) returns (address addr) {
            return addr;
        } catch {
            return address(0);
        }
    }

    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address relayer = vm.envAddress("RELAYER_ADDRESS");

        // Try to read existing contract addresses from .env (optional)
        address existingBridgeAddress = tryEnvAddress("CANTON_BRIDGE_ADDRESS");
        address existingTokenAddress = tryEnvAddress("CANTON_TOKEN_ADDRESS");

        // Check if contracts exist at the provided addresses
        bool bridgeExists = existingBridgeAddress != address(0) && existingBridgeAddress.code.length > 0;
        bool tokenExists = existingTokenAddress != address(0) && existingTokenAddress.code.length > 0;

        bool shouldDeployBridge = true;
        bool shouldDeployToken = true;

        if (bridgeExists) {
            console.log("Existing CantonBridge found at:", existingBridgeAddress);
            console.log("Checking if bytecode has changed...");
            
            // Get runtime bytecode from deployed contract
            bytes memory deployedBridgeBytecode = existingBridgeAddress.code;
            
            // Get expected runtime bytecode by deploying a temporary instance and getting its code
            // Note: We can't easily get runtime bytecode without deploying, so we'll use a different approach
            // We'll compute a hash of the deployed bytecode and compare it with what we expect
            // For now, we'll check if the bytecode length matches and log a warning
            bytes32 deployedBridgeHash = keccak256(deployedBridgeBytecode);
            console.log("Deployed bridge bytecode hash:", vm.toString(deployedBridgeHash));
            console.log("Deployed bridge bytecode length:", deployedBridgeBytecode.length);
            
            // Try to verify it's the right contract by calling a known function
            // If the function exists and returns expected values, it's likely the same contract
            try CantonBridge(existingBridgeAddress).relayer() returns (address deployedRelayer) {
                if (deployedRelayer == relayer) {
                    console.log("Bridge relayer matches. Contract appears to be correctly deployed.");
                    console.log("WARNING: Bytecode comparison not performed. If contract logic changed,");
                    console.log("you may need to deploy a new instance.");
                    shouldDeployBridge = false;
                } else {
                    console.log("Bridge relayer mismatch. Expected:", relayer);
                    console.log("Deployed relayer:", deployedRelayer);
                    console.log("Will deploy new bridge.");
                }
            } catch {
                console.log("Could not verify bridge contract. Will deploy new bridge.");
            }
        }

        if (tokenExists) {
            console.log("Existing WrappedCantonToken found at:", existingTokenAddress);
            console.log("Checking if bytecode has changed...");
            
            // Get runtime bytecode from deployed contract
            bytes memory deployedTokenBytecode = existingTokenAddress.code;
            bytes32 deployedTokenHash = keccak256(deployedTokenBytecode);
            console.log("Deployed token bytecode hash:", vm.toString(deployedTokenHash));
            console.log("Deployed token bytecode length:", deployedTokenBytecode.length);
            
            // Note: Token constructor takes bridge address, so we need to check if bridge is being redeployed
            // If bridge is being redeployed, token should also be redeployed
            if (!shouldDeployBridge) {
                // Try to verify it's the right contract by checking cantonTokenId
                bytes32 expectedCantonTokenId = keccak256("CantonToken");
                try WrappedCantonToken(existingTokenAddress).cantonTokenId() returns (bytes32 deployedCantonTokenId) {
                    if (deployedCantonTokenId == expectedCantonTokenId) {
                        console.log("Token cantonTokenId matches. Contract appears to be correctly deployed.");
                        console.log("WARNING: Bytecode comparison not performed. If contract logic changed,");
                        console.log("you may need to deploy a new instance.");
                        shouldDeployToken = false;
                    } else {
                        console.log("Token cantonTokenId mismatch. Will deploy new token.");
                    }
                } catch {
                    console.log("Could not verify token contract. Will deploy new token.");
                }
            } else {
                console.log("Bridge is being redeployed, so token will also be redeployed.");
            }
        }

        vm.startBroadcast(deployerPrivateKey);

        address bridgeAddress;
        address tokenAddress;

        // Deploy bridge only if needed
        if (shouldDeployBridge) {
            console.log("Deploying new CantonBridge...");
            CantonBridge bridge = new CantonBridge(
                relayer,
                1000 ether, // maxTransferAmount
                0.001 ether // minTransferAmount
            );
            bridgeAddress = address(bridge);
            console.log("CantonBridge deployed to:", bridgeAddress);
        } else {
            bridgeAddress = existingBridgeAddress;
            console.log("Using existing CantonBridge at:", bridgeAddress);
        }

        // Deploy token only if needed
        // Note: If bridge is being redeployed, token should also be redeployed
        if (shouldDeployToken || shouldDeployBridge) {
            console.log("Deploying new WrappedCantonToken...");
            WrappedCantonToken token = new WrappedCantonToken(
                "Wrapped Canton Token",
                "WCT",
                keccak256("CantonToken"),
                bridgeAddress
            );
            tokenAddress = address(token);
            console.log("WrappedCantonToken deployed to:", tokenAddress);
        } else {
            tokenAddress = existingTokenAddress;
            console.log("Using existing WrappedCantonToken at:", tokenAddress);
        }

        console.log("Final addresses:");
        console.log("CantonBridge:", bridgeAddress);
        console.log("WrappedCantonToken:", tokenAddress);
        console.log("Relayer address:", relayer);

        vm.stopBroadcast();
    }
}
