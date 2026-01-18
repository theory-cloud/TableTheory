#!/usr/bin/env bash
set -euo pipefail

# High-risk domain rule: panics in production code are availability vulnerabilities.
# This verifier blocks `panic(...)` in first-party, non-test Go code.

set +e
rg -n --no-heading \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  --glob '!examples/**' \
  --glob '!tests/**' \
  --glob '!scripts/**' \
  --glob '!pkg/testing/**' \
  --glob '!pkg/mocks/**' \
  'panic\(' .
status=$?
set -e

case "${status}" in
  0)
    echo "no-panics: FAIL (panic() found in production code paths)"
    exit 1
    ;;
  1)
    echo "no-panics: clean"
    ;;
  *)
    echo "no-panics: FAIL (rg error: exit ${status})"
    exit 1
    ;;
esac
