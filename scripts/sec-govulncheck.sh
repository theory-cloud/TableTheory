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

while IFS= read -r mod; do
  dir="$(dirname "${mod}")"
  echo "==> govulncheck: ${dir}"
  (cd "${dir}" && govulncheck ./...)
done <<< "${mods}"

