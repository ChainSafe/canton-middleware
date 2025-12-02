# Next Steps for Canton-Ethereum Bridge

## Summary

The bridge implementation is **~85% complete**. The core infrastructure, Ethereum smart contracts, and DAML contracts are fully implemented and tested. The Go middleware needs to be updated to use the new issuer-centric DAML model.

## ✅ Completed

1. **Ethereum Smart Contracts**
   - CantonBridge.sol, WrappedCantonToken.sol (30 tests passing)
   - Contract bindings generated with abigen
   - Event monitoring (WatchDepositEvents)
   - Transaction submission (WithdrawFromCanton)
   
2. **Canton/DAML Contracts** ✅ UPDATED
   - **Issuer-centric fingerprint model** (see `docs/ISSUER_CENTRIC_MODEL.md`)
   - `FingerprintMapping`: Links Canton fingerprint → Party
   - `PendingDeposit`: Created by middleware from EVM events
   - `DepositReceipt`: Proof of successful deposit
   - `WithdrawalRequest/WithdrawalEvent`: Issuer-controlled withdrawals
   - All tests passing (SDK 3.4.x compatible, no contract keys)

3. **Go Middleware Infrastructure**
   - V2 Protobufs generated (`pkg/canton/lapi/v2`)
   - gRPC client with TLS and JWT support
   - Event streaming framework
   - TransferProcessor pattern
   - Database schema and models
   - Configuration, logging, metrics

## 🚧 Remaining Work (15%)

### 1. Update Go Middleware for Issuer-Centric Model

The Go code in `pkg/canton/` and `pkg/relayer/` needs to be updated to use the new DAML templates:

**pkg/canton/client.go changes:**

```go
// OLD: CreateMintProposal (user must accept)
c.commandService.SubmitAndWait(..., Choice: "CreateMintProposal", ...)

// NEW: Issuer-controlled flow (no user acceptance needed)
// Step 1: Create PendingDeposit
c.commandService.SubmitAndWait(..., Choice: "CreatePendingDeposit", ...)
// Step 2: Process and mint
c.commandService.SubmitAndWait(..., Choice: "ProcessDepositAndMint", ...)
```

**New functions needed:**
- `RegisterUser(party, fingerprint)` → Create `FingerprintMapping`
- `GetFingerprintMapping(fingerprint)` → Find mapping by fingerprint
- `CreatePendingDeposit(fingerprint, amount, txHash)` → Create deposit
- `ProcessDeposit(depositCid, mappingCid)` → Mint tokens
- `InitiateWithdrawal(mappingCid, amount, evmDest)` → Start withdrawal
- `StreamWithdrawalEvents()` → Replace `StreamBurnEvents`

### 2. Integration Testing

**Prerequisite**: Docker Compose environment with Canton + Ethereum devnet.

```bash
# Start environment
docker compose up -d

# Verify DARs are uploaded
docker compose logs canton | grep "Successfully uploaded"

# Run integration tests (once middleware is updated)
go test ./pkg/relayer/... -tags=integration
```

### 3. Production Hardening

- [ ] **Security Audit**: Review Go code and smart contracts
- [ ] **Key Management**: Integrate with AWS KMS or HashiCorp Vault
- [ ] **Rate Limiting**: API endpoints and event processing
- [ ] **Monitoring**: Grafana dashboards for Prometheus metrics
- [ ] **Disaster Recovery**: Backup/restore procedures

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         GO MIDDLEWARE (Relayer)                             │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ On Startup:                                                          │   │
│  │  1. Connect to Canton Ledger API (gRPC :5011)                        │   │
│  │  2. Find/Create WayfinderBridgeConfig                                │   │
│  │  3. Start event listeners                                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ EVM → Canton (Deposit):                                              │   │
│  │  1. Watch DepositToCanton events                                     │   │
│  │  2. Extract fingerprint from bytes32                                 │   │
│  │  3. Call CreatePendingDeposit(fingerprint, amount, txHash)           │   │
│  │  4. Look up FingerprintMapping by fingerprint                        │   │
│  │  5. Call ProcessDepositAndMint(depositCid, mappingCid)               │   │
│  │  6. User receives CIP56Holding                                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Canton → EVM (Withdrawal):                                           │   │
│  │  1. Stream WithdrawalEvent contracts                                 │   │
│  │  2. Call bridge.releaseToEvm(token, recipient, amount)               │   │
│  │  3. Exercise CompleteWithdrawal(evmTxHash)                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Key Files

| File | Purpose |
|------|---------|
| `pkg/canton/client.go` | Canton gRPC client - needs updating |
| `pkg/canton/stream.go` | Event streaming - needs updating |
| `pkg/relayer/handlers.go` | Source/Destination adapters - needs updating |
| `contracts/canton-erc20/daml/common/src/Common/FingerprintAuth.daml` | Core fingerprint templates |
| `contracts/canton-erc20/daml/bridge-wayfinder/src/Wayfinder/Bridge.daml` | Bridge config |
| `contracts/canton-erc20/docs/ISSUER_CENTRIC_MODEL.md` | Architecture docs |

## Resources

- Canton Docs: https://docs.digitalasset.com/
- Daml Protobufs: https://github.com/digital-asset/daml/tree/main/ledger-api/grpc-definitions
- Go-Ethereum: https://geth.ethereum.org/docs
- Issuer-Centric Model: `contracts/canton-erc20/docs/ISSUER_CENTRIC_MODEL.md`

## Support

For questions or issues:
1. Check docs: `docs/canton-integration.md`, `docs/relayer-logic.md`
2. Review logs: Check relayer logs for errors
3. Database: Query transfers table for stuck transactions
4. Metrics: Check Prometheus metrics at `:9090/metrics`

---

**Next: Update Go middleware to use issuer-centric DAML templates** 🔧
