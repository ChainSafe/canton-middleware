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
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DAML_DIR="$PROJECT_DIR/contracts/canton-erc20/daml"

# Suppress Daml Assistant deprecation warning (DPM migration planned for SDK 3.5)
DAML_BUILD_FLAGS="--no-legacy-assistant-warning"

echo "Building Canton Bridge DAR packages..."
echo ""

# Build order matters due to dependencies
PACKAGES=(
    "common"
    "cip56-token"
    "native-token"
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
    (cd "$DAML_DIR/$pkg" && daml build $DAML_BUILD_FLAGS)
    echo "    Done!"
done

echo ""
echo "All packages built successfully!"
echo ""
echo "DAR files:"
for pkg in "${PACKAGES[@]}"; do
    ls -1 "$DAML_DIR/$pkg/.daml/dist/"*.dar 2>/dev/null | head -1
done

