# Recreating FingerprintMappings under the new issuer on prod1

This runbook recreates every user's `FingerprintMapping` under the **new**
`canton.issuer_party` after the issuer was changed. It restores fingerprint→party
resolution, which is what brings back **USDCx balances and transfers** (USDCx was
never stranded — only the lookup broke).

One-shot operator task. Safe by default: **dry-runs** unless you pass `--apply`.

## How it works

The OLD `FingerprintMapping` contracts still exist on-ledger (signed by the old
issuer). This script reads them (the OAuth user's `can_read_as_any_party` lets it
read the old issuer's contracts) and re-creates each under the new issuer with the
**same** user party, fingerprint and EVM address. No userstore DB access needed.

Idempotent: a fingerprint already mapped under the new issuer is skipped, so re-runs
are safe.

## Who should run this

AWS read on the prod1 api-server secret + `kubectl` access to the prod1 cluster (read
`participant` service + `port-forward` in the `canton-middleware` namespace). The
OAuth user must have **`can_act_as` on the NEW issuer** (create authority; the script
verifies it).

## Prereqs

```bash
brew install awscli kubectl grpcurl jq go
aws sso login --profile <your-prod-profile>
kubectl config use-context <prod1-context>
aws sts get-caller-identity
kubectl get ns canton-middleware
```

## Get the tooling

```bash
git fetch origin
git checkout ops/prod1-fingerprint-mappings
```

## Step 1 — dry run

```bash
# all existing users:
./scripts/remote/recreate-mappings-prod1.sh --old-issuer 'OLD_ISSUER::1220...'
# or scope to one user first:
./scripts/remote/recreate-mappings-prod1.sh --old-issuer 'OLD_ISSUER::1220...' \
  -p user_f39Fd6e5::1220eab9...
```

Prints each `fp -> party (evm)` it would create, and a summary. No writes.

The new issuer defaults to the prod1 issuer; override with `--new-issuer <party>` if
needed.

## Step 2 — apply

```bash
./scripts/remote/recreate-mappings-prod1.sh --old-issuer 'OLD_ISSUER::1220...' --apply
```

Then verify a user's USDCx balance resolves again:

```bash
curl 'https://middleware-api-prod1.02.chainsafe.dev/api/v1/balance?address=<evm-addr>'
```

### Flags

- `--old-issuer <party>` — **required.** Previous issuer whose mappings to copy.
- `--new-issuer <party>` — issuer to create under (default: prod1 issuer).
- `-p, --party <party>` — only recreate the mapping for this user party.
- `--apply` — perform creates; omit for dry run.
- `-n <namespace>` — chart namespace if not `canton-middleware`.
- `--no-port-forward` — if Canton gRPC is already on `localhost:5001`.

## Failure modes

- **`✗ missing can_act_as: <new-issuer>`** — grant act-as on the new issuer via
  `UserManagementService.GrantUserRights`, then re-run.
- **`0 would create` but users exist** — either mappings already exist under the new
  issuer (good — idempotent skip), or `--old-issuer` is wrong. Double-check the old
  party id.
- **`FAILED to pull secret`** — AWS session missing / no read on the prod1 secret.
  Re-do `aws sso login`; confirm with `aws sts get-caller-identity`.
- **`port-forward did not become ready`** — wrong namespace (`-n`), wrong service
  name, or kubectl context not on prod1.

## What the script does, in detail

1. Pulls the api-server OAuth secret from AWS Secrets Manager.
2. `kubectl port-forward` to `participant:5001` (killed on exit).
3. Temp config in `/tmp` (mode 600, deleted on exit), `issuer_party = new issuer`.
4. `check-canton-rights.sh` verifies `can_act_as` on the new issuer.
5. `recreate-fingerprint-mappings.go`: lists old mappings (read as old issuer),
   skips fingerprints already mapped under the new issuer, and creates the rest via
   the production identity client.

The only state change is new `FingerprintMapping` contracts. Nothing is written to
this repo (beyond the /tmp config), AWS, or any database.
