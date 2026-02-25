#!/bin/bash
# =============================================================================
# Build all DAR packages for Canton Bridge
# =============================================================================
# This script builds all Daml packages required for the bridge.
#
# Usage:
#   ./scripts/setup/build-dars.sh
#
# Requirements:
#   - Daml SDK installed (https://get.daml.com/)
#
# Note: The --no-legacy-assistant-warning flag suppresses the "Daml Assistant
# has been deprecated" warning. Daml Assistant will be replaced by DPM (Digital
# Asset Package Manager) in SDK 3.5. See: https://docs.digitalasset.com/build/3.4/dpm/dpm.html
#
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
DAML_DIR="$PROJECT_DIR/contracts/canton-erc20/daml"

# Suppress Daml Assistant deprecation warning (DPM migration planned for SDK 3.5)
DAML_BUILD_FLAGS="--no-legacy-assistant-warning"

echo "Building Canton Bridge DAR packages..."
echo ""

# Build order matters due to dependencies
PACKAGES=(
    "common"
    "cip56-token"
    "bridge-core"
    "bridge-wayfinder"
)

# Clean old DAR files to avoid version conflicts
echo ">>> Cleaning old DAR files..."
for pkg in "${PACKAGES[@]}"; do
    rm -f "$DAML_DIR/$pkg/.daml/dist/"*.dar 2>/dev/null || true
done
echo "    Done!"
echo ""

for pkg in "${PACKAGES[@]}"; do
    echo ">>> Building $pkg..."
    (cd "$DAML_DIR/$pkg" && daml build)
    echo "    Done!"
done

echo ""
echo "All packages built successfully!"
echo ""
echo "DAR files:"
for pkg in "${PACKAGES[@]}"; do
    ls -1 "$DAML_DIR/$pkg/.daml/dist/"*.dar 2>/dev/null | head -1
done

# =============================================================================
# Extract package IDs and update local config files
# =============================================================================
# All packages currently use version 2.0.0. Update here if versions change.

PKG_VERSION="2.0.0"

extract_package_id() {
    local dar_path="$1"
    local pkg_name="$2"
    daml damlc inspect-dar "$dar_path" 2>/dev/null \
        | grep "^${pkg_name}-${PKG_VERSION}-" \
        | head -1 \
        | sed "s|${pkg_name}-${PKG_VERSION}-\([a-f0-9]*\)/.*|\1|"
}

echo ""
echo ">>> Extracting package IDs from built DARs..."

all_extracted=true
for pkg in "${PACKAGES[@]}"; do
    dar_path="$DAML_DIR/$pkg/.daml/dist/${pkg}-${PKG_VERSION}.dar"
    if [ ! -f "$dar_path" ]; then
        echo "    [WARN] DAR not found: $dar_path"
        all_extracted=false
        continue
    fi
    pkg_id=$(extract_package_id "$dar_path" "$pkg")
    if [ -z "$pkg_id" ]; then
        echo "    [WARN] Could not extract package ID from $dar_path"
        all_extracted=false
        continue
    fi

    # Store in named variables (bash 3.2 compatible, no associative arrays)
    case "$pkg" in
        common)           COMMON_ID="$pkg_id" ;;
        cip56-token)      CIP56_ID="$pkg_id" ;;
        bridge-core)      CORE_ID="$pkg_id" ;;
        bridge-wayfinder) BRIDGE_ID="$pkg_id" ;;
    esac
done

if [ "$all_extracted" = false ]; then
    echo ""
    echo "[WARN] Some package IDs could not be extracted. Skipping config update."
    exit 0
fi

echo "    common_package_id:  $COMMON_ID"
echo "    cip56_package_id:   $CIP56_ID"
echo "    core_package_id:    $CORE_ID"
echo "    bridge_package_id:  $BRIDGE_ID"

# Config files to update (tracked in git, used for local/docker testing)
CONFIG_FILES=(
    "$PROJECT_DIR/config.docker.yaml"
    "$PROJECT_DIR/config.api-server.docker.yaml"
    "$PROJECT_DIR/config.e2e-local.yaml"
)

echo ""
echo ">>> Updating local config files..."

update_config() {
    local cfg="$1"
    sed -i.bak \
        -e "s|\(common_package_id: \"\)[a-f0-9]*\"|\1${COMMON_ID}\"|" \
        -e "s|\(cip56_package_id: \"\)[a-f0-9]*\"|\1${CIP56_ID}\"|" \
        -e "s|\(core_package_id: \"\)[a-f0-9]*\"|\1${CORE_ID}\"|" \
        -e "s|\(bridge_package_id: \"\)[a-f0-9]*\"|\1${BRIDGE_ID}\"|" \
        "$cfg"
    rm -f "${cfg}.bak"
}

for cfg in "${CONFIG_FILES[@]}"; do
    if [ ! -f "$cfg" ]; then
        echo "    [SKIP] $(basename "$cfg") (not found)"
        continue
    fi
    update_config "$cfg"
    echo "    [OK] $(basename "$cfg")"
done

echo ""
echo "Done! Config files updated with current package IDs."

