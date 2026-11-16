#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "python-deps: SKIP (py/pyproject.toml not found)"
  exit 0
fi

# Resolve the pinned interpreter by name, not by PATH order. The tracked proof
# profile names Python 3.14; a host whose `python3` has moved on (or a gate that
# prepended its own tool directory) must still find the governed interpreter
# instead of silently syncing the project against a different runtime.
python_bin=""
for candidate in python3.14 python3; do
  command -v "${candidate}" >/dev/null 2>&1 || continue
  candidate_minor="$("${candidate}" - <<'PY'
import sys
print(f"{sys.version_info.major}.{sys.version_info.minor}")
PY
)"
  if [[ "${candidate_minor}" == "3.14" ]]; then
    python_bin="${candidate}"
    break
  fi
done
if [[ -z "${python_bin}" ]]; then
  echo "python-deps: FAIL (no Python 3.14 interpreter found; require 3.14.x)"
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

uv --directory py sync --frozen --all-extras --python "${python_bin}"

echo "python-deps: ok"

