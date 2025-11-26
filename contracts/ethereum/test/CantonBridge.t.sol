// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "../src/CantonBridge.sol";
import "../src/WrappedCantonToken.sol";
import "../src/MockERC20.sol";

contract CantonBridgeTest is Test {
    CantonBridge public bridge;
    MockERC20 public nativeToken;
    WrappedCantonToken public wrappedToken;
    
    address public owner = address(1);
    address public relayer = address(2);
    address public user = address(3);
    
    bytes32 public constant CANTON_TOKEN_ID = bytes32(uint256(0x1234));
    bytes32 public constant CANTON_WRAPPED_TOKEN_ID = bytes32(uint256(0x5678));
    bytes32 public constant CANTON_RECIPIENT = bytes32(uint256(0xabcd));
    
    uint256 public constant MAX_TRANSFER = 1000 ether;
    uint256 public constant MIN_TRANSFER = 0.001 ether;
    
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
    
    function setUp() public {
        vm.startPrank(owner);
        
        // Deploy bridge
        bridge = new CantonBridge(relayer, MAX_TRANSFER, MIN_TRANSFER);
        
        // Deploy native token (ERC-20 that exists on both chains)
        nativeToken = new MockERC20("Native Token", "NATIVE", 18);
        
        // Deploy wrapped token (Canton token wrapped on Ethereum)
        wrappedToken = new WrappedCantonToken(
            "Wrapped Canton Token",
            "wCANTON",
            CANTON_WRAPPED_TOKEN_ID,
            address(bridge)
        );
        
        // Add token mappings
        bridge.addTokenMapping(address(nativeToken), CANTON_TOKEN_ID, false);
        bridge.addTokenMapping(address(wrappedToken), CANTON_WRAPPED_TOKEN_ID, true);
        
        vm.stopPrank();
        
        // Mint tokens to user
        nativeToken.mint(user, 100 ether);
    }
    
    // ============ Constructor Tests ============
    
    function test_Constructor() public {
        assertEq(bridge.relayer(), relayer);
        assertEq(bridge.maxTransferAmount(), MAX_TRANSFER);
        assertEq(bridge.minTransferAmount(), MIN_TRANSFER);
        assertEq(bridge.depositNonce(), 0);
    }
    
    function test_RevertConstructor_InvalidRelayer() public {
        vm.expectRevert("Invalid relayer address");
        new CantonBridge(address(0), MAX_TRANSFER, MIN_TRANSFER);
    }
    
    function test_RevertConstructor_InvalidLimits() public {
        vm.expectRevert("Invalid limits");
        new CantonBridge(relayer, MIN_TRANSFER, MAX_TRANSFER);
    }
    
    // ============ Deposit Tests (Native Token) ============
    
    function test_DepositToCanton_NativeToken() public {
        uint256 amount = 10 ether;
        
        vm.startPrank(user);
        nativeToken.approve(address(bridge), amount);
        
        vm.expectEmit(true, true, true, true);
        emit DepositToCanton(
            address(nativeToken),
            user,
            CANTON_RECIPIENT,
            amount,
            0,
            false
        );
        
        bridge.depositToCanton(address(nativeToken), amount, CANTON_RECIPIENT);
        vm.stopPrank();
        
        // Verify state
        assertEq(bridge.depositNonce(), 1);
        assertEq(nativeToken.balanceOf(address(bridge)), amount);
        assertEq(nativeToken.balanceOf(user), 90 ether);
    }
    
    function test_DepositToCanton_MultipleDeposits() public {
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 30 ether);
        
        bridge.depositToCanton(address(nativeToken), 10 ether, CANTON_RECIPIENT);
        assertEq(bridge.depositNonce(), 1);
        
        bridge.depositToCanton(address(nativeToken), 10 ether, CANTON_RECIPIENT);
        assertEq(bridge.depositNonce(), 2);
        
        bridge.depositToCanton(address(nativeToken), 10 ether, CANTON_RECIPIENT);
        assertEq(bridge.depositNonce(), 3);
        
        vm.stopPrank();
        
        assertEq(nativeToken.balanceOf(address(bridge)), 30 ether);
    }
    
    function test_RevertDeposit_InvalidToken() public {
        vm.startPrank(user);
        vm.expectRevert("Invalid token address");
        bridge.depositToCanton(address(0), 10 ether, CANTON_RECIPIENT);
        vm.stopPrank();
    }
    
    function test_RevertDeposit_BelowMinimum() public {
        vm.startPrank(user);
        nativeToken.approve(address(bridge), MIN_TRANSFER);
        
        vm.expectRevert("Amount below minimum");
        bridge.depositToCanton(address(nativeToken), MIN_TRANSFER - 1, CANTON_RECIPIENT);
        vm.stopPrank();
    }
    
    function test_RevertDeposit_AboveMaximum() public {
        vm.startPrank(user);
        nativeToken.mint(user, MAX_TRANSFER);
        nativeToken.approve(address(bridge), MAX_TRANSFER + 1);
        
        vm.expectRevert("Amount exceeds maximum");
        bridge.depositToCanton(address(nativeToken), MAX_TRANSFER + 1, CANTON_RECIPIENT);
        vm.stopPrank();
    }
    
    function test_RevertDeposit_InvalidCantonRecipient() public {
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 10 ether);
        
        vm.expectRevert("Invalid Canton recipient");
        bridge.depositToCanton(address(nativeToken), 10 ether, bytes32(0));
        vm.stopPrank();
    }
    
    function test_RevertDeposit_TokenNotSupported() public {
        MockERC20 unsupportedToken = new MockERC20("Unsupported", "UNS", 18);
        unsupportedToken.mint(user, 10 ether);
        
        vm.startPrank(user);
        unsupportedToken.approve(address(bridge), 10 ether);
        
        vm.expectRevert("Token not supported");
        bridge.depositToCanton(address(unsupportedToken), 10 ether, CANTON_RECIPIENT);
        vm.stopPrank();
    }
    
    function test_RevertDeposit_WhenPaused() public {
        vm.prank(owner);
        bridge.pause();
        
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 10 ether);
        
        vm.expectRevert();
        bridge.depositToCanton(address(nativeToken), 10 ether, CANTON_RECIPIENT);
        vm.stopPrank();
    }
    
    // ============ Deposit Tests (Wrapped Token) ============
    
    function test_DepositToCanton_WrappedToken() public {
        uint256 amount = 5 ether;
        
        // Mint wrapped tokens to user (simulate previous withdrawal from Canton)
        vm.prank(relayer);
        bridge.withdrawFromCanton(
            address(wrappedToken),
            user,
            amount,
            0,
            bytes32(uint256(0x9999))
        );
        
        // User deposits wrapped tokens back to Canton
        vm.startPrank(user);
        wrappedToken.approve(address(bridge), amount);
        
        vm.expectEmit(true, true, true, true);
        emit DepositToCanton(
            address(wrappedToken),
            user,
            CANTON_RECIPIENT,
            amount,
            0,
            true
        );
        
        bridge.depositToCanton(address(wrappedToken), amount, CANTON_RECIPIENT);
        vm.stopPrank();
        
        // Wrapped tokens should be burned
        assertEq(wrappedToken.balanceOf(user), 0);
        assertEq(wrappedToken.totalSupply(), 0);
    }
    
    // ============ Withdraw Tests (Native Token) ============
    
    function test_WithdrawFromCanton_NativeToken() public {
        // First, deposit some tokens to the bridge
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 10 ether);
        bridge.depositToCanton(address(nativeToken), 10 ether, CANTON_RECIPIENT);
        vm.stopPrank();
        
        // Relayer processes withdrawal
        uint256 amount = 5 ether;
        bytes32 cantonTxHash = bytes32(uint256(0x1111));
        
        vm.startPrank(relayer);
        
        vm.expectEmit(true, true, false, true);
        emit WithdrawFromCanton(
            address(nativeToken),
            user,
            amount,
            0,
            cantonTxHash
        );
        
        bridge.withdrawFromCanton(
            address(nativeToken),
            user,
            amount,
            0,
            cantonTxHash
        );
        vm.stopPrank();
        
        // Verify state
        assertEq(nativeToken.balanceOf(user), 95 ether);
        assertEq(nativeToken.balanceOf(address(bridge)), 5 ether);
        assertTrue(bridge.processedCantonTxs(cantonTxHash));
    }
    
    function test_RevertWithdraw_NotRelayer() public {
        vm.startPrank(user);
        vm.expectRevert("Only relayer can call");
        bridge.withdrawFromCanton(
            address(nativeToken),
            user,
            1 ether,
            0,
            bytes32(uint256(0x2222))
        );
        vm.stopPrank();
    }
    
    function test_RevertWithdraw_AlreadyProcessed() public {
        bytes32 cantonTxHash = bytes32(uint256(0x3333));
        
        // Deposit first
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 20 ether);
        bridge.depositToCanton(address(nativeToken), 20 ether, CANTON_RECIPIENT);
        vm.stopPrank();
        
        // First withdrawal succeeds
        vm.startPrank(relayer);
        bridge.withdrawFromCanton(
            address(nativeToken),
            user,
            5 ether,
            0,
            cantonTxHash
        );
        
        // Second withdrawal with same hash fails
        vm.expectRevert("Already processed");
        bridge.withdrawFromCanton(
            address(nativeToken),
            user,
            5 ether,
            1,
            cantonTxHash
        );
        vm.stopPrank();
    }
    
    function test_RevertWithdraw_WhenPaused() public {
        vm.prank(owner);
        bridge.pause();
        
        vm.startPrank(relayer);
        vm.expectRevert();
        bridge.withdrawFromCanton(
            address(nativeToken),
            user,
            1 ether,
            0,
            bytes32(uint256(0x4444))
        );
        vm.stopPrank();
    }
    
    // ============ Withdraw Tests (Wrapped Token) ============
    
    function test_WithdrawFromCanton_WrappedToken() public {
        uint256 amount = 7 ether;
        bytes32 cantonTxHash = bytes32(uint256(0x5555));
        
        vm.startPrank(relayer);
        
        bridge.withdrawFromCanton(
            address(wrappedToken),
            user,
            amount,
            0,
            cantonTxHash
        );
        vm.stopPrank();
        
        // Wrapped tokens should be minted to user
        assertEq(wrappedToken.balanceOf(user), amount);
        assertEq(wrappedToken.totalSupply(), amount);
        assertTrue(bridge.processedCantonTxs(cantonTxHash));
    }
    
    // ============ Token Mapping Tests ============
    
    function test_AddTokenMapping() public {
        MockERC20 newToken = new MockERC20("New Token", "NEW", 18);
        bytes32 newCantonId = bytes32(uint256(0xaaaa));
        
        vm.startPrank(owner);
        bridge.addTokenMapping(address(newToken), newCantonId, false);
        vm.stopPrank();
        
        assertEq(bridge.ethereumToCantonToken(address(newToken)), newCantonId);
        assertEq(bridge.cantonToEthereumToken(newCantonId), address(newToken));
        assertFalse(bridge.isWrappedToken(address(newToken)));
    }
    
    function test_RevertAddTokenMapping_NotOwner() public {
        MockERC20 newToken = new MockERC20("New Token", "NEW", 18);
        
        vm.startPrank(user);
        vm.expectRevert();
        bridge.addTokenMapping(address(newToken), bytes32(uint256(0xaaaa)), false);
        vm.stopPrank();
    }
    
    function test_RevertAddTokenMapping_AlreadyMapped() public {
        vm.startPrank(owner);
        vm.expectRevert("Token already mapped");
        bridge.addTokenMapping(address(nativeToken), bytes32(uint256(0xbbbb)), false);
        vm.stopPrank();
    }
    
    function test_RemoveTokenMapping() public {
        vm.startPrank(owner);
        bridge.removeTokenMapping(address(nativeToken));
        vm.stopPrank();
        
        assertEq(bridge.ethereumToCantonToken(address(nativeToken)), bytes32(0));
        assertEq(bridge.cantonToEthereumToken(CANTON_TOKEN_ID), address(0));
    }
    
    // ============ Admin Tests ============
    
    function test_UpdateRelayer() public {
        address newRelayer = address(999);
        
        vm.startPrank(owner);
        bridge.updateRelayer(newRelayer);
        vm.stopPrank();
        
        assertEq(bridge.relayer(), newRelayer);
    }
    
    function test_UpdateLimits() public {
        vm.startPrank(owner);
        bridge.updateLimits(2000 ether, 0.002 ether);
        vm.stopPrank();
        
        assertEq(bridge.maxTransferAmount(), 2000 ether);
        assertEq(bridge.minTransferAmount(), 0.002 ether);
    }
    
    function test_PauseUnpause() public {
        vm.startPrank(owner);
        
        bridge.pause();
        assertTrue(bridge.paused());
        
        bridge.unpause();
        assertFalse(bridge.paused());
        
        vm.stopPrank();
    }
    
    function test_EmergencyWithdraw() public {
        // Deposit tokens
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 10 ether);
        bridge.depositToCanton(address(nativeToken), 10 ether, CANTON_RECIPIENT);
        vm.stopPrank();
        
        // Emergency withdraw
        vm.startPrank(owner);
        bridge.pause();
        
        uint256 bridgeBalance = nativeToken.balanceOf(address(bridge));
        bridge.emergencyWithdraw(address(nativeToken), bridgeBalance);
        vm.stopPrank();
        
        assertEq(nativeToken.balanceOf(owner), bridgeBalance);
        assertEq(nativeToken.balanceOf(address(bridge)), 0);
    }
    
    function test_RevertEmergencyWithdraw_NotPaused() public {
        vm.startPrank(owner);
        vm.expectRevert();
        bridge.emergencyWithdraw(address(nativeToken), 1 ether);
        vm.stopPrank();
    }
    
    function test_RevertEmergencyWithdraw_WrappedToken() public {
        vm.startPrank(owner);
        bridge.pause();
        
        vm.expectRevert("Cannot withdraw wrapped tokens");
        bridge.emergencyWithdraw(address(wrappedToken), 1 ether);
        vm.stopPrank();
    }
    
    function test_GetBridgeBalance() public {
        vm.startPrank(user);
        nativeToken.approve(address(bridge), 15 ether);
        bridge.depositToCanton(address(nativeToken), 15 ether, CANTON_RECIPIENT);
        vm.stopPrank();
        
        assertEq(bridge.getBridgeBalance(address(nativeToken)), 15 ether);
    }
    
    // ============ Fuzz Tests ============
    
    function testFuzz_Deposit(uint256 amount) public {
        amount = bound(amount, MIN_TRANSFER, MAX_TRANSFER);
        
        nativeToken.mint(user, amount);
        
        vm.startPrank(user);
        nativeToken.approve(address(bridge), amount);
        bridge.depositToCanton(address(nativeToken), amount, CANTON_RECIPIENT);
        vm.stopPrank();
        
        assertEq(nativeToken.balanceOf(address(bridge)), amount);
    }
    
    function testFuzz_WithdrawNative(uint256 depositAmount, uint256 withdrawAmount) public {
        depositAmount = bound(depositAmount, MIN_TRANSFER, MAX_TRANSFER);
        withdrawAmount = bound(withdrawAmount, MIN_TRANSFER, depositAmount);
        
        nativeToken.mint(user, depositAmount);
        
        // Deposit
        vm.startPrank(user);
        nativeToken.approve(address(bridge), depositAmount);
        bridge.depositToCanton(address(nativeToken), depositAmount, CANTON_RECIPIENT);
        vm.stopPrank();
        
        // Withdraw
        vm.prank(relayer);
        bridge.withdrawFromCanton(
            address(nativeToken),
            user,
            withdrawAmount,
            0,
            bytes32(uint256(0x7777))
        );
        
        assertEq(nativeToken.balanceOf(address(bridge)), depositAmount - withdrawAmount);
    }
}
