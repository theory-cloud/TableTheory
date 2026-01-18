#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "pip-audit: SKIP (py/pyproject.toml not found)"
  exit 0
fi

if [[ ! -d "py/.venv" ]]; then
  bash scripts/verify-python-deps.sh
fi

# Fail on any known vulnerability (no green-by-severity).
uv --directory py run pip-audit

echo "pip-audit: PASS"

