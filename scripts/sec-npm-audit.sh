#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "ts/package.json" ]]; then
  echo "npm-audit: SKIP (ts/package.json not found)"
  exit 0
fi

command -v npm >/dev/null 2>&1 || {
  echo "npm-audit: FAIL (npm not found)"
  exit 1
}

source scripts/lib/retry.sh

allowlist_file="gov-infra/planning/theorydb-supply-chain-allowlist.txt"
visible_policy_file="gov-infra/planning/theorydb-visible-npm-audit-findings.json"

# Write exactly one audit JSON document per attempt. Redirecting the retry
# helper's output instead would keep a single file descriptor open across
# attempts, concatenating several JSON documents when a finding triggers a
# retry and leaving the policy checker unable to parse the report.
npm_audit_json() {
  npm --prefix "$1" audit --package-lock-only --audit-level=low --json >"$2"
}

run_npm_audit() {
  local prefix="$1"
  local report
  report="$(mktemp)"

  # Audit lockfiles directly so results don't depend on stale local node_modules.
  # The registry query is a single network call with no retry of its own, so a
  # transient registry 5xx would otherwise fail the gate. Retrying is bounded,
  # and a real finding still falls through to the allowlist path below.
  if theorydb_retry_bounded 3 5 "npm-audit(${prefix})" -- \
    npm_audit_json "${prefix}" "${report}"; then
    echo "npm-audit: PASS (${prefix})"
    rm -f "${report}"
    return 0
  fi

  if [[ -f "${allowlist_file}" ]] && node scripts/check-npm-audit-allowlist.mjs "${report}" "${allowlist_file}" "${prefix}" "${visible_policy_file}"; then
    echo "npm-audit: PASS (${prefix}; findings handled by repo policy)"
    rm -f "${report}"
    return 0
  fi

  rm -f "${report}"
  npm --prefix "${prefix}" audit --package-lock-only --audit-level=low
}

run_npm_audit ts

if [[ -f "contract-tests/runners/ts/package.json" ]]; then
  run_npm_audit contract-tests/runners/ts
fi

if [[ -f "examples/cdk-multilang/package.json" ]]; then
  run_npm_audit examples/cdk-multilang
fi

echo "npm-audit: PASS"
