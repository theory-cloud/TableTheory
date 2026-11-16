#!/usr/bin/env bash
set -euo pipefail

# Python package static checks: formatting, lint, typecheck and the wheel/sdist
# build. This is the single entry point the Python proof profile records, so the
# Node/Python floor legs and the default leg run exactly the same command set.

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "python-static: SKIP (py/pyproject.toml not found)"
  exit 0
fi

if [[ ! -d "py/.venv" ]]; then
  uv --directory py sync --frozen --all-extras
fi

uv --directory py run ruff format --check .
uv --directory py run ruff check .
uv --directory py run mypy src
uv --directory py run python -m build

echo "python-static: PASS"
