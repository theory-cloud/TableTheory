#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "pip-audit: SKIP (py/pyproject.toml not found)"
  exit 0
fi

if [[ ! -d "py/.venv" ]]; then
  bash scripts/verify-python-deps.sh
fi

requirements_file="$(mktemp)"
trap 'rm -f "${requirements_file}"' EXIT

# Audit the project lock through an exported requirements view instead of
# installing pip-audit into the project environment. This keeps scanner tooling
# out of py/uv.lock so Dependabot tracks TableTheory dependencies rather than
# the audit tool's own pip/pip-api implementation details.
uv --directory py export \
  --all-extras \
  --frozen \
  --no-emit-project \
  --no-hashes \
  --output-file "${requirements_file}" >/dev/null

# Fail on any known vulnerability (no green-by-severity).
# Temporary exception: pip-audit reports CVE-2026-4539 for Pygments 2.19.2,
# and 2.19.2 is still the latest published release as of 2026-03-28.
# Remove this ignore once an upstream patched release is available.
uv tool run --from pip-audit==2.10.0 pip-audit \
  --requirement "${requirements_file}" \
  --ignore-vuln CVE-2026-4539

echo "pip-audit: PASS"
