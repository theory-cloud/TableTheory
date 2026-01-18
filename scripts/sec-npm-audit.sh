#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "ts/package.json" ]]; then
  echo "npm-audit: SKIP (ts/package.json not found)"
  exit 0
fi

command -v npm >/dev/null 2>&1 || {
  echo "npm-audit: FAIL (npm not found)"
  exit 1
}

# Fail on any known vulnerability (no green-by-severity).
npm --prefix ts audit --audit-level=low

if [[ -f "contract-tests/runners/ts/package.json" ]]; then
  npm --prefix contract-tests/runners/ts audit --audit-level=low
fi

echo "npm-audit: PASS"
