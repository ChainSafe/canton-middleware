#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# check-canton-rights.sh — verify a Canton OAuth user's rights and report
# whether the user can perform IssuerMint (and other admin operations).
#
# Usage:
#   CANTON_AUTH_CLIENT_ID=...        \
#   CANTON_AUTH_CLIENT_SECRET=...    \
#   CANTON_AUTH_AUDIENCE=...         \
#   CANTON_AUTH_TOKEN_URL=...        \
#   CANTON_RPC_URL=host:port         \
#   CANTON_ISSUER_PARTY=...          \   # optional — gives a mint-readiness verdict
#   ./scripts/remote/check-canton-rights.sh
#
# Exit codes:
#   0 — user is ready to mint (or no issuer party provided, just dumped rights)
#   1 — user is NOT ready to mint (missing can_act_as on the configured issuer)
#   2 — setup error (missing deps, missing inputs, auth failure)
#
# Requires: curl, grpcurl, jq, python3.

set -euo pipefail

# ─── Dependency check ────────────────────────────────────────────────────────
for cmd in curl grpcurl jq python3; do
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Missing dependency: $cmd" >&2
    echo "  brew install grpcurl jq      # python3 + curl ship with macOS" >&2
    exit 2
  }
done

# ─── Required inputs ─────────────────────────────────────────────────────────
: "${CANTON_AUTH_CLIENT_ID:?CANTON_AUTH_CLIENT_ID is required}"
: "${CANTON_AUTH_CLIENT_SECRET:?CANTON_AUTH_CLIENT_SECRET is required}"
: "${CANTON_AUTH_AUDIENCE:?CANTON_AUTH_AUDIENCE is required}"
: "${CANTON_AUTH_TOKEN_URL:?CANTON_AUTH_TOKEN_URL is required}"
: "${CANTON_RPC_URL:?CANTON_RPC_URL is required (host:port)}"
ISSUER_PARTY="${CANTON_ISSUER_PARTY:-}"

# ─── Get OAuth token ─────────────────────────────────────────────────────────
echo ">>> Fetching OAuth token from ${CANTON_AUTH_TOKEN_URL}..."

TOKEN_RESP=$(curl -sS -X POST -H "content-type: application/json" \
  -d "$(jq -n \
      --arg id  "$CANTON_AUTH_CLIENT_ID"     \
      --arg sec "$CANTON_AUTH_CLIENT_SECRET" \
      --arg aud "$CANTON_AUTH_AUDIENCE"      \
      '{client_id:$id,client_secret:$sec,audience:$aud,grant_type:"client_credentials"}')" \
  "$CANTON_AUTH_TOKEN_URL")

TOKEN=$(jq -r '.access_token // empty' <<<"$TOKEN_RESP")
if [[ -z "$TOKEN" ]]; then
  echo "FAILED: token endpoint returned:" >&2
  jq . <<<"$TOKEN_RESP" >&2 2>/dev/null || echo "$TOKEN_RESP" >&2
  exit 2
fi

# ─── Decode JWT 'sub' for the ListUserRights user_id ─────────────────────────
USER_ID=$(python3 - "$TOKEN" <<'PY'
import sys, json, base64
parts = sys.argv[1].split('.')
if len(parts) < 2:
    sys.exit("JWT missing payload segment")
payload = parts[1] + '=' * (-len(parts[1]) % 4)
print(json.loads(base64.urlsafe_b64decode(payload)).get('sub', ''))
PY
)

if [[ -z "$USER_ID" ]]; then
  echo "FAILED: could not extract JWT 'sub' claim" >&2
  exit 2
fi

echo "    OAuth user: ${USER_ID}"
echo ""

# ─── List rights via grpcurl ─────────────────────────────────────────────────
echo ">>> Listing user rights at ${CANTON_RPC_URL}..."

GRPC_FLAGS=()
case "$CANTON_RPC_URL" in
  localhost*|127.0.0.1*|0.0.0.0*) GRPC_FLAGS+=("-plaintext") ;;
esac

RIGHTS=$(grpcurl "${GRPC_FLAGS[@]}" \
  -H "authorization: Bearer $TOKEN" \
  -d "$(jq -nc --arg uid "$USER_ID" '{user_id:$uid}')" \
  "$CANTON_RPC_URL" \
  com.daml.ledger.api.v2.admin.UserManagementService.ListUserRights)

echo "$RIGHTS" | jq .
echo ""

# ─── Classify ────────────────────────────────────────────────────────────────
HAS_ADMIN=$(jq      '[.rights[]?|select(.participantAdmin)]      | length > 0' <<<"$RIGHTS")
HAS_EXEC_ANY=$(jq   '[.rights[]?|select(.canExecuteAsAnyParty)]  | length > 0' <<<"$RIGHTS")
HAS_READ_ANY=$(jq   '[.rights[]?|select(.canReadAsAnyParty)]     | length > 0' <<<"$RIGHTS")
ACT_AS_PARTIES=$(jq -r '[.rights[]?|select(.canActAs)|.canActAs.party] | unique[]?' <<<"$RIGHTS")

echo "════════════════════════════════════════════════════════════════════════"
echo "  Rights summary for ${USER_ID}"
echo "════════════════════════════════════════════════════════════════════════"
[[ "$HAS_ADMIN"    == "true" ]] && echo "  ✓ participant_admin"         || echo "  ✗ participant_admin"
[[ "$HAS_EXEC_ANY" == "true" ]] && echo "  ✓ can_execute_as_any_party"  || echo "  ✗ can_execute_as_any_party"
[[ "$HAS_READ_ANY" == "true" ]] && echo "  ✓ can_read_as_any_party"     || echo "  ✗ can_read_as_any_party"
if [[ -n "$ACT_AS_PARTIES" ]]; then
  echo "  can_act_as:"
  while IFS= read -r p; do echo "    - $p"; done <<<"$ACT_AS_PARTIES"
else
  echo "  can_act_as: (none)"
fi
echo ""

# ─── Verdict for mint (only when an issuer party was provided) ───────────────
if [[ -n "$ISSUER_PARTY" ]]; then
  echo "════════════════════════════════════════════════════════════════════════"
  echo "  Verdict for IssuerMint on ${ISSUER_PARTY}"
  echo "════════════════════════════════════════════════════════════════════════"
  if grep -qFx "$ISSUER_PARTY" <<<"$ACT_AS_PARTIES"; then
    echo "  ✓ Ready to mint — explicit can_act_as: $ISSUER_PARTY granted."
    exit 0
  elif [[ "$HAS_ADMIN" == "true" ]]; then
    echo "  ⚠ No explicit can_act_as for the issuer, but participant_admin"
    echo "    grants implicit rights — mint should succeed. Proceed."
    exit 0
  else
    echo "  ✗ Missing can_act_as: $ISSUER_PARTY"
    echo ""
    echo "  → Ask Hamid to grant via UserManagementService.GrantUserRights"
    echo "    (Same shape as the can_execute_as_any_party grant on prod1.)"
    exit 1
  fi
fi
