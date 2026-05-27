#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Idempotently add `SPDX-License-Identifier: Apache-2.0` headers to first-party
# source files. Re-running this script is safe; files that already carry an
# SPDX header in their first 5 lines are skipped, and files that look
# generated are skipped entirely.
#
# Usage: bash scripts/dev/add-spdx-headers.sh
#
# Run from the repository root.

set -euo pipefail

SPDX_LINE_SLASH="// SPDX-License-Identifier: Apache-2.0"
SPDX_LINE_HASH="# SPDX-License-Identifier: Apache-2.0"

# Returns 0 (true) if the file already has an SPDX header in its first 5 lines,
# OR if it looks like generated code we should skip.
should_skip() {
  local file="$1"
  local first_five
  first_five=$(head -n 5 "$file" 2>/dev/null || true)

  case "$first_five" in
    *"SPDX-License-Identifier"*) return 0 ;;
    *"Code generated"*)          return 0 ;;
    *"DO NOT EDIT"*)             return 0 ;;
    *"protoc-gen-go"*)            return 0 ;;
    *"abigen"*)                   return 0 ;;
  esac
  return 1
}

# Prepend a header line to the top of the file.
prepend_top() {
  local file="$1"
  local header="$2"
  local tmp
  tmp=$(mktemp)
  {
    printf '%s\n\n' "$header"
    cat "$file"
  } >"$tmp"
  mv "$tmp" "$file"
}

# Insert a header line immediately after the shebang on line 1.
insert_after_shebang() {
  local file="$1"
  local header="$2"
  local tmp
  tmp=$(mktemp)
  {
    head -n 1 "$file"
    printf '%s\n' "$header"
    tail -n +2 "$file"
  } >"$tmp"
  mv "$tmp" "$file"
}

# Process Go files.
process_go() {
  local count=0 skipped=0
  while IFS= read -r -d '' file; do
    if should_skip "$file"; then
      skipped=$((skipped + 1))
      continue
    fi
    prepend_top "$file" "$SPDX_LINE_SLASH"
    count=$((count + 1))
  done < <(find cmd pkg -type f -name '*.go' \
    -not -path '*/lapi/v2/*' \
    -not -path '*/ethereum/contracts/*' \
    -not -name '*.pb.go' \
    -print0)
  printf 'Go: added headers to %d files (skipped %d)\n' "$count" "$skipped"
}

# Process shell scripts.
process_shell() {
  local count=0 skipped=0
  while IFS= read -r -d '' file; do
    if should_skip "$file"; then
      skipped=$((skipped + 1))
      continue
    fi
    if head -n 1 "$file" | grep -q '^#!'; then
      insert_after_shebang "$file" "$SPDX_LINE_HASH"
    else
      prepend_top "$file" "$SPDX_LINE_HASH"
    fi
    count=$((count + 1))
  done < <(find scripts -type f -name '*.sh' -print0)
  printf 'Shell: added headers to %d files (skipped %d)\n' "$count" "$skipped"
}

# Process Dockerfiles. Targets explicit list rather than a glob to avoid
# accidentally touching vendor/test Dockerfiles outside our scope.
process_dockerfiles() {
  local count=0 skipped=0
  local dockerfiles=(
    "Dockerfile.local"
    "cmd/api-server/Dockerfile"
    "cmd/indexer/Dockerfile"
    "cmd/relayer/Dockerfile"
  )
  for file in "${dockerfiles[@]}"; do
    if [[ ! -f "$file" ]]; then
      continue
    fi
    if should_skip "$file"; then
      skipped=$((skipped + 1))
      continue
    fi
    prepend_top "$file" "$SPDX_LINE_HASH"
    count=$((count + 1))
  done
  printf 'Dockerfile: added headers to %d files (skipped %d)\n' "$count" "$skipped"
}

main() {
  if [[ ! -f go.mod ]]; then
    echo "error: run from repository root (go.mod not found)" >&2
    exit 1
  fi
  process_go
  process_shell
  process_dockerfiles
}

main "$@"
