// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";

/**
 * @title IWrappedToken
 * @notice Interface for wrapped tokens that can be minted and burned by the bridge
 */
interface IWrappedToken is IERC20 {
    /**
     * @notice Mint tokens to an address
     * @param to Recipient address
     * @param amount Amount to mint
     */
    function mint(address to, uint256 amount) external;

    /**
     * @notice Burn tokens from an address
     * @param from Address to burn from
     * @param amount Amount to burn
     */
    function burnFrom(address from, uint256 amount) external;
}
