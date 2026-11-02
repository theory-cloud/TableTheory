#!/usr/bin/env bash
# Purpose: fail when a TableTheory surface that declares an AWS Lambda runtime
# declares one AWS has deprecated (nodejs20.x and older, python3.9 and older,
# provided and provided.al2). CDK stacks, SAM/CloudFormation templates, and the
# fenced deploy snippets in the example docs are all surfaces.
#
# The checker owns the scope, the deprecated set, and the modelled declaration
# forms, and it fails closed on a scanned surface that is missing or unreadable.
# This wrapper deliberately accepts no arguments and always runs the checker's
# self-test before the real scan, so no caller can narrow the scan or waive a
# surface.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
repo_root="$(pwd)"

# shellcheck source=scripts/lib/ensure-node-scanner-deps.sh
source scripts/lib/ensure-node-scanner-deps.sh

if ! command -v node >/dev/null 2>&1; then
  echo "lambda-runtime-deprecations: BLOCKED (node not found)" >&2
  exit 2
fi
if [[ ! -f "scripts/check-lambda-runtime-deprecations.mjs" ]]; then
  echo "lambda-runtime-deprecations: FAIL (missing scripts/check-lambda-runtime-deprecations.mjs)" >&2
  exit 1
fi
if [[ "$#" -ne 0 ]]; then
  echo "lambda-runtime-deprecations: FAIL (this gate takes no arguments)" >&2
  exit 1
fi

# The checker resolves the pinned TypeScript compiler and YAML parser from
# ts/package.json. Provision them from ts/package-lock.json before the checker
# runs, so SEC-2 is deterministic in a clean checkout with no ts/node_modules.
if ! ensure_node_scanner_deps "${repo_root}"; then
  echo "lambda-runtime-deprecations: FAIL (pinned scanner dependencies are unavailable)" >&2
  exit 1
fi

node scripts/check-lambda-runtime-deprecations.mjs --self-test
