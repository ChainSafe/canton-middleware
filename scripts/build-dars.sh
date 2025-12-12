#!/bin/bash
# =============================================================================
# Build all DAR packages for Canton Bridge
# =============================================================================
# This script builds all Daml packages required for the bridge.
#
# Usage:
#   ./scripts/build-dars.sh
#
# Requirements:
#   - Daml SDK installed (https://get.daml.com/)
#
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DAML_DIR="$PROJECT_DIR/contracts/canton-erc20/daml"

echo "Building Canton Bridge DAR packages..."
echo ""

# Build order matters due to dependencies
PACKAGES=(
    "common"
    "cip56-token"
    "bridge-core"
    "bridge-wayfinder"
)

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

