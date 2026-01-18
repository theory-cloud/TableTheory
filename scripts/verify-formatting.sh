#!/usr/bin/env bash
set -euo pipefail

bash scripts/fmt-check.sh

if [[ -f "ts/package.json" ]]; then
  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  npm --prefix ts run format:check
fi

if [[ -f "py/pyproject.toml" ]]; then
  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi
  uv --directory py run ruff format --check .
fi

echo "formatting: PASS"
