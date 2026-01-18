#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "python-deps: SKIP (py/pyproject.toml not found)"
  exit 0
fi

command -v python3 >/dev/null 2>&1 || {
  echo "python-deps: FAIL (python3 not found)"
  exit 1
}

python_minor="$(python3 - <<'PY'
import sys
print(f"{sys.version_info.major}.{sys.version_info.minor}")
PY
)"
if [[ "${python_minor}" != "3.14" ]]; then
  echo "python-deps: FAIL (python3 is ${python_minor}; require 3.14.x)"
  exit 1
fi

command -v uv >/dev/null 2>&1 || {
  echo "python-deps: FAIL (uv not found)"
  exit 1
}

if [[ ! -f "py/uv.lock" ]]; then
  echo "python-deps: FAIL (py/uv.lock missing; run 'uv --directory py lock')"
  exit 1
fi

uv --directory py sync --frozen --all-extras

echo "python-deps: ok"

