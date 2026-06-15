#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# recreate-mappings-prod1.sh — Recreate FingerprintMapping contracts under the NEW
# issuer party on prod1, after canton.issuer_party was changed. Restores
# fingerprint→party resolution (and thus USDCx balances + transfers) for every user.
#
# It reads the OLD mappings off the ledger and re-creates each under the new issuer
# (same user party / fingerprint / EVM address). No userstore DB access needed.
#
# What it does (in order):
#   1. Pulls the api-server's OAuth credentials from AWS Secrets Manager.
#   2. Starts a kubectl port-forward to the in-cluster Canton participant.
#   3. Writes a temporary config in /tmp (mode 600, deleted on exit) whose
#      canton.issuer_party is the NEW issuer.
#   4. Verifies the OAuth user has `can_act_as: <new-issuer>` (create authority).
#   5. Runs scripts/remote/recreate-fingerprint-mappings.go (dry run unless --apply).
#   6. Cleans up port-forward + temp config.
#
# See scripts/remote/RECREATE-MAPPINGS-PROD1.md for the operator runbook.
#
# Safe by default: it DRY-RUNS unless you pass --apply.
#
# Usage:
#   ./scripts/remote/recreate-mappings-prod1.sh --old-issuer 'OLD_ISSUER::1220...'
#   ./scripts/remote/recreate-mappings-prod1.sh --old-issuer 'OLD_ISSUER::1220...' --apply
#
# Required tools: aws (v2), kubectl, grpcurl, jq, python3, curl, go (1.24+).

set -euo pipefail

# ─── Defaults (override via flags) ───────────────────────────────────────────
OLD_ISSUER_PARTY=""
NEW_ISSUER_PARTY="chainsafe-middleware::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b"
OWNER_PARTY=""
K8S_NAMESPACE="canton-middleware"
K8S_SERVICE="svc/participant"
K8S_REMOTE_PORT="5001"
LOCAL_PORT="5001"
AWS_REGION="eu-north-1"
AWS_SECRET_ARN="arn:aws:secretsmanager:eu-north-1:905418303280:secret:infra-prod-canton-middleware-creds-RE1k5E"
DOMAIN_ID="global-domain::1220b1431ef217342db44d516bb9befde802be7d8899637d290895fa58880f19accc"
AUDIENCE="https://canton-ledger-api-prod1.02.chainsafe.dev"
TOKEN_URL="https://prod-chainsafe.eu.auth0.com/oauth/token"
APPLY=0
SKIP_PORT_FORWARD=0

usage() {
  cat <<EOF
Usage: $0 --old-issuer <party> [--new-issuer <party>] [-p <owner>] [--apply] [-n <ns>] [--no-port-forward]

Flags:
  --old-issuer     OLD issuer party whose mappings to recreate (required)
  --new-issuer     NEW issuer party to create mappings under (default: prod1 issuer)
  -p, --party      Optional: only recreate the mapping for this user party
  --apply          Perform creates. WITHOUT this flag the tool only dry-runs.
  -n, --namespace  K8s namespace (default: canton-middleware)
  --no-port-forward
                   Skip kubectl port-forward; assume Canton gRPC is reachable at localhost:${LOCAL_PORT}
  -h, --help       Show this help

Required tools: aws, kubectl, grpcurl, jq, python3, curl, go
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --old-issuer)   OLD_ISSUER_PARTY="$2"; shift 2 ;;
    --new-issuer)   NEW_ISSUER_PARTY="$2"; shift 2 ;;
    -p|--party)     OWNER_PARTY="$2"; shift 2 ;;
    --apply)        APPLY=1; shift ;;
    -n|--namespace) K8S_NAMESPACE="$2"; shift 2 ;;
    --no-port-forward) SKIP_PORT_FORWARD=1; shift ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown flag: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$OLD_ISSUER_PARTY" ]]; then
  echo "ERROR: --old-issuer <party> is required" >&2; usage >&2; exit 2
fi

for cmd in aws kubectl grpcurl jq python3 curl go; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing dependency: $cmd" >&2; exit 2; }
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ─── Pull secret from AWS Secrets Manager ────────────────────────────────────
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
echo "    OAuth client_id: ${CLIENT_ID}"
SECRET_JSON=""

# ─── Port-forward (or skip) ──────────────────────────────────────────────────
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

if [[ "$SKIP_PORT_FORWARD" -eq 0 ]]; then
  echo ">>> Starting kubectl port-forward ${K8S_NAMESPACE}/${K8S_SERVICE} ${LOCAL_PORT}:${K8S_REMOTE_PORT}..."
  kubectl -n "$K8S_NAMESPACE" port-forward "$K8S_SERVICE" "${LOCAL_PORT}:${K8S_REMOTE_PORT}" >/dev/null 2>&1 &
  PF_PID=$!
  for _ in $(seq 1 30); do nc -z localhost "$LOCAL_PORT" 2>/dev/null && break; sleep 0.5; done
  if ! nc -z localhost "$LOCAL_PORT" 2>/dev/null; then
    echo "FAILED: port-forward did not become ready on localhost:${LOCAL_PORT}" >&2; exit 2; fi
  echo "    Port-forward ready: localhost:${LOCAL_PORT} -> ${K8S_NAMESPACE}/${K8S_SERVICE}:${K8S_REMOTE_PORT}"
else
  echo ">>> Skipping kubectl port-forward (per --no-port-forward); assuming localhost:${LOCAL_PORT} is reachable"
fi

# ─── Write temp config (issuer_party = NEW issuer) ───────────────────────────
TMP_CFG=$(mktemp -t prod1-mappings-config.XXXXXX.yaml)
chmod 600 "$TMP_CFG"
cat >"$TMP_CFG" <<EOF
canton:
  domain_id: "${DOMAIN_ID}"
  issuer_party: "${NEW_ISSUER_PARTY}"

  ledger:
    rpc_url: "localhost:${LOCAL_PORT}"
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

# ─── Verify rights: can_act_as NEW issuer (create authority) ─────────────────
echo ""
echo ">>> Verifying OAuth user has can_act_as on the NEW issuer..."
CANTON_AUTH_CLIENT_ID="$CLIENT_ID" \
CANTON_AUTH_CLIENT_SECRET="$CLIENT_SECRET" \
CANTON_AUTH_AUDIENCE="$AUDIENCE" \
CANTON_AUTH_TOKEN_URL="$TOKEN_URL" \
CANTON_RPC_URL="localhost:${LOCAL_PORT}" \
CANTON_ISSUER_PARTY="$NEW_ISSUER_PARTY" \
bash "$SCRIPT_DIR/check-canton-rights.sh" || {
  rc=$?
  if [[ $rc -eq 1 ]]; then
    echo ""
    echo "✗ Rights check failed: missing can_act_as: $NEW_ISSUER_PARTY"
    echo "  This OAuth user cannot create FingerprintMappings under the new issuer."
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

ARGS=(-config "$TMP_CFG" -old-issuer "$OLD_ISSUER_PARTY")
[[ -n "$OWNER_PARTY" ]] && ARGS+=(-party "$OWNER_PARTY")
[[ "$APPLY" -eq 1 ]]    && ARGS+=(-apply)

( cd "$REPO_ROOT" && go run scripts/remote/recreate-fingerprint-mappings.go "${ARGS[@]}" )

CLIENT_SECRET=""

echo ""
if [[ "$APPLY" -eq 1 ]]; then
  echo "  Mapping recreation complete. USDCx balances/transfers should resolve again."
else
  echo "  Dry run complete. Re-run with --apply to create the mappings."
fi
