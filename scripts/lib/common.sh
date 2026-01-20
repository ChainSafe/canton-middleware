#!/bin/bash
# =============================================================================
# Common utilities for Canton-Ethereum bridge test scripts
# =============================================================================

# Suppress foundry nightly warnings
export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
RESET='\033[0m'

# Print functions
print_header() {
    echo -e "\n${BLUE}══════════════════════════════════════════════════════════════════════${RESET}"
    echo -e "${BLUE}  $1${RESET}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════${RESET}"
}

print_step() {
    echo -e "${CYAN}>>> $1${RESET}"
}

print_success() {
    echo -e "${GREEN}✓ $1${RESET}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${RESET}"
}

print_error() {
    echo -e "${RED}✗ $1${RESET}"
    exit 1
}

print_info() {
    echo -e "    $1"
}

# Truncate string to max length
truncate() {
    local str=$1
    local max_len=$2
    if [ ${#str} -le $max_len ]; then
        echo "$str"
    else
        echo "${str:0:$max_len}..."
    fi
}
