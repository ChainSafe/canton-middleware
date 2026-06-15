#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# recreate-mappings.sh — Recreate FingerprintMapping contracts under the current
# canton.issuer_party after the issuer was changed. Restores fingerprint→party
# resolution (and thus USDCx balances + transfers) for ALL users in one run.
#
# Works for multiple environments via --env (default: prod1):
#   prod1 — creds from AWS Secrets Manager, reaches Canton via kubectl port-forward.
#   dev1  — creds from CANTON_AUTH_CLIENT_ID/SECRET env vars, reaches Canton directly
#           (no AWS, no port-forward).
#
# It reads every FingerprintMapping off the ledger and re-creates the ones not already
# under the current issuer. No userstore DB, no old-issuer party needed.
#
# See scripts/remote/RECREATE-MAPPINGS.md for the operator runbook.
#
# Safe by default: it DRY-RUNS unless you pass --apply.
#
# Usage:
#   # prod1 (default): dry-run all users, then apply
#   ./scripts/remote/recreate-mappings.sh
#   ./scripts/remote/recreate-mappings.sh --apply
#   # dev1: set creds first, then run
#   export CANTON_AUTH_CLIENT_ID=... CANTON_AUTH_CLIENT_SECRET=...
#   ./scripts/remote/recreate-mappings.sh --env dev1 --apply
#
# Required tools: kubectl/aws (prod1 only), grpcurl, jq, curl, go (1.24+).

set -euo pipefail

# ─── Args ────────────────────────────────────────────────────────────────────
ENV_NAME="prod1"
OWNER_PARTY=""
NEW_ISSUER_OVERRIDE=""
APPLY=0
SKIP_PORT_FORWARD=0
K8S_NAMESPACE="canton-middleware"
K8S_SERVICE="svc/participant"
K8S_REMOTE_PORT="5001"
LOCAL_PORT="5001"

usage() {
  cat <<EOF
Usage: $0 [--env prod1|dev1] [--new-issuer <party>] [-p <owner>] [--apply] [-n <ns>] [--no-port-forward]

Migrates ALL users by default (recreates every mapping not already under the current
issuer). Use -p to scope to a single user.

Flags:
  --env            Target environment: prod1 (default) or dev1
  --new-issuer     Override the current issuer party (default: per-env)
  -p, --party      Optional: only recreate the mapping for this user party
  --apply          Perform creates. WITHOUT this flag the tool only dry-runs.
  -n, --namespace  K8s namespace (prod1 port-forward; default: canton-middleware)
  --no-port-forward
                   Skip kubectl port-forward; assume Canton gRPC is already reachable
  -h, --help       Show this help

Credentials:
  prod1 — pulled from AWS Secrets Manager (needs aws + kubectl access).
  dev1  — read from CANTON_AUTH_CLIENT_ID and CANTON_AUTH_CLIENT_SECRET env vars.

Required tools: grpcurl, jq, curl, go (+ aws, kubectl for prod1)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)          ENV_NAME="$2"; shift 2 ;;
    --new-issuer)   NEW_ISSUER_OVERRIDE="$2"; shift 2 ;;
    -p|--party)     OWNER_PARTY="$2"; shift 2 ;;
    --apply)        APPLY=1; shift ;;
    -n|--namespace) K8S_NAMESPACE="$2"; shift 2 ;;
    --no-port-forward) SKIP_PORT_FORWARD=1; shift ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown flag: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# ─── Environment presets ─────────────────────────────────────────────────────
AWS_REGION="eu-north-1"
case "$ENV_NAME" in
  prod1)
    USE_AWS=1
    USE_PORT_FORWARD=1
    AWS_SECRET_ARN="arn:aws:secretsmanager:eu-north-1:905418303280:secret:infra-prod-canton-middleware-creds-RE1k5E"
    DOMAIN_ID="global-domain::1220b1431ef217342db44d516bb9befde802be7d8899637d290895fa58880f19accc"
    AUDIENCE="https://canton-ledger-api-prod1.02.chainsafe.dev"
    TOKEN_URL="https://prod-chainsafe.eu.auth0.com/oauth/token"
    NEW_ISSUER_PARTY="chainsafe-middleware::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b"
    DIRECT_RPC=""  # via port-forward
    BALANCE_API="https://middleware-api-prod1.02.chainsafe.dev"
    ;;
  dev1)
    USE_AWS=0
    USE_PORT_FORWARD=0
    DOMAIN_ID="global-domain::1220be58c29e65de40bf273be1dc2b266d43a9a002ea5b18955aeef7aac881bb471a"
    AUDIENCE="https://canton-ledger-api-dev1.01.chainsafe.dev"
    TOKEN_URL="https://dev-2j3m40ajwym1zzaq.eu.auth0.com/oauth/token"
    NEW_ISSUER_PARTY="daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c"
    DIRECT_RPC="canton-ledger-api-grpc-dev1.chainsafe.dev:80"
    BALANCE_API="https://middleware-api-dev1.01.chainsafe.dev"
    ;;
  *) echo "ERROR: unknown --env '$ENV_NAME' (expected prod1 or dev1)" >&2; exit 2 ;;
esac
[[ -n "$NEW_ISSUER_OVERRIDE" ]] && NEW_ISSUER_PARTY="$NEW_ISSUER_OVERRIDE"
[[ "$SKIP_PORT_FORWARD" -eq 1 ]] && USE_PORT_FORWARD=0

# ─── Dependency check ────────────────────────────────────────────────────────
DEPS=(grpcurl jq curl go)
[[ "$USE_AWS" -eq 1 ]] && DEPS+=(aws)
[[ "$USE_PORT_FORWARD" -eq 1 ]] && DEPS+=(kubectl)
for cmd in "${DEPS[@]}"; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing dependency: $cmd" >&2; exit 2; }
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ─── Credentials ─────────────────────────────────────────────────────────────
if [[ "$USE_AWS" -eq 1 ]]; then
  echo ">>> Fetching OAuth credentials from AWS Secrets Manager (${AWS_REGION})..."
  SECRET_JSON=$(aws secretsmanager get-secret-value \
    --region "$AWS_REGION" --secret-id "$AWS_SECRET_ARN" \
    --query SecretString --output text 2>&1) || {
    echo "FAILED to pull secret. AWS error:" >&2; echo "$SECRET_JSON" >&2; exit 2; }
  extract_key() {
    local needles=("$@")
    for k in "${needles[@]}"; do
      local v; v=$(jq -r --arg k "$k" '.[$k] // empty' <<<"$SECRET_JSON")
      if [[ -n "$v" && "$v" != "null" ]]; then echo "$v"; return 0; fi
    done
    return 1
  }
  CLIENT_ID=$(extract_key CANTON_AUTH_CLIENT_ID client_id canton_auth_client_id) || {
    echo "FAILED: secret has no client_id key. Available keys:" >&2; jq 'keys' <<<"$SECRET_JSON" >&2; exit 2; }
  CLIENT_SECRET=$(extract_key CANTON_AUTH_CLIENT_SECRET client_secret canton_auth_client_secret) || {
    echo "FAILED: secret has no client_secret key. Available keys:" >&2; jq 'keys' <<<"$SECRET_JSON" >&2; exit 2; }
  SECRET_JSON=""
else
  echo ">>> Reading OAuth credentials from environment (CANTON_AUTH_CLIENT_ID/SECRET)..."
  CLIENT_ID="${CANTON_AUTH_CLIENT_ID:-}"
  CLIENT_SECRET="${CANTON_AUTH_CLIENT_SECRET:-}"
  if [[ -z "$CLIENT_ID" || -z "$CLIENT_SECRET" ]]; then
    echo "ERROR: --env ${ENV_NAME} needs CANTON_AUTH_CLIENT_ID and CANTON_AUTH_CLIENT_SECRET set" >&2
    exit 2
  fi
fi
echo "    OAuth client_id: ${CLIENT_ID}"

# ─── Port-forward (prod1) or direct (dev1) ───────────────────────────────────
PF_PID=""
cleanup() {
  local exit_code=$?
  if [[ -n "$PF_PID" ]] && kill -0 "$PF_PID" 2>/dev/null; then
    kill "$PF_PID" 2>/dev/null || true; wait "$PF_PID" 2>/dev/null || true
  fi
  [[ -n "${TMP_CFG:-}" && -f "$TMP_CFG" ]] && rm -f "$TMP_CFG"
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

if [[ "$USE_PORT_FORWARD" -eq 1 ]]; then
  echo ">>> Starting kubectl port-forward ${K8S_NAMESPACE}/${K8S_SERVICE} ${LOCAL_PORT}:${K8S_REMOTE_PORT}..."
  kubectl -n "$K8S_NAMESPACE" port-forward "$K8S_SERVICE" "${LOCAL_PORT}:${K8S_REMOTE_PORT}" >/dev/null 2>&1 &
  PF_PID=$!
  for _ in $(seq 1 30); do nc -z localhost "$LOCAL_PORT" 2>/dev/null && break; sleep 0.5; done
  if ! nc -z localhost "$LOCAL_PORT" 2>/dev/null; then
    echo "FAILED: port-forward did not become ready on localhost:${LOCAL_PORT}" >&2; exit 2; fi
  RPC_URL="localhost:${LOCAL_PORT}"
  echo "    Port-forward ready: ${RPC_URL} -> ${K8S_NAMESPACE}/${K8S_SERVICE}:${K8S_REMOTE_PORT}"
else
  RPC_URL="${DIRECT_RPC:-localhost:${LOCAL_PORT}}"
  echo ">>> Using Canton gRPC at ${RPC_URL} (no port-forward)"
fi

# ─── Temp config (issuer_party = current/new issuer) ─────────────────────────
TMP_CFG=$(mktemp -t "${ENV_NAME}-mappings-config.XXXXXX.yaml")
chmod 600 "$TMP_CFG"
cat >"$TMP_CFG" <<EOF
canton:
  domain_id: "${DOMAIN_ID}"
  issuer_party: "${NEW_ISSUER_PARTY}"

  ledger:
    rpc_url: "${RPC_URL}"
    ledger_id: ""
    max_inbound_message_size: 52428800
    tls:
      enabled: false
    auth:
      client_id: "${CLIENT_ID}"
      client_secret: "${CLIENT_SECRET}"
      audience: "${AUDIENCE}"
      token_url: "${TOKEN_URL}"
      expiry_leeway: "60s"

  identity:
    package_id: "#common"

  token:
    cip56_package_id: "#cip56-token"
    splice_transfer_package_id: "#splice-api-token-transfer-instruction-v1"
    splice_holding_package_id: "#splice-api-token-holding-v1"

# Fields below satisfy config.LoadAPIServer's validator but are unused by this flow.
token:
  supported_tokens:
    "0x0000000000000000000000000000000000000001":
      name: "Placeholder"
      symbol: "PLC"
      decimals: 18
      instrument_id: "PLC"
eth_rpc:
  enabled: false
monitoring:
  enabled: false
key_management:
  master_key_env: "CANTON_MASTER_KEY"
  key_derivation: "generate"
server:
  host: "0.0.0.0"
  port: 8081
database:
  url: "postgres://unused"
  ssl_mode: "disable"
  timeout: 10
  pool_size: 1
logging:
  level: "info"
  format: "console"
  output_path: "stdout"
EOF

# ─── Verify rights: can_act_as current issuer (create authority) ─────────────
echo ""
echo ">>> Verifying OAuth user has can_act_as on the current issuer..."
CANTON_AUTH_CLIENT_ID="$CLIENT_ID" \
CANTON_AUTH_CLIENT_SECRET="$CLIENT_SECRET" \
CANTON_AUTH_AUDIENCE="$AUDIENCE" \
CANTON_AUTH_TOKEN_URL="$TOKEN_URL" \
CANTON_RPC_URL="$RPC_URL" \
CANTON_ISSUER_PARTY="$NEW_ISSUER_PARTY" \
bash "$SCRIPT_DIR/check-canton-rights.sh" || {
  rc=$?
  if [[ $rc -eq 1 ]]; then
    echo ""
    echo "✗ Rights check failed: missing can_act_as: $NEW_ISSUER_PARTY"
    echo "  This OAuth user cannot create FingerprintMappings under the current issuer."
    echo "  Grant the right via UserManagementService.GrantUserRights and re-run."
    exit 1
  else
    echo "✗ Rights check errored with exit code $rc — see above" >&2
    exit "$rc"
  fi
}

# ─── Run (dry-run unless --apply) ────────────────────────────────────────────
echo ""
if [[ "$APPLY" -eq 1 ]]; then
  echo ">>> Recreating FingerprintMappings under ${NEW_ISSUER_PARTY}..."
else
  echo ">>> DRY RUN — listing mappings that WOULD be recreated..."
fi

ARGS=(-config "$TMP_CFG")
[[ -n "$OWNER_PARTY" ]] && ARGS+=(-party "$OWNER_PARTY")
[[ "$APPLY" -eq 1 ]]    && ARGS+=(-apply)

( cd "$REPO_ROOT" && go run scripts/remote/recreate-fingerprint-mappings.go "${ARGS[@]}" )

CLIENT_SECRET=""

echo ""
if [[ "$APPLY" -eq 1 ]]; then
  echo "  Mapping recreation complete. USDCx balances/transfers should resolve again."
  echo "  Verify (replace <evm-addr>): curl '${BALANCE_API}/api/v1/balance?address=<evm-addr>'"
else
  echo "  Dry run complete. Re-run with --apply to create the mappings."
fi
