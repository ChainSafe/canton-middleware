#!/bin/bash
# Query Canton Holdings Script
#
# This script queries CIP56Holding contracts from Canton via the Go verification tool,
# which uses gRPC for reliable contract querying.
#
# Usage:
#   ./scripts/query-canton-holdings.sh                    # Show all holdings
#   ./scripts/query-canton-holdings.sh -summary           # Alias for standard output
#
# Prerequisites:
#   - Canton running with gRPC API on port 5011
#   - Go installed
#   - config.e2e-local.yaml configured

set -e

CONFIG_FILE="${CONFIG_FILE:-config.e2e-local.yaml}"

# Parse arguments (for backwards compatibility)
while [[ $# -gt 0 ]]; do
    case $1 in
        -config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -summary|-party|-url)
            # Ignore legacy flags, Go tool shows all
            shift
            if [[ $# -gt 0 && ! "$1" =~ ^- ]]; then
                shift
            fi
            ;;
        -h|--help)
            echo "Usage: $0 [-config <config_file>]"
            echo ""
            echo "Options:"
            echo "  -config <file>  Config file (default: config.e2e-local.yaml)"
            echo ""
            echo "This script wraps the Go verification tool for querying Canton holdings."
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check if config exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Config file not found: $CONFIG_FILE"
    exit 1
fi

# Run the Go verification tool
exec go run scripts/utils/verify-canton-holdings.go -config "$CONFIG_FILE"
