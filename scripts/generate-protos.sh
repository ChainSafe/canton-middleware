#!/bin/bash
# Script to generate Go protobuf stubs for Canton Ledger API

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
PROTO_DIR="$PROJECT_ROOT/proto"
OUT_DIR="$PROJECT_ROOT/pkg/canton/lapi"

echo "Generating Canton Ledger API protobuf stubs..."

# Create directories
mkdir -p "$PROTO_DIR"
mkdir -p "$OUT_DIR"

# Clone Daml repository to get proto files
if [ ! -d "$PROTO_DIR/daml" ]; then
    echo "Getting Daml proto definitions..."
    # cd $PROTO_DIR
    # curl -O https://github.com/digital-asset/daml/releases/download/v2.10.2/protobufs-2.10.2.zip
    # unzip ./protobufs-2.10.2.zip
    # rm ./protobufs-2.10.2.zip
    # cd -


    protoc \
        --proto_path=proto/daml/com/daml/ledger/api/v1 \
        --go_out=pkg/canton/lapi \
        --go_opt=paths=source_relative \

fi

# Find proto files
DAML_PROTO_V1_DIR="$PROTO_DIR/daml/com/daml/ledger/api/v1"
DAML_PROTO_V2_DIR="$PROTO_DIR/daml/com/daml/ledger/api/v2"

if [ ! -d "$DAML_PROTO_V1_DIR" ]; then
    echo "Error: Daml proto V1 directory not found at $DAML_PROTO_V1_DIR"
    exit 1
fi

if [ ! -d "$DAML_PROTO_V2_DIR" ]; then
    echo "Error: Daml proto V2 directory not found at $DAML_PROTO_V2_DIR"
    exit 1
fi

# Inject go_package option for V1
echo "Injecting go_package option for V1..."
for f in "$DAML_PROTO_V1_DIR"/*.proto; do
    # Remove existing go_package option if present
    sed -i '' '/option go_package/d' "$f"
    
    if grep -q "syntax =" "$f"; then
        sed -i '' '/syntax =/a\
option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1";
' "$f"
    else
        echo 'option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1";' | cat - "$f" > temp && mv temp "$f"
    fi
done

# Inject go_package option for V2
echo "Injecting go_package option for V2..."
for f in "$DAML_PROTO_V2_DIR"/*.proto; do
    # Remove existing go_package option if present
    sed -i '' '/option go_package/d' "$f"

    if grep -q "syntax =" "$f"; then
        sed -i '' '/syntax =/a\
option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2";
' "$f"
    else
        echo 'option go_package = "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2";' | cat - "$f" > temp && mv temp "$f"
    fi
done

echo "Generating Go code for V1..."
protoc \
    --proto_path="$PROTO_DIR/daml" \
    --proto_path="$PROTO_DIR" \
    --go_out="$PROJECT_ROOT" \
    --go_opt=module=github.com/chainsafe/canton-middleware \
    --go-grpc_out="$PROJECT_ROOT" \
    --go-grpc_opt=module=github.com/chainsafe/canton-middleware \
    "$DAML_PROTO_V1_DIR"/*.proto

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
1. Review generated files in pkg/canton/lapi/
2. Update canton/client.go to use the generated types
3. Implement event streaming and command submission
4. Run: go mod tidy

EOF
