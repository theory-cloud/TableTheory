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

allowlist_file="gov-infra/planning/theorydb-supply-chain-allowlist.txt"
visible_policy_file="gov-infra/planning/theorydb-visible-npm-audit-findings.json"

run_npm_audit() {
  local prefix="$1"
  local report
  report="$(mktemp)"

  # Audit lockfiles directly so results don't depend on stale local node_modules.
  if npm --prefix "${prefix}" audit --package-lock-only --audit-level=low --json >"${report}"; then
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
