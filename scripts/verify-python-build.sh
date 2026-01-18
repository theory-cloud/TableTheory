#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "python-build: SKIP (py/pyproject.toml not found)"
  exit 0
fi

if [[ ! -d "py/.venv" ]]; then
  bash scripts/verify-python-deps.sh
fi

uv --directory py run mypy src
uv --directory py run python -m build

echo "python-build: PASS"

