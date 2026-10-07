# API Documentation

How to integrate against the Canton middleware: what it is, which custody path you want, how to
authenticate, and the order in which to call things.

This is the narrative guide. The field-level reference is the set of OpenAPI 3.1 specifications in
[`docs/openapi/`](openapi/), which carry every route, request body, response and status code. Where the two
disagree, the specs are generated against the implementation and this document is prose, so check the code.

---

## 1. What this API is, and which path you want

Canton Middleware puts an Ethereum-shaped surface in front of Canton Network CIP-56 tokens. One process,
the API server, serves both a REST API for registration, transfers and balances and an Ethereum JSON-RPC
endpoint at `/eth` that unmodified MetaMask and ethers.js talk to directly. Two processes run beside it: a
relayer that bridges PROMPT between an Ethereum chain and Canton, and an indexer that streams ledger events
into Postgres to back the read endpoints.

Every user is allocated a Canton external party, and every token movement goes through Canton's Interactive
Submission API: the server prepares a transaction, something signs the resulting hash, the server submits
it. The only thing that varies between custody modes is who holds the signing key.

### 1.1 The three custody modes

| Mode | Who holds the Canton key | What the user does | What the operator can do | Status |
|---|---|---|---|---|
| Custodial | The server, AES-256-GCM encrypted under a master key | Signs one EIP-191 message per request; never handles a Canton key | Sign on the user's behalf and move their assets | Shipped, and the default |
| External (non-custodial) | The user, derived inside the MetaMask Snap from `snap_getEntropy` | Signs the topology transaction at registration and every transaction hash after | Prepare only; it cannot produce a valid signature | Server routes shipped; reference dApp UI is flag-gated |
| Institutional | A custody provider behind the same signer interface | Not applicable yet | Not applicable yet | Specified only, not implemented |

**Institutional custody is a design, not code.** There is no signer package, no KMS dependency, and no third
mode in `pkg/user/user.go`. Do not plan against it.

**`key_mode` tells you the mode, but not everywhere.** `GET /profile` serialises the stored user and always
carries it, set to `custodial` or `external`. The registration response does not: its field is declared
`json:"key_mode,omitzero"` in `pkg/user/user.go` and the custodial path never sets it, so a custodial
registration comes back with the field absent. Read the mode from the profile, and treat its absence in a
registration response as custodial rather than as an error.

**The non-custodial dApp flow is off in published images.** The API server mounts
`/register/prepare-topology` and the prepare/execute transfer routes unconditionally, so the non-custodial
API is always there. The reference dApp gates its registration-choice screen on the build-time flag
`VITE_ENABLE_NON_CUSTODIAL`, which defaults off (in the canton-snap repository), and the
container build folds that comparison to `false` at minify time, so image deployments cannot switch it on at
runtime (in the canton-snap repository). Calling the API directly is unaffected.

### 1.2 Which path you want

- **An end user with MetaMask and nothing else.** Use the reference dApp. In published images that means a
  custodial account: you sign EIP-191 messages, the operator signs on Canton, you hold no Canton key.
- **A service integrating programmatically.** Drive the REST API. Register the address, sign each transfer
  write with EIP-191 over a timestamped message, and hold a JWT from SIWE login for the read endpoints.
- **You hold your own Canton key.** Register as an external user through the two-call prepare-topology
  handshake, then use prepare/execute for every transfer. The server never sees your private key.

### 1.3 Service surfaces

| Surface | Default port | Authentication | Reference |
|---|---|---|---|
| REST API (`/register`, `/profile`, `/tokens`, `/api/v2/transfer/*`) | 8081 | Mixed; see the authentication section | `docs/openapi/api-server.yaml` |
| Ethereum JSON-RPC (`/eth`), same process and port | 8081 | None, by design | `docs/openapi/api-server.yaml` |
| Splice registry (`/registry/transfer-instruction/v1/transfer-factory`) | 8081 | None | `docs/openapi/api-server.yaml` |
| Admin whitelist (`/admin/whitelist`), mounted when `admin.enabled` | 8081 | Static bearer token | `docs/openapi/api-server.yaml` |
| Indexer read API | 8082 | None; internal, restrict network access | `docs/openapi/indexer.yaml` |
| Relayer status API | 8080 | None; internal, restrict network access | `docs/openapi/relayer.yaml` |

Ports are the defaults shipped in `pkg/config/defaults/`, and each service also serves Prometheus metrics on
its own port. The indexer and relayer have specs because operators run them, not because you call them.

### 1.4 Where to look things up

- **`docs/openapi/*.yaml`** is the field-level reference: every route, body, response and status code.
- **`docs/ARCHITECTURE.md`** covers how the processes fit together and how the two chains stay in step.
- **`docs/SECURITY_AND_PRIVACY_MODEL.md`** states what each party sees and where key material lives.

---

## 2. Authentication

Three mechanisms run on one server, and which one applies depends on the endpoint, not on the client.
Transfer writes use an EIP-191 signature carried in headers, read endpoints use a JWT obtained through
Sign-In With Ethereum, and the `/eth` facade authenticates by recovering the sender from the signed
transaction. Field-level detail for every route is in `docs/openapi/api-server.yaml`.

### 2.1 EIP-191 signature headers

**Two headers, verified on every transfer write.** `X-Message` carries a plain string and `X-Signature`
carries its `personal_sign` signature as 65 bytes of hex. The server recovers the signer with
`VerifyEIP191Signature` (`pkg/auth/evm.go`), normalizes it to a checksummed address, and treats that
address as the acting user. There is no session and no server-side state: every request is signed.

**The message must end with a colon and a Unix timestamp in seconds.** `ValidateTimedMessage`
(`pkg/auth/evm.go`) takes everything after the LAST colon and parses it as seconds. The freshness window
is five minutes (`messageMaxAge` in `pkg/transfer/http.go`) and it is **symmetric**: the handler takes the
absolute difference, so a timestamp five minutes in the future is accepted exactly as one five minutes in
the past. Clock skew on the client is forgiving in both directions.

**The prefix is not validated.** `transfer:1710000000` and `hello:1710000000` are equally acceptable. The
conventional prefix is documentation for your users in the wallet prompt, nothing more. Do not build a
scheme that assumes the server distinguishes a transfer signature from an accept signature, because it
does not. Binding of intent comes from the request body and, for execute, from the Canton prepared hash.

```ts
import { createWalletClient, custom } from "viem";

const [account] = await window.ethereum.request({ method: "eth_requestAccounts" });
const wallet = createWalletClient({ account, transport: custom(window.ethereum) });

const message = `transfer:${Math.floor(Date.now() / 1000)}`;
const signature = await wallet.signMessage({ account, message });

await fetch(`${API}/api/v2/transfer/prepare`, {
  method: "POST",
  headers: { "Content-Type": "application/json", "X-Message": message, "X-Signature": signature },
  body: JSON.stringify({ validity_seconds: 3600, to: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8", amount: "1.5", token: "DEMO" }),
});
```

Registration accepts the same pair, either as these headers or as `signature` and `message` fields in the
JSON body, with the body taking precedence (`pkg/user/service/http.go`). Registration does not apply the
timestamp check.

### 2.2 Sign-In With Ethereum, producing a JWT

**Read endpoints take a bearer token, not a signature header.** Fetch a nonce, sign an EIP-4361 message,
exchange it for a token, then send that token:

```bash
curl -s "$API/auth/nonce?address=0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"   # {"nonce":"..."}
curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"message":"<the signed SIWE message, verbatim>","signature":"0x..."}'  # {"token":"...","expires_at":...}
curl -s "$API/api/v2/transfer/incoming" -H "Authorization: Bearer $TOKEN"
```

Nonces are issued per address and are single use, consumed only after the signature verifies, so a bad
signature cannot burn an outstanding nonce (`pkg/auth/service/login.go`). The verifier checks the
message's chain id, uri, domain and time window against the server's `auth` config
(`pkg/auth/jwt/verifier.go`), so those three values must match the deployment you are talking to. Login
fails with 401 if the recovered address is not a registered user: SIWE authenticates, it does not enroll.

The token is RS256, `sub` is the user's Canton party id and `evm_address` carries the address
(`pkg/auth/jwt/issuer.go`). Lifetime is whatever `auth.token_ttl` says: the code default is 30 minutes,
and all three shipped configurations under `pkg/config/defaults/` set 6 hours. The audience is checked on
every request, and verification keys are published at `/.well-known/jwks.json`.

**A `?address=` that disagrees with the token is a 403, not a silent override** (`callerAddress` in
`pkg/transfer/http.go`). Omit it when you hold a token.

### 2.3 Transaction signature recovery on `/eth`

`/eth` has no header authentication, by design, because unmodified Ethereum tooling does not send bearer
tokens. Reads such as `eth_call` and `eth_getBalance` are answered to anyone who can reach the port.
`eth_sendRawTransaction` authenticates by recovering the sender from the transaction's own signature with
`types.Sender` (`pkg/ethrpc/service/service.go`), then checks that address against the whitelist before
doing any work. Operators are responsible for restricting network access to this port.

### 2.4 Which mechanism applies where

| Endpoint group | Mechanism |
|---|---|
| `POST /api/v2/transfer/*` (prepare, execute, custodial, accept, withdraw) | `X-Signature` + `X-Message` |
| `POST /register`, `POST /register/prepare-topology` | `X-Signature` + `X-Message`, or the same pair in the body |
| `GET /api/v2/transfer/{incoming,outgoing,completed}`, `GET /profile` | `Authorization: Bearer <JWT>` |
| `GET /auth/nonce`, `POST /auth/login`, `GET /.well-known/jwks.json` | None, these issue the credential |
| `GET /tokens`, `/registry/...`, health | None |
| `/eth` | None at the HTTP layer; writes recover the sender from the signed transaction |
| `/admin/whitelist` | Static bearer API key from `admin.api_key` |

### 2.5 The failure mode: read auth is conditional

**When the `auth` config block is absent, the read endpoints are not access-controlled.** `buildReadAuth`
(`pkg/app/api/server.go`) logs a warning and installs a passthrough middleware, and `callerAddress` then
resolves the caller from an unverified `?address=` query parameter. Any caller can read any registered
address's transfer history and profile.

All three shipped configurations supply the block, so this is a misconfiguration rather than a mode. It
matters only because a stripped local config will exhibit it, and an integrator who tests there will build
a client that never acquires a token and then fails against every real deployment. Treat the JWT path as
the only read path, and check the startup log for `read-endpoint authentication is DISABLED` if reads
succeed without one.

---

## 3. Registering a user

Every user is a Canton external party. Registration allocates that party and binds it to an EVM
address through a `FingerprintMapping` contract on the ledger. What differs between the modes is who
holds the Canton signing key, and that choice is fixed for the account: it decides which transfer
endpoints the account may use later.

### 3.1 Custodial

**One call.** `POST /register` with an EIP-191 signature and the signed message, in the body or in the
`X-Signature` and `X-Message` headers. The address is recovered from the signature, never read from the
request, and the message content is not constrained.

```bash
curl -sS -X POST "$API/register" -H 'Content-Type: application/json' \
  -d '{"signature":"0x<eip191-sig>","message":"Register for Canton EVM Middleware"}'
```

**What the server does.** It checks the whitelist, generates a fresh secp256k1 keypair
(`keys.GenerateCantonKeyPair`), allocates an external party signed with that key, creates the
fingerprint mapping, then encrypts the private key and stores it (`pkg/user/service/service.go`).

**What comes back.** `party`, `fingerprint`, `mapping_cid` and `evm_address`. On this path the Canton
private key appears in no response: the user never sees it and cannot sign Canton transactions
themselves.

**The Canton-native form is different, and the difference matters.** The same route also accepts
`canton_party_id` plus `canton_signature`, for a party that already exists elsewhere such as a Loop
wallet. That branch generates an EVM keypair for MetaMask access and returns its private key in the
response body, in `private_key`. It also stores a key server side, so the account still lands
custodial. If you use this form, treat the response as secret material in transit and at rest, and note
that the field is absent on every other registration path because it is declared `omitzero`.

### 3.2 Non-custodial

**Call one: prepare the topology.** `POST /register/prepare-topology` with `canton_public_key`, a
hex-encoded 33-byte compressed secp256k1 public key, plus the EIP-191 signature and message. The server
verifies the signature, rejects an already-registered address with 409, checks the whitelist, converts
the key to SPKI, and has the participant generate the onboarding topology.

```bash
curl -sS -X POST "$API/register/prepare-topology" -H 'Content-Type: application/json' \
  -H 'X-Signature: 0x<eip191-sig>' -H 'X-Message: register-external-1' \
  -d '{"canton_public_key":"02<compressed-pubkey-hex>"}'
```

It returns `topology_hash`, the `0x`-prefixed multi-hash over the onboarding transactions;
`public_key_fingerprint`, the fingerprint the participant derived from the key you sent; and
`registration_token`, a UUID naming the cached topology, good for 5 minutes (`pkg/app/api/server.go`).

**Sign the hash.** SHA-256 the decoded `topology_hash` bytes, then sign that digest with the Canton key:
ECDSA over secp256k1, DER encoded, hex. `scripts/testing/test-prepare-execute.go` does both calls.

**Call two: register.** `POST /register` with `key_mode: "external"`, the same `canton_public_key`, the
`registration_token`, the `topology_signature`, and a fresh EIP-191 signature and message. The server
recovers the address again, checks the whitelist again, and consumes the token atomically: unknown or
already used gives 404, expired gives 410. It re-derives SPKI from the submitted public key and compares
it byte for byte with the key cached in step one, so you cannot prepare with one key and register with
another. The party is allocated with your signature attributed to the fingerprint the server derived in
step one, so a signature from any other key is rejected at the participant. Only the public key ever
crosses the wire, and `user.NewExternal` (`pkg/user/user.go`) never stores an encrypted key.

### 3.3 The whitelist

Both calls check `whitelist.IsWhitelisted` before doing any work, and a non-whitelisted address gets
`403` with `{"error":"address not whitelisted for registration","code":403}`. Operators manage the list
through `/admin/whitelist`. The `skip_whitelist_check` flag makes the gate authorize every address
without touching the store (`pkg/user/whitelist/whitelist.go`), so registration may be open on one
deployment and closed on another. Do not assume either.

### 3.4 Telling the modes apart afterwards

`key_mode` is tagged `omitzero` on the registration response (`pkg/user/user.go`) and the custodial path
leaves it empty, so a successful custodial registration returns **no `key_mode` field at all**, while a
non-custodial one returns `"key_mode":"external"`. Checking for `"custodial"` there is always false.
`GET /profile` is the other way round: it serves the stored record, whose `key_mode` column is
`notnull default 'custodial'` (`pkg/userstore/model.go`), so the field is always present. One rule
holds for both: `external` is the only positive case.

---

## 4. Sending and receiving tokens

A send does not settle on its own. Both custody paths create a `TransferOffer` on Canton that the
recipient must accept, and an offer nobody accepts within `validity_seconds` expires on the ledger
and becomes reclaimable by the sender. Write endpoints authenticate with the EIP-191 `X-Signature`
and `X-Message` headers, read endpoints with the session token, and the field detail is in
`docs/openapi/api-server.yaml`.

### 4.1 Custodial send

`POST /api/v2/transfer/custodial` is one call: the server holds the user's Canton key, so it
prepares and submits in the same request. A `key_mode` other than custodial is a 400, and the
response is `{"status":"submitted"}`, not settled, because the recipient still has to accept.

**The recipient is a party id, never an address.** The body takes `to_party_id`, `amount`, `token`
and `validity_seconds`, with no EVM-address form here. The party id must parse as
`<hint>::<hex fingerprint>` and be neither the sender nor the issuer party, and the sender is always
whitelist-checked. An unregistered party is refused unless the token is marked `external_transfer`
in token config, and then only if the participant's topology knows it (`pkg/transfer/service.go`).

```bash
# Sign this exact string, then send the same string. Generating the timestamp
# twice would sign one message and send another.
MSG="transfer:$(date +%s)"
SIG=$(cast wallet sign --private-key "$PRIVATE_KEY" "$MSG")

curl -X POST https://api.example.org/api/v2/transfer/custodial \
  -H "X-Signature: $SIG" -H "X-Message: $MSG" -H 'Content-Type: application/json' \
  -d '{"to_party_id": "<hint>::<hex-fingerprint>","amount":"10.5","token":"DEMO","validity_seconds":86400}'
```

### 4.2 Non-custodial send

Three steps, and the middleware never sees the key. A `key_mode` other than `external` is a 400.

**Prepare.** `POST /api/v2/transfer/prepare` with `amount`, `token`, `validity_seconds` and exactly
one recipient field: `to`, a registered user's EVM address, or `to_party_id`, any Canton party, a
branch that adds the whitelist and topology checks from 4.1. Both set, or both empty, is a 400
reading `exactly one of to or to_party_id is required`. Back come `transfer_id`, `transaction_hash`
and `expires_at`.

**Sign.** Hex-decode `transaction_hash`, SHA-256 those bytes, then ECDSA-sign the digest on
secp256k1 with the party's Canton key, DER-encoded and hex-encoded. Canton's algorithm is
`EC_DSA_SHA_256`, so do not sign the hash bytes directly. The MetaMask Snap does this behind
`canton_signHash`, which returns both the signature and the fingerprint.

**Execute.** `POST /api/v2/transfer/execute` with `transfer_id`, `signature` and `signed_by`, the
Canton fingerprint registered for the caller. A mismatch is a 403 before the ledger is touched.

**There are two clocks, and the short one is easy to miss.** `validity_seconds` governs the
on-ledger offer and can be days. `expires_at` governs the prepared transaction: prepare time plus
the prepared-transfer cache TTL of two minutes (`transferCacheTTL` in `pkg/app/api/server.go`).
Execute later and the response is 410 Gone. The entry is consumed on read, so a replayed or unknown
`transfer_id` is a 404, not a second submission. Binding a prepared transfer to its preparing party,
so a foreign `transfer_id` is refused outright, is approved but not yet merged.

### 4.3 Receiving

**Non-custodial.** An inbound send arrives as a pending offer. `GET /api/v2/transfer/incoming` gives
`contract_id` and `instrument_admin` per item; `POST /api/v2/transfer/incoming/{contractID}/prepare`
with `{"instrument_admin": "..."}` returns a hash to sign as in 4.2, and
`POST /api/v2/transfer/incoming/{contractID}/execute` takes the usual execute body.

**Custodial.** There is no accept endpoint, and `GET /api/v2/transfer/incoming` returns a 400 for
custodial users. Acceptance is a background job: the worker in `pkg/custodial/accept_worker.go`
polls the indexer and accepts every pending offer addressed to a custodial party. It runs only where
the deployment sets `accept_worker`, which defaults to a ten second poll.

### 4.4 Withdrawing an offer you sent

Same shape as sending, keyed by the offer's contract id. Non-custodial senders call
`POST /api/v2/transfer/outgoing/{contractID}/withdraw/prepare`, sign, then `.../withdraw/execute`;
custodial senders call `.../withdraw/custodial` once. The server resolves instrument routing itself.

**Only the sender may claim back, and only a live offer.** An offer that does not exist, or belongs
to someone else, is a 404 in both cases, deliberately, so contract ids cannot be probed. One already
completed, cancelled, or rejected is a 400, and losing the race to a receiver is a 409.

### 4.5 Reading state

`GET /api/v2/transfer/incoming`, `/outgoing` and `/completed` return the caller's own data, paged by
`?page=` and `?limit=` (page 1 and 50 by default, 200 maximum), with `items`, `total`, `page`,
`limit` and `has_more` in each response. `/outgoing` also takes `?status=`: `pending`, `expired`,
`completed`, `canceled`, `rejected` or `all`, where `accepted` aliases `completed`. Of the three,
only `/incoming` is restricted to external-key users.

**Party ids in these responses are truncated and cannot be fed back in.** Each is rendered as its
first eight characters, a `…`, and its last eight, for example `user_2dA…4680b7ec`. That is
deliberate, so polling an address cannot enumerate counterparties, and it is lossy: passed back as
`to_party_id` it fails validation, being neither a well-formed hint nor a hex fingerprint.

**Use the contract id to act on an offer.** `contract_id` is returned intact and is what the accept
and withdraw routes take. To send to a registered user, put their EVM address in `to`. These
listings never carry a full party id; `GET /profile` returns `canton_party` for the caller alone.

---

## 5. The Ethereum JSON-RPC facade

`/eth` speaks Ethereum JSON-RPC 2.0 and runs no EVM. Each supported call is translated into a CIP-56
operation on Canton: a balance read becomes a holdings query against the caller's Canton party, and a
signed `transfer(address,uint256)` becomes a Canton transfer. The point of the endpoint is that MetaMask
and generic Ethereum libraries work against Canton balances without knowing Canton exists.

It is mounted only when `eth_rpc.enabled` is set (`pkg/app/api/server.go`). Transport is HTTP POST only,
with no WebSocket. Responses are always HTTP 200 and failures arrive as JSON-RPC error objects, because
the endpoint is served by go-ethereum's `rpc.Server` (`pkg/ethrpc/service/http.go`).

### 5.1 Connecting a wallet

**The chain id is configuration, not a constant.** It comes from `eth_rpc.chain_id`, and it differs
between deployments. The three shipped configurations in `pkg/config/defaults/` do not agree with each
other, and a running cluster may differ from all of them. Do not hardcode one. Ask the endpoint:

```bash
curl -s -X POST http://localhost:8081/eth \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'
```

**The native currency is cosmetic.** `eth_getBalance` always returns zero (`pkg/token/native.go`) and gas
price is fixed at zero, which is deliberate: MetaMask's pre-flight check is
`balance >= value + gasLimit*gasPrice`, and only a zero gas price lets that collapse to `0 >= 0` for the
zero-value ERC-20 transfers this facade accepts. Whatever symbol you type into the network form is a
label. Real balances appear once you add each token as a custom token at its address from
`token.supported_tokens`.

### 5.2 Supported methods

Three namespaces are registered by the middleware. Anything outside them fails, with the exception of
`rpc_modules`, which the underlying RPC server registers itself.

| Namespace | Methods |
|---|---|
| `eth` | `chainId`, `blockNumber`, `gasPrice`, `maxPriorityFeePerGas`, `estimateGas`, `getBalance`, `getTransactionCount`, `getCode`, `syncing`, `call`, `sendRawTransaction`, `getTransactionReceipt`, `getTransactionByHash`, `getLogs`, `getBlockByNumber`, `getBlockByHash` |
| `net` | `version`, `listening`, `peerCount` |
| `web3` | `clientVersion`, `sha3` |

Blocks are synthetic. A miner seals accepted transfers into blocks on a timer, and `eth_blockNumber`
advances with wall-clock time so that wallets see confirmations accumulate (`pkg/ethrpc/service/service.go`).
`eth_sendRawTransaction` returns as soon as the intent is queued; the Canton submission happens
asynchronously, so poll `eth_getTransactionReceipt` for the terminal status.

### 5.3 The ERC-20 facade

`eth_call` dispatches on the function selector and supports `balanceOf`, `decimals`, `symbol`, `name`,
`totalSupply` and `allowance`. Any other selector returns an unsupported-method error, as does any `to`
address outside `token.supported_tokens`. `balanceOf` for an address that has never registered returns
zero rather than an error.

**`allowance` returns a constant zero.** CIP-56 has no approval concept, `approve` is unimplemented
(`pkg/token/erc20.go`), and the write path decodes only `transfer`, so no allowance can ever exist. Zero
is the true answer, not a stub.

**`totalSupply` reads zero for instruments issued on another participant.** It is computed by summing the
`Holding` contracts visible to this deployment's issuer party (`pkg/cantonsdk/token/client.go`). For a
token this deployment issues, that sum is complete. For one issued elsewhere, such as USDCx, the issuer
party is a stakeholder on nothing, so the sum is zero. That is Canton's privacy model doing its job.
Please do not file it as a bug.

### 5.4 Authentication

There is none at the transport level. `/eth` carries no JWT check and no signed-header check, and CORS is
open. Reads are answered for any address because they expose only what the participant can already see.

Writes authenticate themselves. `eth_sendRawTransaction` recovers the sender from the transaction
signature using the configured chain id, then checks that sender against the registration whitelist before
doing any work (`pkg/ethrpc/service/service.go`). An unknown or non-whitelisted sender is rejected
synchronously, so the wallet shows an error instead of waiting for a receipt that never comes.
`skip_whitelist_check` disables the gate for local work.

### 5.5 What is not supported

Wiring a generic Ethereum library against this endpoint will work for balances and transfers and fail
elsewhere. Absent entirely: `eth_accounts`, `eth_sendTransaction`, `eth_sign` and `personal_*`; filters
and subscriptions (`eth_newFilter`, `eth_subscribe`); `eth_feeHistory`, `eth_getStorageAt`,
`eth_getBlockReceipts`, index-based block lookups and uncles. Rejected at submission: contract deploys,
any transaction with a non-zero `value`, any calldata that is not `transfer(address,uint256)`, and any
contract address not in the configured token set.

---

## 6. Errors, limits and operational behaviour

Every REST handler returns errors through one writer, `DefaultErrorHandler` in `pkg/app/http/handler.go`.
The status comes from the error's category in `pkg/app/errors/errors.go`; an error never given a category
becomes a 500 carrying the fixed string `Unexpected Service Error`.

### 6.1 Status codes and the error body

| Code | Category | What produces it |
|---|---|---|
| 400 | `CategoryDataError` | Malformed or unknown-field JSON, a missing required field, neither or both of `to` and `to_party_id`, a non-positive amount, a party id that is not `<hint>::<hex>`, insufficient balance, a ledger `INVALID_ARGUMENT` / `NOT_FOUND` / `FAILED_PRECONDITION` on prepare |
| 401 | `CategoryUnauthorized` | Missing or invalid `X-Signature` / `X-Message`, a timestamp outside the freshness window, a missing or expired bearer token, an audience mismatch, a SIWE nonce that is unknown or already used, a caller that is not registered |
| 403 | `CategoryForbidden` | Address not on the registration whitelist, `signed_by` not matching the user's registered fingerprint, Canton rejecting the signature, `?address=` disagreeing with the bearer identity |
| 404 | `CategoryResourceNotFound` | Unknown transfer id, unknown contract id, no balance record for the party |
| 409 | `CategoryDataConflict` | Address or party already registered, an offer no longer claimable, ledger contention (`ABORTED`) |
| 410 | `CategoryGone` | An expired prepared transfer, or an expired registration token |
| 500 | `CategoryGeneralError` | Prepared-transfer cache full, nonce store full, anything uncategorised |
| 502 | `CategoryDependencyFailure` | Canton returning `UNAVAILABLE` or `DEADLINE_EXCEEDED` |
| 504 | chi `Timeout` | The request outran the 60 second server timeout |

**The body is `{"error": "transfer expired", "code": 410}`, and `code` repeats the HTTP status.** It is not
a separate application error code, so branch on the status line and treat `error` as a human-readable
string carrying no compatibility promise. `/eth` is the exception to all of the above: it answers HTTP 200
and encodes the failure in the JSON-RPC `error` object, mapping data errors to `-32602`, unsupported
methods to `-32601`, and everything else to `-32000`.

### 6.2 What is worth retrying

**Retry with backoff:** 502 and 504, which report a dependency being slow rather than a bad request. A 409
reading `transfer conflicted with a concurrent operation, try again` is ledger contention, also retryable,
but from prepare. **Do not retry** 400, 403 or 404. A 401 from a stale `X-Message` is the exception:
re-sign with a fresh timestamp.

**A 410 on execute means prepare again, not retry execute.** The prepared transfer lives in an in-process
cache (`pkg/transfer/cache.go`) whose `GetAndDelete` is single use, so any execute attempt removes the
entry: a 403 for a bad signature destroys the prepared transfer exactly as a success does, and the same
`transfer_id` can never be re-signed. Call prepare again, sign the new hash, execute that.

### 6.3 Behaviours that look like bugs and are not

- **Truncated party ids on the list endpoints**, first eight characters then an ellipsis then the
  last eight (`truncatePartyID`, `pkg/transfer/service.go`). Lossy by design, to stop counterparty
  enumeration, and unusable as `to_party_id`; the `contract_id` beside it is intact and is what
  accept and withdraw take.
- **`allowance` is a constant zero** (`pkg/token/erc20.go`). Canton holdings have no allowance, and
  `approve` returns an error. Never gate a transfer on an approval check.
- **`totalSupply` is zero for instruments issued on another participant.** The participant is not a
  stakeholder on that issuance, so there is no figure to return. It is correct for instruments this
  deployment issues.

### 6.4 Caps

Request bodies cap at 1 MB, prepared transfers at 10000 in flight process-wide on a 2 minute TTL (a 500
reading `too many pending transfers` on breach), live login nonces at 65536 addresses, list `limit` at 1
to 200 with a default of 50. Signed messages are accepted within 5 minutes either side of server time,
registration tokens for 5 minutes, sessions for `auth.token_ttl`, 6h as shipped. **Nothing is rate
limited:** no 429 exists anywhere in the server. Both caches are per process, so a prepared transfer must
be executed against the instance that prepared it.

### 6.5 Where to look when something fails

`GET /health` returns 200 and the body `OK` unconditionally. It is liveness only and tests neither Canton
nor the indexer nor the database, so a healthy response says nothing about whether transfers work. Metrics
are served on the monitoring port when `monitoring.enabled` is set, namespaced per service; here
`api_server_http_requests_total{endpoint,status_code}` and `api_server_transfer_cache_gets_total{result}`
matter most, the second separating clients that signed too slowly from clients sending a wrong id. On
ownership: the API server answers registration, authentication and prepare/execute itself; transfer lists
come from the indexer, whose failures other than a 404 arrive uncategorised and so reach you as a 500
rather than a 502; balance and supply reads go to Canton unless `token_provider.mode` is `indexer`.
