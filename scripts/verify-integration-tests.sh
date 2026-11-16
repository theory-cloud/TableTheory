#!/usr/bin/env bash
set -euo pipefail

skip="${SKIP_INTEGRATION:-}"
if [[ "${skip}" == "1" || "${skip}" == "true" ]]; then
  echo "integration-tests: SKIP (SKIP_INTEGRATION=${skip})"
  exit 0
fi

# Composition, not a second execution: when the rubric gate composes this
# gate's reviewed record, the record is verified against these exact inputs and
# this script exits without running its commands again. A record that does not
# apply (stale, missing, failed, or another profile) makes this a no-op and the
# real command below runs.
if bash scripts/evidence/reuse-if-recorded.sh integration; then
  echo "integration-tests: PASS (reused the reviewed evidence record)"
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
