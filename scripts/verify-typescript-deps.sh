#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "ts/package.json" ]]; then
  echo "typescript-deps: SKIP (ts/package.json not found)"
  exit 0
fi

command -v node >/dev/null 2>&1 || {
  echo "typescript-deps: FAIL (node not found)"
  exit 1
}
command -v npm >/dev/null 2>&1 || {
  echo "typescript-deps: FAIL (npm not found)"
  exit 1
}

node_version="$(node --version | tr -d '\n' || true)"
if [[ "${node_version}" =~ ^v([0-9]+)\. ]]; then
  major="${BASH_REMATCH[1]}"
  if [[ "${major}" -lt 20 ]]; then
    echo "typescript-deps: FAIL (node ${node_version}; require >= v20)"
    exit 1
  fi
else
  echo "typescript-deps: FAIL (unable to parse node version: ${node_version})"
  exit 1
fi

npm --prefix ts ci

echo "typescript-deps: ok"

