#!/usr/bin/env bash
# Purpose: fail when a dependency in the repo's npm lockfile set declares an
# engines.node range that excludes the repository's Node floor.
#
# The checker owns the policy and derives the floor from ts/package.json.
# This wrapper deliberately accepts no arguments: the shipped gate always scans
# the whole audit-covered lockfile set, so no caller can narrow it or waive a
# package.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! command -v node >/dev/null 2>&1; then
  echo "npm-engines-floor: BLOCKED (node not found)" >&2
  exit 2
fi
if [[ ! -f "ts/package.json" ]]; then
  echo "npm-engines-floor: FAIL (missing ts/package.json)" >&2
  exit 1
fi
if [[ "$#" -ne 0 ]]; then
  echo "npm-engines-floor: FAIL (this gate takes no arguments)" >&2
  exit 1
fi

node scripts/check-npm-engines-floor.mjs --self-test
