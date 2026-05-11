#!/bin/bash
# =============================================================================
# Docker Bootstrap Script
# =============================================================================
# This script runs inside the bootstrap container to set up the Canton bridge.
# It:
# 1. Waits for Canton to be ready
# 2. Allocates a BridgeIssuer party
# 3. Gets the domain ID
# 4. Updates the config file
# 5. Runs bootstrap-bridge
# 6. Runs register-user
# =============================================================================

set -e

CONFIG_FILE="${CONFIG_FILE:-/app/config.yaml}"
API_SERVER_CONFIG_FILE="${API_SERVER_CONFIG_FILE:-/app/api-server-config.yaml}"
INDEXER_CONFIG_FILE="${INDEXER_CONFIG_FILE:-/app/indexer-config.yaml}"
SELECTED_ENV="${ENV:-docker}"
CONFIG_DEFAULTS_DIR="${CONFIG_DEFAULTS_DIR:-/app/config/defaults}"
CANTON_HTTP="${CANTON_HTTP:-http://canton:5013}"
BROADCAST_DIR="${BROADCAST_DIR:-/app/broadcast}"
MAX_RETRIES=60

echo "========================================================================"
echo "DOCKER BOOTSTRAP"
echo "========================================================================"
echo "Canton HTTP API: $CANTON_HTTP"
echo "Config env: $SELECTED_ENV"
echo "Config file: $CONFIG_FILE"
echo "API Server config: $API_SERVER_CONFIG_FILE"
echo "Broadcast dir: $BROADCAST_DIR"
echo ""

case "$SELECTED_ENV" in
    docker)
        RELAYER_TEMPLATE="${CONFIG_DEFAULTS_DIR}/config.relayer.docker.yaml"
        API_SERVER_TEMPLATE="${CONFIG_DEFAULTS_DIR}/config.api-server.docker.yaml"
        INDEXER_TEMPLATE="${CONFIG_DEFAULTS_DIR}/config.indexer.docker.yaml"
        ;;
    devnet|local-devnet)
        RELAYER_TEMPLATE="${CONFIG_DEFAULTS_DIR}/config.relayer.local-devnet.yaml"
        API_SERVER_TEMPLATE="${CONFIG_DEFAULTS_DIR}/config.api-server.local-devnet.yaml"
        INDEXER_TEMPLATE="${CONFIG_DEFAULTS_DIR}/config.indexer.local-devnet.yaml"
        ;;
    *)
        echo "ERROR: Unsupported ENV '$SELECTED_ENV' (expected docker|devnet)"
        exit 1
        ;;
esac

if [ ! -f "$RELAYER_TEMPLATE" ] || [ ! -f "$API_SERVER_TEMPLATE" ] || [ ! -f "$INDEXER_TEMPLATE" ]; then
    echo "ERROR: Missing config templates in ${CONFIG_DEFAULTS_DIR}"
    echo "  relayer template: $RELAYER_TEMPLATE"
    echo "  api template:     $API_SERVER_TEMPLATE"
    echo "  indexer template: $INDEXER_TEMPLATE"
    exit 1
fi

mkdir -p "$(dirname "$CONFIG_FILE")" "$(dirname "$API_SERVER_CONFIG_FILE")" "$(dirname "$INDEXER_CONFIG_FILE")"
cp "$RELAYER_TEMPLATE" "$CONFIG_FILE"
cp "$API_SERVER_TEMPLATE" "$API_SERVER_CONFIG_FILE"
cp "$INDEXER_TEMPLATE" "$INDEXER_CONFIG_FILE"
echo ">>> Selected templates:"
echo "    Relayer:    $RELAYER_TEMPLATE"
echo "    API Server: $API_SERVER_TEMPLATE"
echo "    Indexer:    $INDEXER_TEMPLATE"
echo ""

# =============================================================================
# Update Ethereum contract addresses from broadcast
# =============================================================================
echo ">>> Updating Ethereum contract addresses from deployment..."
BROADCAST_FILE="${BROADCAST_DIR}/Deployer.s.sol/31337/run-latest.json"
if [ -f "$BROADCAST_FILE" ]; then
    TOKEN_ADDR=$(jq -r '.transactions[] | select(.contractName == "PromptToken" and .transactionType == "CREATE") | .contractAddress' "$BROADCAST_FILE" 2>/dev/null || echo "")
    BRIDGE_ADDR=$(jq -r '.transactions[] | select(.contractName == "CantonBridge" and .transactionType == "CREATE") | .contractAddress' "$BROADCAST_FILE" 2>/dev/null || echo "")
    
    if [ -n "$TOKEN_ADDR" ] && [ -n "$BRIDGE_ADDR" ]; then
        echo "    Token contract: $TOKEN_ADDR"
        echo "    Bridge contract: $BRIDGE_ADDR"
        sed -i "s|token_contract: \"0x[a-fA-F0-9]*\"|token_contract: \"$TOKEN_ADDR\"|" "$CONFIG_FILE"
        sed -i "s|bridge_contract: \"0x[a-fA-F0-9]*\"|bridge_contract: \"$BRIDGE_ADDR\"|" "$CONFIG_FILE"
        echo "    Config updated with deployed contract addresses"
    else
        echo "    [WARN] Could not extract contract addresses from broadcast"
    fi
else
    echo "    [WARN] Broadcast file not found: $BROADCAST_FILE"
    echo "    Using default contract addresses from config template"
fi
echo ""

# =============================================================================
# Wait for Canton HTTP API
# =============================================================================
echo ">>> Waiting for Canton HTTP API..."
attempt=0
while [ $attempt -lt $MAX_RETRIES ]; do
    if curl -s "${CANTON_HTTP}/v2/version" >/dev/null 2>&1; then
        echo "    Canton HTTP API is ready!"
        break
    fi
    echo -n "."
    sleep 2
    attempt=$((attempt + 1))
done

if [ $attempt -ge $MAX_RETRIES ]; then
    echo ""
    echo "ERROR: Canton HTTP API not ready after $MAX_RETRIES attempts"
    exit 1
fi

# =============================================================================
# Wait for Canton to connect to synchronizer
# =============================================================================
echo ""
echo ">>> Waiting for Canton to connect to synchronizer..."
attempt=0
while [ $attempt -lt $MAX_RETRIES ]; do
    sync_count=$(curl -s "${CANTON_HTTP}/v2/state/connected-synchronizers" 2>/dev/null | jq '.connectedSynchronizers | length' 2>/dev/null || echo "0")
    if [ "$sync_count" -gt 0 ] 2>/dev/null; then
        echo "    Canton connected to synchronizer!"
        break
    fi
    echo -n "."
    sleep 2
    attempt=$((attempt + 1))
done

if [ $attempt -ge $MAX_RETRIES ]; then
    echo ""
    echo "ERROR: Canton not connected to synchronizer"
    exit 1
fi

# =============================================================================
# Wait for DAR packages
# =============================================================================
echo ""
echo ">>> Waiting for DAR packages to be uploaded..."
attempt=0
while [ $attempt -lt $MAX_RETRIES ]; do
    pkg_count=$(curl -s "${CANTON_HTTP}/v2/packages" 2>/dev/null | jq '.packageIds | length' 2>/dev/null || echo "0")
    if [ "$pkg_count" -ge 30 ] 2>/dev/null; then
        echo "    Found $pkg_count packages!"
        break
    fi
    echo -n "."
    sleep 2
    attempt=$((attempt + 1))
done

if [ $attempt -ge $MAX_RETRIES ]; then
    echo ""
    echo "ERROR: DAR packages not uploaded"
    exit 1
fi

# Wait for package vetting to propagate on the synchronizer
echo ""
echo ">>> Waiting for package vetting to propagate..."
sleep 10
echo "    Vetting propagation wait complete"

# =============================================================================
# Allocate BridgeIssuer party
# =============================================================================
echo ""
echo ">>> Allocating BridgeIssuer party..."
EXISTING_PARTY=$(curl -s "${CANTON_HTTP}/v2/parties" | jq -r '.partyDetails[].party' | grep "^BridgeIssuer::" | head -1 || true)

if [ -n "$EXISTING_PARTY" ]; then
    echo "    BridgeIssuer already exists: $EXISTING_PARTY"
    PARTY_ID="$EXISTING_PARTY"
else
    PARTY_RESPONSE=$(curl -s -X POST "${CANTON_HTTP}/v2/parties" \
        -H 'Content-Type: application/json' \
        -d '{"partyIdHint": "BridgeIssuer"}')
    PARTY_ID=$(echo "$PARTY_RESPONSE" | jq -r '.partyDetails.party // empty')
    echo "    Allocated: $PARTY_ID"
fi

if [ -z "$PARTY_ID" ]; then
    echo "ERROR: Failed to allocate party"
    exit 1
fi

# =============================================================================
# Get domain ID
# =============================================================================
echo ""
echo ">>> Getting domain ID..."
SYNC_RESPONSE=$(curl -s "${CANTON_HTTP}/v2/state/connected-synchronizers")
DOMAIN_ID=$(echo "$SYNC_RESPONSE" | jq -r '.connectedSynchronizers[0].synchronizerId // empty')
echo "    Domain ID: $DOMAIN_ID"

if [ -z "$DOMAIN_ID" ]; then
    echo "ERROR: Failed to get domain ID"
    exit 1
fi

# =============================================================================
# Update config file
# =============================================================================
echo ""
echo ">>> Updating config file..."
sed -i "s|domain_id: \".*\"|domain_id: \"$DOMAIN_ID\"|" "$CONFIG_FILE"
sed -i "s|issuer_party: \".*\"|issuer_party: \"$PARTY_ID\"|" "$CONFIG_FILE"
echo "    Config updated with issuer_party and domain_id"

# =============================================================================
# Update API server config file
# =============================================================================
if [ -f "$API_SERVER_CONFIG_FILE" ]; then
    echo ""
    echo ">>> Updating API server config file..."
    sed -i "s|domain_id: \".*\"|domain_id: \"$DOMAIN_ID\"|" "$API_SERVER_CONFIG_FILE"
    sed -i "s|issuer_party: \".*\"|issuer_party: \"$PARTY_ID\"|" "$API_SERVER_CONFIG_FILE"
    sed -i "s|\${CANTON_ISSUER_PARTY}|$PARTY_ID|g" "$API_SERVER_CONFIG_FILE"
    echo "    API server config updated with issuer_party, domain_id, and instrument admins"
fi

# =============================================================================
# Update indexer config file
# =============================================================================
if [ -f "$INDEXER_CONFIG_FILE" ]; then
    echo ""
    echo ">>> Updating indexer config file..."
    sed -i "s|party: \".*\"|party: \"$PARTY_ID\"|" "$INDEXER_CONFIG_FILE"
    echo "    Indexer config updated with party=$PARTY_ID"
fi

# =============================================================================
# Run bootstrap-bridge
# =============================================================================
echo ""
echo ">>> Running bootstrap-bridge..."
/app/bootstrap-bridge -config "$API_SERVER_CONFIG_FILE" -issuer "$PARTY_ID" || {
    echo "    [WARN] Bootstrap may have failed or contracts already exist"
}

# =============================================================================
# Run bootstrap-demo (TokenConfig only — no user minting)
# =============================================================================
# Creates the DEMO CIP56.TokenConfig and CIP56TransferFactory on the Canton
# ledger so that E2E tests can call MintToken("DEMO") for any party.
# User minting is skipped (-no-mint) because test users are registered
# per-test and their parties are not known at bootstrap time.
echo ""
echo ">>> Running bootstrap-demo (no-mint mode)..."
/app/bootstrap-demo -config "$API_SERVER_CONFIG_FILE" \
    -issuer "$PARTY_ID" -domain "$DOMAIN_ID" \
    -no-mint || {
    echo "    [WARN] bootstrap-demo may have failed or TokenConfig already exists"
}

# =============================================================================
# Run register-user
# =============================================================================
echo ""
echo ">>> Running register-user..."
/app/register-user -config "$API_SERVER_CONFIG_FILE" -party "$PARTY_ID" || {
    echo "    [WARN] User registration may have failed or user already exists"
}

# =============================================================================
# Bootstrap USDCx on participant2
# =============================================================================
# USDCxIssuer lives on participant2 (the "external" node). The middleware
# (participant1) sees all USDCx events via FiltersForAnyParty on the indexer.
CANTON_P2_HTTP="${CANTON_P2_HTTP:-http://canton:5023}"
CANTON_P2_GRPC="${CANTON_P2_GRPC:-canton:5021}"
CANTON_P2_AUDIENCE="${CANTON_P2_AUDIENCE:-http://canton:5021}"
CANTON_AUTH_CLIENT_ID="${CANTON_AUTH_CLIENT_ID:-local-test-client}"
CANTON_AUTH_CLIENT_SECRET="${CANTON_AUTH_CLIENT_SECRET:-local-test-secret}"

echo ""
echo ">>> Waiting for participant2 HTTP API..."
attempt=0
while [ $attempt -lt $MAX_RETRIES ]; do
    if curl -s "${CANTON_P2_HTTP}/v2/version" >/dev/null 2>&1; then
        echo "    Participant2 HTTP API is ready!"
        break
    fi
    echo -n "."
    sleep 2
    attempt=$((attempt + 1))
done

if [ $attempt -ge $MAX_RETRIES ]; then
    echo ""
    echo "[WARN] Participant2 HTTP API not ready — skipping USDCx bootstrap"
else
    # The docker-compose canton healthcheck ensures P2 has >= 30 packages uploaded,
    # but package vetting (the topology transaction that authorises packages for use
    # in contracts) propagates asynchronously on the synchronizer. Wait for it here,
    # mirroring the equivalent sleep done for P1 above.
    echo ""
    echo ">>> Waiting for P2 package vetting to propagate..."
    sleep 10
    echo "    P2 vetting propagation wait complete"

    # P2 reports HTTP-ready before its synchronizer connection is fully established.
    # Allocating a party in that window returns PARTY_ALLOCATION_WITHOUT_CONNECTED_SYNCHRONIZER.
    # Poll until P2 is connected, mirroring the P1 wait above.
    echo ""
    echo ">>> Waiting for participant2 to connect to synchronizer..."
    attempt=0
    while [ $attempt -lt $MAX_RETRIES ]; do
        p2_sync_count=$(curl -s "${CANTON_P2_HTTP}/v2/state/connected-synchronizers" 2>/dev/null | jq '.connectedSynchronizers | length' 2>/dev/null || echo "0")
        if [ "$p2_sync_count" -gt 0 ] 2>/dev/null; then
            echo "    Participant2 connected to synchronizer!"
            break
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done

    echo ""
    echo ">>> Allocating USDCxIssuer party on participant2..."
    USDCX_EXISTING=$(curl -s "${CANTON_P2_HTTP}/v2/parties" | jq -r '.partyDetails[].party' | grep "^USDCxIssuer::" | head -1 || true)

    if [ -n "$USDCX_EXISTING" ]; then
        echo "    USDCxIssuer already exists: $USDCX_EXISTING"
        USDCX_PARTY_ID="$USDCX_EXISTING"
    else
        # P2 occasionally returns an empty body for the first allocation request
        # immediately after package vetting completes, even though /v2/parties
        # GET succeeds. Retry up to 12 times (≈60s) before giving up. Without the
        # retry, USDCx bootstrap silently skips and downstream consumers (registry,
        # api-server, e2e tests) fail with confusing "USDCxIssuer not found" errors.
        USDCX_PARTY_ID=""
        for attempt in $(seq 1 12); do
            USDCX_PARTY_RESPONSE=$(curl -s -X POST "${CANTON_P2_HTTP}/v2/parties" \
                -H 'Content-Type: application/json' \
                -d '{"partyIdHint": "USDCxIssuer"}')
            USDCX_PARTY_ID=$(echo "$USDCX_PARTY_RESPONSE" | jq -r '.partyDetails.party // empty')
            if [ -n "$USDCX_PARTY_ID" ]; then
                echo "    Allocated: $USDCX_PARTY_ID"
                break
            fi
            echo "    Attempt $attempt/12 failed (response: ${USDCX_PARTY_RESPONSE:-<empty>}), retrying in 5s..."
            sleep 5
        done
    fi

    if [ -z "$USDCX_PARTY_ID" ]; then
        echo "[WARN] Failed to allocate USDCxIssuer — skipping USDCx bootstrap"
    else
        echo ""
        echo ">>> Running bootstrap-usdcx on participant2..."
        # Retry to absorb INVALID_PRESCRIBED_SYNCHRONIZER_ID errors that occur
        # when P2's package vetting (e.g. CIP56) hasn't yet propagated to the
        # synchronizer. Vetting is async after package upload + party allocation,
        # and the time before the first command succeeds is non-deterministic.
        usdcx_ok=0
        for attempt in $(seq 1 10); do
            if /app/bootstrap-usdcx \
                -p2           "$CANTON_P2_GRPC" \
                -p2-audience  "$CANTON_P2_AUDIENCE" \
                -issuer       "$USDCX_PARTY_ID" \
                -domain       "$DOMAIN_ID" \
                -token-url    "http://mock-oauth2:8088/oauth/token" \
                -client-id    "$CANTON_AUTH_CLIENT_ID" \
                -client-secret "$CANTON_AUTH_CLIENT_SECRET"; then
                usdcx_ok=1
                break
            fi
            echo "    [WARN] bootstrap-usdcx attempt $attempt/10 failed, retrying in 10s..."
            sleep 10
        done
        if [ "$usdcx_ok" -ne 1 ]; then
            echo "    [WARN] bootstrap-usdcx exhausted retries — USDCx contracts may be missing"
        fi

        # Substitute USDCx issuer party in api-server config
        if [ -f "$API_SERVER_CONFIG_FILE" ]; then
            sed -i "s|\${CANTON_USDCX_ISSUER_PARTY}|$USDCX_PARTY_ID|g" "$API_SERVER_CONFIG_FILE"
            echo "    API server config updated with CANTON_USDCX_ISSUER_PARTY=$USDCX_PARTY_ID"
        fi
    fi
fi

# =============================================================================
# Write E2E deploy manifest
# =============================================================================
E2E_MANIFEST_FILE="${E2E_MANIFEST_FILE:-/tmp/e2e-deploy.json}"
# Both instruments are administered by the same party in the single-party dev
# stack. If the stack ever gains separate per-instrument admin parties, update
# PROMPT_INSTRUMENT_ADMIN and DEMO_INSTRUMENT_ADMIN independently here.
cat > "$E2E_MANIFEST_FILE" <<JSON
{
  "prompt_token":            "${TOKEN_ADDR}",
  "canton_bridge":           "${BRIDGE_ADDR}",
  "prompt_instrument_admin": "${PARTY_ID}",
  "prompt_instrument_id":    "PROMPT",
  "demo_instrument_admin":   "${PARTY_ID}",
  "demo_instrument_id":      "DEMO",
  "usdcx_instrument_admin":  "${USDCX_PARTY_ID}",
  "usdcx_instrument_id":     "USDCx"
}
JSON
echo ">>> Deploy manifest written to $E2E_MANIFEST_FILE"

# =============================================================================
# Done
# =============================================================================
echo ""
echo "========================================================================"
echo "BOOTSTRAP COMPLETE"
echo "========================================================================"
echo "Party ID:       $PARTY_ID"
echo "Domain ID:      $DOMAIN_ID"
echo "USDCx Party ID: ${USDCX_PARTY_ID:-<not bootstrapped>}"
echo ""
echo "The relayer can now be started."
echo "========================================================================"

# If WAIT_FOREVER is set, keep the container running (useful for debugging)
if [ "${WAIT_FOREVER:-false}" = "true" ]; then
    echo "WAIT_FOREVER is set, keeping container alive..."
    tail -f /dev/null
fi
