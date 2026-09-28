#!/usr/bin/env bash
set -euo pipefail

# Policy test for the shared bounded-retry helper (scripts/lib/retry.sh) and for
# its wiring into the network-dependent scanners.
#
# Two properties matter and both are asserted here:
#   1. Transient failure is absorbed: an attempt sequence that eventually
#      succeeds returns 0 (and the retry count is bounded).
#   2. Failure stays terminal: an always-failing command returns non-zero after
#      exactly <attempts> attempts, so a real finding or a sustained outage can
#      never be turned green by retrying.

failures=0

fail() {
  echo "network-retry: $1"
  failures=$((failures + 1))
}

retry_lib="scripts/lib/retry.sh"
if [[ ! -f "${retry_lib}" ]]; then
  echo "network-retry: FAIL (missing ${retry_lib})"
  exit 1
fi

# shellcheck source=scripts/lib/retry.sh
source "${retry_lib}"

if ! declare -F theorydb_retry_bounded >/dev/null; then
  echo "network-retry: FAIL (${retry_lib} does not define theorydb_retry_bounded)"
  exit 1
fi

scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

# Case 1: fails twice, then succeeds. The helper must return 0.
counter="${scratch}/eventual"
: >"${counter}"
eventual() {
  local n
  n="$(wc -l <"${counter}" | tr -d ' ')"
  echo "x" >>"${counter}"
  if (( n < 2 )); then
    return 7
  fi
  return 0
}
if theorydb_retry_bounded 3 1 "retry-test-eventual" -- eventual >/dev/null 2>&1; then
  attempts="$(wc -l <"${counter}" | tr -d ' ')"
  if [[ "${attempts}" != "3" ]]; then
    fail "eventual-success case ran ${attempts} attempt(s); expected exactly 3"
  fi
else
  fail "eventual-success case returned non-zero; transient failure was not absorbed"
fi

# Case 2: always fails. The helper must return non-zero and stop at the bound.
counter="${scratch}/terminal"
: >"${counter}"
terminal() {
  echo "x" >>"${counter}"
  return 9
}
if theorydb_retry_bounded 2 1 "retry-test-terminal" -- terminal >/dev/null 2>&1; then
  fail "always-failing command returned 0; retry must stay fail-closed"
else
  attempts="$(wc -l <"${counter}" | tr -d ' ')"
  if [[ "${attempts}" != "2" ]]; then
    fail "always-failing case ran ${attempts} attempt(s); expected exactly 2 (bounded)"
  fi
fi

# Case 3: an invalid bound must be refused rather than treated as "no retries"
# or as an unbounded loop.
if theorydb_retry_bounded 0 1 "retry-test-invalid" -- true >/dev/null 2>&1; then
  fail "attempts=0 returned 0; a non-positive bound must be refused"
fi

# Wiring: the scanners whose upstream registries have no retry of their own must
# route through the helper, and must not swallow a failure with `|| true`.
for scanner in \
  "scripts/sec-pip-audit.sh" \
  "scripts/sec-npm-audit.sh" \
  "scripts/sec-govulncheck.sh"; do
  if [[ ! -f "${scanner}" ]]; then
    fail "missing ${scanner}"
    continue
  fi
  grep -Fq 'scripts/lib/retry.sh' "${scanner}" || \
    fail "${scanner} must source scripts/lib/retry.sh"
  grep -Fq 'theorydb_retry_bounded' "${scanner}" || \
    fail "${scanner} must run its network call through theorydb_retry_bounded"
  if grep -Eq 'theorydb_retry_bounded[^|]*\|\|[[:space:]]*true' "${scanner}"; then
    fail "${scanner} must not swallow a retry failure with '|| true'"
  fi
done

if [[ "${failures}" -ne 0 ]]; then
  echo "network-retry: FAIL (${failures} issue(s))"
  exit 1
fi

echo "network-retry: PASS (bounded retry absorbs transients and stays fail-closed)"
