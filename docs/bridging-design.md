# Multi-Token Bridging Design (EVM <-> Canton)

Status: proposal
Scope: `pkg/relayer`, `pkg/cantonsdk/bridge`, api-server bridge endpoints

## Problem

The relayer bridges exactly one token (PROMPT) through one mechanism we fully
control: lock/unlock in `CantonBridge.sol` on Ethereum, mint/burn via
`WayfinderBridgeConfig` on Canton. We now need USDCx, whose bridge is owned by
Circle (xReserve): we can never mint or burn it ourselves, we can only
orchestrate Circle's entry points. Future tokens may bring yet other
mechanisms. We want one interface under which all of these coexist.

Current hard single-token assumptions:

- One EVM token address in config (`relayer.Config.EthTokenContract`), stamped
  on every Canton->ETH event (`pkg/relayer/engine/source.go:73`).
- Decimals fixed at 18 (`pkg/relayer/engine/destination.go:20`).
- One Canton mint path (`WayfinderBridgeConfig` in `pkg/cantonsdk/bridge/client.go`);
  the deposit's EVM token address is stored but never used for routing.
- `transfers` table has a free-text `token_address` and no mechanism dimension.
- `Destination.SubmitTransfer` assumes a transfer completes in one synchronous
  call — xReserve deposits take ~15 min of external attestation latency and
  cannot fit that shape.

## Key insight

Do not abstract over lock/mint verbs — mechanisms disagree on those. Abstract
over the one thing every bridge mechanism shares:

> A transfer is a durable record that is advanced by an idempotent step
> function until it reaches a terminal state.

- PROMPT deposit: `detected -> mint on Canton -> completed` (one step).
- USDCx deposit: `detected -> await Circle attestation -> exercise mint (or
  observe auto-mint) -> completed` (multiple steps, external latency).
- Any future token: some other sequence of steps — same engine.

The existing reconcile loop (`engine.go:465-603`) already half-implements this
pattern; the design promotes it from "retry fallback" to the primary execution
model.

## Core interface

```go
// pkg/relayer/bridge.go
type TokenBridge interface {
    // Stable identifier, stored on every transfer row: "wayfinder", "xreserve".
    Key() string

    // Event streams that detect new transfer intents on either chain.
    // Emitted events are tagged with BridgeKey, TokenSymbol, Direction.
    Sources(ctx context.Context) ([]Source, error)

    // Advance one transfer one step. Idempotent: called repeatedly by the
    // engine until Status is terminal. Never blocks on external latency —
    // return the current state and a RetryAfter hint instead.
    Step(ctx context.Context, t *relayer.Transfer) (StepResult, error)
}

type StepResult struct {
    Status     relayer.TransferStatus // pending | in_progress | completed | failed
    Stage      string                 // mechanism-defined: "awaiting_attestation", "minted", ...
    DestTxHash *string
    Metadata   map[string]any         // merged into transfers.metadata (jsonb)
    RetryAfter time.Duration          // 0 = engine default interval
}
```

`Source` keeps its current shape (`StreamEvents`, `GetChainID`,
`ExtractOffset`); `relayer.Event` gains `BridgeKey` and `TokenSymbol`.
`Destination` and the per-direction processors are absorbed into `Step` —
`SubmitTransfer` becomes the single-step case.

Sources exist for **executor** mechanisms: we perform the bridge, so a missed
event means stuck funds — watching is load-bearing. Adapters for
**externally-executed** bridges (xreserve) return no sources at all: Circle
completes the transfer whether or not we observe it, so nothing needs
watching for correctness. Their transfer rows are registered at initiation
time instead — the relayer's ops HTTP service gains an internal
`POST /api/v1/transfers`, called by the api-server when the dapp initiates a
deposit or executes a burn — and `Step` then polls status for the dapp.

## How the interface is used

### Startup wiring (`pkg/app/relayer/server.go`)

The registry is built from config; each `mechanism` string selects an adapter
constructor. This replaces the current hardwired
`NewCantonSource`/`NewEthereumDestination` pairing in `Engine.Start`:

```go
registry := relayer.NewRegistry()
for symbol, tc := range cfg.Bridge.Tokens {
    switch tc.Mechanism {
    case "wayfinder":
        registry.Register(wayfinder.New(symbol, tc, cantonClient.Bridge, ethClient, logger))
    case "xreserve":
        registry.Register(xreserve.New(symbol, tc, ledgerClient, ethClient,
            circle.NewAttestationClient(tc.XReserve.AttestationAPI), logger))
    default:
        return fmt.Errorf("token %s: unknown bridge mechanism %q", symbol, tc.Mechanism)
    }
}
engine := relayerengine.NewEngine(cfg.Bridge, registry, store, metrics, logger)
```

Adding a second wayfinder-style token is a new config entry, no new code.

### Engine: one ingest loop per source, one driver loop for everything

```go
func (e *Engine) Start(ctx context.Context) error {
    for _, b := range e.registry.Bridges() {
        sources, err := b.Sources(ctx)
        if err != nil { return err }
        for _, src := range sources { // may be empty: observer adapters have no sources
            go e.runIngest(ctx, b, src)
        }
    }
    go e.runDriver(ctx)
    return nil
}

// Ingest: detection only. No submission logic lives here anymore.
func (e *Engine) runIngest(ctx context.Context, b TokenBridge, src Source) {
    offset := e.loadOffset(ctx, b.Key(), src.GetChainID())
    events, errs := src.StreamEvents(ctx, offset)
    for ev := range events {
        t := relayer.TransferFromEvent(b.Key(), ev)      // status=pending, stage=""
        _, err := e.store.CreateTransfer(ctx, t)         // ON CONFLICT (id) DO NOTHING
        if err != nil { e.logger.Error(...); continue }
        e.saveOffset(ctx, b.Key(), src.GetChainID(), src.ExtractOffset(ev))
    }
}

// Driver: the only place Step is called. Subsumes Processor.processEvent
// and the reconcile loop.
func (e *Engine) runDriver(ctx context.Context) {
    ticker := time.NewTicker(e.cfg.ProcessingInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
        }
        // non-terminal rows whose next_step_at <= now
        transfers, err := e.store.GetSteppableTransfers(ctx)
        if err != nil { continue }
        for _, t := range transfers {
            b, ok := e.registry.ByKey(t.BridgeKey)
            if !ok {
                e.store.MarkFailed(ctx, t.ID, "no adapter for bridge key")
                continue
            }
            res, err := b.Step(ctx, t)
            if err != nil {
                e.store.RecordStepError(ctx, t.ID, err) // retry_count++, backoff via next_step_at
                continue
            }
            e.store.ApplyStep(ctx, t.ID, res) // status, stage, dest_tx_hash,
                                              // metadata merge, next_step_at = now + RetryAfter
        }
    }
}
```

Retry, exponential backoff, stuck-transfer alerting, and metrics all live in
`runDriver`/`ApplyStep` — written once, shared by every mechanism. Adapters
never loop, sleep, or retry; they inspect `t.Stage`, do at most one unit of
work, and report what happened.

### Transfer lifecycle by mechanism

| | wayfinder deposit | wayfinder withdrawal | xreserve deposit | xreserve withdrawal |
|---|---|---|---|---|
| row created by | `CantonBridge.sol` `DepositToCanton` log (Source — load-bearing) | `WithdrawalEvent` ACS stream (Source — load-bearing) | api-server at initiation (dapp reports deposit tx) | api-server burn endpoint |
| stage flow | `"" -> completed` | `"" -> eth_submitted -> completed` | `"" -> awaiting_attestation -> awaiting_mint -> completed` | `"" -> awaiting_release -> completed` |
| who acts | relayer (operator mints) | relayer (unlock + complete) | Circle + user party (relayer observes; exercises mint only for custodial non-pre-approved) | Circle (relayer observes release) |
| if relayer is down | deposits/withdrawals halt until restart (resume from offset) | same | bridge unaffected; only status display lags | same |

### Registry and config

```yaml
bridge:
  tokens:
    PROMPT:
      mechanism: wayfinder
      evm_address: "0x..."
      decimals: 18
      canton:
        package_id: "#bridge-wayfinder"
        core_package_id: "#bridge-core"
        module: "Wayfinder.Bridge"
    USDCX:
      mechanism: xreserve
      evm_address: "0x..."        # native USDC on Ethereum
      decimals: 6
      canton_instrument_admin: "decentralized-usdc-interchain-rep::1220..."
      xreserve:
        contract: "0x..."          # Circle xReserve on Ethereum
        canton_domain: 10001
        attestation_api: "https://xreserve-api.circle.com"
        utility_backend: "https://api.utilities.digitalasset.com"
```

Startup builds the registry from config: each `mechanism` value maps to a
constructor in `pkg/relayer/bridges/<mechanism>/`. Adding a token that uses an
existing mechanism is config-only; a new mechanism is one new package
implementing `TokenBridge`.

Per-token decimals replace the `decimalPlaces = 18` constant. Amounts stay
strings end-to-end; conversion happens only inside adapters at chain
boundaries.

### Schema changes (`pkg/migrations/relayerdb`)

- `transfers`: add `bridge_key varchar`, `token_symbol varchar`,
  `stage varchar`, `metadata jsonb` (attestation IDs, request UUIDs,
  mechanism-specific breadcrumbs). Keep `token_address` for the EVM address.
- `chain_state`: key offsets by `(bridge_key, chain_id)` — two mechanisms
  watching the same chain must not share a cursor.

## Package layout

```
pkg/relayer/
├── types.go                 # Transfer, Event, TransferStatus, StepResult
│                            #   (+ BridgeKey, TokenSymbol, Stage, Metadata)
├── config.go                # Tokens map[string]TokenConfig {mechanism, evm_address,
│                            #   decimals, canton{...}, xreserve{...}}
├── bridge.go                # TokenBridge + Source interfaces, Registry
├── engine/
│   ├── engine.go            # Start/Stop: spawn ingest loops + driver; readiness
│   ├── ingest.go            # runIngest: Source events -> CreateTransfer, offsets
│   ├── driver.go            # runDriver: Step loop, backoff, ApplyStep, stuck alerts
│   └── metrics.go
├── bridges/
│   ├── wayfinder/           # executor: we run the bridge
│   │   ├── bridge.go        # Key, Sources, Step (deposit + withdrawal stages)
│   │   ├── source_eth.go    # CantonBridge.sol deposit-log watcher
│   │   ├── source_canton.go # WithdrawalEvent ACS stream wrapper
│   │   └── convert.go       # per-token decimals, event/amount mapping
│   └── xreserve/            # observer: Circle runs the bridge
│       ├── bridge.go        # Sources() nil; Step = status polling
│       ├── circle.go        # attestation API client
│       └── ledger.go        # USDCx Holding lookup; BridgeUserAgreement_Mint
│                            #   exercise for custodial non-pre-approved parties
├── store/
│   ├── model.go             # TransferDao (+ bridge_key, token_symbol, stage,
│   │                        #   metadata, next_step_at); ChainStateDao keyed
│   │                        #   (bridge_key, chain_id)
│   ├── pg.go                # + GetSteppableTransfers, ApplyStep, RecordStepError
│   ├── instrumented.go
│   └── metrics.go
├── service/
│   └── http.go              # existing read endpoints + internal
│                            #   POST /api/v1/transfers (observer registration)
└── mocks/                   # regenerated for TokenBridge, Source, BridgeStore
```

File moves/deletions relative to today: `engine/source.go` moves into
`bridges/wayfinder/`; `engine/processor.go` and `engine/destination.go` are
deleted (ingest half -> `ingest.go`, submit half -> the wayfinder `Step`,
retry/reconcile half -> `driver.go`).

Unchanged: `pkg/cantonsdk/bridge/` (becomes the wayfinder adapter's SDK — its
`Bridge` interface is consumed only by `bridges/wayfinder`), `cmd/relayer`,
`pkg/app/relayer/server.go` (rewired to build the registry),
`pkg/ethereum/` (client gains nothing bridge-specific).

Outside `pkg/relayer`: `pkg/bridgeapi/` in the api-server hosts
`DepositQuoter` implementations and the `/api/v2/bridge/*` handlers;
`pkg/migrations/relayerdb/3_multi_token.go` adds the new columns and the
composite chain-state key.

## Adapter 1: `wayfinder` (PROMPT — self-managed lock/mint)

Pure refactor of existing code, no behavior change.

Sources: current `ethereumSource` (deposit events from `CantonBridge.sol`)
and `cantonSource` (`WithdrawalEvent` stream), tagging events with the token
resolved from the deposit's `token` field via the config map — this finally
uses the `ethereumToCantonToken` mappings the Solidity side already has.

```go
// pkg/relayer/bridges/wayfinder/bridge.go
func (w *Bridge) Step(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
    if t.Direction == relayer.DirectionEthereumToCanton {
        return w.stepDeposit(ctx, t)
    }
    return w.stepWithdrawal(ctx, t)
}

// Deposit: single-step, as today. Idempotency guard first, so a re-Step
// after a crash between mint and ApplyStep converges to completed.
func (w *Bridge) stepDeposit(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
    if done, err := w.canton.IsDepositProcessed(ctx, t.SourceTxHash); err != nil {
        return relayer.StepResult{}, err
    } else if done {
        return relayer.StepResult{Status: relayer.StatusCompleted}, nil
    }
    amount := toDecimal(t.Amount, w.token.Decimals) // per-token, not const 18
    dep, err := w.canton.CreatePendingDeposit(ctx, bridge.CreatePendingDepositRequest{
        Fingerprint: t.Recipient, Amount: amount, EvmTxHash: t.SourceTxHash,
    })
    if err != nil {
        return relayer.StepResult{}, err
    }
    minted, err := w.canton.ProcessDepositAndMint(ctx, bridge.ProcessDepositRequest{
        DepositCID: dep.ContractID, MappingCID: dep.MappingCID,
    })
    if err != nil {
        return relayer.StepResult{}, err
    }
    return relayer.StepResult{Status: relayer.StatusCompleted, DestTxHash: &minted.UpdateID}, nil
}

// Withdrawal: two stages. Today CompleteWithdrawal is a post-submit hook the
// retry path silently skips; as a stage it can never be lost.
func (w *Bridge) stepWithdrawal(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
    switch t.Stage {
    case "":
        if done, err := w.eth.IsWithdrawalProcessed(ctx, cantonTxHash(t)); err != nil {
            return relayer.StepResult{}, err
        } else if done {
            return relayer.StepResult{Status: relayer.StatusInProgress, Stage: "eth_submitted"}, nil
        }
        hash, err := w.eth.WithdrawFromCanton(ctx, w.token.EVMAddress,
            common.HexToAddress(t.Recipient), toWei(t.Amount, w.token.Decimals),
            big.NewInt(t.Nonce), cantonTxHash(t))
        if err != nil {
            return relayer.StepResult{}, err
        }
        h := hash.Hex()
        return relayer.StepResult{Status: relayer.StatusInProgress, Stage: "eth_submitted", DestTxHash: &h}, nil

    case "eth_submitted":
        err := w.canton.CompleteWithdrawal(ctx, bridge.CompleteWithdrawalRequest{
            WithdrawalEventCID: t.SourceContractID(), EvmTxHash: *t.DestinationTxHash,
        })
        if err != nil {
            return relayer.StepResult{}, err // driver retries; funds already released, only bookkeeping pending
        }
        return relayer.StepResult{Status: relayer.StatusCompleted}, nil
    }
    return relayer.StepResult{}, fmt.Errorf("unknown stage %q", t.Stage)
}
```

`pkg/cantonsdk/bridge` stays as the wayfinder adapter's Canton SDK; it does
not need to generalize.

## Adapter 2: `xreserve` (USDCx — externally-managed by Circle)

Confirmed mechanics (Circle docs, DA docs, as of 2026-07):

- Deposit: anyone calls `depositToRemote(amount, remoteDomain=10001,
  remoteRecipient, localToken, maxFee, hookData)` on Circle's xReserve
  contract on Ethereum; `remoteRecipient = keccak256(cantonPartyId)`, full
  party ID in `hookData`. After ~15 min finality Circle attests; a
  `DepositAttestation` lands on Canton and the recipient party exercises
  `BridgeUserAgreement_Mint` — or it auto-mints if the user's
  `BridgeUserAgreement` has pre-approval enabled.
- Withdrawal: recipient party exercises `BridgeUserAgreement_Burn`
  (destination domain 0 = Ethereum, EVM recipient, request UUID, holding
  CIDs); Circle validates and releases USDC on Ethereum minus a fee.
- Factory/choice-context discovery via the DA Utilities registry endpoint
  (`.../registry/burn-mint-instruction/v0/burn-mint-factory`) — same
  disclosed-contract pattern our `pkg/cantonsdk/token/registry_client.go`
  already implements for USDCx transfers.
- Once minted, USDCx is a plain CIP-56 token; all existing token/indexer
  support applies unchanged.

Adapter mapping:

- Onboarding: each bridging user needs a `BridgeUserAgreement`. Recommend
  enabling pre-approval at registration so inbound deposits auto-mint.
- **No sources — `Sources()` returns nil.** Circle executes the bridge; we
  are not in the critical path, so there is nothing we must watch for
  correctness. Transfer rows are registered at initiation instead:
  - Deposit: the dapp submits `depositToRemote` on Ethereum, then reports the
    tx hash (and deposit nonce) via the api-server, which registers a
    transfer with the relayer for status tracking.
  - Withdrawal: the burn already goes through our api-server (custodial
    signer or prepare/execute), which registers the transfer with the burn
    request UUID on execute.

  Trade-off: bridging done entirely outside our dapp (e.g. Circle's hosted
  deposit UI) won't appear in our transfer history — acceptable; the indexer
  still reflects the resulting USDCx balance.

```go
// pkg/relayer/bridges/xreserve/bridge.go
func (x *Bridge) Step(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
    if t.Direction == relayer.DirectionEthereumToCanton {
        return x.stepDeposit(ctx, t)
    }
    return x.stepWithdrawal(ctx, t)
}

// Deposit: Circle and the user party do the work; with pre-approval this
// adapter mostly observes. Same Step signature, radically different body —
// that is the point of the abstraction.
func (x *Bridge) stepDeposit(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
    switch t.Stage {
    case "", "awaiting_attestation":
        att, err := x.circle.GetAttestation(ctx, t.SourceTxHash, t.Metadata["deposit_nonce"])
        if errors.Is(err, circle.ErrNotReady) { // Ethereum finality ~15 min
            return relayer.StepResult{Status: relayer.StatusInProgress,
                Stage: "awaiting_attestation", RetryAfter: time.Minute}, nil
        }
        if err != nil {
            return relayer.StepResult{}, err
        }
        return relayer.StepResult{Status: relayer.StatusInProgress, Stage: "awaiting_mint",
            Metadata: map[string]any{"attestation_id": att.ID}}, nil

    case "awaiting_mint":
        // Pre-approved parties auto-mint. For custodial parties without
        // pre-approval, exercise BridgeUserAgreement_Mint here (factory +
        // disclosed contracts from the DA Utilities burn-mint-factory
        // endpoint, same pattern as registry_client.go).
        holding, err := x.ledger.FindUSDCxHoldingForDeposit(ctx, t.Recipient, t.Amount, t.SourceTxHash)
        if err != nil {
            return relayer.StepResult{}, err
        }
        if holding == nil {
            return relayer.StepResult{Status: relayer.StatusInProgress,
                Stage: "awaiting_mint", RetryAfter: 30 * time.Second}, nil
        }
        return relayer.StepResult{Status: relayer.StatusCompleted, DestTxHash: &holding.UpdateID}, nil
    }
    return relayer.StepResult{}, fmt.Errorf("unknown stage %q", t.Stage)
}

// Withdrawal: the burn is NOT submitted here — BridgeUserAgreement_Burn is a
// user-party choice, exercised by the api-server (custodial signer or
// prepare/execute for external keys), which registers this transfer on
// execute. The adapter only tracks Circle's release.
func (x *Bridge) stepWithdrawal(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
    switch t.Stage {
    case "", "awaiting_release":
        release, err := x.eth.FindXReserveRelease(ctx, t.Metadata["burn_request_id"])
        if err != nil {
            return relayer.StepResult{}, err
        }
        if release == nil {
            return relayer.StepResult{Status: relayer.StatusInProgress,
                Stage: "awaiting_release", RetryAfter: time.Minute}, nil
        }
        h := release.TxHash.Hex()
        return relayer.StepResult{Status: relayer.StatusCompleted, DestTxHash: &h}, nil
    }
    return relayer.StepResult{}, fmt.Errorf("unknown stage %q", t.Stage)
}
```

Authorization caveat — the biggest structural difference from PROMPT:
`BridgeUserAgreement_Mint/_Burn` are **user-party choices, not operator
choices**. For custodial users the middleware signs directly. For
external-key users, burns must go through the existing prepare/execute
signing flow in the api-server; the relayer then only tracks progress. This
is why `Step` must tolerate stages it merely observes rather than drives.

## API surface (dapp)

There are currently no bridge endpoints (PROMPT withdrawals are initiated by
scripts only). Add token-agnostic endpoints to the api-server, dispatching
through the same registry:

Design rule: **the dapp never encodes a bridge transaction.** Different
tokens need different EVM calls (PROMPT: `approve` + `depositToCanton` on our
`CantonBridge`; USDCx: `approve` + `depositToRemote` on Circle's xReserve
with domain 10001, keccak-hashed recipient, party ID in hookData). Instead of
teaching the dapp each shape, the api-server returns a **quote**: fully
ABI-encoded unsigned transactions the wallet just signs and sends. This is
the EVM mirror of the Canton prepare/execute pattern — server builds, user
signs.

- `GET  /api/v2/bridge/tokens` — supported tokens, mechanism, limits, fees,
  indicative ETA.
- `POST /api/v2/bridge/deposit/quote` — body `{token, amount}`; the
  authenticated user's Canton party is resolved server-side (SIWE session ->
  userstore), so recipient encoding never leaks to the dapp. Response:

  ```json
  {
    "quote_id": "q_7f3a...",
    "chain_id": 1,
    "steps": [
      {"kind": "approve", "to": "0x<token>",   "data": "0x095ea7b3...", "value": "0"},
      {"kind": "deposit", "to": "0x<bridge>",  "data": "0x...",         "value": "0"}
    ],
    "fees": {"bridge_fee": "0", "currency": "USDC"},
    "estimated_seconds": 900,
    "expires_at": "2026-07-11T12:34:56Z"
  }
  ```

  The `approve` step is included only when the current on-chain allowance is
  insufficient (server checks via its eth client). `estimated_seconds` and
  `fees` come from the mechanism: seconds and zero fee for wayfinder, ~15 min
  and Circle's withdrawal fee for xreserve. Quote params are persisted
  against `quote_id` so the later registration can be validated against what
  was quoted. (Future optimization: EIP-2612 permit to collapse the approve
  step — USDC supports it.)
- `POST /api/v2/bridge/deposits` — body `{quote_id, tx_hash}`: registers the
  transfer for status tracking. No-op-ish for wayfinder (the Source detects
  it independently); required for xreserve, where nothing watches the chain.
- `POST /api/v2/bridge/withdraw/prepare|execute` — mechanism-dispatched
  Canton-side signing: wayfinder builds `InitiateWithdrawal`; xreserve builds
  `BridgeUserAgreement_Burn`. Execute also registers the transfer.
- `GET  /api/v2/bridge/transfers?address=...` and `/{id}` — unified status
  (`status` + `stage`).

Dapp flow for any token, present and future:
`GET tokens` -> pick token/amount -> `POST deposit/quote` -> sign & send each
step -> `POST deposits` -> poll `GET transfers/{id}`. Adding a token changes
the token list, nothing else.

Server-side, quoting is a second, api-server-local interface — quote
construction is stateless ABI encoding and doesn't belong in the relayer:

```go
// pkg/bridgeapi (api-server), one implementation per mechanism,
// built from the same per-token config as the relayer registry.
type DepositQuoter interface {
    DepositQuote(ctx context.Context, req QuoteRequest) (*Quote, error)
}
```

## Phasing

1. **Refactor (no behavior change).** Introduce `TokenBridge`, registry,
   driver loop; port PROMPT into `bridges/wayfinder`; migrate schema and
   config to per-token maps. Existing e2e tests must pass unchanged.
2. **USDCx inbound.** `bridges/xreserve` deposit tracking (no sources —
   rows registered via `POST /api/v2/bridge/deposits`) with pre-approval
   auto-mint; `BridgeUserAgreement` onboarding at user registration;
   attestation polling; devnet simulation (devstack has no real Circle — stub
   the attestation API + registrar, as `usdcx-registry` already does for
   transfers).
3. **USDCx outbound + API.** Burn via prepare/execute for external keys,
   direct exercise for custodial; the `/api/v2/bridge/*` endpoints including
   deposit quotes (`DepositQuoter` per mechanism); dapp integration for both
   tokens.
4. **Hardening.** Fees/limits per token, stuck-transfer alerting on external
   stages (Circle outage = transfers parked in `awaiting_attestation`),
   mainnet config (needs DA Utilities onboarding + mainnet xReserve address).

## Risks / open questions

- **Mainnet access**: MainNet USDCx bridge ops require DA Utilities
  onboarding and a validator node; the documented Daml choice names are
  devnet-scoped and may drift.
- **Unknowns to pin down**: Ethereum mainnet xReserve address, attestation API
  auth/schema, Circle fee schedule.
- **Finality UX**: xReserve inbound is ~15 min and outbound timing is
  Circle-controlled; the dapp must present `stage` honestly rather than
  implying tx-inclusion = settled (known MetaMask-semantics gap from
  `usdcx.md`).
- **Devnet fidelity**: local devstack cannot run real xReserve; phase 2
  includes a stub attester so e2e tests can exercise the state machine.
