#!/usr/bin/env bash
set -euo pipefail

default_threshold="90.0"
threshold="${TS_COVERAGE_THRESHOLD:-${default_threshold}}"
suite="${TS_COVERAGE_SUITE:-unit}"

awk -v t="${threshold}" -v d="${default_threshold}" 'BEGIN { exit !(t+0 >= d+0) }' || {
  echo "ts-coverage: TS_COVERAGE_THRESHOLD (${threshold}) must be >= default (${default_threshold})"
  exit 1
}

if [[ "${suite}" != "unit" && "${suite}" != "all" ]]; then
  echo "ts-coverage: FAIL (TS_COVERAGE_SUITE must be 'unit' or 'all'; got ${suite})"
  exit 1
fi

bash scripts/coverage-ts.sh "${suite}" >/dev/null

summary="ts/coverage/coverage-${suite}.json"
if [[ ! -f "${summary}" ]]; then
  echo "ts-coverage: FAIL (missing coverage summary: ${summary})"
  exit 1
fi

lines="$(
  python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("${summary}").read_text(encoding="utf-8"))
val = data.get("lines", None)
if val is None:
    raise SystemExit(2)
print(val)
PY
)"

awk -v total="${lines}" -v threshold="${threshold}" 'BEGIN { exit !(total+0 >= threshold+0) }' || {
  echo "ts-coverage: FAIL (${lines}% < ${threshold}%)"
  exit 1
}

echo "ts-coverage: PASS (${lines}% >= ${threshold}%)"

