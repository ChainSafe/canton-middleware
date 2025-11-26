# Canton Bridge - Daml Smart Contracts

Daml smart contracts for the Canton-Ethereum token bridge.

## Overview

The Canton bridge contracts handle:
- **Lock/Unlock**: Canton-native CIP-56 tokens bridged to Ethereum (locked in escrow)
- **Mint/Burn**: Wrapped Ethereum tokens on Canton (minted/burned as needed)
- **Deposit Requests**: Users initiate deposits to Ethereum
- **Withdrawal Confirmations**: Relayer confirms withdrawals from Ethereum
- **Replay Protection**: Prevent double-processing of Ethereum transactions

## Contracts

### Bridge
Main bridge contract with operator-controlled privileged operations.

**Fields:**
- `operator`: Bridge operator party (runs the relayer)
- `tokenRef`: Reference to CIP-56 token
- `mode`: AssetMode (LockUnlock or MintBurn)
- `ethChainId`: Ethereum chain ID (1 = mainnet, 11155111 = sepolia)
- `minAmount`: Minimum transfer amount
- `maxAmount`: Maximum transfer amount

**Choices:**
- `InitiateDeposit`: User initiates deposit to Ethereum
- `ConfirmWithdrawal`: Operator confirms withdrawal from Ethereum
- `Pause`: Emergency pause
- `UpdateLimits`: Update min/max transfer amounts

### DepositRequest (Event)
Created when user initiates deposit. Observed by relayer.

**Signatories:** depositor  
**Observers:** operator

### WithdrawalReceipt (Event)
Created when operator confirms withdrawal. Visible to recipient.

**Signatories:** operator  
**Observers:** recipient

### ProcessedEthEvent (Replay Protection)
Tracks processed Ethereum transactions by (chainId, txHash) key.

**Signatories:** operator  
**Key:** (ethChainId, txHash)

### TokenMapping
Maps Canton tokens to Ethereum ERC-20 addresses.

## Setup

### Install Daml SDK

```bash
curl -sSL https://get.daml.com/ | sh
```

### Build

```bash
cd contracts/canton
daml build
```

### Test

```bash
daml test
```

### Generate DAR

```bash
daml build -o canton-bridge.dar
```

## Deployment

### Deploy to Canton Network

```bash
# Upload DAR to participant node
participant1.dars.upload("canton-bridge.dar")

# Verify upload
participant1.dars.list()
```

### Create Bridge Contract

```daml
-- As bridge operator
let tokenRef = TokenRef with
      issuer = operator
      symbol = "USDC"

let bridgeCid = submit operator do
      createCmd Bridge with
        operator
        tokenRef
        mode = LockUnlock
        ethChainId = 1  -- Ethereum mainnet
        minAmount = 0.001
        maxAmount = 10000.0
```

## Integration with Relayer

The Go relayer integrates via gRPC Ledger API:

### 1. Monitor Deposit Requests

```go
// Subscribe to DepositRequest created events
stream := transactionService.GetTransactions(ctx, &GetTransactionsRequest{
    Filter: &TransactionFilter{
        FiltersByParty: map[string]*Filters{
            operatorParty: {
                Inclusive: &Filters_Inclusive{
                    TemplateIds: []*Identifier{
                        {ModuleName: "CantonBridge", EntityName: "DepositRequest"},
                    },
                },
            },
        },
    },
    Begin: offset,
})

// Process each deposit request
for event := range stream {
    // Extract: amount, ethRecipient, tokenRef, mode
    // Mint/unlock on Ethereum
    // Record Ethereum tx hash
}
```

### 2. Confirm Withdrawals

```go
// When Ethereum burn detected, exercise ConfirmWithdrawal
cmdService.SubmitAndWait(ctx, &SubmitAndWaitRequest{
    Commands: &Commands{
        ActAs: []string{operatorParty},
        Commands: []*Command{
            {
                Command: &Command_Exercise{
                    Exercise: &ExerciseCommand{
                        TemplateId: bridgeTemplateId,
                        ContractId: bridgeContractId,
                        Choice: "ConfirmWithdrawal",
                        ChoiceArgument: &Value{
                            Sum: &Value_Record{
                                Record: &Record{
                                    Fields: []*RecordField{
                                        {Label: "ethTxHash", Value: textValue(txHash)},
                                        {Label: "ethSender", Value: textValue(sender)},
                                        {Label: "recipient", Value: partyValue(recipient)},
                                        {Label: "amount", Value: decimalValue(amount)},
                                        {Label: "nonce", Value: int64Value(nonce)},
                                    },
                                },
                            },
                        },
                    },
                },
            },
        },
    },
})
```

## Privacy Model

- **Deposit Requests**: Only depositor and operator can see
- **Withdrawal Receipts**: Only operator and recipient can see
- **Processed Events**: Only operator can see
- **Bridge Config**: Only operator can see

This ensures Canton's sub-transaction privacy where parties only see what they need to know.

## Security

### Replay Protection
ProcessedEthEvent with contract key prevents double-processing of Ethereum transactions.

### Authorization
- Only users can initiate deposits (their own funds)
- Only operator can confirm withdrawals (requires Ethereum validation)
- Only operator can pause/unpause bridge

### Amount Limits
- minAmount and maxAmount enforced on all transfers
- Prevents dust attacks and excessive transfers

### Emergency Controls
- Pause choice stops all bridge operations
- Unpause restores functionality

## Testing

Run the test suite:

```bash
daml test --all
```

Tests cover:
- Bridge creation
- Deposit initiation
- Withdrawal confirmation
- Replay protection (duplicate tx hash)
- Pause/unpause
- Limit updates
- Amount validation

## Integration with CIP-56

This contract provides simplified token references. For production:

1. Import actual CIP-56 modules
2. Use CIP-56 Account.Transfer for lock/unlock
3. Use CIP-56 Issuer.Mint/Burn for wrapped tokens
4. Ensure bridge operator is authorized on CIP-56 contracts

Example integration:

```daml
import qualified CIP56.Account as Account
import qualified CIP56.Issuer as Issuer

-- In InitiateDeposit choice
case mode of
  LockUnlock -> do
    -- Transfer from user account to escrow
    exercise userAccountCid Account.Transfer with
      to = escrowAccountCid
      amount
  MintBurn -> do
    -- Burn user's wrapped tokens
    exercise userAccountCid Account.Burn with amount
```

## License

MIT
