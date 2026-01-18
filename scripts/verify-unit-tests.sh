#!/usr/bin/env bash
set -euo pipefail

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
