#!/bin/sh
set -e

# entrypoint.sh - Entrypoint script for Go services to handle migrations and startup
# This script encapsulates the database migration logic and starts the application.
#
# Usage:
#   ENTRYPOINT ["/app/entrypoint.sh"]
#   CMD ["/app/binary", "-config", "/app/config.yaml"]

# The first argument should be the binary to run (e.g., /app/relayer)
# If the first argument starts with a dash, it's treated as an argument to the DEFAULT_BINARY.
if [ -z "$1" ] || [ "${1#-}" != "$1" ]; then
    if [ -z "$DEFAULT_BINARY" ]; then
        echo ">>> [ERROR] No binary specified and DEFAULT_BINARY is not set."
        exit 1
    fi
    BINARY="$DEFAULT_BINARY"
else
    BINARY="$1"
    shift
fi

# Determine the migration binary name (convention: <binary>-migrate)
MIGRATE_BINARY="${BINARY}-migrate"

# Default config path to check for migration settings
CONFIG_PATH=""
HAS_CONFIG_ARG=false

# Attempt to find the config path from the remaining arguments
prev_arg=""
for arg in "$@"; do
    if [ "$prev_arg" = "-config" ]; then
        CONFIG_PATH="$arg"
        HAS_CONFIG_ARG=true
        break
    fi
    prev_arg="$arg"
done

# If no -config was provided, select a built-in default based on service and ENV.
if [ "$HAS_CONFIG_ARG" = "false" ]; then
    SERVICE_NAME="$(basename "$BINARY")"
    SELECTED_ENV="${ENV:-docker}"
    CONFIG_SUFFIX=""

    case "$SELECTED_ENV" in
        docker)
            CONFIG_SUFFIX="docker"
            ;;
        devnet|local-devnet)
            CONFIG_SUFFIX="local-devnet"
            ;;
        mainnet)
            CONFIG_SUFFIX="mainnet"
            ;;
        *)
            echo ">>> [WARN] Unknown ENV '$SELECTED_ENV'; defaulting to docker."
            CONFIG_SUFFIX="docker"
            ;;
    esac

    case "$SERVICE_NAME" in
        relayer)
            CONFIG_PATH="/app/config/defaults/config.relayer.${CONFIG_SUFFIX}.yaml"
            ;;
        api-server)
            CONFIG_PATH="/app/config/defaults/config.api-server.${CONFIG_SUFFIX}.yaml"
            ;;
        indexer)
            CONFIG_PATH="/app/config/defaults/config.indexer.${CONFIG_SUFFIX}.yaml"
            ;;
        *)
            CONFIG_PATH=""
            ;;
    esac

    if [ -n "$CONFIG_PATH" ] && [ -f "$CONFIG_PATH" ]; then
        echo ">>> [INFO] Auto-selecting config from ENV=${SELECTED_ENV}: $CONFIG_PATH"
        set -- -config "$CONFIG_PATH" "$@"
    elif [ -n "$CONFIG_PATH" ]; then
        echo ">>> [WARN] Auto-selected config not found: $CONFIG_PATH"
    fi
fi

# Fallback for migration logging if config couldn't be determined.
if [ -z "$CONFIG_PATH" ]; then
    CONFIG_PATH="/app/config.yaml"
fi

# Run migrations if the migrate binary exists and is executable
if [ -x "$MIGRATE_BINARY" ]; then
    if [ -f "$CONFIG_PATH" ]; then
        echo ">>> [INFO] Migration binary found: $MIGRATE_BINARY"
        echo ">>> [INFO] Using config: $CONFIG_PATH"
        
        echo ">>> [INFO] Initializing migrations (if needed)..."
        # 'init' is expected to fail with a non-zero exit when the migrations table already
        # exists. Treat exit code 1 as "already initialized"; any other non-zero exit code
        # (binary not found, permission denied, DB unreachable) is a real error and fails loudly.
        init_output=$("$MIGRATE_BINARY" -config "$CONFIG_PATH" init 2>&1)
        init_code=$?
        if [ $init_code -eq 0 ]; then
            echo ">>> [INFO] Migrations initialized."
        elif echo "$init_output" | grep -qi "already\|exist"; then
            echo ">>> [INFO] Migrations already initialized (skipping)."
        else
            echo ">>> [ERROR] Migration init failed (exit $init_code): $init_output"
            exit 1
        fi
        
        echo ">>> [INFO] Running 'up' migrations..."
        if "$MIGRATE_BINARY" -config "$CONFIG_PATH" up; then
            echo ">>> [INFO] Migrations completed successfully."
        else
            echo ">>> [ERROR] Migrations failed. Exiting."
            exit 1
        fi
    else
        echo ">>> [WARN] Migration binary found at $MIGRATE_BINARY but config file not found at $CONFIG_PATH. Skipping migrations."
    fi
else
    echo ">>> [DEBUG] No migration binary found at $MIGRATE_BINARY. Skipping automatic migrations."
fi

# Execute the application binary with the remaining arguments
echo ">>> [INFO] Starting $BINARY..."
exec "$BINARY" "$@"
