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
# Temporary exception: pip-audit reports CVE-2026-4539 for Pygments 2.19.2,
# and 2.19.2 is still the latest published release as of 2026-03-28.
# Remove this ignore once an upstream patched release is available.
uv --directory py run pip-audit --ignore-vuln CVE-2026-4539

echo "pip-audit: PASS"
