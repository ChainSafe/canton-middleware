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
CONFIG_PATH="/app/config.yaml"

# Attempt to find the config path from the remaining arguments
prev_arg=""
for arg in "$@"; do
    if [ "$prev_arg" = "-config" ]; then
        CONFIG_PATH="$arg"
        break
    fi
    prev_arg="$arg"
done

# Run migrations if the migrate binary exists and is executable
if [ -x "$MIGRATE_BINARY" ]; then
    if [ -f "$CONFIG_PATH" ]; then
        echo ">>> [INFO] Migration binary found: $MIGRATE_BINARY"
        echo ">>> [INFO] Using config: $CONFIG_PATH"
        
        echo ">>> [INFO] Initializing migrations (if needed)..."
        # We allow 'init' to fail as the database might already be initialized
        "$MIGRATE_BINARY" -config "$CONFIG_PATH" init || echo ">>> [INFO] Migration already initialized or init command not supported (skipping)."
        
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
