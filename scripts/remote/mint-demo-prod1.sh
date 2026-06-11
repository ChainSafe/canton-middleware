#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# mint-demo-prod1.sh — Mint DEMO tokens to a Canton party on prod1.
#
# Designed to be run by the infra team (or anyone with AWS access to the prod1
# api-server secret + kubectl access to the prod1 cluster).
#
# What it does (in order):
#   1. Pulls the api-server's OAuth credentials from AWS Secrets Manager.
#   2. Starts a kubectl port-forward to the in-cluster Canton participant.
#   3. Writes a temporary config in /tmp (mode 600, deleted on exit).
#   4. Verifies the OAuth user has `can_act_as: <issuer>` rights on Canton.
#   5. Runs scripts/remote/mint-to-party.go to exercise IssuerMint for DEMO.
#   6. Cleans up port-forward + temp config.
#
# See scripts/remote/MINT-DEMO-PROD1.md for the operator runbook.
#
# Usage:
#   ./scripts/remote/mint-demo-prod1.sh \
#     -p user_f39Fd6e5::1220eab9dc9b61bd5db2206550aacf9530c93c44c1935fd0412823c3afe5164a136a \
#     -a 1000
#
# Required tools: aws (v2), kubectl, grpcurl, jq, python3, curl, go (1.24+).

set -euo pipefail

# ─── Defaults (override via flags) ───────────────────────────────────────────
RECIPIENT_PARTY=""
AMOUNT="1000"
K8S_NAMESPACE="canton-middleware"
K8S_SERVICE="svc/participant"
K8S_REMOTE_PORT="5001"
LOCAL_PORT="5001"
AWS_REGION="eu-north-1"
AWS_SECRET_ARN="arn:aws:secretsmanager:eu-north-1:905418303280:secret:infra-prod-canton-middleware-creds-RE1k5E"
ISSUER_PARTY="daml-autopilot::122043f0b94e28125e4c65aa7e0f0ded912472731695f01cc83aa41ad3f03965a19b"
DOMAIN_ID="global-domain::1220b1431ef217342db44d516bb9befde802be7d8899637d290895fa58880f19accc"
AUDIENCE="https://canton-ledger-api-prod1.02.chainsafe.dev"
TOKEN_URL="https://prod-chainsafe.eu.auth0.com/oauth/token"
DRY_RUN=0
SKIP_PORT_FORWARD=0

# ─── Parse flags ─────────────────────────────────────────────────────────────
usage() {
  cat <<EOF
Usage: $0 -p <recipient-party> [-a <amount>] [-n <namespace>] [--dry-run] [--no-port-forward]

Flags:
  -p, --party      Recipient Canton party ID (required)
  -a, --amount     Amount of DEMO to mint (default: 1000)
  -n, --namespace  K8s namespace (default: canton-middleware)
  --dry-run        Verify rights only, do not mint
  --no-port-forward
                   Skip kubectl port-forward; assume Canton gRPC is reachable
                   at localhost:${LOCAL_PORT} already
  -h, --help       Show this help

Required tools: aws, kubectl, grpcurl, jq, python3, curl, go
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--party)     RECIPIENT_PARTY="$2"; shift 2 ;;
    -a|--amount)    AMOUNT="$2"; shift 2 ;;
    -n|--namespace) K8S_NAMESPACE="$2"; shift 2 ;;
    --dry-run)      DRY_RUN=1; shift ;;
    --no-port-forward) SKIP_PORT_FORWARD=1; shift ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown flag: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$RECIPIENT_PARTY" ]]; then
  echo "ERROR: -p <recipient-party> is required" >&2
  usage >&2
  exit 2
fi

# ─── Dependency check ────────────────────────────────────────────────────────
for cmd in aws kubectl grpcurl jq python3 curl go; do
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Missing dependency: $cmd" >&2
    exit 2
  }
done

# ─── Resolve repo root (script lives in scripts/remote/) ─────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ─── Pull secret from AWS Secrets Manager ────────────────────────────────────
echo ">>> Fetching OAuth credentials from AWS Secrets Manager (${AWS_REGION})..."

SECRET_JSON=$(aws secretsmanager get-secret-value \
  --region "$AWS_REGION" \
  --secret-id "$AWS_SECRET_ARN" \
  --query SecretString \
  --output text 2>&1) || {
  echo "FAILED to pull secret. AWS error:" >&2
  echo "$SECRET_JSON" >&2
  exit 2
}

# Look for client_id / client_secret under common key names.
extract_key() {
  local needles=("$@")
  for k in "${needles[@]}"; do
    local v
    v=$(jq -r --arg k "$k" '.[$k] // empty' <<<"$SECRET_JSON")
    if [[ -n "$v" && "$v" != "null" ]]; then
      echo "$v"
      return 0
    fi
  done
  return 1
}

CLIENT_ID=$(extract_key CANTON_AUTH_CLIENT_ID client_id canton_auth_client_id) || {
  echo "FAILED: secret has no client_id key. Available keys:" >&2
  jq 'keys' <<<"$SECRET_JSON" >&2
  exit 2
}

CLIENT_SECRET=$(extract_key CANTON_AUTH_CLIENT_SECRET client_secret canton_auth_client_secret) || {
  echo "FAILED: secret has no client_secret key. Available keys:" >&2
  jq 'keys' <<<"$SECRET_JSON" >&2
  exit 2
}

echo "    OAuth client_id: ${CLIENT_ID}"
SECRET_JSON=""

# ─── Start kubectl port-forward (or skip) ────────────────────────────────────
PF_PID=""
cleanup() {
  local exit_code=$?
  if [[ -n "$PF_PID" ]] && kill -0 "$PF_PID" 2>/dev/null; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi
  [[ -n "${TMP_CFG:-}" && -f "$TMP_CFG" ]] && rm -f "$TMP_CFG"
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

if [[ "$SKIP_PORT_FORWARD" -eq 0 ]]; then
  echo ">>> Starting kubectl port-forward ${K8S_NAMESPACE}/${K8S_SERVICE} ${LOCAL_PORT}:${K8S_REMOTE_PORT}..."
  kubectl -n "$K8S_NAMESPACE" port-forward "$K8S_SERVICE" "${LOCAL_PORT}:${K8S_REMOTE_PORT}" >/dev/null 2>&1 &
  PF_PID=$!

  # Wait for port to be listening
  for _ in $(seq 1 30); do
    if nc -z localhost "$LOCAL_PORT" 2>/dev/null; then
      break
    fi
    sleep 0.5
  done
  if ! nc -z localhost "$LOCAL_PORT" 2>/dev/null; then
    echo "FAILED: port-forward did not become ready on localhost:${LOCAL_PORT}" >&2
    exit 2
  fi
  echo "    Port-forward ready: localhost:${LOCAL_PORT} -> ${K8S_NAMESPACE}/${K8S_SERVICE}:${K8S_REMOTE_PORT}"
else
  echo ">>> Skipping kubectl port-forward (per --no-port-forward); assuming localhost:${LOCAL_PORT} is reachable"
fi

# ─── Write temp config ───────────────────────────────────────────────────────
TMP_CFG=$(mktemp -t prod1-mint-config.XXXXXX.yaml)
chmod 600 "$TMP_CFG"

cat >"$TMP_CFG" <<EOF
canton:
  domain_id: "${DOMAIN_ID}"
  issuer_party: "${ISSUER_PARTY}"

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

# Fields below are required by config.LoadAPIServer's validator but are not
# used by the mint flow. Stub values just satisfy validation.

token:
  supported_tokens:
    "0x0000000000000000000000000000000000000001":
      name: "Placeholder"
      symbol: "PLC"
      decimals: 18
      instrument_id: "PLC"
  native_balance_wei: "0"

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

# ─── Step 1: Verify rights ───────────────────────────────────────────────────
echo ""
echo ">>> Verifying OAuth user has can_act_as on issuer..."

CANTON_AUTH_CLIENT_ID="$CLIENT_ID" \
CANTON_AUTH_CLIENT_SECRET="$CLIENT_SECRET" \
CANTON_AUTH_AUDIENCE="$AUDIENCE" \
CANTON_AUTH_TOKEN_URL="$TOKEN_URL" \
CANTON_RPC_URL="localhost:${LOCAL_PORT}" \
CANTON_ISSUER_PARTY="$ISSUER_PARTY" \
bash "$SCRIPT_DIR/check-canton-rights.sh" || {
  rc=$?
  if [[ $rc -eq 1 ]]; then
    echo ""
    echo "✗ Rights check failed: missing can_act_as: $ISSUER_PARTY"
    echo "  This OAuth user cannot perform IssuerMint. Grant the right via"
    echo "  UserManagementService.GrantUserRights (same shape as the"
    echo "  can_execute_as_any_party grant) and re-run."
    exit 1
  else
    echo "✗ Rights check errored with exit code $rc — see above" >&2
    exit "$rc"
  fi
}

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo ""
  echo ">>> Dry run complete. Rights look good; mint not attempted (--dry-run)."
  exit 0
fi

# ─── Step 2: Mint ────────────────────────────────────────────────────────────
echo ""
echo ">>> Minting ${AMOUNT} DEMO to ${RECIPIENT_PARTY}..."

(
  cd "$REPO_ROOT"
  go run scripts/remote/mint-to-party.go \
    -config "$TMP_CFG" \
    -party "$RECIPIENT_PARTY" \
    -amount "$AMOUNT"
)

# Scrub the in-process secret now that the mint has run.
CLIENT_SECRET=""

echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo "  Mint complete."
echo "════════════════════════════════════════════════════════════════════════"
echo "  Recipient: ${RECIPIENT_PARTY}"
echo "  Amount:    ${AMOUNT} DEMO"
echo "  Issuer:    ${ISSUER_PARTY}"
echo ""
echo "  Verify via the api-server (replace <evm-addr> with the recipient's"
echo "  registered EVM address):"
echo "    curl 'https://middleware-api-prod1.02.chainsafe.dev/api/v1/balance?address=<evm-addr>'"
echo ""
echo "  Allow 10-30s for the indexer to pick up the new holding."
