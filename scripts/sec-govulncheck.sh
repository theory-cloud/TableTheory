#!/usr/bin/env bash
set -euo pipefail

# Scan all Go modules in-repo (excluding repo-local caches).
mods="$(find . -name go.mod \
  -not -path './.gomodcache/*' \
  -not -path './vendor/*' \
  | sort)"

if [[ -z "${mods}" ]]; then
  echo "no go.mod files found"
  exit 1
fi

source scripts/lib/retry.sh

while IFS= read -r mod; do
  dir="$(dirname "${mod}")"
  echo "==> govulncheck: ${dir}"
  # govulncheck queries vuln.go.dev on every run and has no retry of its own;
  # absorb a bounded transient window and keep failing closed afterwards.
  (
    cd "${dir}"
    theorydb_retry_bounded 3 5 "govulncheck(${dir})" -- govulncheck ./...
  )
done <<< "${mods}"

