# USDCx End-to-End Support in canton-middleware — Implementation Plan

## Why this work exists

USDCx is the first external (non-ChainSafe-issued) Splice CIP-56 token we have to support, and it exposes a stack of assumptions in the middleware that were silently true for our DEMO/PROMPT tokens but false in general. Until we fix them, MetaMask users with a USDCx balance see zero, transfers fail, and incoming USDCx sits as un-accepted offers indefinitely.

What "external token" forces us to confront, in plain terms:

1. **We don't issue USDCx, so we can't see its TransferFactory by ACS query.** Our existing send path discovers the TransferFactory by querying Canton's ACS as `IssuerParty`. For DEMO/PROMPT we *are* the issuer, so the factory is visible to us. For USDCx the issuer is Circle, hosted on Circle's participant — our queries return nothing. The Splice token-standard provides a public **registrar HTTP API** that returns the factory id, the choice context, and the disclosed contracts needed for a sender to exercise transfer. Wiring this in is what PR #214 attempted.

2. **The choice-context schema PR #214 used is wrong.** PR #214 encodes `choiceContext` as `map[string]string` (Splice `Metadata`). The actual registrar response is `Map Text AnyValue` — Daml's tagged-union value type. Real responses contain `AV_ContractId`, `AV_List`, `AV_Bool` (and likely the rest of the 11-tag ADT for other tokens). Encoding these as `Metadata` produces a Daml type-mismatch at submission, so even after the HTTP plumbing succeeds the exercise fails. We need a full AnyValue encoder, ported from the working reference in [scripts/testing/accept-via-interface.go](scripts/testing/accept-via-interface.go).

3. **Disclosed contracts must round-trip correctly.** The registrar response carries `disclosedContracts` (e.g. `TransferRule`, `InstrumentConfiguration`) that live on Circle's participant. Without them in `PrepareSubmissionRequest.DisclosedContracts`, Canton's interpreter fails with `Missing context entry for: utility.digitalasset.com/transfer-rule`. PR #214's `ConvertDisclosedContracts` doesn't handle the string-form `templateId` ("`<pkgHex>:<Module.Path>:<Entity>`", split-on-`:`-with-limit-3 because module names contain dots) or the empty `synchronizerId` (must fall back to `cfg.Canton.DomainID`).

4. **Transfers on USDCx are 2-phase: offer + accept.** ERC-20 has no accept step — recipients receive passively. USDCx, like all Splice CIP-56 transfers, creates a `TransferOffer` that locks the sender's holding; settlement happens only when the receiver exercises `TransferInstruction_Accept` on the Splice interface. The middleware currently has **no notion of pending offers** — no list, no accept endpoint, no background acceptor. Without this, any USDCx sent to one of our users sits forever in a pending state and `balanceOf` keeps reporting the pre-transfer amount.

5. **Receiving from MetaMask is invisible to MetaMask.** MetaMask only knows about ETH-style "tx settles when included." It can't drive an accept step. So either (a) the server auto-accepts on the user's behalf — easy for **custodial users** because we have the key, (b) the dApp companion site lists pending offers and the user clicks Accept — required for **non-custodial users** because the Snap holds the key. Both paths share the registrar receive-side endpoint and the same prepare/sign/execute pipeline, but differ in who pulls the trigger.

6. **Canton API now requires package *names*, not hex IDs.** Devnet rejects `splice_holding_package_id: 718a0f77...` with `Invalid field packageId: ... but expected a package name`. Our YAML configs ship hex; needs to be `#splice-api-token-holding-v1`. PR #228's `GetHoldingsByParty` (the unified holdings query) is correct in shape but never exercised against USDCx because this config error trips it first. Fixing the config is a one-line change that unblocks runtime verification of USDCx visibility.

7. **There is no `holder-service`, `verifyTransferProof`, or operator-mediated accept involved in basic transfer.** Earlier exploration burned time chasing a `HolderService` onboarding flow that turned out to be xReserve-bridge-only. End-to-end accept on devnet 2026-05-06 used **only** the public registrar HTTP API + Splice interface choice + Interactive Submission — no operator interaction at all. This narrows the receive-flow design considerably: it's just another prepare/sign/execute, parametrized by a different choice context.

### What is empirically validated

End-to-end **receive** ran on chainsafe devnet 2026-05-06 via [scripts/testing/accept-via-interface.go](scripts/testing/accept-via-interface.go). A 10 USDCx transfer from a Loop wallet to a standalone external party hosted on our participant was accepted by:
- POSTing `{"meta":{},"excludeDebugFields":false}` to `https://api.utilities.digitalasset-dev.com/api/token-standard/v0/registrars/{registrar}/registry/transfer-instruction/v1/{cid}/choice-contexts/accept`
- Decoding the `choiceContextData.values` AnyValue map and `disclosedContracts` array
- Exercising `Splice.Api.Token.TransferInstructionV1:TransferInstruction_Accept` with the encoded `extraArgs.context` and the disclosed contracts attached to `PrepareSubmissionRequest`
- Signing with the receiver's external party key via Interactive Submission

`Utility.Registry.Holding.V0.Holding` for 10.00 USDCx is now active on our participant, and an `ExecutedTransfer` confirmation contract exists. No HolderService, no operator accept, no auth.

This script is the executable spec. The middleware port is mechanical translation, not new design.

### The five concrete deltas this plan delivers

| # | Delta | Without it |
|---|---|---|
| 1 | Package-name fix on `splice_holding_package_id` (1 line × N YAMLs) | All Splice interface queries fail on current devnet |
| 2 | Real AnyValue encoder + ChoiceContext/ExtraArgs builder (replaces PR #214's `map[string]string` shim) | USDCx send fails with Daml type mismatch even after the HTTP call lands |
| 3 | Receive-side registrar endpoint + `Accept/Reject/Withdraw` token-client method | Incoming USDCx never settles; user balance permanently stale |
| 4 | `/api/v2/offers/*` HTTP endpoints (list pending, prepare/execute accept) | No way for the dApp to surface or drive accept; non-custodial path completely blocked |
| 5 | Optional auto-accept worker for custodial users (feature-flagged) | Custodial UX requires manual accept clicks; "MetaMask just works" promise broken |

### Scope: who signs what

- **Custodial** users — server holds the Canton key (`CANTON_MASTER_KEY`-encrypted). Server signs everything in-process via `prepareAndExecuteAsUser`. Auto-accept worker is feasible.
- **Non-custodial** users — Canton key lives in canton-snap; server runs `PrepareSubmission`, returns the `PreparedTransactionHash` to the dApp; Snap signs via `canton_signHash`; server runs `ExecuteSubmission`. The Snap's signing surface is op-agnostic — no Snap-side changes needed for accept/reject/withdraw.

### What does *not* change

- canton-erc20 — bridge-core / CIP-56 templates are already token-agnostic; USDCx flows through Splice interfaces at runtime, not through any of our DARs.
- canton-snap — `canton_signHash` ([packages/snap/src/index.ts:28-57](#)) is a generic 32-byte-hash signer. Accept/reject/withdraw use the same prepare/sign/execute pipeline as transfer. Confirmation-dialog polish (showing op type / amount) is a v2 nice-to-have.
- The MetaMask `eth_sendTransaction` send-side surface — the registrar HTTP call, AnyValue encoding, and DisclosedContracts plumbing all happen server-side. MetaMask is unaware. The only ETH-vs-Canton mismatch is finality semantics (offer creation ≠ settlement); decision deferred to the open questions below.

### Goal

Produce an ordered issue list with the **custodial happy-path shippable first** (Phases 0-2), then the non-custodial accept layered on top (Phase 3). Each phase is one PR.

---

## Scope of changes by repo

| Repo | Required? | Summary |
|---|---|---|
| **canton-middleware** | **Yes — all functional work lives here** | AnyValue encoder, receive-flow endpoints, holdings package-name fix, factory routing fixes, optional auto-accept worker |
| **canton-erc20** (Daml) | **No blocking changes.** Bridge-core / CIP-56 templates are token-agnostic; USDCx is consumed via Splice interfaces at runtime, not via any of our DARs. *Nice-to-have:* doc note that `Wayfinder.Bridge` metadata is PROMPT-specific while `bridge-core` is reusable. | — |
| **canton-snap** | **No blocking changes.** `canton_signHash` ([packages/snap/src/index.ts:28-57](#)) signs any 32-byte hash agnostically; accept/reject/withdraw flow through the same prepare → sign → execute pipeline as a regular transfer. *Nice-to-have:* test-dapp hard-codes DEMO/PROMPT in a `<select>` ([packages/test-dapp/index.html:86](#)); confirmation UI could show op type / amount. Defer to v2. | — |

---

## Current middleware state (verified)

- `pkg/cantonsdk/token/registry_client.go` — has `GetTransferFactory` (send-side only). Receive-side `GetTransferInstructionChoiceContext` does **not** exist.
- `pkg/cantonsdk/values/meta.go:110` — `EncodeExtraArgs(map[string]string)` encodes Splice `Metadata` (TextMap Text). **Wrong shape for choice-context.**
- `pkg/cantonsdk/token/registry_client.go:135` — `ConvertChoiceContext` returns `map[string]string`. **Lossy** for AnyValue tags `AV_ContractId | AV_List | AV_Bool | AV_Int | …`.
- `pkg/cantonsdk/token/client.go:561` — `resolveTransferFactory` correctly routes by `InstrumentAdmin` (local ACS vs. registry) and threads `DisclosedContracts` into `PrepareSubmissionRequest` via `prepareAndExecuteAsUser` ([client.go:691](#)). The pipe is right; what flows through it is wrong.
- `pkg/cantonsdk/token/client.go:281` — `GetHoldingsByParty` queries `HoldingV1` interface using config field `splice_holding_package_id` ([token/config.go:19](#)). Devnet rejects hex package IDs; must be the package **name** `#splice-api-token-holding-v1`.
- `pkg/transfer/http.go:32-33` — only `/api/v2/transfer/prepare` and `/api/v2/transfer/execute` endpoints exist (singular `transfer`). No accept / reject / withdraw / pending-list.
- `pkg/transfer/service.go` + `pkg/cantonsdk/token/client.go:793` — non-custodial: `PrepareTransfer` returns `{TransactionHash, PreparedTransaction}` cached in `PreparedTransferCache` (2-min TTL); `Execute` consumes a DER signature, fingerprint-validated against `users.canton_public_key_fingerprint`.
- `pkg/app/api/server.go:162-189` — custodial: master-key cipher resolves user's encrypted Canton key via `userstore.GetUserKeyByCantonPartyID`, signs in-process.
- `pkg/userstore/model.go` — `users.key_mode` ∈ `{custodial, external}`; both flows already coexist.
- Background workers wired in `pkg/app/api/server.go:100-134` (reconciler ticker, transfer cache cleanup, topology cache) — natural place to mount an auto-accept worker.

---

## Recommended approach

### Phase 0 — Foundations (one PR)

**Issue 1: Fix `splice_holding_package_id` to use package names.**
- Update [pkg/cantonsdk/token/config.go:19](#) doc + all YAML configs (`config.api-server.*.yaml`) from hex hash to `#splice-api-token-holding-v1`.
- Verify `GetHoldingsByParty` ([client.go:277](#)) returns USDCx for the test party `devnet_usdcx::…`.
- Acceptance: `go run scripts/testing/check-holdings.go -party <test>` lists USDCx holdings.

**Issue 2: Add full Splice `AnyValue` encoder.**
- New `pkg/cantonsdk/values/anyvalue.go`: port `encodeAnyValue` from [scripts/testing/accept-via-interface.go:232-331](scripts/testing/accept-via-interface.go#L232-L331). Cover all 11 tags (`AV_ContractId`, `AV_Text`, `AV_Party`, `AV_Bool`, `AV_Int`, `AV_Decimal`, `AV_Date`, `AV_Time`, `AV_RelTime`, `AV_List`, `AV_Map`).
- New `EncodeChoiceContext(map[string]json.RawMessage) (*lapiv2.Value, error)` wrapping `ChoiceContext{ values: TextMap AnyValue }`.
- New `BuildExtraArgs(ctxValue) *lapiv2.Value` wrapping `ExtraArgs{ context, meta }`.
- Unit tests with the live registry response from §1.3 of handoff as fixture.

**Issue 3: Fix registry response conversion.**
- `pkg/cantonsdk/token/registry_client.go`: replace `ConvertChoiceContext map[string]string` with `map[string]json.RawMessage` (preserves AnyValue envelope).
- `parseTemplateID` ([accept-via-interface.go:385](scripts/testing/accept-via-interface.go#L385)) — port: handles string `<pkg>:<module>:<entity>` (split limit 3) **and** object form.
- `ConvertDisclosedContracts(raw, fallbackDomainID)`: base64-decode `createdEventBlob`, fall back to `cfg.Canton.DomainID` when `synchronizerId` is empty.

### Phase 1 — Custodial send (one PR, on top of Phase 0)

**Issue 4: Wire AnyValue encoding into the send path.**
- Update `resolveTransferFactory` / `transferViaFactory` ([client.go:484-608](#)) to call the new `EncodeChoiceContext` + `BuildExtraArgs` instead of `EncodeExtraArgs(map[string]string)`.
- Confirm `DisclosedContracts` are attached to `PrepareSubmissionRequest`; set `ReadAs: nil` (disclosure provides visibility).
- Don't put `instrument_admin` in YAML — discover it from a USDCx Holding's `instrument.source` and cache, or once at startup via `/api/utilities/v0/contract/instrument-configuration/all`.
- Acceptance: `eth_sendTransaction transfer(receiver, amount)` against USDCx ABI succeeds end-to-end for a custodial user; sender's holding becomes locked, receiver sees a `TransferOffer`.

### Phase 2 — Receive flow, custodial (one PR)

**Issue 5: Add registrar receive-side endpoint.**
- `pkg/cantonsdk/token/registry_client.go`: add
  ```go
  GetTransferInstructionChoiceContext(ctx, baseURL, registrar, instructionCID, action string) (*ChoiceContextResponse, error)
  ```
  where `action ∈ {accept, reject, withdraw}`. Reuses the same response parser as `GetTransferFactory`.

**Issue 6: Add `Accept / Reject / Withdraw` token-client method.**
- New method in [pkg/cantonsdk/token/client.go](#) that:
  1. Calls `GetTransferInstructionChoiceContext`.
  2. Encodes choice context via `BuildExtraArgs`.
  3. Builds `Splice.Api.Token.TransferInstructionV1:TransferInstruction` exercise (`TransferInstruction_Accept`) — packageId is the **interface** package name `#splice-api-token-transfer-instruction-v1`.
  4. Goes through the existing `prepareAndExecuteAsUser` (custodial) / `PrepareTransfer` (non-custodial) split.
- Reference: `acceptViaInterface` in [accept-via-interface.go:435-511](scripts/testing/accept-via-interface.go#L435-L511).

**Issue 7: Pending-offers list query.**
- Helper in `pkg/cantonsdk/token/client.go`: `ListPendingTransferOffers(ctx, party) ([]TransferOffer, error)` via `GetActiveContractsByTemplate` for `#utility-registry-app-v0:Utility.Registry.App.V0.Model.Transfer:TransferOffer` (concrete template — interface query for `TransferInstructionV1` would also work and is more generic; recommend the interface variant for forward-compat across registrars).
- Returns sender, amount, instrumentId, expiresAt for dApp display.

**Issue 8: HTTP endpoints — `/api/v2/offers/*`.**
- Extend [pkg/transfer/http.go:32-34](#) routes:
  - `GET  /api/v2/offers/pending`
  - `POST /api/v2/offers/accept/prepare`, `POST /api/v2/offers/accept/execute`
  - `POST /api/v2/offers/reject/prepare`, `POST /api/v2/offers/reject/execute`
  - `POST /api/v2/offers/withdraw/prepare`, `POST /api/v2/offers/withdraw/execute`
- Reuse `PreparedTransferCache` shape (rename to `PreparedActionCache` if cleaner) with the existing fingerprint-validated execute path.
- Mirror the existing `Prepare` / `Execute` request DTOs.

**Issue 9: Auto-accept worker (custodial users only, behind a feature flag).**
- New package `pkg/app/autoaccept`. Mount alongside reconciler in [pkg/app/api/server.go:100-134](#).
- Loop: every `cfg.AutoAccept.Interval`, `userstore.ListUsers(ctx, key_mode='custodial')`, then per user `ListPendingTransferOffers`, then prepare → in-process sign → execute (no HTTP roundtrip).
- Idempotent: a second accept on an already-archived contract is a benign error — log and continue.
- Config: `auto_accept: { enabled: bool, interval: 30s, per_token_optout: [symbol] }`.
- Open question for user (see below).

### Phase 3 — Non-custodial receive (one PR, layers on Phase 2)

**Issue 10: dApp-driven accept via Snap.**
- No new endpoints — Phase 2's `/offers/accept/prepare` + `/offers/accept/execute` already serve the Snap. Verify response payload includes enough metadata (amount, sender, instrument symbol) for a richer Snap confirmation dialog later. Sign DER via `canton_signHash` exactly as for transfers.
- No Snap RPC additions needed; `canton_signHash` is op-agnostic.
- Acceptance: from test dApp, list pending offers → click Accept → MetaMask Snap prompts → execute → holding flips.

### Phase 4 — Polish (separate, optional)

- USDCx config defaults committed in `config.api-server.*.yaml` (PR #229 already in flight).
- Indexer support for USDCx events (#215) — out of MVP; ACS query bypass is sufficient for balances and pending offers.
- (canton-snap, nice-to-have) Confirmation dialog rendering operation type + amount/symbol; remove hard-coded DEMO/PROMPT select in [packages/test-dapp/index.html:86](#).
- (canton-erc20, nice-to-have) README note clarifying Wayfinder bridge specificity.

---

## Critical files to modify

- [pkg/cantonsdk/values/](pkg/cantonsdk/values/) — add `anyvalue.go`, extend value helpers if needed.
- [pkg/cantonsdk/token/registry_client.go](pkg/cantonsdk/token/registry_client.go) — add receive-side method; fix `ConvertChoiceContext` and `ConvertDisclosedContracts` (string-form templateId, base64 blob).
- [pkg/cantonsdk/token/client.go](pkg/cantonsdk/token/client.go) — wire AnyValue encoder into `transferViaFactory` / `resolveTransferFactory`; add `Accept/Reject/Withdraw` methods + `ListPendingTransferOffers`.
- [pkg/cantonsdk/token/config.go](pkg/cantonsdk/token/config.go) + YAML configs — package names, not hex, for `splice_holding_package_id`.
- [pkg/transfer/http.go](pkg/transfer/http.go) + [pkg/transfer/service.go](pkg/transfer/service.go) — new offer endpoints.
- [pkg/app/api/server.go](pkg/app/api/server.go) — wire auto-accept worker (Issue 9).
- New: `pkg/app/autoaccept/` package.

## Reuse (don't reinvent)

- `values.TextValue / PartyValue / NumericValue / ContractIDValue / ListValue / EmptyMetadata / EncodeInstrumentId` ([pkg/cantonsdk/values/](#)) — building blocks for AnyValue.
- `lapiv2.TextMap_Entry` proto for the `values: TextMap AnyValue` shape.
- `prepareAndExecuteAsUser` ([client.go:691](#)) for custodial; `PreparedTransferCache` for non-custodial — both already handle disclosed contracts and Interactive Submission.
- `userstore.GetUserKeyByCantonPartyID` for in-process custodial signing inside the auto-accept worker.

## Verification

- Unit: encoder round-trips fixture from §1.3 of handoff against canton's `Value` proto.
- Integration / live devnet:
  1. `scripts/testing/check-holdings.go` returns USDCx for `devnet_usdcx::…` (Phase 0 done).
  2. `eth_sendTransaction transfer(...)` for USDCx via test dApp → offer visible to receiver (Phase 1).
  3. `POST /api/v2/offers/accept/*` via dApp + Snap on a pending offer → holding archived, new holding minted (Phase 2 + 3).
  4. With auto-accept enabled + a custodial user, send 1 USDCx from Loop wallet → balance reflects within `interval` (Phase 2 Issue 9).
- Reference truth: a working accept must match the on-ledger effect of `go run scripts/testing/accept-via-interface.go`.

## Open questions for the user (to settle before issues are filed)

1. **Auto-accept for custodial users — yes / no / behind a flag?** Recommendation: behind a flag, default-on for the MVP, per-token opt-out.
2. **Pending-offer UI** — does the dApp roadmap have a place for this list, or does it need to be designed alongside?
3. **Time-to-finality semantics for `eth_sendTransaction`** — return on offer creation (current Ethereum-like behavior, but balance only finalizes on receiver-accept) or block until accept (breaks ETH semantics; can hang)?
4. **Snap-side polling for pending offers** (option C') — defer to v2, or scope into Phase 3?
