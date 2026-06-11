# Minting DEMO to a Canton party on prod1

This runbook walks the infra team through minting DEMO tokens to any Canton
party on the prod1 environment. It's a one-shot operator task — the script
is idempotent (re-running mints again, no state mutation in this repo).

## Who should run this

Anyone with **both** of the following:

- **AWS IAM read access** to the prod1 api-server secret in Secrets Manager
  (`arn:aws:secretsmanager:eu-north-1:905418303280:secret:infra-prod-canton-middleware-creds-RE1k5E`).
- **`kubectl` access** to the prod1 EKS cluster (specifically: read on the
  `participant` service and `port-forward` permission in the
  `canton-middleware` namespace — or whichever namespace hosts the chart).

If you don't have one, ask whoever does to run it for you.

## Prereqs on your workstation

```bash
brew install awscli kubectl grpcurl jq go     # python3 and curl ship with macOS
aws sso login --profile <your-prod-profile>   # or whatever auth your team uses
kubectl config use-context <prod1-context>
```

Verify:

```bash
aws sts get-caller-identity        # confirms AWS session
kubectl get ns canton-middleware    # confirms cluster access
```

## First-time setup: bootstrap DEMO

The participant ships without any CIP56 TokenConfig contracts. Before
any mint can succeed, DEMO must be bootstrapped once. This creates the
`CIP56Manager` + `TokenConfig` under the configured issuer
(`chainsafe-middleware::122043f0b94e…` on prod1).

```bash
cd /path/to/canton-middleware

./scripts/remote/mint-demo-prod1.sh --bootstrap-demo
```

Idempotent — re-running detects the existing TokenConfig and exits cleanly.
Run once per environment, then skip this step on subsequent mint sessions.

## Minting after bootstrap

```bash
./scripts/remote/mint-demo-prod1.sh \
  -p user_f39Fd6e5::1220eab9dc9b61bd5db2206550aacf9530c93c44c1935fd0412823c3afe5164a136a \
  -a 1000
```

Replace `-p <party>` and `-a <amount>` for other recipients.

### Optional flags

- `--dry-run` — verify rights without minting (good first invocation).
- `--bootstrap-demo` — one-time setup; create the DEMO CIP56Manager + TokenConfig.
- `--list-token-configs` — diagnostic; list all TokenConfig contracts on the
  participant. Use to confirm DEMO exists after bootstrap, or to debug
  "no TokenConfig found" errors.
- `-n <namespace>` — if the chart is deployed in a namespace other than
  `canton-middleware`.
- `--no-port-forward` — if you're running from inside the cluster or already
  have a port-forward open on `localhost:5001`.

### Expected output (happy path)

```
>>> Fetching OAuth credentials from AWS Secrets Manager (eu-north-1)...
    OAuth client_id: RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients
>>> Starting kubectl port-forward canton-middleware/svc/participant 5001:5001...
    Port-forward ready: localhost:5001 -> canton-middleware/svc/participant:5001

>>> Verifying OAuth user has can_act_as on issuer...
>>> Fetching OAuth token from https://prod-chainsafe.eu.auth0.com/oauth/token...
    OAuth user: RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients
>>> Listing user rights at localhost:5001...
... (rights JSON) ...
  Rights summary for RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients
  ✓ participant_admin                    (or)
  ✓ can_execute_as_any_party
  ✓ can_read_as_any_party
  can_act_as:
    - daml-autopilot::122043f0b94e...
    - <other parties>

  Verdict for IssuerMint on daml-autopilot::122043f0b94e...
  ✓ Ready to mint — explicit can_act_as: daml-autopilot::122043f0b94e... granted.

>>> Minting 1000 DEMO to user_f39Fd6e5::...
... (mint script output) ...
  Mint Complete
  Holding CID: 00abc123...
  Owner:       user_f39Fd6e5::...
  Amount:      1000 DEMO

  Mint complete.
  Recipient: user_f39Fd6e5::...
  Amount:    1000 DEMO
  Issuer:    daml-autopilot::122043f0b94e...

  Verify via the api-server (replace <evm-addr> with the recipient's
  registered EVM address):
    curl 'https://middleware-api-prod1.02.chainsafe.dev/api/v1/balance?address=<evm-addr>'

  Allow 10-30s for the indexer to pick up the new holding.
```

### Verifying the mint landed

If the recipient party has a registered EVM address in the api-server's user
table (which is the case for our standard test address
`0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266`):

```bash
curl 'https://middleware-api-prod1.02.chainsafe.dev/api/v1/balance?address=0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266'
```

Allow 10-30s for the indexer to pick up the new holding.

## Failure modes

### `FAILED to pull secret. AWS error: ...`

Your AWS session is missing or lacks read on the prod1 secret. Re-do
`aws sso login` with the right profile, and confirm with
`aws sts get-caller-identity`.

### `FAILED: secret has no client_id key. Available keys: [...]`

The Secrets Manager entry uses different key names than the script tries
(`CANTON_AUTH_CLIENT_ID`, `client_id`, `canton_auth_client_id`). Look at the
list of keys printed and edit the `extract_key` calls in the script.

### `FAILED: port-forward did not become ready on localhost:5001`

Either the namespace is wrong (try `-n <correct-namespace>`), the
`participant` service doesn't exist under that name, or your kubectl context
isn't pointing at prod1. Confirm with:
```bash
kubectl -n <namespace> get svc | grep participant
kubectl config current-context
```

### Rights check `✗ Missing can_act_as: daml-autopilot::...`

The api-server's OAuth user doesn't have `can_act_as` rights on the DEMO
issuer party. Grant via Canton's `UserManagementService.GrantUserRights` —
same shape as the `can_execute_as_any_party` grant from late May. Once
granted, re-run.

### Mint reports `no TokenConfig found for DEMO`

DEMO has not been bootstrapped on prod1's Canton ledger. Run
[scripts/setup/bootstrap-demo.go](../../scripts/setup/bootstrap-demo.go)
once first (uses the same config — pass `--dry-run` first if you want to
verify rights again).

### Mint reports `PermissionDenied: ...`

Same as the rights-check failure. Re-run with `--dry-run` to see the
detailed verdict and act on it.

### Balance not showing up after 30+ seconds

Check the indexer logs for the recipient party. If the indexer's
`instruments` config doesn't include the DEMO ↔ issuer-party mapping, the
holding is on-ledger but not surfaced through the api-server. Ask whoever
owns the indexer config.

## What the script does, in detail

1. **Pulls the api-server secret** from AWS Secrets Manager (the same secret
   the prod1 api-server pod loads at startup), extracts `client_id` and
   `client_secret`.
2. **Starts a `kubectl port-forward`** from `localhost:5001` to the in-cluster
   `participant:5001` Canton gRPC. The port-forward runs in the background
   and is killed on exit (clean or otherwise) via a bash trap.
3. **Writes a temporary api-server config** to `/tmp/prod1-mint-config.*.yaml`
   with mode 600 and deletes it on exit. The config contains the OAuth
   `client_secret` for the brief life of the script — no on-disk persistence
   beyond exit.
4. **Calls [scripts/remote/check-canton-rights.sh](./check-canton-rights.sh)**
   to verify the OAuth user can `act_as` the DEMO issuer. Bails with exit 1
   if not.
5. **Runs [scripts/remote/mint-to-party.go](./mint-to-party.go)**, which:
   - Authenticates to Canton via OAuth2 client_credentials → bearer token.
   - Finds the active `CIP56.Config.TokenConfig` contract for DEMO.
   - Exercises `IssuerMint` on it, creating a new `CIP56Holding` for the
     recipient with the specified amount.
6. **Prints the holding CID** and a verification curl command.

The script does not write to any of: this git repo (except the temp config
in /tmp), any AWS resource, the Canton database, or the api-server DB. The
only state change is the new `CIP56Holding` contract on the Canton ledger.

## Customizing for other environments

Hard-coded prod1 defaults (override at the top of the script if running
against a different deployment):

| Setting | Default |
|---|---|
| AWS region | `eu-north-1` |
| AWS Secret ARN | the prod1 api-server creds ARN |
| K8s namespace | `canton-middleware` |
| K8s service | `svc/participant` |
| Remote port | `5001` |
| Canton domain_id | `global-domain::1220b1431ef217342db44d516bb9befde802be7d8899637d290895fa58880f19accc` |
| DEMO issuer party | `daml-autopilot::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b` |
| Auth0 audience | `https://canton-ledger-api-prod1.02.chainsafe.dev` |
| Auth0 token URL | `https://prod-chainsafe.eu.auth0.com/oauth/token` |

For devnet / other envs, copy the script and override these defaults. Most
of the underlying machinery (`check-canton-rights.sh`, `mint-to-party.go`)
is environment-agnostic.
