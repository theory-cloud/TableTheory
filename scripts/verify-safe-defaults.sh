#!/usr/bin/env bash
set -euo pipefail

# Verifies "safe-by-default" posture for security-critical defaults.
#
# Current policy:
# - The unsafe marshaler constructor `marshal.New(...)` must not be used by default DB construction.
# - Unsafe behavior should be reachable only via explicit opt-in and acknowledgment (implemented in pkg/marshal).

failures=0

# Unsafe marshaler should not be wired into the default constructor(s).
set +e
unsafe_refs="$(rg -n --no-heading --glob '*.go' --glob '!**/*_test.go' --glob '!examples/**' --glob '!tests/**' --glob '!scripts/**' --glob '!pkg/marshal/**' 'marshal\.New\(' .)"
unsafe_status=$?
set -e
if [[ "${unsafe_status}" -gt 1 ]]; then
  echo "safe-defaults: FAIL (rg error searching for marshal.New: exit ${unsafe_status})"
  exit 1
fi
if [[ "${unsafe_status}" -eq 0 ]]; then
  echo "safe-defaults: found unsafe marshaler constructor used outside pkg/marshal (must be opt-in only):"
  echo "${unsafe_refs}"
  failures=$((failures + 1))
fi

# Ensure there is at least one non-test usage of the safe marshaler factory/constructor in the repo.
# (This will be satisfied once defaults are rewired.)
set +e
safe_refs="$(rg -n --no-heading --glob '*.go' --glob '!**/*_test.go' --glob '!examples/**' --glob '!tests/**' --glob '!scripts/**' 'marshal\.(NewSafeMarshaler|NewMarshalerFactory)\(' .)"
safe_status=$?
set -e
if [[ "${safe_status}" -gt 1 ]]; then
  echo "safe-defaults: FAIL (rg error searching for safe marshaler usage: exit ${safe_status})"
  exit 1
fi
if [[ "${safe_status}" -ne 0 ]]; then
  echo "safe-defaults: no production usage of safe marshaler detected (expected defaults to use safe marshaling)"
  failures=$((failures + 1))
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "safe-defaults: FAIL (${failures} issue(s))"
  exit 1
fi

echo "safe-defaults: enforced"
