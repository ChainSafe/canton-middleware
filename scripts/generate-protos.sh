#!/bin/bash
# Script to generate Go protobuf stubs for Canton Ledger API
# Updated for Canton 3.4.8 which uses v2 API only

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
PROTO_DIR="$PROJECT_ROOT/proto"
OUT_DIR="$PROJECT_ROOT/pkg/canton/lapi"

echo "Generating Canton Ledger API protobuf stubs..."

# Create directories
mkdir -p "$PROTO_DIR"
mkdir -p "$OUT_DIR"

# Download Canton 3.4.8 protos if not present
if [ ! -f "$PROTO_DIR/daml/com/daml/ledger/api/v2/update_service.proto" ]; then
    echo "Downloading Canton 3.4.8 proto definitions..."
    cd "$PROTO_DIR"
    curl -sL "https://github.com/digital-asset/daml/releases/download/v3.4.8/protobufs-3.4.8.zip" -o protobufs.zip
    rm -rf daml
    unzip -q protobufs.zip
    mv protos-3.4.8 daml
    rm protobufs.zip
    cd -
fi

# Find proto files - Canton 3.4.8 uses v2 only
DAML_PROTO_V2_DIR="$PROTO_DIR/daml/com/daml/ledger/api/v2"

if [ ! -d "$DAML_PROTO_V2_DIR" ]; then
    echo "Error: Daml proto V2 directory not found at $DAML_PROTO_V2_DIR"
    exit 1
fi

# Inject go_package option for V2
echo "Injecting go_package option for V2..."
for f in "$DAML_PROTO_V2_DIR"/*.proto; do
    # Remove existing go_package option if present
    sed -i '' '/option go_package/d' "$f" 2>/dev/null || sed -i '/option go_package/d' "$f"

    if grep -q "syntax =" "$f"; then
        sed -i '' '/syntax =/a\
option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2";
' "$f" 2>/dev/null || sed -i '/syntax =/a option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2";' "$f"
    fi
done

# Also handle admin protos if present
ADMIN_DIR="$DAML_PROTO_V2_DIR/admin"
if [ -d "$ADMIN_DIR" ]; then
    echo "Injecting go_package option for admin..."
    for f in "$ADMIN_DIR"/*.proto; do
        sed -i '' '/option go_package/d' "$f" 2>/dev/null || sed -i '/option go_package/d' "$f"
        if grep -q "syntax =" "$f"; then
            sed -i '' '/syntax =/a\
option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2/admin";
' "$f" 2>/dev/null || true
        fi
    done
fi

# Clean old generated files
rm -rf "$OUT_DIR/v1" "$OUT_DIR/v2"
mkdir -p "$OUT_DIR/v2"

echo "Generating Go code for V2..."
protoc \
    --proto_path="$PROTO_DIR/daml" \
    --proto_path="$PROTO_DIR" \
    --go_out="$PROJECT_ROOT" \
    --go_opt=module=github.com/chainsafe/canton-middleware \
    --go-grpc_out="$PROJECT_ROOT" \
    --go-grpc_opt=module=github.com/chainsafe/canton-middleware \
    "$DAML_PROTO_V2_DIR"/*.proto

echo "✓ Proto generation complete"
echo "Generated files in: $OUT_DIR"

# Instructions
cat << EOF

Next steps:
1. Review generated files in pkg/canton/lapi/v2/
2. Update Go code: replace lapiv1 imports with lapiv2
3. Run: go mod tidy

NOTE: Canton 3.4.8 consolidated all types into v2 API.
      Types like Command, Value, Record are now in v2.

EOF
