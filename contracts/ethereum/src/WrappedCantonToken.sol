// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import "@openzeppelin/contracts/access/AccessControl.sol";
import "./IWrappedToken.sol";

/**
 * @title WrappedCantonToken
 * @notice ERC-20 token representing Canton Network tokens on Ethereum
 * @dev Only the bridge contract can mint and burn tokens
 */
contract WrappedCantonToken is ERC20, ERC20Burnable, AccessControl, IWrappedToken {
    bytes32 public constant MINTER_ROLE = keccak256("MINTER_ROLE");
    bytes32 public constant BURNER_ROLE = keccak256("BURNER_ROLE");

    // Canton token identifier this wraps
    bytes32 public cantonTokenId;

    /**
     * @notice Contract constructor
     * @param name Token name
     * @param symbol Token symbol
     * @param _cantonTokenId Canton Network token identifier
     * @param bridge Address of the bridge contract (receives minter/burner roles)
     */
    constructor(
        string memory name,
        string memory symbol,
        bytes32 _cantonTokenId,
        address bridge
    ) ERC20(name, symbol) {
        require(bridge != address(0), "Invalid bridge address");
        require(_cantonTokenId != bytes32(0), "Invalid Canton token ID");

        cantonTokenId = _cantonTokenId;

        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(MINTER_ROLE, bridge);
        _grantRole(BURNER_ROLE, bridge);
    }

    /**
     * @notice Mint tokens to an address
     * @dev Only callable by accounts with MINTER_ROLE (bridge)
     * @param to Recipient address
     * @param amount Amount to mint
     */
    function mint(address to, uint256 amount) external override onlyRole(MINTER_ROLE) {
        _mint(to, amount);
    }

    /**
     * @notice Burn tokens from an address
     * @dev Only callable by accounts with BURNER_ROLE (bridge)
     * @param from Address to burn from
     * @param amount Amount to burn
     */
    function burnFrom(address from, uint256 amount) public override(ERC20Burnable, IWrappedToken) onlyRole(BURNER_ROLE) {
        _burn(from, amount);
    }
}
