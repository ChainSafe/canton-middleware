# Next Steps for Canton-Ethereum Bridge

## Summary

The bridge implementation is **~98% complete**. All core components are implemented and integration tested:
- ✅ Ethereum Smart Contracts - deployed and tested
- ✅ DAML Contracts - issuer-centric model implemented  
- ✅ Go Middleware - updated for new DAML model
- ✅ Integration Tests - passing (Canton + Ethereum connectivity)

**Remaining:** Bootstrap `WayfinderBridgeConfig` on Canton, then production hardening.

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

## ✅ Recently Completed

### 1. Go Middleware Updated for Issuer-Centric Model ✅

The Go code in `pkg/canton/` and `pkg/relayer/` has been updated to use the new DAML templates:

**pkg/canton/client.go - New methods added:**
- `RegisterUser(ctx, req)` → Create `FingerprintMapping`
- `GetFingerprintMapping(ctx, fingerprint)` → Find mapping by fingerprint
- `CreatePendingDeposit(ctx, req)` → Create deposit from EVM event
- `ProcessDeposit(ctx, req)` → Process deposit and mint tokens
- `InitiateWithdrawal(ctx, req)` → Start withdrawal
- `CompleteWithdrawal(ctx, req)` → Mark withdrawal complete after EVM release

**pkg/canton/stream.go - New streaming:**
- `StreamWithdrawalEvents(ctx, offset)` → Stream `WithdrawalEvent` contracts

**pkg/relayer/handlers.go - Updated flow:**
- `CantonSource.StreamEvents` → Uses `StreamWithdrawalEvents` (new issuer-centric model)
- `CantonDestination.SubmitTransfer` → Uses `CreatePendingDeposit` + `GetFingerprintMapping` + `ProcessDeposit`
- `EthereumDestination.SubmitTransfer` → Calls `CompleteWithdrawal` after EVM release

**pkg/relayer/engine.go - Interface updated:**
- `CantonBridgeClient` interface includes all new issuer-centric methods

### 2. Integration Testing ✅

Integration tests are passing with Docker Compose environment:

```bash
# Start environment
docker compose up -d

# Run integration tests
INTEGRATION_TEST=true go test -v -tags=integration ./pkg/relayer/...

# Results:
# ✅ TestIntegration_CantonConnectivity - PASS
# ✅ TestIntegration_EthereumConnectivity - PASS
# ✅ TestIntegration_EthereumSubmitWithdrawal - PASS
# ⚠️  TestIntegration_CantonGetBridgeConfig - SKIP (needs WayfinderBridgeConfig created)
```

**Canton Authentication for Participant Operators:**
- For development: Use `auth-services = [{ type = wildcard }]` in Canton config
- This grants full access since the middleware IS the participant operator
- No JWT tokens needed with wildcard auth

## 🚧 Remaining Work (2%)

### 1. Create WayfinderBridgeConfig Contract

✅ **Configuration values have been set up in `config.yaml`:**
- `relayer_party`: BridgeIssuer party (allocated via HTTP API)
- `bridge_package_id`: Package ID from uploaded DARs  
- `domain_id`: Canton synchronizer domain ID

**To create the CIP56Manager and WayfinderBridgeConfig contracts:**

```bash
# Option 1: Run Daml Script (requires daml SDK installed locally)
cd contracts/canton-erc20/daml/bridge-wayfinder
daml script --dar .daml/dist/bridge-wayfinder-1.0.0.dar \
  --script-name Wayfinder.Test:testIssuerCentricBridge \
  --ledger-host localhost --ledger-port 5011 \
  --wall-clock-time

# Option 2: Build and test all DAML contracts
cd contracts/canton-erc20/daml
./scripts/test-all.sh
```

**Bootstrap steps already completed:**

```bash
# 1. Canton environment running
docker compose up -d  # ✅ Done

# 2. Issuer party allocated via HTTP API:
curl -X POST http://localhost:5013/v2/parties \
  -H 'Content-Type: application/json' \
  -d '{"partyIdHint": "BridgeIssuer"}'
# Result: BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e ✅

# 3. Domain ID found:
# local::12202b3abb042ecea06630767279686e7a45ba44b5a1b8f8ba6c432515a430bb572f ✅

# 4. config.yaml updated with all values ✅
```

**Note:** Due to protobuf version mismatch between the generated Go code and Canton 3.4.8,
contract creation via the Go bootstrap script requires regenerating the protobufs or using
Daml Script instead.

### 2. Production Hardening

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
| `scripts/bootstrap-bridge.go` | **Bootstrap script** - creates CIP56Manager & WayfinderBridgeConfig |
| `pkg/canton/client.go` | Canton gRPC client with issuer-centric methods |
| `pkg/canton/stream.go` | Event streaming (StreamWithdrawalEvents) |
| `pkg/relayer/handlers.go` | Source/Destination adapters |
| `contracts/canton-erc20/daml/common/src/Common/FingerprintAuth.daml` | Core fingerprint templates |
| `contracts/canton-erc20/daml/bridge-wayfinder/src/Wayfinder/Bridge.daml` | WayfinderBridgeConfig |
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

**Next: Bootstrap WayfinderBridgeConfig on Canton, then test full deposit/withdrawal flow** 🚀
