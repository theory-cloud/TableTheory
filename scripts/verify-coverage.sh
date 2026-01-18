#!/usr/bin/env bash
set -euo pipefail

default_threshold="90.0"
threshold="${COVERAGE_THRESHOLD:-${default_threshold}}"
profile="${COVER_PROFILE:-coverage_lib.out}"

# Prevent "green by drift" via env overrides: allow raising the bar locally, but not lowering it.
awk -v t="${threshold}" -v d="${default_threshold}" 'BEGIN { exit !(t+0 >= d+0) }' || {
  echo "COVERAGE_THRESHOLD (${threshold}) must be >= default (${default_threshold})"
  exit 1
}

total_line="$(bash scripts/coverage.sh "${profile}")"
total_pct="$(echo "${total_line}" | awk '{print $NF}' | sed 's/%$//')"

awk -v total="${total_pct}" -v threshold="${threshold}" 'BEGIN { exit !(total+0 >= threshold+0) }' || {
  echo "coverage: FAIL (${total_pct}% < ${threshold}%)"
  exit 1
}

echo "coverage: PASS (${total_pct}% >= ${threshold}%)"

# Enforce TypeScript + Python library coverage parity in the same gate.
# These scripts are raise-only and default to 90% as well.
export TS_COVERAGE_THRESHOLD="${TS_COVERAGE_THRESHOLD:-${threshold}}"
export PY_COVERAGE_THRESHOLD="${PY_COVERAGE_THRESHOLD:-${threshold}}"

bash scripts/verify-typescript-coverage.sh
bash scripts/verify-python-coverage.sh
