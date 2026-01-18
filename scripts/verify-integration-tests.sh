#!/usr/bin/env bash
set -euo pipefail

skip="${SKIP_INTEGRATION:-}"
if [[ "${skip}" == "1" || "${skip}" == "true" ]]; then
  echo "integration-tests: SKIP (SKIP_INTEGRATION=${skip})"
  exit 0
fi

make integration

if [[ -f "ts/package.json" ]]; then
  bash scripts/verify-typescript-integration.sh
fi

if [[ -f "py/pyproject.toml" ]]; then
  bash scripts/verify-python-integration.sh
fi

bash scripts/verify-contract-tests.sh

echo "integration-tests: PASS"
