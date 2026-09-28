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

source scripts/lib/retry.sh

# Fail on any known vulnerability (no green-by-severity). pip-audit resolves
# every pinned requirement against pypi.org, which intermittently returns 5xx
# (observed 2026-09-28: a pypi 503 failed a gating SEC-2 run). Absorb that
# bounded transient window with backoff; a sustained outage or a real finding
# still fails the gate after the retries.
theorydb_retry_bounded 3 5 "pip-audit" -- \
  uv tool run --from pip-audit==2.10.0 pip-audit \
  --requirement "${requirements_file}"

echo "pip-audit: PASS"
