# MetaMask Interactive Testing Guide

The `metamask-test.sh` script prepares a complete test environment and then pauses to let you manually test the Canton bridge using MetaMask. This is the best way to verify the end-user experience.

## Quick Start

```bash
# Start the test (will prepare environment and pause for MetaMask testing)
./scripts/metamask-test.sh --skip-docker

# Or start from scratch (includes Docker startup)
./scripts/metamask-test.sh

# With cleanup after testing
./scripts/metamask-test.sh --cleanup
```

## What the Script Does

### Automated Setup (Steps 1-5)
1. ✅ Starts Docker services (optional)
2. ✅ Waits for all services to be healthy
3. ✅ Whitelists test accounts in database
4. ✅ Registers users and creates Canton parties
5. ✅ Deposits 100 tokens to Canton for User1

### Manual Testing (Step 6)
6. ⏸️  **PAUSES** - You perform transfers via MetaMask

### Automated Verification (Steps 7-8)
7. ✅ Verifies final balances
8. ✅ Tests ERC20 metadata endpoints

## MetaMask Setup

When the script pauses, it will display detailed instructions. Here's what you need to do:

### 1. Add the Local Network

Open MetaMask → Networks → Add Network → Add Manually

- **Network Name**: Canton Local
- **RPC URL**: `http://localhost:8081/eth`
- **Chain ID**: `31337`
- **Currency Symbol**: ETH

### 2. Import Test Accounts

The script displays the private keys for both test accounts:

**User1 (Has tokens)**
```
Private Key: 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
Address: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
```

**User2 (Empty, will receive tokens)**
```
Private Key: 0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d
Address: 0x70997970C51812dc3A010C7d01b50e0d17dc79C8
```

To import:
1. Click the account icon in MetaMask
2. Select "Import Account"
3. Paste the private key
4. Click "Import"

### 3. Add the PROMPT Token

After importing accounts, add the token to MetaMask:

1. Switch to the Canton Local network
2. Click "Import tokens" at the bottom
3. Enter the token details (script displays these):
   - **Token Address**: `0x5FbDB2315678afecb367f032d93F642f64180aa3`
   - **Token Symbol**: PROMPT
   - **Decimals**: 18

You should now see your PROMPT token balance!

## Testing Scenarios

### Test 1: Simple Transfer ✅

**What to test**: Basic token transfer functionality

1. Switch to User1 in MetaMask (should have 100+ PROMPT tokens)
2. Click "Send" on the PROMPT token
3. Enter User2's address: `0x70997970C51812dc3A010C7d01b50e0d17dc79C8`
4. Enter amount: `25`
5. Click "Next" → "Confirm"
6. Wait for confirmation (~5 seconds)
7. Switch to User2 account
8. Verify User2 received 25 PROMPT tokens

**Expected**:
- ✅ Transaction succeeds
- ✅ User1 balance decreases by 25
- ✅ User2 balance increases by 25
- ✅ Transaction appears in activity

### Test 2: Multiple Transfers ✅

**What to test**: Consecutive transfers and balance tracking

1. Send another transfer from User1 to User2
2. Try different amounts (10, 5, 1)
3. Switch to User2 and send some back to User1
4. Verify balances update correctly each time

**Expected**:
- ✅ All transfers succeed
- ✅ Balances always add up correctly
- ✅ No delays or stuck transactions

### Test 3: MetaMask UX ✅

**What to test**: User experience and UI behavior

1. Check transaction details in MetaMask before confirming
2. Verify gas estimation is reasonable
3. Check that transaction history is accurate
4. Test refreshing balances

**Expected**:
- ✅ Gas fees shown (should be minimal on local network)
- ✅ Transaction details are clear
- ✅ History updates properly
- ✅ Balance refreshes work

### Test 4: Error Handling ❌

**What to test**: Proper error messages

1. Try sending more tokens than you have
2. Try sending to an invalid address
3. Try sending 0 tokens

**Expected**:
- ❌ Transaction rejected with clear error
- ❌ No tokens lost
- ❌ MetaMask shows appropriate error message

## Common Issues

### "Transaction failed" or "Insufficient funds"

**Cause**: Not enough tokens in the account
**Solution**: Make sure you're using User1 (which received the deposit) for the first transfer

### "Unknown chain ID"

**Cause**: MetaMask doesn't recognize the network
**Solution**: Make sure you added the network with Chain ID `31337`

### Balance not updating

**Cause**: Canton needs a moment to process transfers
**Solution**: Wait 5-10 seconds and refresh (click the refresh icon in MetaMask)

### Can't see PROMPT token

**Cause**: Token not added to MetaMask
**Solution**: Follow step 3 in MetaMask Setup above

### "RPC Error" or "Cannot connect"

**Cause**: API server not running
**Solution**:
```bash
# Check if services are running
docker ps

# If not, start them
docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml up -d
```

## What to Look For

### ✅ Success Indicators

- Transactions confirm within 5-10 seconds
- Balances update correctly
- No error messages
- Transaction history shows all transfers
- Gas fees are reasonable
- UI is responsive

### ❌ Problems to Report

- Transactions hang or timeout
- Balances don't update
- Error messages that aren't clear
- Gas estimation failures
- UI glitches or freezes

## After Testing

When you're done testing, press **ENTER** in the terminal where the script is running.

The script will then:
1. Fetch final balances
2. Calculate balance changes
3. Show a summary of what changed
4. Test metadata endpoints
5. Clean up (if `--cleanup` flag was used)

## Example Session

```bash
$ ./scripts/metamask-test.sh --skip-docker

══════════════════════════════════════════════════════════════════════
  Canton-Ethereum Bridge MetaMask Interactive Test
══════════════════════════════════════════════════════════════════════

[... automated setup runs ...]

══════════════════════════════════════════════════════════════════════
  Step 6: Manual MetaMask Testing
══════════════════════════════════════════════════════════════════════

MetaMask Setup Instructions
1. Open MetaMask and add the local network...
2. Import test accounts...
3. Add the token to MetaMask...

Manual Transfer Instructions
Test 1: Simple Transfer
  1. Switch to User1 account in MetaMask
  2. Send 25 PROMPT tokens to User2...

════════════════════════════════════════════════════════════════
  Press ENTER when you've finished testing with MetaMask
════════════════════════════════════════════════════════════════

[... you test with MetaMask ...]
[... press ENTER when done ...]

══════════════════════════════════════════════════════════════════════
  Step 7: Verify Final Balances
══════════════════════════════════════════════════════════════════════
✓ User1 final balance: 75
✓ User2 final balance: 25

Balance changes:
  User1: -25 (sent)
  User2: +25 (received)

══════════════════════════════════════════════════════════════════════
  MetaMask Testing Completed!
══════════════════════════════════════════════════════════════════════
```

## Tips for Testing

1. **Keep Both Accounts Open**: Have User1 and User2 in separate browser windows to see real-time updates
2. **Test Edge Cases**: Try sending fractional amounts (0.5, 0.001, etc.)
3. **Check Activity**: Verify all transfers appear in MetaMask activity tab
4. **Refresh Often**: Use the refresh button in MetaMask to see updated balances
5. **Take Notes**: Keep track of any issues or unexpected behavior

## Next Steps

After successful MetaMask testing:
- Try the automated e2e test: `./scripts/e2e-local.sh`
- Review the documentation: `scripts/README-e2e-local.md`
- Test with different networks (devnet, mainnet)
- Integrate with your application
