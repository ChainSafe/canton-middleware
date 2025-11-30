// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/CantonBridge.sol";
import "../src/WrappedCantonToken.sol";

contract DeployScript is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address relayer = vm.envAddress("RELAYER_ADDRESS");

        vm.startBroadcast(deployerPrivateKey);

        // Deploy bridge
        CantonBridge bridge = new CantonBridge(
            relayer,
            1000 ether, // maxTransferAmount
            0.001 ether // minTransferAmount
        );

        // Deploy token
        WrappedCantonToken token = new WrappedCantonToken(
            "Wrapped Canton Token",
            "WCT",
            keccak256("CantonToken"),
            address(bridge)
        );

        console.log("CantonBridge deployed to:", address(bridge));
        console.log("WrappedCantonToken deployed to:", address(token));
        console.log("Relayer address:", relayer);

        vm.stopBroadcast();
    }
}
