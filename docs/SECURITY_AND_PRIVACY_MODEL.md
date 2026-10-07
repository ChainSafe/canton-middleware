# Security and Privacy Model

How identity is established, how actions are authorized, and what each party can see.

Canton enforces privacy at the ledger. This middleware adds an Ethereum-compatible surface on top of it,
and the two have different properties. This document states both, including where the Ethereum surface is
deliberately more open than the ledger beneath it.

---

## 1. Identity mapping

A user arrives with an Ethereum wallet. Canton has no concept of an Ethereum address, so the two identities
must be bound.

**EVM address to Canton party.** Each registered user is allocated a Canton external party. The binding is
recorded on the ledger in `FingerprintMapping` (`common/FingerprintAuth.daml` in canton-erc20), which
carries the issuer, the user's Canton party, the party's fingerprint, and the user's EVM address. The
issuer is the signatory and the user party is an observer, so the mapping is visible to the user and to the
operator and to nobody else.

**Key derivation.** Canton parties are not derived from keys; they are allocated. For custodial users the
middleware derives a secp256k1 keypair deterministically from the user's EVM address and a server-held
seed (`pkg/keys/canton_keys.go`), and allocates a party against that key. For non-custodial users the key
is derived inside the MetaMask Snap from `snap_getEntropy`, never leaves the browser, and the middleware
only ever sees public material.

**Two independent choices.** Party type (internal or external) and key custody (custodial, Snap, or
institutional) are separate. Every user is an external party. Custody varies per user and is recorded as
`key_mode`. A user registered with `key_mode: external` has no private key stored server side:
`user.NewExternal` (`pkg/user/user.go`) never populates the encrypted key column.

---

## 2. Authorization flow

**Registration.** A user proves control of their EVM address by signing an EIP-191 message
(`personal_sign` prefix, verified in `pkg/auth/evm.go`). The middleware verifies the signature, then either
allocates a party and stores an encrypted key (custodial) or returns a topology transaction for the user to
sign themselves (non-custodial). Registration is gated by an operator whitelist.

**Sessions.** Read endpoints are protected by JWT sessions obtained through Sign-In With Ethereum. A client
fetches a nonce from `/auth/nonce`, signs a SIWE message, and exchanges it at `/auth/login` for a bearer
token. Verification keys are published at `/.well-known/jwks.json`.

**Transaction authorization.** All token movement uses Canton's Interactive Submission API. The middleware
builds the command and calls `PrepareSubmission`, which returns a hash. That hash is signed by whichever
signer the user's `key_mode` selects, then submitted via `ExecuteSubmission`. The middleware never holds
authority to move a non-custodial user's assets: it can prepare a transaction, but without a signature from
the user's key the ledger rejects it.

**Outbound authorization.** Transfers out are additionally checked against a whitelist policy before
submission (`pkg/user/whitelist`), and `eth_sendRawTransaction` is gated by the same check.

---

## 3. Data access restrictions

### 3.1 At the Canton ledger

Canton enforces per-participant visibility. A participant node sees only contracts its parties are
stakeholders on. Concretely:

| Party | Sees |
|---|---|
| A user's party | Their own holdings and the transfers they are party to |
| The operator / issuer party | Contracts it is signatory or observer on, including mappings it issued and holdings of tokens it issues |
| Other Canton participants | Nothing |

Holdings belong to user parties, not to the operator. Balances are never broadcast network-wide.

### 3.2 Two operating modes

**Full visibility.** For a token this deployment issues, the operator is a stakeholder on every holding of
that instrument and can therefore compute complete answers, including total supply and the full holder set.

**Scoped visibility.** For a token issued elsewhere, such as USDCx, the deployment sees only the holdings
its own users hold. It has no view of the instrument's total issuance, and no view of holders on other
participants. Reads that require issuer-level visibility are therefore not answerable, and a total supply
query for such an instrument does not return a network-wide figure. This is a property of the privacy
model, not a limitation of the indexer.

The indexer inherits this boundary rather than implementing it. It subscribes to the Ledger API of one
participant and indexes what that participant can see, which is why each operator runs their own.

### 3.3 At the middleware API

Two surfaces with different properties, and the distinction matters.

**REST endpoints** (`/profile`, transfer history, and other read routes) require a valid JWT bearer token.
A caller sees only their own data.

**The Ethereum JSON-RPC facade** (`/eth`) is not authenticated. This is deliberate: it exists so that
unmodified Ethereum tooling works, and wallets, explorers and libraries do not send bearer tokens. The
consequence is that any caller who can reach the port may issue `eth_call` and `eth_getLogs` for any
address within the deployment's visibility, and will receive an answer.

**So the ledger's user isolation does not extend to the `/eth` surface.** The data returned is still
bounded by what the participant can see, so a caller cannot learn anything the operator could not, but they
are not restricted to their own address.

**Operators are responsible for restricting network access to `/eth`** according to their deployment's
requirements. An operator serving a closed set of institutional users should not expose it publicly.

### 3.4 Internal services

The indexer's HTTP routes are registered as private admin routes. They are unauthenticated by design and
intended for trusted callers on a restricted network. The indexer is not exposed publicly, and callers are
responsible for restricting access to its port. An authenticated public read API is future work.

---

## 4. Key material

| Custody mode | Where the Canton signing key lives | Operator can sign on the user's behalf | Status |
|---|---|---|---|
| Custodial | Server side, AES-256-GCM encrypted under a master key | Yes, by design | Shipping |
| Snap (non-custodial) | Derived inside MetaMask from `snap_getEntropy`, never persisted, never transmitted | No | Implemented, disabled by default |
| Institutional | Held by the custody provider behind the same signer interface | No | Design target, not implemented |

The status column is load-bearing, because two of these three rows describe something a user cannot reach
today. The Snap is complete and tested, but the dApp gates it behind a build-time flag that is off unless
`VITE_ENABLE_NON_CUSTODIAL` is set, so deployed images offer only the custodial path today. The institutional
path has no implementation at all: there is no signer package, no KMS dependency, and no key-reference column
in the schema. It is described here because the signer interface is designed to accept it, not because it
exists. `docs/SIGNER_ARCHITECTURE.md` sets out all three in full.

The Snap requests no `endowment:network-access` permission, which is enforced by the MetaMask platform and
verifiable by inspecting the published package. It therefore cannot transmit key material anywhere,
including to us.

Custodial key custody is a real trust assumption and is stated as one. A custodial user is trusting the
operator with authority over their assets, in exchange for not managing a Canton key. The Snap exists so
that users who do not want to make that trade do not have to, and the institutional path is intended to
serve the same purpose for counterparties who cannot hold their own keys either.

---

## 5. Known limitations

- The `/eth` read surface is unauthenticated, as described in 3.3.
- Total supply and holder-set queries are not answerable for externally issued instruments, as described
  in 3.2.
- The indexer's internal API has no authentication and depends on network restriction.
- Custodial keys for different users share a master encryption key, so compromise of the API process
  together with the master key compromises all custodial users.
