# Recreating FingerprintMappings under the current issuer

After `canton.issuer_party` is changed, every user's `FingerprintMapping` is still
signed by the old issuer, so the new issuer can't resolve fingerprints → parties. This
breaks balance/transfer lookups — including **USDCx**, whose holdings were never
stranded (only the lookup broke). This tool recreates each mapping under the current
issuer, restoring resolution.

One-shot operator task. Migrates **all users in one run**. Safe by default: it
**dry-runs** unless you pass `--apply`.

## How it works

The OLD `FingerprintMapping` contracts still exist on-ledger. The script wildcard-lists
every mapping (via `can_read_as_any_party`) and re-creates the ones **not** already
under the current issuer — same user party, fingerprint and EVM address. It reads the
old issuer off each contract, so you don't pass an old-issuer party. No userstore DB.

Idempotent: a fingerprint already mapped under the current issuer is skipped, so
re-runs are safe.

## Environments

Selected with `--env` (default `prod1`):

| | prod1 | dev1 |
|---|---|---|
| Credentials | AWS Secrets Manager | `CANTON_AUTH_CLIENT_ID` / `CANTON_AUTH_CLIENT_SECRET` env vars |
| Canton access | `kubectl port-forward` to `participant:5001` | direct (`canton-ledger-api-grpc-dev1.chainsafe.dev:80`) |
| Tools needed | aws, kubectl, grpcurl, jq, curl, go | grpcurl, jq, curl, go |

The current issuer, domain, audience and token URL are baked in per environment;
override the issuer with `--new-issuer` if needed.

## Who should run this

- **prod1:** AWS read on the prod1 api-server secret + `kubectl` access to the prod1
  cluster (read `participant` svc + `port-forward` in `canton-middleware`).
- **dev1:** the dev OAuth `client_id`/`client_secret`.

Either way the OAuth user must have **`can_act_as` on the current issuer** (create
authority; the script verifies it) and `can_read_as_any_party`.

## Get the tooling

```bash
git fetch origin
git checkout ops/prod1-fingerprint-mappings
```

## prod1

```bash
aws sso login --profile <prod-profile>
kubectl config use-context <prod1-context>

# dry run (all users), then scope-test one user, then apply:
./scripts/remote/recreate-mappings.sh
./scripts/remote/recreate-mappings.sh -p user_f39Fd6e5::1220eab9...
./scripts/remote/recreate-mappings.sh --apply
```

## dev1

```bash
export CANTON_AUTH_CLIENT_ID=...
export CANTON_AUTH_CLIENT_SECRET=...

./scripts/remote/recreate-mappings.sh --env dev1            # dry run, all users
./scripts/remote/recreate-mappings.sh --env dev1 --apply    # create
```

Dry runs print each `fp -> party (evm)` and a summary; no writes. After `--apply`,
allow 10–30s, then verify a user's balance resolves:

```bash
# prod1: https://middleware-api-prod1.02.chainsafe.dev   dev1: https://middleware-api-dev1.01.chainsafe.dev
curl '<balance-api>/api/v1/balance?address=<evm-addr>'
```

### Flags

- `--env prod1|dev1` — target environment (default `prod1`).
- `--new-issuer <party>` — override the current issuer (default: per-env).
- `-p, --party <party>` — only recreate the mapping for this user.
- `--apply` — perform creates; omit for dry run.
- `-n <namespace>` — chart namespace for the prod1 port-forward.
- `--no-port-forward` — skip the port-forward (Canton gRPC already reachable).

## Failure modes

- **`✗ missing can_act_as: <issuer>`** — grant act-as on the current issuer via
  `UserManagementService.GrantUserRights`, then re-run.
- **dev1 `needs CANTON_AUTH_CLIENT_ID/SECRET`** — export both before running.
- **`FAILED to pull secret` (prod1)** — AWS session missing / no read on the secret.
  Re-do `aws sso login`; confirm with `aws sts get-caller-identity`.
- **`port-forward did not become ready` (prod1)** — wrong namespace (`-n`), service
  name, or kubectl context.
- **`Would create 0` but users exist** — mappings already under the current issuer
  (idempotent skip), or `--new-issuer` doesn't match the live issuer.

## What the script does, in detail

1. Gets OAuth creds (AWS for prod1, env vars for dev1).
2. Reaches Canton (port-forward for prod1, direct for dev1).
3. Writes a temp config in `/tmp` (mode 600, deleted on exit), `issuer_party =`
   current issuer.
4. `check-canton-rights.sh` verifies `can_act_as` on the current issuer.
5. `recreate-fingerprint-mappings.go` wildcard-lists all mappings, skips those already
   under the current issuer, and creates the rest via the production identity client.

The only state change is new `FingerprintMapping` contracts. Nothing is written to
this repo (beyond the /tmp config), AWS, or any database.
