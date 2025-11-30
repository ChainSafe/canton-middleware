// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/Pausable.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "./IWrappedToken.sol";

/**
 * @title CantonBridge
 * @notice Bridge contract for transferring tokens between Ethereum and Canton Network
 * @dev Supports both lock/unlock (native tokens) and mint/burn (wrapped tokens)
 */
contract CantonBridge is Ownable, Pausable, ReentrancyGuard {
    using SafeERC20 for IERC20;

    // Events
    event DepositToCanton(
        address indexed token,
        address indexed sender,
        bytes32 indexed cantonRecipient,
        uint256 amount,
        uint256 nonce,
        bool isWrapped
    );

    event WithdrawFromCanton(
        address indexed token,
        address indexed recipient,
        uint256 amount,
        uint256 nonce,
        bytes32 cantonTxHash
    );

    event TokenMappingAdded(
        address indexed ethereumToken,
        bytes32 indexed cantonTokenId,
        bool isWrapped
    );

    event TokenMappingRemoved(address indexed ethereumToken);

    event RelayerUpdated(
        address indexed oldRelayer,
        address indexed newRelayer
    );

    event BridgeLimitsUpdated(
        uint256 maxTransferAmount,
        uint256 minTransferAmount
    );

    // State variables
    address public relayer;
    uint256 public depositNonce;
    uint256 public maxTransferAmount;
    uint256 public minTransferAmount;

    // Token mappings: Ethereum address => Canton token ID
    mapping(address => bytes32) public ethereumToCantonToken;
    // Track if token is wrapped (minted on Ethereum) or native
    mapping(address => bool) public isWrappedToken;
    // Canton token ID => Ethereum address
    mapping(bytes32 => address) public cantonToEthereumToken;

    // Processed Canton transaction hashes to prevent replay
    mapping(bytes32 => bool) public processedCantonTxs;

    /**
     * @notice Contract constructor
     * @param _relayer Address of the relayer authorized to process withdrawals
     * @param _maxTransferAmount Maximum amount per transfer
     * @param _minTransferAmount Minimum amount per transfer
     */
    constructor(
        address _relayer,
        uint256 _maxTransferAmount,
        uint256 _minTransferAmount
    ) Ownable(msg.sender) {
        require(_relayer != address(0), "Invalid relayer address");
        require(_maxTransferAmount > _minTransferAmount, "Invalid limits");

        relayer = _relayer;
        maxTransferAmount = _maxTransferAmount;
        minTransferAmount = _minTransferAmount;
    }

    /**
     * @notice Deposit ERC-20 tokens to bridge and transfer to Canton
     * @param token Address of the ERC-20 token
     * @param amount Amount to deposit
     * @param cantonRecipient Recipient address on Canton Network (32 bytes)
     */
    function depositToCanton(
        address token,
        uint256 amount,
        bytes32 cantonRecipient
    ) external nonReentrant whenNotPaused {
        require(token != address(0), "Invalid token address");
        require(amount >= minTransferAmount, "Amount below minimum");
        require(amount <= maxTransferAmount, "Amount exceeds maximum");
        require(cantonRecipient != bytes32(0), "Invalid Canton recipient");
        require(
            ethereumToCantonToken[token] != bytes32(0),
            "Token not supported"
        );

        bool wrapped = isWrappedToken[token];

        if (wrapped) {
            // Burn wrapped tokens
            IWrappedToken(token).burnFrom(msg.sender, amount);
        } else {
            // Lock native tokens in bridge
            IERC20(token).safeTransferFrom(msg.sender, address(this), amount);
        }

        emit DepositToCanton(
            token,
            msg.sender,
            cantonRecipient,
            amount,
            depositNonce,
            wrapped
        );

        depositNonce++;
    }

    /**
     * @notice Withdraw tokens from Canton Network to Ethereum
     * @dev Only callable by relayer
     * @param token Address of the ERC-20 token
     * @param recipient Recipient address on Ethereum
     * @param amount Amount to withdraw
     * @param nonce Canton withdrawal nonce
     * @param cantonTxHash Canton transaction hash for idempotency
     */
    function withdrawFromCanton(
        address token,
        address recipient,
        uint256 amount,
        uint256 nonce,
        bytes32 cantonTxHash
    ) external nonReentrant whenNotPaused onlyRelayer {
        require(token != address(0), "Invalid token address");
        require(recipient != address(0), "Invalid recipient");
        require(amount >= minTransferAmount, "Amount below minimum");
        require(amount <= maxTransferAmount, "Amount exceeds maximum");
        require(cantonTxHash != bytes32(0), "Invalid Canton tx hash");
        require(!processedCantonTxs[cantonTxHash], "Already processed");
        require(
            ethereumToCantonToken[token] != bytes32(0),
            "Token not supported"
        );

        // Mark as processed to prevent replay
        processedCantonTxs[cantonTxHash] = true;

        bool wrapped = isWrappedToken[token];

        if (wrapped) {
            // Mint wrapped tokens to recipient
            IWrappedToken(token).mint(recipient, amount);
        } else {
            // Unlock native tokens from bridge
            IERC20(token).safeTransfer(recipient, amount);
        }

        emit WithdrawFromCanton(token, recipient, amount, nonce, cantonTxHash);
    }

    /**
     * @notice Add a token mapping between Ethereum and Canton
     * @dev Only callable by owner
     * @param ethereumToken Ethereum ERC-20 token address
     * @param cantonTokenId Canton token identifier
     * @param wrapped Whether token is wrapped (minted on Ethereum) or native
     */
    function addTokenMapping(
        address ethereumToken,
        bytes32 cantonTokenId,
        bool wrapped
    ) external onlyOwner {
        require(ethereumToken != address(0), "Invalid token address");
        require(cantonTokenId != bytes32(0), "Invalid Canton token ID");
        require(
            ethereumToCantonToken[ethereumToken] == bytes32(0),
            "Token already mapped"
        );

        ethereumToCantonToken[ethereumToken] = cantonTokenId;
        cantonToEthereumToken[cantonTokenId] = ethereumToken;
        isWrappedToken[ethereumToken] = wrapped;

        emit TokenMappingAdded(ethereumToken, cantonTokenId, wrapped);
    }

    /**
     * @notice Remove a token mapping
     * @dev Only callable by owner
     * @param ethereumToken Ethereum ERC-20 token address
     */
    function removeTokenMapping(address ethereumToken) external onlyOwner {
        require(
            ethereumToCantonToken[ethereumToken] != bytes32(0),
            "Token not mapped"
        );

        bytes32 cantonTokenId = ethereumToCantonToken[ethereumToken];
        delete ethereumToCantonToken[ethereumToken];
        delete cantonToEthereumToken[cantonTokenId];
        delete isWrappedToken[ethereumToken];

        emit TokenMappingRemoved(ethereumToken);
    }

    /**
     * @notice Update the relayer address
     * @dev Only callable by owner
     * @param newRelayer New relayer address
     */
    function updateRelayer(address newRelayer) external onlyOwner {
        require(newRelayer != address(0), "Invalid relayer address");
        address oldRelayer = relayer;
        relayer = newRelayer;
        emit RelayerUpdated(oldRelayer, newRelayer);
    }

    /**
     * @notice Update bridge transfer limits
     * @dev Only callable by owner
     * @param _maxTransferAmount New maximum transfer amount
     * @param _minTransferAmount New minimum transfer amount
     */
    function updateLimits(
        uint256 _maxTransferAmount,
        uint256 _minTransferAmount
    ) external onlyOwner {
        require(_maxTransferAmount > _minTransferAmount, "Invalid limits");
        maxTransferAmount = _maxTransferAmount;
        minTransferAmount = _minTransferAmount;
        emit BridgeLimitsUpdated(_maxTransferAmount, _minTransferAmount);
    }

    /**
     * @notice Pause the bridge
     * @dev Only callable by owner
     */
    function pause() external onlyOwner {
        _pause();
    }

    /**
     * @notice Unpause the bridge
     * @dev Only callable by owner
     */
    function unpause() external onlyOwner {
        _unpause();
    }

    /**
     * @notice Emergency withdraw of locked tokens
     * @dev Only callable by owner when paused
     * @param token Token address to withdraw
     * @param amount Amount to withdraw
     */
    function emergencyWithdraw(
        address token,
        uint256 amount
    ) external onlyOwner whenPaused {
        require(!isWrappedToken[token], "Cannot withdraw wrapped tokens");
        IERC20(token).safeTransfer(owner(), amount);
    }

    /**
     * @notice Get bridge balance for a token
     * @param token Token address
     * @return Balance held by bridge
     */
    function getBridgeBalance(address token) external view returns (uint256) {
        return IERC20(token).balanceOf(address(this));
    }

    /**
     * @notice Modifier to restrict function to relayer only
     */
    modifier onlyRelayer() {
        require(msg.sender == relayer, "Only relayer can call");
        _;
    }
}
