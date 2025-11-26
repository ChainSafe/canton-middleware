# Canton-Ethereum Bridge - Project Status

## Executive Summary

A **centralized token bridge** connecting Canton Network (CIP-56 tokens) with Ethereum Mainnet (ERC-20 tokens), implemented as a single relayer node running as a sidecar to a Canton Network Partner Node.

**Implementation Progress: ~80% Complete** ✅

## ✅ Completed Components

### Phase 1: Foundation & Setup
- [x] Project structure (Go + Foundry + Daml)
- [x] PostgreSQL database schema (transfers, chain state, nonces)
- [x] Configuration management (Viper)
- [x] Logging (Zap) and metrics (Prometheus)
- [x] Docker compose setup
- [x] Makefile build system
- [x] Documentation structure

### Phase 2: Smart Contracts

#### Ethereum (Solidity)
- [x] **CantonBridge.sol** - Main bridge contract
  - Lock/unlock for native tokens
  - Mint/burn for wrapped tokens
  - Relayer authorization
  - Pause/unpause
  - Replay protection
  - Transfer limits
- [x] **WrappedCantonToken.sol** - ERC-20 wrapper for Canton tokens
- [x] **30 passing tests** (Foundry)
- [x] Deployment scripts
- [x] Gas optimized (~1.15M deployment, ~28-93k operations)

#### Canton (Daml)
- [x] **CantonBridge.daml** - Bridge contract
  - Bridge template (lock/unlock, mint/burn)
  - DepositRequest events (user→relayer)
  - WithdrawalReceipt events (relayer→user)
  - ProcessedEthEvent (replay protection via contract key)
  - TokenMapping configuration
  - Pause/unpause, limit updates
- [x] **BridgeTest.daml** - 8 comprehensive tests
- [x] Privacy model (only relevant parties see transactions)
- [x] Integration guide

### Phase 3: Go Relayer

#### Core Infrastructure
- [x] **Canton Client** (`pkg/canton/`)
  - gRPC connection with TLS
  - JWT authentication
  - Type definitions
  - Client skeleton (ready for protobufs)
  
- [x] **Ethereum Client** (`pkg/ethereum/`)
  - go-ethereum integration
  - Transaction signing
  - Gas management
  - Type definitions
  - Client skeleton (ready for bindings)
  
- [x] **Relayer Engine** (`pkg/relayer/`)
  - Dual-chain event processors
  - State persistence
  - Offset management
  - Reconciliation
  - Graceful shutdown

- [x] **Database Layer** (`pkg/db/`)
  - Transfer tracking
  - Chain state management
  - Nonce tracking
  - Balance monitoring

- [x] **HTTP API** (`cmd/relayer/`)
  - Health checks
  - Status endpoints
  - Transfer queries
  - Prometheus metrics

## 🚧 Pending Implementation (25%)

### 1. Code Generation (Prerequisites)

#### Canton Protobuf Generation
```bash
# Install prerequisites
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate
make generate-protos
```

**Result:** Go types for Canton Ledger API (TransactionService, CommandService, etc.)

#### Ethereum Contract Bindings
```bash
# Install abigen (part of go-ethereum)
go install github.com/ethereum/go-ethereum/cmd/abigen@latest

# Generate
make generate-eth-bindings
```

**Result:** Go bindings for CantonBridge and WrappedCantonToken contracts

### 2. Final Implementation (~3-5 days)

#### Canton Integration
- [ ] Implement `StreamTransactions()` using generated protobuf types
- [ ] Implement `SubmitWithdrawalConfirmation()` with Daml Value encoding
- [ ] Add Daml Value encoding/decoding helpers
- [ ] Parse DepositRequest events from Canton stream
- [ ] Handle Canton transaction finality

#### Ethereum Integration
- [x] Implement `WatchDepositEvents()` using contract bindings
- [x] Implement `WithdrawFromCanton()` transaction submission
- [x] Handle Ethereum confirmations (12-64 blocks)
- [x] Gas price estimation and retry logic

#### End-to-End Flows
- [ ] **Canton→Ethereum**: Stream deposits, submit Ethereum mints/unlocks
- [ ] **Ethereum→Canton**: Poll deposits, exercise Canton ConfirmWithdrawal
- [ ] Error handling and retries
- [ ] Idempotency and deduplication

#### Testing
- [ ] Unit tests for encoding/decoding
- [ ] Integration tests with Canton testnet
- [ ] Integration tests with Sepolia testnet
- [ ] End-to-end flow testing
- [ ] Load testing

### 3. Deployment Preparation

- [ ] Deploy Daml contracts to Canton testnet
- [ ] Deploy Solidity contracts to Sepolia
- [ ] Configure relayer for testnet
- [ ] Run end-to-end tests
- [ ] Security audit (internal)
- [ ] Documentation review
- [ ] Production configuration
- [ ] Monitoring and alerting setup

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Canton Network                            │
│  ┌──────────────┐         ┌─────────────────────────┐      │
│  │ CIP-56 Token │◄────────┤ Canton Bridge Contract  │      │
│  │  (Daml)      │         │ (Lock/Mint/Burn)        │      │
│  └──────────────┘         └──────────▲──────────────┘      │
│                                      │                       │
│                          gRPC Ledger API + JWT              │
└──────────────────────────────────────┼───────────────────────┘
                                       │
                  ┌────────────────────┼────────────────────┐
                  │    Canton-Ethereum Bridge Relayer       │
                  │                                          │
                  │  ┌────────────────────────────────────┐ │
                  │  │ Canton Processor                   │ │
                  │  │ - Stream DepositRequest events     │ │
                  │  │ - Submit ConfirmWithdrawal cmds    │ │
                  │  └────────────────────────────────────┘ │
                  │                                          │
                  │  ┌────────────────────────────────────┐ │
                  │  │ Ethereum Processor                 │ │
                  │  │ - Watch DepositToCanton events     │ │
                  │  │ - Submit WithdrawFromCanton txs    │ │
                  │  └────────────────────────────────────┘ │
                  │                                          │
                  │  ┌────────────────────────────────────┐ │
                  │  │ PostgreSQL State Store             │ │
                  │  │ - Pending transfers                │ │
                  │  │ - Offsets & nonces                 │ │
                  │  │ - Processed events                 │ │
                  │  └────────────────────────────────────┘ │
                  └────────────────────┼────────────────────┘
                                       │
                                 JSON-RPC / WS
┌──────────────────────────────────────┼───────────────────────┐
│                  Ethereum Mainnet    │                       │
│  ┌──────────────┐         ┌──────────▼──────────────┐       │
│  │ ERC-20 Token │◄────────┤ Ethereum Bridge Contract│       │
│  │  (Solidity)  │         │ (Lock/Mint/Burn)        │       │
│  └──────────────┘         └─────────────────────────┘       │
└─────────────────────────────────────────────────────────────┘
```

## Key Features

### Security
- **Replay Protection**: Contract keys (Canton) and tx hash tracking (Ethereum)
- **Relayer Authorization**: Only authorized relayer can process withdrawals
- **Transfer Limits**: Min/max amounts enforced on both chains
- **Emergency Controls**: Pause functionality on both sides
- **Privacy**: Canton sub-transaction privacy (only stakeholders see details)
- **Confirmation Blocks**: 12-64 blocks on Ethereum, configurable on Canton

### Scalability
- **Async Processing**: Independent Canton and Ethereum processors
- **Offset Persistence**: Resume from last checkpoint on restart
- **Database-Backed**: PostgreSQL for reliable state management
- **Reconciliation**: Periodic checks for stuck transfers

### Monitoring
- **Prometheus Metrics**: Transfer counts, latencies, errors, pending counts
- **HTTP API**: Status, health checks, transfer queries
- **Structured Logging**: Zap JSON logging for observability
- **Database Auditing**: Full transfer history and state

## Technology Stack

| Component | Technology |
|-----------|-----------|
| **Canton Contracts** | Daml 2.10.2 |
| **Ethereum Contracts** | Solidity 0.8.20 (Foundry) |
| **Relayer** | Go 1.23 |
| **Canton Client** | gRPC + Daml Ledger API |
| **Ethereum Client** | go-ethereum |
| **Database** | PostgreSQL 15 |
| **Metrics** | Prometheus |
| **Logging** | Zap |
| **Config** | Viper |
| **HTTP** | Chi |
| **Testing** | Go testing, Foundry, Daml Script |

## File Structure

```
canton-middleware/
├── cmd/
│   ├── relayer/main.go          # Main application entry
│   └── tools/                   # CLI utilities
├── contracts/
│   ├── ethereum/                # Solidity contracts (Foundry)
│   │   ├── src/
│   │   │   ├── CantonBridge.sol
│   │   │   ├── WrappedCantonToken.sol
│   │   │   └── MockERC20.sol
│   │   ├── test/CantonBridge.t.sol
│   │   └── script/Deploy.s.sol
│   └── canton/                  # Daml contracts
│       ├── daml/
│       │   ├── CantonBridge.daml
│       │   └── BridgeTest.daml
│       └── daml.yaml
├── pkg/
│   ├── canton/                  # Canton client
│   │   ├── client.go
│   │   └── types.go
│   ├── ethereum/                # Ethereum client
│   │   ├── client.go
│   │   └── types.go
│   ├── relayer/                 # Core relayer logic
│   │   └── engine.go
│   ├── db/                      # Database layer
│   │   ├── models.go
│   │   ├── store.go
│   │   └── schema.sql
│   └── config/                  # Configuration
│       ├── config.go
│       └── logger.go
├── internal/
│   ├── metrics/                 # Prometheus metrics
│   └── security/                # Key management
├── docs/
│   ├── canton-integration.md
│   ├── relayer-implementation.md
│   └── setup.md
├── scripts/
│   └── generate-protos.sh
├── deployments/
│   ├── docker-compose.yaml
│   └── prometheus.yml
├── Makefile
├── go.mod
├── BRIDGE_IMPLEMENTATION_PLAN.md
├── AGENTS.md
└── README.md
```

## Next Steps (Priority Order)

### Immediate (Week 1)
1. **Generate Canton Protobufs** ⚠️ PRIORITY
   - [ ] Install protoc and Go plugins
   - [ ] Run `make generate-protos`
   - [ ] Update imports in `canton/client.go`

2. **Implement Canton Integration**
   - [ ] Complete `StreamTransactions()` implementation
   - [ ] Implement `SubmitWithdrawalConfirmation()`
   - [ ] Add Canton event parsing
   - [x] Daml Value encoding helpers ✅

3. **Ethereum Integration** ✅ COMPLETE
   - [x] Contract bindings generated
   - [x] `WatchDepositEvents()` implemented
   - [x] `WithdrawFromCanton()` implemented
   - [x] Gas management implemented

### Testing (Week 3)
4. **Integration Testing**
   - [ ] Deploy to Canton testnet
   - [ ] Deploy to Ethereum Sepolia
   - [ ] Test Canton→Ethereum flow
   - [ ] Test Ethereum→Canton flow
   - [ ] Test error scenarios
   - [ ] Load testing

### Production Prep (Week 4)
5. **Security & Operations**
   - [ ] Security audit (internal)
   - [ ] Key management (KMS integration)
   - [ ] Monitoring setup
   - [ ] Alerting configuration
   - [ ] Runbooks and documentation

6. **Deployment**
   - [ ] Mainnet contract deployment
   - [ ] Relayer production deployment
   - [ ] Gradual rollout with limits
   - [ ] 24/7 monitoring

## Success Metrics

- [x] Smart contracts compile and pass tests
- [x] Relayer compiles successfully
- [ ] Canton testnet integration working
- [ ] Ethereum testnet integration working
- [ ] 100+ successful transfers on testnet
- [ ] < 5 minute average transfer time
- [ ] 99.9% uptime
- [ ] Zero security incidents
- [ ] Zero lost funds

## Resources

- **Canton Docs**: https://docs.digitalasset.com/
- **Daml Docs**: https://docs.daml.com/
- **Go-Ethereum**: https://geth.ethereum.org/docs
- **Project Plan**: [BRIDGE_IMPLEMENTATION_PLAN.md](BRIDGE_IMPLEMENTATION_PLAN.md)
- **Canton Integration**: [docs/canton-integration.md](docs/canton-integration.md)
- **Relayer Guide**: [docs/relayer-implementation.md](docs/relayer-implementation.md)

## Team & Contact

- Repository: https://github.com/chainsafe/canton-middleware
- Bridge Operator: (Canton party to be created)
- Ethereum Contract: (To be deployed)
- Canton Contract: (To be deployed)

---

**Last Updated**: 2025-01-02  
**Status**: Development Phase (75% Complete)  
**Next Milestone**: Code Generation & Final Implementation
