#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "ts/package.json" ]]; then
  echo "typescript-build: SKIP (ts/package.json not found)"
  exit 0
fi

if [[ ! -d "ts/node_modules" ]]; then
  bash scripts/verify-typescript-deps.sh
fi

npm --prefix ts run typecheck
npm --prefix ts run build

echo "typescript-build: PASS"

