#!/usr/bin/env bash
set -euo pipefail

suite="${1:-unit}"

if [[ ! -f "ts/package.json" ]]; then
  echo "coverage-ts: SKIP (ts/package.json not found)"
  exit 0
fi

if [[ ! -d "ts/node_modules" ]]; then
  bash scripts/verify-typescript-deps.sh
fi

outdir="ts/coverage"
mkdir -p "${outdir}"

export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

tests=(test/unit/*.test.ts)
if [[ "${suite}" == "all" ]]; then
  tests+=(test/integration/*.test.ts)
elif [[ "${suite}" != "unit" ]]; then
  echo "coverage-ts: FAIL (unknown suite: ${suite}; expected 'unit' or 'all')"
  exit 1
fi

log="${outdir}/coverage-${suite}.txt"
summary_json="${outdir}/coverage-${suite}.json"

pushd ts >/dev/null
set +e
output="$(
  node --import tsx --test --experimental-test-coverage \
    --test-coverage-include=src/**/*.ts \
    --test-coverage-exclude=test/** \
    "${tests[@]}" 2>&1
)"
status=$?
set -e
popd >/dev/null

printf "%s\n" "${output}" | tee "${log}"

if [[ "${status}" -ne 0 ]]; then
  echo "coverage-ts: FAIL (tests failed; exit ${status})"
  exit "${status}"
fi

summary_line="$(printf "%s\n" "${output}" | grep -F 'all files' | tail -n 1 || true)"
if [[ -z "${summary_line}" ]]; then
  echo "coverage-ts: FAIL (missing 'all files' summary line in coverage output)"
  exit 1
fi

lines="$(printf "%s\n" "${summary_line}" | awk -F'|' '{gsub(/[[:space:]]/, "", $2); print $2}')"
branches="$(printf "%s\n" "${summary_line}" | awk -F'|' '{gsub(/[[:space:]]/, "", $3); print $3}')"
functions="$(printf "%s\n" "${summary_line}" | awk -F'|' '{gsub(/[[:space:]]/, "", $4); print $4}')"

if [[ -z "${lines}" || -z "${branches}" || -z "${functions}" ]]; then
  echo "coverage-ts: FAIL (unable to parse coverage percentages from: ${summary_line})"
  exit 1
fi

cat >"${summary_json}" <<JSON
{
  "suite": "${suite}",
  "lines": ${lines},
  "branches": ${branches},
  "functions": ${functions}
}
JSON

echo "coverage-ts: PASS (${suite}; lines ${lines}%, branches ${branches}%, functions ${functions}%)"
