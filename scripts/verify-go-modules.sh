#!/usr/bin/env bash
set -euo pipefail

mods="$(find . -name go.mod \
  -not -path './.gomodcache/*' \
  -not -path './vendor/*' \
  | sort)"

if [[ -z "${mods}" ]]; then
  echo "no go.mod files found"
  exit 1
fi

failures=0

while IFS= read -r mod; do
  dir="$(dirname "${mod}")"
  echo "==> compile: ${dir}"

  # Compile-only (do not execute test binaries).
  if ! (cd "${dir}" && go test -run=^$ -count=0 -exec=true ./...); then
    failures=$((failures + 1))
  fi
done <<< "${mods}"

if [[ "${failures}" -ne 0 ]]; then
  echo "module compile failures: ${failures}"
  exit 1
fi

echo "modules: clean"

