#!/usr/bin/env bash
set -euo pipefail

# Enforces network hygiene defaults in first-party code:
# - http.Client literals must set Timeout
# - aws.NopRetryer must not be used in production paths unless explicitly allowlisted

failures=0

# Check http.Client literals for Timeout: within a small window.
set +e
client_matches="$(rg -n --no-heading --glob '*.go' --glob '!**/*_test.go' --glob '!examples/**' --glob '!tests/**' --glob '!scripts/**' '(^|[^[:alnum:]_])&?http\.Client\{' .)"
client_status=$?
set -e
if [[ "${client_status}" -gt 1 ]]; then
  echo "network-hygiene: FAIL (rg error searching for http.Client literals: exit ${client_status})"
  exit 1
fi
if [[ "${client_status}" -eq 0 ]]; then
  while IFS=: read -r file line _; do
    snippet="$(sed -n "${line},$((line + 40))p" "${file}")"
    if ! grep -q 'Timeout:' <<<"${snippet}"; then
      echo "network-hygiene: ${file}:${line}: http.Client literal without Timeout"
      failures=$((failures + 1))
    fi
  done <<< "${client_matches}"
fi

# Disallow aws.NopRetryer usage in non-test code by default (allowlist via comment).
set +e
nop_matches="$(rg -n --no-heading --glob '*.go' --glob '!**/*_test.go' --glob '!examples/**' --glob '!tests/**' --glob '!scripts/**' 'aws\.NopRetryer\s*\{' .)"
nop_status=$?
set -e
if [[ "${nop_status}" -gt 1 ]]; then
  echo "network-hygiene: FAIL (rg error searching for aws.NopRetryer: exit ${nop_status})"
  exit 1
fi
if [[ "${nop_status}" -eq 0 ]]; then
  while IFS=: read -r file line _; do
    context="$(sed -n "${line},$((line + 3))p" "${file}")"
    if ! grep -q 'theorydb: allow-nop-retryer' <<<"${context}"; then
      echo "network-hygiene: ${file}:${line}: aws.NopRetryer used without allowlist comment"
      failures=$((failures + 1))
    fi
  done <<< "${nop_matches}"
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "network-hygiene: FAIL (${failures} issue(s))"
  exit 1
fi

echo "network-hygiene: clean"
