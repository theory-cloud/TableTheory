#!/usr/bin/env bash
set -euo pipefail

make lint

if [[ -f "ts/package.json" ]]; then
  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  npm --prefix ts run lint
fi

if [[ -f "py/pyproject.toml" ]]; then
  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi
  uv --directory py run ruff check .
fi

echo "lint: PASS"
