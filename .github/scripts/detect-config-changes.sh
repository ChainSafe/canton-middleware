#!/usr/bin/env bash
# Decides whether the devnet deploy PR may auto-merge by diffing config paths
# between the version devnet currently runs and the commit being deployed.
#
# Comparing against the DEPLOYED version (the image tags recorded in the infra
# values files), not the push range, keeps a hold sticky: a config change that
# was never shipped keeps blocking auto-merge even when later config-free
# commits deploy on top of it. Every unresolvable state fails safe to a hold:
# missing values file, per-service tag drift, non-main-<sha> tag, unknown sha.
#
# Inputs (env):
#   DIR            directory in the infra-kubernetes checkout (cwd) holding
#                  the Helm values files
#   SRC            path to the middleware checkout with full history
#   GITHUB_SHA     commit being deployed
#   RUNNER_TEMP    scratch directory
#   GITHUB_OUTPUT  step outputs file
#
# Outputs (GITHUB_OUTPUT):
#   config_changed  true|false — true means hold the deploy PR for manual merge
#   comment_file    markdown report to post on the PR (written when true)
set -euo pipefail

# Helm values files the deploy job bumps — keep in sync with FILES in
# docker-build-release.yml. Top-level key = file name minus "-values.yml".
VALUES_FILES=(
  canton-middleware-api-values.yml
  canton-indexer-values.yml
  canton-middleware-values.yml
)

# Paths that can break a deploy when the Helm values are stale: per-package
# config structs, the default YAMLs baked into the images, and yaml-tagged
# types embedded in config (indexer InstrumentKey).
CONFIG_PATHS=(
  pkg/config
  'pkg/*config.go'
  'cmd/*config*.go'
  pkg/indexer/types.go
  ':!*_test.go'
  ':!pkg/config/tests'
)

CHANGED=false
REASON=""
DEPLOYED_TAG=""
DEPLOYED_SHA=""
COMMENT_FILE="${RUNNER_TEMP}/config-comment.md"

# Values from the infra repo are untrusted input for the markdown report.
sanitize() { printf '%s' "$1" | tr -cd 'A-Za-z0-9._-' | cut -c1-64; }

# Read the deployed tag from every service; all must agree. Per-service drift
# (e.g. a manual rollback of one service) means there is no single safe
# baseline to diff against.
TAG_REPORT=""
TAG_VALUES=()
for f in "${VALUES_FILES[@]}"; do
  key="${f%-values.yml}"
  tag=$(yq e ".[\"${key}\"].image.tag" "${DIR}/${f}" 2>/dev/null) || tag=""
  [ "${tag}" = "null" ] && tag=""
  TAG_REPORT="${TAG_REPORT}${key}: $(sanitize "${tag:-missing}"); "
  TAG_VALUES+=("${tag}")
done

if printf '%s\n' "${TAG_VALUES[@]}" | grep -q '^$'; then
  CHANGED=true
  REASON="Could not read the deployed image tag from every values file (${TAG_REPORT}). Holding for manual review."
elif [ "$(printf '%s\n' "${TAG_VALUES[@]}" | sort -u | wc -l)" -ne 1 ]; then
  CHANGED=true
  REASON="Deployed image tags differ between services (${TAG_REPORT}), so there is no single safe baseline to diff against. Holding for manual review."
else
  DEPLOYED_TAG="${TAG_VALUES[0]}"
fi
echo "Deployed devnet tags: ${TAG_REPORT}"

if [ -n "${DEPLOYED_TAG}" ]; then
  if ! [[ "${DEPLOYED_TAG}" =~ ^main-[0-9a-f]{7,40}$ ]]; then
    CHANGED=true
    REASON="Deployed tag \`$(sanitize "${DEPLOYED_TAG}")\` is not a \`main-<sha>\` tag, so the config diff against the deployed version cannot be computed. Holding for manual review."
  else
    DEPLOYED_SHA="${DEPLOYED_TAG#main-}"
    if ! git -C "${SRC}" cat-file -e "${DEPLOYED_SHA}^{commit}" 2>/dev/null; then
      CHANGED=true
      REASON="Deployed commit \`${DEPLOYED_SHA}\` was not found in the repository history, so the config diff cannot be computed. Holding for manual review."
      DEPLOYED_SHA=""
    else
      CHANGED_FILES=$(git -C "${SRC}" diff --name-only "${DEPLOYED_SHA}..${GITHUB_SHA}" -- "${CONFIG_PATHS[@]}")
      if [ -n "${CHANGED_FILES}" ]; then
        CHANGED=true
        REASON="Config files changed since deployed \`${DEPLOYED_TAG}\`."
        echo "Changed config files since ${DEPLOYED_TAG}:"
        echo "${CHANGED_FILES}"
      fi
    fi
  fi
fi

# Best-effort headline for the PR comment: keys added/removed between the
# yaml:"..." struct tags of the deployed and new trees, scanned over the same
# CONFIG_PATHS the gate diffs. The raw diff in the report is the source of
# truth (key extraction misses type/default changes).
keys_with_files() {
  git -C "${SRC}" grep -o -E 'yaml:"[a-zA-Z0-9_.-]+' "$1" -- "${CONFIG_PATHS[@]}" 2>/dev/null \
    | sed -E 's|^[^:]+:([^:]+):yaml:"(.+)$|\2\t\1|' \
    | sort -u
}

# Prints "- `key` (file)" for each key, resolving the file from the tsv map.
print_keys() {
  local keys=$1 tsv=$2 k f
  while IFS= read -r k; do
    f=$(awk -F'\t' -v k="$k" '$1==k {print $2; exit}' "${tsv}")
    echo "- \`${k}\` (${f})"
  done <<< "${keys}"
}

# The heading below doubles as the comment's idempotency marker — the deploy
# step in docker-build-release.yml matches on this prefix before commenting.
write_report() {
  local include_diff=$1
  echo "## Config changed since deployed (\`$(sanitize "${DEPLOYED_TAG:-unknown}")\`) — auto-merge disabled"
  echo
  echo "${REASON}"
  echo

  # Without a resolvable deployed commit there is nothing to diff against.
  if [ -n "${DEPLOYED_SHA}" ]; then
    keys_with_files "${DEPLOYED_SHA}" > "${RUNNER_TEMP}/old_keys.tsv" || true
    keys_with_files "${GITHUB_SHA}" > "${RUNNER_TEMP}/new_keys.tsv" || true
    cut -f1 "${RUNNER_TEMP}/old_keys.tsv" | sort -u > "${RUNNER_TEMP}/old_keys.txt"
    cut -f1 "${RUNNER_TEMP}/new_keys.tsv" | sort -u > "${RUNNER_TEMP}/new_keys.txt"
    ADDED=$(comm -13 "${RUNNER_TEMP}/old_keys.txt" "${RUNNER_TEMP}/new_keys.txt")
    REMOVED=$(comm -23 "${RUNNER_TEMP}/old_keys.txt" "${RUNNER_TEMP}/new_keys.txt")
    if [ -n "${ADDED}" ]; then
      echo "**Schema keys added** (verify the Helm values cover these):"
      print_keys "${ADDED}" "${RUNNER_TEMP}/new_keys.tsv"
      echo
    fi
    if [ -n "${REMOVED}" ]; then
      echo "**Schema keys removed:**"
      print_keys "${REMOVED}" "${RUNNER_TEMP}/old_keys.tsv"
      echo
    fi

    if [ "${include_diff}" = "true" ]; then
      git -C "${SRC}" diff "${DEPLOYED_SHA}..${GITHUB_SHA}" -- "${CONFIG_PATHS[@]}" > "${RUNNER_TEMP}/config.diff"
      echo "<details><summary>Config diff since <code>${DEPLOYED_TAG}</code></summary>"
      echo
      # Four-backtick fence so diff context lines containing ``` cannot close it
      echo '````diff'
      head -400 "${RUNNER_TEMP}/config.diff"
      if [ "$(wc -l < "${RUNNER_TEMP}/config.diff")" -gt 400 ]; then
        echo "... (diff truncated at 400 lines)"
      fi
      echo '````'
      echo
      echo "</details>"
    else
      echo "_Raw config diff omitted (too large for a PR comment). Run locally:_"
      echo '`git diff '"${DEPLOYED_SHA}..${GITHUB_SHA}"' -- pkg/config "pkg/*config.go" "cmd/*config*.go" pkg/indexer/types.go`'
    fi
    echo

    git -C "${SRC}" log --oneline --no-decorate "${DEPLOYED_SHA}..${GITHUB_SHA}" -- "${CONFIG_PATHS[@]}" > "${RUNNER_TEMP}/config-commits.txt"
    echo "**Commits touching config:**"
    head -100 "${RUNNER_TEMP}/config-commits.txt" | sed 's/^/- /'
    if [ "$(wc -l < "${RUNNER_TEMP}/config-commits.txt")" -gt 100 ]; then
      echo "- ... ($(wc -l < "${RUNNER_TEMP}/config-commits.txt" | tr -d ' ') commits total, truncated at 100)"
    fi
    echo
  fi
  echo "- [ ] Verify the Helm values files cover the added/changed config, then merge manually."
}

if [ "${CHANGED}" = "true" ]; then
  write_report true > "${COMMENT_FILE}"
  # Stay under GitHub's 65536-char comment limit: drop the inline diff if the
  # full report is too large (long diff lines can exceed it despite head -400).
  if [ "$(wc -c < "${COMMENT_FILE}")" -gt 60000 ]; then
    write_report false > "${COMMENT_FILE}"
  fi
fi

echo "config_changed=${CHANGED}" >> "${GITHUB_OUTPUT}"
echo "comment_file=${COMMENT_FILE}" >> "${GITHUB_OUTPUT}"
