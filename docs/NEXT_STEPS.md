# Next Steps for Canton-Ethereum Bridge

## Summary

The bridge implementation is **~80% complete**! All core infrastructure, smart contracts, and client scaffolding is in place.

## ✅ Completed

1. **Smart Contracts**
   - Ethereum: CantonBridge.sol, WrappedCantonToken.sol (30 tests passing)
   - Canton: CantonBridge.daml (8 tests passing)
   
2. **Ethereum Integration**
   - Contract bindings generated with abigen
   - Event monitoring implemented (WatchDepositEvents)
   - Transaction submission implemented (WithdrawFromCanton)
   - Full go-ethereum integration
   
3. **Infrastructure**
   - Database schema and models
   - Configuration management
   - Logging and metrics
   - HTTP API
   - Docker setup

## 🚧 Remaining Work (20%)

### 1. Canton Protobuf Generation

**Prerequisite**: Install protoc and Go plugins

```bash
# Install protoc (if not already installed)
# macOS
brew install protobuf

# Linux
apt-get install -y protobuf-compiler

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

**Generate Canton Ledger API stubs:**

```bash
make generate-protos
```

This will:
- Clone Daml repository (v2.10.2)
- Extract proto files from `ledger-api/grpc-definitions`
- Generate Go code in `pkg/canton/lapi/`

### 2. Implement Canton Client

Once protobufs are generated, update `pkg/canton/client.go`:

```go
import (
    lapi "github.com/chainsafe/canton-middleware/pkg/canton/lapi/com/daml/ledger/api/v1"
)

// Add service clients
type Client struct {
    // ... existing fields ...
    transactionService     lapi.TransactionServiceClient
    commandService         lapi.CommandServiceClient
    activeContractsService lapi.ActiveContractsServiceClient
    ledgerIdentityService  lapi.LedgerIdentityServiceClient
}

// Initialize in NewClient
func NewClient(config *Config, logger *zap.Logger) (*Client, error) {
    // ... existing connection code ...
    
    return &Client{
        // ... existing fields ...
        transactionService:     lapi.NewTransactionServiceClient(conn),
        commandService:         lapi.NewCommandServiceClient(conn),
        activeContractsService: lapi.NewActiveContractsServiceClient(conn),
        ledgerIdentityService:  lapi.NewLedgerIdentityServiceClient(conn),
    }, nil
}
```

### 3. Implement Stream Transactions

```go
func (c *Client) StreamTransactions(ctx context.Context, offset string) (<-chan *DepositRequest, error) {
    authCtx := c.GetAuthContext(ctx)
    
    stream, err := c.transactionService.GetTransactions(authCtx, &lapi.GetTransactionsRequest{
        LedgerId: c.config.LedgerID,
        Begin: &lapi.LedgerOffset{
            Value: &lapi.LedgerOffset_Absolute{Absolute: offset},
        },
        Filter: &lapi.TransactionFilter{
            FiltersByParty: map[string]*lapi.Filters{
                c.config.RelayerParty: {
                    Inclusive: &lapi.Filters_Inclusive{
                        TemplateIds: []*lapi.Identifier{
                            {
                                PackageId:  c.config.BridgePackageID,
                                ModuleName: c.config.BridgeModule,
                                EntityName: "DepositRequest",
                            },
                        },
                    },
                },
            },
        },
    })
    
    depositCh := make(chan *DepositRequest, 10)
    
    go func() {
        defer close(depositCh)
        for {
            resp, err := stream.Recv()
            if err != nil {
                c.logger.Error("Stream error", zap.Error(err))
                return
            }
            
            for _, tx := range resp.Transactions {
                for _, event := range tx.Events {
                    if createdEvent, ok := event.Event.(*lapi.Event_Created); ok {
                        deposit, err := DecodeDepositRequest(
                            event.EventId,
                            tx.TransactionId,
                            createdEvent.Created.CreateArguments,
                        )
                        if err != nil {
                            c.logger.Error("Failed to decode deposit", zap.Error(err))
                            continue
                        }
                        depositCh <- deposit
                    }
                }
            }
        }
    }()
    
    return depositCh, nil
}
```

### 4. Implement Submit Withdrawal

```go
func (c *Client) SubmitWithdrawalConfirmation(ctx context.Context, req *WithdrawalRequest) error {
    authCtx := c.GetAuthContext(ctx)
    
    _, err := c.commandService.SubmitAndWait(authCtx, &lapi.SubmitAndWaitRequest{
        Commands: &lapi.Commands{
            LedgerId:      c.config.LedgerID,
            ApplicationId: c.config.ApplicationID,
            CommandId:     generateUUID(),
            ActAs:         []string{c.config.RelayerParty},
            Commands: []*lapi.Command{
                {
                    Command: &lapi.Command_Exercise{
                        Exercise: &lapi.ExerciseCommand{
                            TemplateId: &lapi.Identifier{
                                PackageId:  c.config.BridgePackageID,
                                ModuleName: c.config.BridgeModule,
                                EntityName: "Bridge",
                            },
                            ContractId: c.config.BridgeContract,
                            Choice:     "ConfirmWithdrawal",
                            ChoiceArgument: EncodeWithdrawalArgs(req),
                        },
                    },
                },
            },
        },
    })
    
    return err
}
```

### 5. Wire Up Relayer Engine

Update `pkg/relayer/engine.go`:

```go
func (e *Engine) processCantonDeposits(ctx context.Context) error {
    depositCh, err := e.cantonClient.StreamTransactions(ctx, e.cantonOffset)
    if err != nil {
        return fmt.Errorf("failed to stream transactions: %w", err)
    }
    
    for deposit := range depositCh {
        // Check if already processed
        existing, _ := e.store.GetTransfer(deposit.EventID)
        if existing != nil {
            continue
        }
        
        // Create transfer record
        transfer := &db.Transfer{
            ID:               deposit.EventID,
            Direction:        db.DirectionCantonToEthereum,
            Status:           db.TransferStatusPending,
            SourceChain:      "canton",
            DestinationChain: "ethereum",
            SourceTxHash:     deposit.TransactionID,
            TokenAddress:     deposit.TokenSymbol,
            Amount:           deposit.Amount,
            Sender:           deposit.Depositor,
            Recipient:        deposit.EthRecipient,
            Nonce:            0, // Will be set on Ethereum
        }
        
        if err := e.store.CreateTransfer(transfer); err != nil {
            e.logger.Error("Failed to create transfer", zap.Error(err))
            continue
        }
        
        // Submit to Ethereum
        if err := e.processCantonToEthereum(ctx, deposit); err != nil {
            e.logger.Error("Failed to process Canton->Ethereum", zap.Error(err))
            metrics.ErrorsTotal.WithLabelValues("canton_processor", "eth_submission").Inc()
        }
    }
    
    return nil
}
```

### 6. Testing Workflow

**Testnet Deployment:**

1. Deploy Daml contracts to Canton testnet:
   ```bash
   cd contracts/canton
   daml build
   # Upload to Canton participant node
   ```

2. Deploy Solidity contracts to Sepolia:
   ```bash
   cd contracts/ethereum
   export PRIVATE_KEY=...
   export SEPOLIA_RPC_URL=...
   forge script script/Deploy.s.sol:DeployScript --rpc-url $SEPOLIA_RPC_URL --broadcast
   ```

3. Configure relayer:
   ```yaml
   canton:
     rpc_url: "https://canton-testnet:4001"
     relayer_party: "BridgeOperator::..."
     bridge_contract: "..."  # From Daml deployment
   ethereum:
     rpc_url: "https://sepolia.infura.io/v3/..."
     bridge_contract: "0x..."  # From Solidity deployment
   ```

4. Run relayer:
   ```bash
   ./bin/relayer -config config.yaml
   ```

**Test Scenarios:**

1. **Canton → Ethereum**:
   - User initiates deposit on Canton
   - Relayer detects DepositRequest event
   - Relayer mints/unlocks tokens on Ethereum
   - Verify Ethereum transaction succeeds

2. **Ethereum → Canton**:
   - User deposits tokens on Ethereum
   - Relayer detects DepositToCanton event
   - Relayer confirms withdrawal on Canton
   - Verify Canton WithdrawalReceipt created

3. **Error Scenarios**:
   - Test insufficient balance
   - Test below minimum amount
   - Test replay protection
   - Test pause/unpause

### 7. Production Checklist

- [ ] Security audit (internal + external)
- [ ] Key management (AWS KMS / HashiCorp Vault)
- [ ] Rate limiting and circuit breakers
- [ ] Comprehensive monitoring and alerting
- [ ] Runbooks for operations
- [ ] Disaster recovery procedures
- [ ] Gradual rollout with transfer limits
- [ ] 24/7 on-call rotation

## Timeline Estimate

- **Protobuf generation**: 1-2 hours
- **Canton client implementation**: 1 day
- **Relayer integration**: 1 day
- **Testing**: 2-3 days
- **Production prep**: 1 week

**Total**: ~2 weeks to production-ready

## Resources

- Canton Docs: https://docs.digitalasset.com/
- Daml Protobufs: https://github.com/digital-asset/daml/tree/main/ledger-api/grpc-definitions
- Go-Ethereum: https://geth.ethereum.org/docs
- Project Status: [PROJECT_STATUS.md](../PROJECT_STATUS.md)

## Support

For questions or issues:
1. Check docs: `docs/canton-integration.md`, `docs/relayer-implementation.md`
2. Review logs: Check relayer logs for errors
3. Database: Query transfers table for stuck transactions
4. Metrics: Check Prometheus metrics at `:9090/metrics`

---

**You're almost there! The hard work is done, just need to connect the final pieces.** 🚀
