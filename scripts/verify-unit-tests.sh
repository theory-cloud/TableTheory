#!/usr/bin/env bash
set -euo pipefail

# Composition, not a second execution: when the rubric gate composes this
# gate's reviewed record, the record is verified against these exact inputs and
# this script exits without running its commands again. A record that does not
# apply (stale, missing, failed, or another profile) makes this a no-op and the
# real command below runs.
if bash scripts/evidence/reuse-if-recorded.sh unit; then
  echo "unit-tests: PASS (reused the reviewed evidence record)"
  exit 0
fi

make test-unit

if [[ -f "ts/package.json" ]]; then
  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  npm --prefix ts run test:unit
fi

if [[ -f "py/pyproject.toml" ]]; then
  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi
  uv --directory py run pytest -q tests/unit
fi

echo "unit-tests: PASS"
