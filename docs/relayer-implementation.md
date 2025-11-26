# Relayer Implementation Guide

## Overview

The relayer is a Go application that monitors both Canton Network and Ethereum, facilitating cross-chain token transfers.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Relayer Engine                            │
│                                                               │
│  ┌──────────────────┐         ┌───────────────────┐         │
│  │ Canton Processor │         │ Ethereum Processor│         │
│  │                  │         │                   │         │
│  │ - Stream events  │         │ - Poll for events │         │
│  │ - Parse deposits │         │ - Parse withdrawals│        │
│  │ - Submit cmds    │         │ - Submit txs      │         │
│  └────────┬─────────┘         └─────────┬─────────┘         │
│           │                             │                     │
│           └──────────┬──────────────────┘                     │
│                      │                                        │
│           ┌──────────▼──────────┐                            │
│           │   State Store (DB)  │                            │
│           │  - Pending transfers│                            │
│           │  - Offsets/nonces   │                            │
│           │  - Processed events │                            │
│           └─────────────────────┘                            │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. Canton Client

**File:** `pkg/canton/client.go`

Responsibilities:
- Connect to Canton participant node via gRPC
- Authenticate using JWT tokens
- Stream DepositRequest events
- Submit ConfirmWithdrawal commands
- Handle TLS and connection management

**Key Methods:**
- `StreamTransactions()`: Monitor Canton events
- `SubmitWithdrawalConfirmation()`: Confirm Ethereum→Canton withdrawals
- `GetLedgerEnd()`: Get current ledger offset

### 2. Ethereum Client

**File:** `pkg/ethereum/client.go`

Responsibilities:
- Connect to Ethereum RPC/WebSocket
- Watch for DepositToCanton events
- Submit WithdrawFromCanton transactions
- Manage gas prices and nonces
- Handle transaction signing

**Key Methods:**
- `WatchDepositEvents()`: Monitor Ethereum deposits
- `WithdrawFromCanton()`: Process Canton→Ethereum withdrawals
- `GetTransactor()`: Create transaction signer

### 3. Relayer Engine

**File:** `pkg/relayer/engine.go`

Responsibilities:
- Orchestrate Canton and Ethereum processors
- Load and persist offsets
- Handle reconciliation
- Manage graceful shutdown

**Workflows:**

#### Canton→Ethereum Flow
1. Canton processor streams DepositRequest events
2. Parse event: amount, recipient, token, nonce
3. Check if already processed (DB lookup)
4. Submit Ethereum transaction (mint or unlock)
5. Store transfer in DB with Ethereum tx hash
6. Update Canton offset

#### Ethereum→Canton Flow
1. Ethereum processor polls for DepositToCanton events
2. Wait for confirmations (12-64 blocks)
3. Check if already processed (DB lookup)
4. Exercise Canton ConfirmWithdrawal choice
5. Store transfer in DB with Canton event ID
6. Update Ethereum last block

## Implementation Status

### ✅ Completed
- Project structure
- Database schema and models
- Configuration management
- Logging and metrics
- Canton client skeleton (auth, TLS)
- Ethereum client skeleton (connection, signing)
- Relayer engine orchestration

### 🚧 Pending (Requires Dependencies)

#### Canton Protobuf Generation
```bash
# Generate Go stubs from Daml Ledger API protobufs
./scripts/generate-protos.sh
```

**Required packages:**
- `protoc` compiler
- `protoc-gen-go`
- `protoc-gen-go-grpc`

**After generation:**
1. Import generated types in `canton/client.go`
2. Implement `StreamTransactions()` using `TransactionServiceClient`
3. Implement `SubmitWithdrawalConfirmation()` using `CommandServiceClient`
4. Add Daml Value encoding/decoding helpers

#### Ethereum Contract Bindings
```bash
# Generate Go bindings from Solidity contracts
cd contracts/ethereum
forge build
abigen --abi out/CantonBridge.sol/CantonBridge.json --pkg contracts --out ../../pkg/ethereum/contracts/bridge.go
```

**After generation:**
1. Import bindings in `ethereum/client.go`
2. Implement `WatchDepositEvents()` using contract event filters
3. Implement `WithdrawFromCanton()` using contract method
4. Add error handling for reverted transactions

### 📋 TODO Implementation

#### 1. Canton Event Processing
```go
// In canton/client.go

func (c *Client) StreamTransactions(ctx context.Context, offset string) (<-chan *DepositRequest, error) {
    stream, err := c.transactionService.GetTransactions(ctx, &lapi.GetTransactionsRequest{
        LedgerId: c.config.LedgerID,
        Begin: &lapi.LedgerOffset{Value: &lapi.LedgerOffset_Absolute{Absolute: offset}},
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
    
    depositCh := make(chan *DepositRequest)
    go c.processStream(stream, depositCh)
    return depositCh, nil
}
```

#### 2. Canton Command Submission
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
                            ChoiceArgument: encodeWithdrawalArgs(req),
                        },
                    },
                },
            },
        },
    })
    
    return err
}
```

#### 3. Ethereum Event Watching
```go
// In ethereum/client.go

func (c *Client) WatchDepositEvents(ctx context.Context, fromBlock uint64, handler func(*DepositEvent) error) error {
    opts := &bind.FilterOpts{
        Start:   fromBlock,
        Context: ctx,
    }
    
    sink := make(chan *contracts.CantonBridgeDepositToCanton)
    sub, err := c.bridge.WatchDepositToCanton(opts, sink, nil, nil, nil)
    if err != nil {
        return err
    }
    defer sub.Unsubscribe()
    
    for {
        select {
        case event := <-sink:
            if err := handler(convertEvent(event)); err != nil {
                c.logger.Error("Failed to handle deposit event", zap.Error(err))
            }
        case err := <-sub.Err():
            return err
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

#### 4. Daml Value Encoding
```go
// In canton/encoding.go

func encodeWithdrawalArgs(req *WithdrawalRequest) *lapi.Value {
    return &lapi.Value{
        Sum: &lapi.Value_Record{
            Record: &lapi.Record{
                Fields: []*lapi.RecordField{
                    {Label: "ethTxHash", Value: &lapi.Value{Sum: &lapi.Value_Text{Text: req.EthTxHash}}},
                    {Label: "ethSender", Value: &lapi.Value{Sum: &lapi.Value_Text{Text: req.EthSender}}},
                    {Label: "recipient", Value: &lapi.Value{Sum: &lapi.Value_Party{Party: req.Recipient}}},
                    {Label: "amount", Value: encodeDecimal(req.Amount)},
                    {Label: "nonce", Value: &lapi.Value{Sum: &lapi.Value_Int64{Int64: req.Nonce}}},
                },
            },
        },
    }
}

func encodeDecimal(s string) *lapi.Value {
    // Parse string to Decimal representation
    // Canton uses scaled integers for decimals
    return &lapi.Value{Sum: &lapi.Value_Numeric{Numeric: s}}
}
```

## Testing

### Unit Tests
```bash
go test ./pkg/...
```

### Integration Tests
Requires:
- Canton testnet participant node
- Ethereum testnet (Sepolia)
- Test tokens deployed

```bash
# Set environment variables
export CANTON_RPC_URL=https://canton-testnet:4001
export ETHEREUM_RPC_URL=https://sepolia.infura.io/v3/...
export RELAYER_PRIVATE_KEY=...

# Run integration tests
go test -tags=integration ./test/integration/...
```

### End-to-End Test Flow
1. Deploy bridge contracts on both chains
2. Start relayer with testnet config
3. Initiate Canton→Ethereum deposit
4. Verify Ethereum mint transaction
5. Initiate Ethereum→Canton deposit
6. Verify Canton withdrawal receipt
7. Check database state consistency

## Deployment

### Prerequisites
- Canton participant node access
- Ethereum RPC endpoint (Infura, Alchemy)
- PostgreSQL database
- Bridge contracts deployed

### Configuration
```yaml
# config.yaml
canton:
  rpc_url: "canton-node:4001"
  ledger_id: "canton-network"
  application_id: "bridge-relayer"
  relayer_party: "BridgeOperator::abc123..."
  bridge_contract: "00xyz..."
  bridge_package_id: "abc123..."
  tls:
    enabled: true
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
    ca_file: "/path/to/ca.pem"
  auth:
    jwt_secret: "${CANTON_JWT_SECRET}"

ethereum:
  rpc_url: "https://mainnet.infura.io/v3/..."
  chain_id: 1
  bridge_contract: "0x..."
  relayer_private_key: "${ETH_RELAYER_PRIVATE_KEY}"
  confirmation_blocks: 12

database:
  host: "postgres"
  port: 5432
  database: "canton_bridge"
  user: "bridge"
  password: "${DB_PASSWORD}"
```

### Run Relayer
```bash
# Using binary
./bin/relayer -config config.yaml

# Using Docker
docker run -v $(pwd)/config.yaml:/app/config.yaml canton-bridge-relayer

# Using Docker Compose
docker-compose up relayer
```

### Monitoring
- Prometheus metrics: `http://localhost:9090/metrics`
- Health check: `http://localhost:8080/health`
- API status: `http://localhost:8080/api/v1/status`

## Security Considerations

### Key Management
- Store private keys in secure vaults (AWS KMS, HashiCorp Vault)
- Use environment variables, never commit keys
- Rotate keys periodically
- Use HSM for production

### JWT Tokens
- Generate short-lived tokens (1-24 hours)
- Implement automatic refresh
- Minimal claims (only relayer party)

### Transaction Validation
- Verify Ethereum confirmation blocks before processing
- Check Canton transaction finality
- Validate event signatures
- Implement amount limits

### Database Security
- Encrypt sensitive data at rest
- Use SSL connections
- Regular backups
- Access control and auditing

## Troubleshooting

### Canton Connection Issues
```
Error: failed to dial Canton node
```
- Check RPC URL and TLS configuration
- Verify network connectivity
- Check JWT token validity
- Review Canton node logs

### Ethereum Transaction Failures
```
Error: transaction reverted
```
- Check gas limit and price
- Verify relayer has sufficient ETH
- Check bridge contract state (paused?)
- Review Ethereum node logs

### Stuck Transfers
- Check pending transfers: `curl http://localhost:8080/api/v1/transfers?status=pending`
- Review database for failed entries
- Check relayer logs for errors
- Run manual reconciliation

## Next Steps

1. Generate Canton protobufs: `./scripts/generate-protos.sh`
2. Generate Ethereum bindings: `cd contracts/ethereum && make bindings`
3. Implement Canton event streaming
4. Implement Canton command submission
5. Implement Ethereum event watching
6. Add comprehensive error handling
7. Write integration tests
8. Deploy to testnet
9. Security audit
10. Production deployment
