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
    echo "    API server config updated with issuer_party and domain_id"
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
# Run register-user
# =============================================================================
echo ""
echo ">>> Running register-user..."
/app/register-user -config "$API_SERVER_CONFIG_FILE" -party "$PARTY_ID" || {
    echo "    [WARN] User registration may have failed or user already exists"
}

# =============================================================================
# Done
# =============================================================================
echo ""
echo "========================================================================"
echo "BOOTSTRAP COMPLETE"
echo "========================================================================"
echo "Party ID:  $PARTY_ID"
echo "Domain ID: $DOMAIN_ID"
echo ""
echo "The relayer can now be started."
echo "========================================================================"

# If WAIT_FOREVER is set, keep the container running (useful for debugging)
if [ "${WAIT_FOREVER:-false}" = "true" ]; then
    echo "WAIT_FOREVER is set, keeping container alive..."
    tail -f /dev/null
fi
