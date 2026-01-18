#!/usr/bin/env bash
set -euo pipefail

# Scan first-party code only. We explicitly exclude repo-local caches and example/test harness code to keep the signal high.
args=(
  -exclude-dir=.gomodcache
  -exclude-dir=.gocache
  -exclude-dir=examples
  -exclude-dir=tests
)

if [[ -n "${GOSEC_SARIF_PATH:-}" ]]; then
  gosec "${args[@]}" -fmt=sarif -out="${GOSEC_SARIF_PATH}" ./...
else
  gosec "${args[@]}" ./...
fi

