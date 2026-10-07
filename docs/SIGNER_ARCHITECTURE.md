# Signer Architecture and Custody Model

How a Canton signing key is created, held and used by this middleware, across the three signer
implementations it supports, and what each one is trusted for.

Canton exposes one signing interface, the Interactive Submission API. Everything above it is
interchangeable, so this document describes one seam and three things that plug into it. They differ only
in where the private key lives and who holds the authority to use it. One of the three ships and is the
default. One is implemented and tested but gated off in deployed builds. One is a specification with no
implementation. The status of each is stated where it is described rather than left to be inferred.

This document extends the component view in `docs/ARCHITECTURE.md` and the key-material section of
`docs/SECURITY_AND_PRIVACY_MODEL.md`, which remains the reference for the privacy model and the data
access rules. Where the two overlap, this one is more specific and more recent.

---

## 1. The signer seam

**Canton's Interactive Submission API is the seam.** `PrepareSubmission` returns a
`PreparedTransactionHash` plus an opaque `PreparedTransaction` protobuf that the caller hands back
unmodified. `ExecuteSubmission` takes that message together with a `PartySignatures` block naming the
acting party, a DER signature, the signing key's fingerprint, and
`SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256` (`pkg/cantonsdk/token/client.go`). The signature is over sha256 of
the prepared hash, applied by the signer: `SignDER` hashes its argument and delegates to `SignHashDER`
(`pkg/keys/canton_keys.go`). Party allocation has the same shape, with `GenerateExternalPartyTopology`
returning a multi-hash and `AllocateExternalPartyWithSignature` taking a DER signature over it
(`pkg/cantonsdk/identity/client.go`). Whoever can produce that signature under the party's registered key
can execute, so everything above the line is swappable.

**The in-tree abstraction is two methods wide.** `token.Signer` is `SignDER(message []byte) ([]byte, error)`
and `Fingerprint() (string, error)` (`pkg/cantonsdk/token/types.go`); `identity.ExternalPartyKey` is
`SignDER` alone (`pkg/cantonsdk/identity/client.go`). Nothing in either interface mentions custody.

**The custodial seam as actually implemented.** `token.KeyResolver` is
`func(partyID string) (Signer, error)` (`pkg/cantonsdk/token/types.go`), and the API server builds exactly
one closure for it during wiring in `openCantonClient` (`pkg/app/api/server.go`). It calls
`GetUserKeyByCantonPartyID`, decrypts with the master-key cipher, returns an error when the stored column
is empty, and otherwise returns a `keys.CantonKeyPair`. What it does not do matters as much. It takes no
`context.Context` parameter and closes over the server's startup context, so the lookup is not bound to the
request. It takes no caller identity, no session, and no `key_mode`. Any party id whose row carries an
encrypted key yields a usable signer. Authorization lives entirely above the resolver: the transfer service
resolves the sender's party from the authenticated EVM address, rejects `key_mode` mismatches, and on the
external path refuses a signature whose `SignedBy` does not equal the user's registered fingerprint
(`pkg/transfer/service.go`).

| Seam point | Varies by implementation | Fixed by Canton |
|---|---|---|
| Where the private key lives | Postgres ciphertext (custodial), Snap entropy in MetaMask, cloud KMS (PROPOSED) | secp256k1 |
| Who holds signing authority | Operator, end user, operator-provisioned HSM (PROPOSED) | The party's registered topology key |
| How the signature is obtained | In-process `SignDER`, wallet JSON-RPC round trip, `kms:Sign` (PROPOSED) | DER, EC_DSA_SHA_256 over sha256(hash) |
| Call shape | One server call, or a prepare and execute pair | `PreparedTransaction` returned opaque and replayed unmodified |

**Status, stated plainly.** The custodial signer ships and is the default `key_mode`
(`pkg/user/user.go`). The external, user-held path ships too: `/api/v2/transfer/prepare` and
`/execute`, the accept and withdraw pairs, and `/register/prepare-topology` are all registered
unconditionally (`pkg/transfer/http.go`, `pkg/user/service/http.go`). Neither path is behind a feature
flag; selection is per user, by `key_mode`, at registration. The KMS-backed custodial signer and the
unified `pkg/signer` interface are design only, from `docs/plans/SNAP_KMS_CUSTODY_PROPOSAL.md`. No
`pkg/signer` package exists: `git ls-files pkg/signer` returns nothing, and the tree contains no
`SignDigest` method and no KMS client.

---

## 2. Three signer implementations

### 2.1 Custodial (shipping)

**Registration generates the key server side.** `RegisterWeb3User` in `pkg/user/service/service.go` verifies
the EIP-191 signature, checks the operator whitelist, then calls `keys.GenerateCantonKeyPair`. That function
reads from `crypto/rand` (`pkg/keys/canton_keys.go`). Each user therefore gets an independent random
secp256k1 key, not one derived from their address.

```
POST /register (signature, message)
      │
      ▼
 verify EIP-191 ──> whitelist ──> GenerateCantonKeyPair (crypto/rand)
                                        │
                                        ├─> SPKI DER ──> AllocateExternalParty ──> Canton topology
                                        │
                                        └─> AES-256-GCM(masterKey) ──> users.canton_private_key_encrypted
```

**One master key for every user, not per-user keys.** `encryptPrivateKey` requires a 32-byte key and wraps
each private key as base64 of `nonce || ciphertext || tag` (`pkg/keys/canton_keys.go`). The key comes from
the environment variable named by `key_management.master_key_env`, which defaults to `CANTON_MASTER_KEY`
(`pkg/config/config.go`), is decoded once at startup, and is held in process memory for the life of the
server (`pkg/app/api/server.go`). Every user row is sealed under that single value.

**There is no envelope encryption, no HSM, and no rotation mechanism.** The master key is raw AES key
material in an environment variable, not a wrapping key over per-user data keys. No key version or key id
column exists on the users table, and no re-encryption command exists in `cmd/`. Rotating the master key
today would mean decrypting and re-encrypting every row with code that has not been written. A database
dump plus that one environment value is complete compromise of every custodial user.

**One config field promises more than the code delivers.** `key_management.key_derivation` is declared,
defaulted to `generate`, and validated as `oneof=generate derive` (`pkg/config/config.go`), but nothing
reads it. `keys.DeriveCantonKeyPair`, the HKDF path over an EVM address and a server seed, is defined and
unit tested and never called from production code. Shipping behavior is random generation whatever the
config says.

**The registration signature does not authorize custody.** The EIP-191 message proves control of the EVM
address and nothing else. Its contents are chosen by the dapp and say nothing about key generation,
encryption, or who may sign. Custody is decided by which registration path the client takes, not by
anything the user signed. A user cannot tell from the signed payload that a key is about to be created for
them and held on the operator's behalf.

**Signing.** A key resolver closure fetches the encrypted key by Canton party id, decrypts it with
`cipher.Decrypt`, and rebuilds a `CantonKeyPair` (`pkg/app/api/server.go`, `pkg/userstore/pg.go`). The
resulting signer calls `SignDER` on the `PrepareSubmission` hash, which sha256-hashes it and delegates to
`SignHashDER`, the same two steps the Snap reproduces. `/api/v2/transfer/custodial`
does prepare, sign, and execute in one request with no user interaction.

**A third custody posture exists and should be named: key transport.** The Canton native path,
`RegisterCantonNativeUser` in `pkg/user/service/service.go`, returns a freshly generated secp256k1 private
key to the caller in the `private_key` field of `RegisterResponse` (`pkg/user/user.go`), documented there as
"Returned for Canton native users (for MetaMask import)". The same path accepts an inbound
`canton_private_key`: `selectKeyToStore` decodes a hex 32-byte key from the request and stores that instead
of the generated one, so the middleware will sign with a key the user transmitted over HTTP. On this path
the Canton signature check can also be disabled outright by `skip_canton_sig_verify` (`pkg/config/config.go`).
Neither raw key direction is Snap related and neither is covered by the custodial or non-custodial labels.

### 2.2 Snap (implemented, flag-gated off)

**The key is re-derived on every call and never persisted.** `deriveCantonKey`
(`canton-snap/packages/snap/src/keyDerivation.ts`) asks for `snap_getEntropy` with
`{version: 1, salt: "canton-network-key-<keyIndex>"}` and no `source` field, then takes sha256 of that
entropy with rejection sampling for a valid secp256k1 scalar. Nothing is written to snap state except an
origin allowlist for silent fingerprint reads (`state.ts`). Because the platform scopes `snap_getEntropy` by
snap ID rather than by calling origin, every consented origin reaches the same Canton identity, and a
`local:` install is a different identity from the `npm:` one.

```
MetaMask vault ──> snap_getEntropy{salt:"canton-network-key-N"}
                        │
                        ▼
                 sha256 + rejection sample ──> privKey ──> compressed pubkey ──> SPKI DER ──> 1220… fingerprint
                        │
 middleware /transfer/prepare ──> hash ──> sha256(hash) ──> snap_dialog ──> ECDSA low-S ──> DER sig ──> /execute
```

**Permissions requested** (`git show origin/main:packages/snap/snap.manifest.json`):

| Permission | What it grants |
|---|---|
| `snap_getEntropy` | Snap-scoped deterministic entropy, unlinkable from the user's BIP-44 wallet keys. The sole key source. |
| `snap_dialog` | The confirmation prompt shown before every export, transaction signature, and topology signature. |
| `snap_manageState` | Encrypted snap-local storage. Used only for the per-origin, per-keyIndex fingerprint allowlist. |
| `endowment:rpc` with `dapps: true` | Lets web pages call the snap's four `canton_*` methods. |

No `endowment:network-access` is requested, so the snap cannot reach the middleware or any other host
itself. Signing methods additionally refuse non-HTTPS callers other than loopback (`origin.ts`), and every
signature requires a dialog approval in `index.ts`.

**A deployed image has the flow turned off.** `NON_CUSTODIAL_ENABLED` is
`import.meta.env.VITE_ENABLE_NON_CUSTODIAL === "true"` (`packages/dapp/src/lib/features.ts`) and gates the
registration choice screen, the install and sign pages, and the snap hook. The Dockerfile writes
`VITE_ENABLE_NON_CUSTODIAL=__VITE_ENABLE_NON_CUSTODIAL__` as a build-time placeholder, and its own comment
records the consequence: the `=== "true"` comparison against a placeholder is constant-folded to `false` by
the minifier, so the entrypoint's runtime substitution cannot switch it back on. Image deployments always
get the non-custodial path off. Enabling it requires rebuilding the dapp with the variable set, or moving to
a runtime config object. The server side is not gated: `/register/prepare-topology` and the
`key_mode: external` branch of `/register` are always live, as are the prepare and execute transfer
endpoints that a snap-signed client uses.

### 2.3 Institutional (design only)

**It does not exist in this repository.** There is no `pkg/signer` package (`ls pkg/signer` fails). `go.mod`
contains no match for `aws`, `kms`, `vault`, or `hsm`. No migration under `pkg/migrations` mentions a KMS key
id or any key reference column, and no source file references `ECDSA_SHA_256` or `ECC_SECG_P256K1`. The only
custody abstraction that ships is `keys.KeyCipher`, which presupposes that raw key material is recoverable
in process.

```
NOT IMPLEMENTED

 API ──> Signer interface ──> LocalSigner  ──> AES keystore (today's behavior)
                          └─> KMSSigner    ──> kms:Sign(DIGEST, ECDSA_SHA_256) ──> DER sig
                                                users.kms_key_id replaces canton_private_key_encrypted
```

**The intended shape** is set out in `docs/plans/SNAP_KMS_CUSTODY_PROPOSAL.md`: a `Signer` interface
returning a DER signature from a key reference and a digest, with `LocalSigner` wrapping the current
keystore for migration and `KMSSigner` calling AWS KMS with `MessageType=DIGEST` and low-S normalization
applied afterward. Provisioning would replace `canton_private_key_encrypted` with a `kms_key_id`, and the
backend would be selected by config at the point where the cipher is built today. None of this is code.
Treat it as the design that discharges the rotation and blast radius problems named in 2.1, not as a
capability any operator can turn on.

---

## 3. Key derivation and identity

**The Snap holds no key.** It re-derives one on every call and discards it. The chain is four steps,
each of them a pure function of the step before (`packages/snap/src/keyDerivation.ts`,
`packages/snap/src/spki.ts`, `packages/snap/src/fingerprint.ts`):

```
MetaMask wallet seed
  │  snap_getEntropy { version: 1, salt: "canton-network-key-<keyIndex>" }    (no `source` field)
  ▼
32 bytes of snap-scoped entropy
  │  sha256, rejection-sampled against the secp256k1 order (re-hash with a counter byte)
  ▼
secp256k1 private key ──► 33-byte compressed public key
  │  X.509 SubjectPublicKeyInfo DER: ecPublicKey + secp256k1 OIDs, uncompressed point (88 bytes)
  ▼
spkiDer
  │  multihash(sha256(uint32_be(12) || spkiDer)), 12 = TopologyTransactionSignature
  ▼
Canton key fingerprint: "1220" + 64 hex
```

**The MetaMask boundary sits after the first step.** The platform owns the seed and the entropy
derivation, and returns entropy that is deterministic per snap and salt and unlinkable from the user's
BIP-44 account keys. Everything below that line is this Snap's own arithmetic, written against `@noble`
primitives so it can be audited and cross-checked against the Go SDK. The private key exists only inside
the Snap's execution environment, is never persisted to Snap state, and is never returned by any handler.

**Entropy is scoped to the snap ID, not to the calling origin.** The platform derives the entropy path
from a keccak over the snap ID. Two consequences, and neither is obvious. Every origin a user consents to
addresses the same Canton identity, so a dApp cannot get a per-site key and a key leak is not contained
to one site. And a `local:http://localhost:4040` install is a different snap ID from
`npm:@chainsafe/canton-snap`, so it derives a different key, a different fingerprint, and ultimately a
different Canton party. A party allocated during local development cannot be reached from the published
Snap. The derivation module says so in its own header comment, and `packages/snap/test/index.test.js`
asserts the origin half of it under "recovery after wallet restore".

**Recovery follows from the same property.** Because the key is a pure function of the wallet seed and a
fixed salt, restoring MetaMask from the secret recovery phrase restores the Canton identity, with no Snap
backup, no exported key, and no server-side escrow. This is now tested rather than assumed.
`packages/snap/test/index.test.js` installs the Snap twice from one recovery phrase in the MetaMask
simulator and asserts the fingerprint matches and has the shape `1220` plus 64 hex, and asserts that a
different phrase yields a different identity. `packages/snap/test/derivation.test.ts` pins the salt
template literally, asserts that no `source` field is sent, and pins the compressed key, SPKI DER and
fingerprint for known entropy as golden values. Those pins are the recovery guarantee in executable form:
changing any of them orphans every party already allocated against the old derivation.

### 3.1 Three things called fingerprint

Three distinct values in this system are called "fingerprint", two of them live in the same database row
and one of them is called `fingerprint` in the public API while not being the Canton one.

| | Canton key fingerprint | EVM mapping id | Party id suffix |
|---|---|---|---|
| Computed from | `multihash(sha256(uint32_be(12) ‖ spkiDer))` | `keccak256(20-byte EVM address)` | assigned by Canton at allocation |
| Shape | `1220` + 64 hex, no `0x` | `0x` + 64 hex | the part after `::` in `hint::fingerprint` |
| Source | `pkg/keys/canton_keys.go`, `packages/snap/src/fingerprint.ts` | `auth.ComputeFingerprint` in `pkg/auth/evm.go` | participant node |
| In the API | `public_key_fingerprint` on prepare-topology, `signed_by` on transfer execute, `SignedBy` on every Canton signature | `fingerprint` on the register response, the key of `FingerprintMapping` on the ledger | `canton_party` |
| Stored as | `canton_public_key_fingerprint` | `fingerprint` | `canton_party_id` |

**The party id suffix carries the Canton key fingerprint, which is why the two are easy to merge.**
`auth.ExtractFingerprintFromPartyID` (`pkg/auth/canton.go`) splits on `::` and returns the suffix, which
is the full multihash-prefixed Canton key fingerprint and not the EVM mapping id. It carries a branch that
strips a leading `1220`, but that branch requires a suffix longer than 68 characters and a real one is
exactly 68, so it never fires for a party id Canton actually emits. Callers receive the multihash.
Party hints are `user_` plus the first 8 hex characters of the EVM address, which makes a party id look
address-derived when it is not.

**Confusing them fails in three places.** A transfer submitted with the EVM mapping id in `signed_by` is
rejected by `pkg/transfer/service.go`, which compares it against the user's stored
`CantonPublicKeyFingerprint`. A `FingerprintMapping` keyed on a Canton key fingerprint is invisible to
`GetFingerprintMapping` lookups that pass the keccak mapping id, which is how `TransferByFingerprint` and
the bridge resolve an address to a party (`pkg/cantonsdk/token/client.go`, `pkg/cantonsdk/bridge/client.go`).
And a Canton signature whose `SignedBy` is not the exact multihash of the registered key is rejected by the
participant, not by us. One existing inconsistency is worth naming: `RegisterCantonNativeUser`
(`pkg/user/service/service.go`) writes the party's key fingerprint into the same mapping field that the
other two registration paths fill with the keccak mapping id, so mappings created by that path are not
reachable by an EVM-address lookup.

### 3.2 Binding party to key at registration

**The server never trusts a client-asserted fingerprint.** Non-custodial registration is two calls.
On prepare-topology the client submits a hex compressed public key, which `compressedKeyToSPKI`
(`pkg/user/service/service.go`) decompresses and re-encodes to SPKI DER with
`keys.MarshalSPKIPublicKey`; the client's own fingerprint is not part of the request. That SPKI is handed
to the participant's `GenerateExternalPartyTopology`, and the fingerprint returned to the client as
`public_key_fingerprint` is the participant's `PublicKeyFingerprint`, computed by Canton from the key
bytes. The middleware can compute the same value independently through `CantonKeyPair.Fingerprint()`, and
the Snap computes it a third time; all three agree by construction, and the Go and TypeScript
implementations are cross-checked by vector tests.

**The second call closes the loop.** The cached SPKI is compared byte for byte against the key resubmitted
with the signature, the user's DER signature over the topology multi-hash is passed to
`AllocateExternalParty` with `SignedBy` set to that fingerprint, and the ledger accepts the allocation only
if the signature verifies under the key named in the topology. The resulting party id and the fingerprint
are then stored together on the user row, which is what every later authorization check reads.

---

## 4. Signing flows

### 4.1 Registration and topology

Non-custodial registration is a two round trip handshake. It carries two signatures from two different
keys: an EIP-191 signature from the user's EVM key, which proves who is registering, and an ECDSA
signature from the Snap's Canton key, which authorises the party's own onboarding topology.

```
dApp (browser)        Canton Snap            API server             Canton participant
      |                    |                      |                         |
      | canton_getPublicKey (keyIndex 0)          |                         |
      |------------------->| dialog, derive       |                         |
      |<-------------------| compressedPubKey, spkiDer, fingerprint         |
      |                    |                      |                         |
      | personal_sign("register:<unix>")          |                         |
      |                    |                      |                         |
      | POST /register/prepare-topology           |                         |
      |   X-Signature, X-Message, canton_public_key                         |
      |------------------------------------------>| verify EIP-191,         |
      |                    |                      | not registered,         |
      |                    |                      | whitelist, 33b -> SPKI  |
      |                    |                      |------------------------>| GenerateExternal-
      |                    |                      |<------------------------| PartyTopology
      |<------------------------------------------| topology_hash,          |
      |                    |                      | registration_token      |
      | canton_signTopology(topology_hash)        |                         |
      |------------------->| dialog, sha256(multiHash), ECDSA DER           |
      |<-------------------| derSignature         |                         |
      |                    |                      |                         |
      | POST /register {key_mode: external, registration_token,             |
      |   canton_public_key, topology_signature, signature, message}        |
      |------------------------------------------>| GetAndDelete token,     |
      |                    |                      | SPKI bytes.Equal check  |
      |                    |                      |------------------------>| AllocateExternalParty
      |                    |                      |<------------------------| party id
      |                    |                      |-- CreateFingerprintMapping -->
      |<------------------------------------------| party, fingerprint      |
```

**The Snap generates the key material and the server never sees the private half.**
`canton_getPublicKey` derives the key, shows a confirmation dialog naming the origin, the key index and
the Canton fingerprint, and returns the compressed secp256k1 public key, its SPKI DER encoding and the
fingerprint (`packages/snap/src/index.ts` in canton-snap). The dApp forwards only the 33 byte compressed
key as `canton_public_key` (`packages/dapp/src/hooks/useRegistration.ts`).

**The server generates the topology.** `PrepareExternalRegistration` (`pkg/user/service/service.go`)
verifies the EIP-191 signature, rejects an address that is already registered, checks the whitelist,
decompresses the 33 byte key back to SPKI DER (`compressedKeyToSPKI`), and calls Canton's
`GenerateExternalPartyTopology` with a party hint built from the first eight hex characters of the EVM
address. The resulting topology transactions, multi-hash and key fingerprint are held in an in-process
cache under a UUID registration token with a five minute TTL (`pkg/user/service/topology_cache.go`). The
response carries `topology_hash`, `public_key_fingerprint` and `registration_token`, and nothing else.

**What the user signs in step two is the topology multi-hash.** `canton_signTopology` requires exactly 34
bytes with the `0x1220` SHA-256 multihash prefix (`packages/snap/src/validation.ts`), sha256 hashes it to
match Canton's `EC_DSA_SHA_256`, and produces a low-S DER signature. The dialog warns that topology
transactions can register an identity, rotate keys or change party membership.

**What the server enforces on register.** `registerExternalWeb3User` re-checks that the address is still
unregistered, consumes the registration token atomically through `GetAndDelete` so a token is single use
(a missing token is a 404 and an expired one a 410), re-derives SPKI from the resubmitted
`canton_public_key` and compares it byte for byte against the key cached in step one, rejecting a mismatch
with "canton_public_key does not match the key from prepare-topology". It then calls
`AllocateExternalPartyWithSignature` (`pkg/cantonsdk/identity/client.go`), which attaches the DER
signature with `SignedBy` set to the topology fingerprint. The middleware does not verify that signature
itself. Canton does, when it validates the onboarding transactions. The user row is written by
`user.NewExternal`, which records the Canton key fingerprint and leaves the encrypted key column empty.

**The EIP-191 signature on both registration endpoints is not timestamp checked.** Both call
`auth.VerifyEIP191Signature` and neither calls `ValidateTimedMessage`, so the message is any string the
client chooses. Replay is bounded by the already-registered conflict and the whitelist rather than by
message freshness.

### 4.2 Transfer

```
dApp (browser)        Canton Snap            API server             Canton participant
      |                    |                      |                         |
      | personal_sign("transfer:<unix>")          |                         |
      | POST /api/v2/transfer/prepare             |                         |
      |------------------------------------------>| recover address,        |
      |                    |                      | key_mode=external,      |
      |                    |                      | recipient + token guards|
      |                    |                      |------------------------>| PrepareSubmission
      |                    |                      |<------------------------| prepared tx + hash
      |                    |                      | cache PreparedTransfer  |
      |<------------------------------------------| transfer_id,            |
      |                    |                      | transaction_hash,       |
      |                    |                      | party_id, expires_at    |
      | canton_signHash(transaction_hash, metadata)                         |
      |------------------->| origin gate, dialog, sha256(hash), ECDSA DER   |
      |<-------------------| derSignature, fingerprint                      |
      |                    |                      |                         |
      | personal_sign("transfer:<unix>")  (a second, fresh auth signature)  |
      | POST /api/v2/transfer/execute             |                         |
      |   {transfer_id, signature, signed_by}     |                         |
      |------------------------------------------>| fingerprint binding,    |
      |                    |                      | GetAndDelete prepared tx|
      |                    |                      |------------------------>| ExecuteSubmission-
      |                    |                      |<------------------------| AndWait
      |<------------------------------------------| {"status":"completed"}  |
```

**Prepare does all the policy work.** `TransferService.Prepare` (`pkg/transfer/service.go`) rejects an
unsupported token symbol, validates `validity_seconds` as positive and small enough that the conversion to
a `time.Duration` cannot overflow, loads the sender by recovered EVM address and refuses anything that is
not `key_mode=external`, and requires exactly one of `to` or `to_party_id`. A raw party id additionally
goes through the sender whitelist and `checkRecipientParty`, which allows an unregistered recipient only
for tokens marked `external_transfer` in token config and only when the participant's topology knows the
party. Self transfers and transfers to the issuer party are refused. Only then does it call Canton's
`PrepareSubmission`, cache the prepared transaction, and return the hash hex encoded with an `0x` prefix.

**Execute binds the signature to the registered key.** `TransferService.Execute` looks the sender up by
the authenticated EVM address and compares the submitted `signed_by` against the fingerprint stored at
registration: `if sender.CantonPublicKeyFingerprint != req.SignedBy`, it returns 403 "signature fingerprint
does not match registered key". That is the whole of the server side binding. It is an equality check
against the user row, not a cryptographic verification. The prepared transaction is then pulled from the
cache with `GetAndDelete`, so a `transfer_id` is single use (404 if unknown, 410 if expired), and the DER
bytes are submitted through `ExecuteSubmissionAndWait` with `SignatureFormat_SIGNATURE_FORMAT_DER` and
`SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256` (`pkg/cantonsdk/token/client.go`). Canton
performs the actual signature verification against the key in the party's topology, and a
`InvalidArgument` or `PermissionDenied` rejection is mapped back to a 403.

**Accept and withdraw follow the same shape.** `POST /api/v2/transfer/incoming/{contractID}/prepare` and
`.../execute`, and the withdraw pair under `/outgoing/{contractID}/`, reuse the same prepare, sign, execute
sequence and the same cache (`pkg/transfer/http.go`). Custodial users get single call variants instead,
where the middleware signs server side.

### 4.3 Authentication on the transfer endpoints

**These endpoints do not take a bearer token.** `authenticateEVM` (`pkg/transfer/http.go`) reads
`X-Signature` and `X-Message`, requires both, validates the message's age, recovers the address with
`auth.VerifyEIP191Signature` and returns it checksummed. The EIP-191 prefix applied during verification is
the standard `personal_sign` one (`pkg/auth/evm.go`). The message convention is `{prefix}:{unix_seconds}`,
and the dApp sends prefixes such as `transfer`, `prepare-accept`, `execute-accept`, `prepare-withdraw`,
`execute-withdraw` and `withdraw-custodial` (`packages/dapp/src/lib/transfer.ts`). The maximum age is five
minutes, from `messageMaxAge` in the same file. The read endpoints are different: they are wrapped in the
`readAuth` middleware. That is JWT middleware when the `auth` configuration block is present and a
passthrough when it is absent. All three shipped api-server configurations supply the block, so a
deployment built from them is access-controlled. The caveat is the failure mode rather than the shipped
default: omitting the block does not fail startup, it logs a warning and leaves the handlers resolving the
caller from an unverified address query parameter. Section 8 records it.

**The prefix is not validated.** `auth.ValidateTimedMessage` takes the substring after the last colon,
parses it as a Unix timestamp and checks that it is within `maxAge`. Nothing inspects the text before the
colon, so the prefixes are documentation for humans and not an enforced binding. A header pair signed for
`transfer` is accepted verbatim by the withdraw and accept endpoints, and so is any string at all that ends
in a fresh timestamp.

**The signed message is also not bound to the request body.** It covers a prefix and a timestamp, not the
amount, the recipient, the `transfer_id` or the token. One captured `X-Signature` and `X-Message` pair
therefore authorises any call to any of these endpoints as that address until the timestamp ages out. The
consequences, and what still limits them, are in the threat model section.

### 4.4 What the Snap actually signs

**An opaque 32 byte hash.** `canton_signHash` validates only that the parameter is 32 bytes of hex
(`packages/snap/src/validation.ts`) and gates the origin to HTTPS or a loopback host
(`packages/snap/src/origin.ts`). It has no Daml interpreter, no view of the prepared transaction, and no
network permission with which to fetch one, so it cannot independently derive what that hash commits to.

**The human readable details in the dialog come from the dApp.** The operation, token symbol, amount,
recipient and sender are free text `metadata` parameters, length capped at 200 characters each, rendered
beside the hash. The dialog says so in as many words: "Transaction details (reported by the dApp; the snap
cannot yet verify these against the hash):". When no metadata is supplied the dialog instead shows
"⚠ RAW HASH SIGNING — the dApp did not provide any transaction context. Approve only if you trust this
dApp." (`packages/snap/src/dialogs.ts`).

**A compromised or malicious dApp can therefore show one transfer and have the user sign another.** The
prepared transaction envelope path that would let the Snap recompute the hash from a canonical encoding is
not shipped. `validation.ts` records the reason: the envelope check is "temporarily removed" pending
envelope emission from canton-middleware. This is the single largest residual weakness in the signing path
and it is treated as such in the threat model section.

---

## 5. Authorization policy

**Authentication says who is calling. Authorization says what that caller may do with which key.** The
seam described in section 1 deliberately knows nothing about either, so both live above it, and the
policy is thinner than the architecture allows for.

**What is enforced per request today.** Four checks, in this order, on the external transfer path:

| Check | Where | What it prevents |
|---|---|---|
| EIP-191 signature over a timestamped message | `pkg/auth/evm.go` | A caller acting as an address they do not control |
| `key_mode` must be `external` | `pkg/transfer/service.go` | A custodial user driving the non-custodial flow, and the reverse |
| `signed_by` must equal the fingerprint stored at registration | `pkg/transfer/service.go` | A signature from a key the party never registered |
| The prepared transfer's party must equal the caller's party | `pkg/transfer/service.go` | Consuming or destroying another user's prepared transfer |

**The key resolver has no policy at all.** It takes a party id and returns a signer. It receives no caller
identity, no session, and no `key_mode`, so every custodial key in the database is reachable from it given
only a party id. That is sound exactly as long as every caller above it resolves the party from an
authenticated identity rather than from request input, which is the invariant the four checks above
maintain. It is an invariant held by convention rather than by the type system, and a future endpoint that
takes a party id from the request body would break it silently. A resolver that took a caller identity
alongside the party id would make the invariant structural.

**Per-key policy is the dimension that does not exist yet.** The grant's institutional path implies
policies attached to a key rather than to a user: value limits, allowed counterparties, time windows,
dual control above a threshold. Nothing in the tree expresses any of that. A custodial key signs whatever
the service layer asks it to sign, without limit. For the institutional path this is not optional, because
it is usually the custody provider's own policy engine that enforces it, and the middleware has to be able
to express the policy in the provider's terms and to fail closed when the provider refuses.

**Where that policy should live.** Not in the signer, which should stay two methods wide, and not scattered
across handlers. The natural home is a policy check between the service layer and the resolver, evaluated
against the prepared transaction rather than the raw request, because the prepared transaction is the only
representation that both the operator and the ledger agree on. That is a design position, not an
implementation: no such layer exists today.


---

## 6. Threat model

The three custody modes are three separate signing surfaces. They share the transaction-building code and
nothing else, so a compromise of one does not propagate to the others, and each has to be reasoned about
on its own terms.

### 6.1 Custodial signing

**The master key is the entire boundary.** Custodial private keys are stored AES-256-GCM encrypted in the
`users` table, and the key that decrypts them is a single 32-byte value read from one environment variable
at startup (`CANTON_MASTER_KEY` by default, `pkg/config/config.go`). One key covers every custodial user in
the deployment, and `encryptPrivateKey` in `pkg/keys/canton_keys.go` passes no additional authenticated
data, so a ciphertext is not cryptographically bound to the row it sits in. An attacker with write access to
the database and no key at all can move user A's encrypted blob onto user B's row and the decrypt will
succeed.

**Signing has no per-request authorization inside the process.** The key resolver wired in
`pkg/app/api/server.go` takes a Canton party id and returns a signer. Any code path that reaches it with a
custodial party id gets a signature. Authorization lives in the HTTP layer above, so a request-forgery or
deserialization bug anywhere in the API server is directly a signing bug.

**Custodial users do not consent to inbound transfers.** `pkg/custodial/accept_worker.go` polls the indexer
and accepts every pending `TransferOffer` addressed to a custodial party, with no user in the loop.

| Threat | Attacker | Impact | Status today |
|---|---|---|---|
| Master key disclosure (env dump, core dump, config leak) | Anyone who reads process environment or deploy config | Every custodial key decrypts; total loss of all custodial assets | Not mitigated. Single key, no HSM, no per-user wrapping |
| Database read without the master key | DB-level attacker, backup theft | Ciphertext only; no signing capability | Mitigated by AES-256-GCM |
| Database write without the master key | DB-level attacker | Key blobs swappable between rows because GCM is used with no AAD | Not mitigated |
| Compromise of the API process | RCE, supply chain, malicious operator | Signer available for any custodial party on demand | Not mitigated. Accepted trust assumption of the mode |
| Unwanted inbound asset delivery | Any party that can address an offer | Auto-accepted by the worker | Not mitigated, by design |

### 6.2 Snap signing

**The Snap signs a 32-byte hash and cannot verify what it represents.** Its own dialog says so. From
`packages/snap/src/dialogs.ts` on `origin/main`, the transaction dialog renders the literal string
`"Transaction details (reported by the dApp; the snap cannot yet verify these against the hash):"` above the
operation, token, amount, recipient and sender fields. Every one of those fields arrives as a dApp-supplied
string in `params.metadata`, length-capped at 200 characters by `validateMetadata` in
`packages/snap/src/validation.ts` and otherwise unchecked. So a malicious or compromised dApp can present a
1 DEMO transfer to a familiar recipient while handing over the hash of a transfer of the user's entire
balance to an attacker party, and the user who approves signs the second one. This is the most serious
weakness in the non-custodial path.

**With no metadata the dialog degrades to an explicit warning** rather than a silent signature. The else
branch renders `"⚠ RAW HASH SIGNING — the dApp did not provide any transaction context. Approve only if you
trust this dApp."` That is the correct failure mode, but it is a warning, not a check.

**The fix is specified and not shipped.** The header comment in `packages/snap/src/validation.ts` records
that the prepared-transaction envelope path, in which the Snap recomputes the canonical SHA-256 multihash
from the transaction body itself, is temporarily removed pending envelope emission from canton-middleware.
Until the middleware emits that envelope, there is no version of this Snap that can close the gap.

**Keys are scoped to the snap ID, not to the calling origin.** `snap_getEntropy` is keyed by the installed
snap's ID, so every origin the user has ever consented to addresses the same Canton identity at a given
`keyIndex`. There is no per-site identity separation. A corollary that matters in testing is that a `local:`
install and an `npm:` install are different snap IDs and therefore different parties.

**When the Snap prompts, and when it does not.** `canton_getPublicKey`, `canton_signHash` and
`canton_signTopology` prompt on every single call with no caching of any kind (`packages/snap/src/index.ts`).
Only `canton_getFingerprint` caches consent, and it caches per `(origin, keyIndex)` pair in
`packages/snap/src/state.ts`. The dialog discloses this: `"This dApp wants to read this Canton identity.
Approving will let this dApp read it silently from now on for this same key index. Other key indices will
still require a fresh prompt."` The store holds at most 200 origins and 32 key indices per origin. Both caps
evict FIFO, and the global cap evicts the oldest origin outright when a new one is approved. Eviction only
causes a re-prompt, so it fails safe. There is no RPC method to revoke a granted consent. Reinstalling the
Snap is the only way to clear the store.

**Origin gating covers the signing methods only.** `assertSigningOrigin` in `packages/snap/src/origin.ts`
rejects anything that is not HTTPS or an IANA loopback host, and it is called from `handleSignHash` and
`handleSignTopology` and from nowhere else. A plain-HTTP non-loopback page can therefore still reach
`canton_getPublicKey` and `canton_getFingerprint`, both of which are dialog-gated and disclose public
material only.

| Threat | Attacker | Impact | Status today |
|---|---|---|---|
| Metadata lies about the hash | Malicious or XSS-compromised dApp | User approves a transfer of attacker's choosing | Not mitigated. Disclosed in the dialog; envelope verification is pending |
| Raw hash with no context | Any dApp | Blind signature | Warned, not blocked |
| Cross-origin identity linkage | Any two consented origins | Both see the same Canton party | Not mitigated. Platform behavior of `snap_getEntropy` |
| Silent fingerprint enumeration across key indices | A consented origin | Limited to the exact `(origin, keyIndex)` pairs approved | Mitigated by per-keyIndex consent |
| Stale consent after a site is compromised | Attacker who takes over a previously trusted origin | Silent fingerprint reads resume | Not mitigated. No revoke method |
| Key exfiltration from the Snap | Malicious bundle | None over the network | Mitigated. `snap.manifest.json` requests no `endowment:network-access` |

### 6.3 Institutional signing

**No institutional signing integration exists in this repository.** Grepping `pkg/` for `institutional` and
for any key mode other than the two defined in `pkg/user/user.go`, `custodial` and `external`, returns
nothing. What exists is the `Signer` interface in `pkg/cantonsdk/token/types.go`, two methods wide, and the
`key_mode: external` prepare and execute HTTP flow, which accepts a DER signature produced anywhere. An
institutional deployment is therefore an external-mode user whose signature comes from an HSM or a custody
provider instead of a browser, and the middleware cannot distinguish the two.

| Threat | Attacker | Impact | Status today |
|---|---|---|---|
| Hash substitution between middleware and custody provider | Attacker on the integration path | Institution signs the wrong transaction | Not mitigated here. Inherits 6.2's opaque-hash problem with no dialog at all |
| Provider-side key compromise | Custody provider insider | Full authority over that party | Out of scope. Belongs to the provider's own model |
| Policy enforcement at the provider | n/a | Nothing in this middleware expresses or checks a provider policy | Not implemented |

### 6.4 Threats common to every surface

**A prepared transfer is bound to the party that prepared it.** The cache's owner-aware retrieval in
`pkg/transfer/cache.go` evaluates ownership under its own lock and refuses a foreign transfer id without
removing the entry, so `Execute` returns 403 and the rightful owner is unaffected. Before that existed,
retrieval deleted first and the caller checked afterwards. Any authenticated caller who learned another
user's transfer id could therefore destroy it: never steal it, because Canton verifies the signature
against the sending party's key, but consume it before the signature was ever checked. Doing the check
under the lock also means a refused probe cannot reset the entry's deadline, which a restore-after-delete
shape would have allowed.

**The topology hash encoding the Snap requires is not guaranteed by anything upstream.**
`parseTopologyHash` demands exactly 34 bytes prefixed with `0x1220`, the SHA-256 multihash prefix. On the
middleware side, `ExternalPartyTopology.MultiHash` is a bare `[]byte` copied verbatim from the Ledger API's
`GenerateExternalPartyTopologyResponse.multi_hash` (`pkg/cantonsdk/identity/client.go`), and no code in this
repository asserts its length or its prefix before handing it to a client. The two sides agree today because
Canton happens to emit that encoding. If it ever changes, or if a participant emits a bare digest, the Snap
rejects a hash the middleware considers valid and external-party registration breaks with an error that
points at the wrong component. This is a latent interop risk, not an exploit.

**Canton signature verification at registration can be switched off.** `skip_canton_sig_verify` in
`pkg/config/config.go` defaults to false, and the mainnet defaults file pins it to false, but the Docker
defaults file reads it from an environment variable. An operator who sets it lets any caller register
against a Canton party id they do not control.

---

## 7. Custody evaluation

Two of the three models below are implemented. The institutional column is a specification: there is no
`pkg/kms`, no provider SDK in `go.mod`, and no occurrence of KMS or of any provider name from the grant's
evaluation set anywhere in the Go tree. It appears because the interface it will implement already exists.

| | Custodial | Snap (non-custodial) | Institutional |
|---|---|---|---|
| Who holds the key | The operator. Generated server side at registration by `keys.GenerateCantonKeyPair`, AES-256-GCM encrypted under a master key read from the environment, stored in `users.canton_private_key_encrypted` | The user. Re-derived inside MetaMask from `snap_getEntropy` on every call, never persisted, never transmitted | The custody provider, or a cloud KMS under the operator's account |
| Who can sign | The operator, unilaterally, with no user involvement | Only the user, per transaction, behind a MetaMask dialog | The provider, within whatever per-key policy the operator configures there |
| What a server compromise yields | The database alone yields ciphertext. The API process together with the master key yields signing authority over every custodial user's assets | A signature on a transaction of the attacker's choosing, for any user who approves once. The prompt is not a stop: until the Snap verifies the prepared envelope it cannot bind the hash it signs to the details it renders, so a substituted hash is approved as if it were the real one | The ability to request signatures. What that is worth depends on the provider-side policy, not on this middleware |
| Recovery | Operator restores from a database backup. The user holds nothing to restore from | Restore the same MetaMask secret recovery phrase and install the same snap ID | The provider's own key recovery procedure |
| Status | Shipped | Implemented in canton-snap | Not implemented |

### 7.1 Migration between modes

**A registered user cannot change custody mode, and the Canton key cannot be rotated.** The store interface
in `pkg/user/service` exposes `CreateUser`, two getters, `UserExists` and `DeleteUser`, with no update of
any kind, and `pkg/userstore` matches it; `evm_address` is unique, so re-registering the same address is
rejected as a conflict. `pkg/cantonsdk/identity` writes topology in its three allocation methods,
`AllocateExternalParty` and `AllocateExternalPartyWithSignature`, both of which onboard a new party. There
is no rotate, no add-key and no topology-update method on the client.

**So migration means a new identity.** A custodial user who later wants self-custody registers a different
EVM address in external mode and gets a different Canton party. Holdings do not follow: they have to be
moved as ordinary CIP-56 transfers, and the old party keeps whatever else it was a stakeholder on. The Snap
has the same property internally: canton-snap's README states there is no migration path between snap IDs.
Custody is a one-way decision at signup rather than a setting, and integrators should present it that way.

### 7.2 The `key_mode` field

**Two values, defined in `pkg/user/user.go`** as `KeyModeCustodial` ("custodial") and `KeyModeExternal`
("external"). The column is `notnull default 'custodial'`.

**The field is not serialized consistently, and the asymmetry is load-bearing.** On `RegisterRequest` and
`RegisterResponse` it carries `omitzero`. The custodial registration path returns a response that never
sets it, so the field is absent from the body, while the external path sets it explicitly. On the `User`
model returned by `GET /profile` the tag is a plain `json:"key_mode"`, so there it is always present. A
caller must read an absent `key_mode` on `/register` as custodial, and must not write one parser that
requires the field on both surfaces. The transfer endpoints then enforce the distinction rather than infer
it: a custodial user calling a prepare/execute route is rejected with "prepare/execute API requires
key_mode=external", an external user calling a server-signed route with "this endpoint requires
key_mode=custodial". The mode does not select which flow is faster. It selects which flows are reachable.

### 7.3 Institutional evaluation framework

**The first filter is technical, and it eliminates most Ethereum-oriented providers.** Canton signs with
`SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256` over `SIGNING_KEY_SPEC_EC_SECP256K1`, submitted as
`SIGNATURE_FORMAT_DER`. The contract is `SignDER(message []byte) ([]byte, error)` in
`pkg/cantonsdk/token/types.go`, and what it signs is sha256 of the hash returned by `PrepareSubmission`.
Nothing in that path is an EVM transaction, so a provider that only signs Ethereum-style transactions, with
its own hashing over an RLP or typed envelope, cannot serve this interface. Raw secp256k1 digest signing
with a DER or recoverable (r, s) output is the entry condition, checked before anything else.

**The remaining criteria**, applied only to providers that pass that filter: whether a party can be
allocated against a provider-generated key with only SPKI public material exported; per-key authorization
policy expressive enough to bound what the middleware may request; signing latency inside the
PrepareSubmission → sign → ExecuteSubmission round trip; and audit log fidelity.

**The gate is the grant's, reproduced unchanged.** Track A, preferred, is a partnership integration against
an established custody provider, evaluation set Fireblocks, BitGo, Anchorage Digital and Copper plus any
further providers identified. Track B is the fallback if no partnership is reached on commercially or
technically acceptable terms within the first four weeks of the workstream, and is an in-house KMS-backed
signer against AWS KMS as primary target, abstracted so a second provider can be added without rework. The
trigger is week four, the criterion is acceptable terms, the default outcome is Track B. Both tracks end at
the same signer interface, so the choice never reaches orchestration code. The workstream has not started.

---

## 8. Known limitations

**The Snap cannot bind what it displays to what it signs.** This is the most important limitation in this
document and it is disclosed in the Snap's own dialog. Until the prepared transaction is parsed and
verified inside the Snap, an approving user is trusting the dApp's rendering. Section 6.2 sets out the
attack. The fix is envelope verification in the Snap, which is not implemented.

**The non-custodial path is not reachable in a deployed image.** The server routes ship unconditionally,
but the dApp gates the registration choice behind a build-time flag that is off unless
`VITE_ENABLE_NON_CUSTODIAL` is set, and the Docker build does not set it. Everything in sections 2.2, 3
and 4.1 is implemented and tested, and none of it is reachable by a user of the published image today.

**The institutional path has no implementation.** No signer package, no KMS dependency, no key-reference
column. Section 7 describes criteria and a process, not a system.

**Read-endpoint authorization fails open when it is not configured.** The three shipped api-server
configurations all supply an `auth` block, so read endpoints on a deployment built from them are behind
JWT sessions. An operator who omits the block does not get a startup failure. They get a warning in the
log and a passthrough middleware, after which the handlers resolve the caller from an unverified address
query parameter and any caller can read any registered address's transfer history. Security that depends
on remembering a configuration block is weaker than security that refuses to start without it. The `/eth`
JSON-RPC facade is separately unauthenticated by design and documented as such in
`docs/SECURITY_AND_PRIVACY_MODEL.md`.

**Canton-native registration cannot verify a real party.** `auth.VerifyCantonSignature` computes the
expected fingerprint as the hex SHA-256 of the compressed public key and compares it against the party id
suffix, which is a multihash over a purpose-prefixed SPKI encoding. The two are different values of
different lengths, and the function's multihash-stripping branch does not fire because it requires a
suffix longer than 68 characters while a real one is exactly 68. The comparison therefore cannot succeed
for any genuine Canton party, which makes `skip_canton_sig_verify` effectively mandatory on that path. With
it set there is no proof of party control on that endpoint at all. This is a defect, not a design choice,
and it is tracked separately from this document.

**Key material cannot be rotated or erased through any API.** There is no update path on the user store and
no key-rotation method on the identity client, so the custody decision made at registration is final for
the life of the party. Section 7 covers the consequence for migration between modes.

**There is no envelope authenticity on custodial key blobs.** AES-256-GCM is used without associated data,
so a ciphertext is not bound to the row that holds it. The practical exposure is described in section 6.1
and the mitigation is to include the party id as associated data, which is a schema-compatible change.
