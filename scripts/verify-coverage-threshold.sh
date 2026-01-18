#!/usr/bin/env bash
set -euo pipefail

required="90.0"
script="scripts/verify-coverage.sh"

extract_default() {
  local file="$1"
  awk -F= '
    /^default_threshold=/ {
      v=$2
      gsub(/"/, "", v)
      gsub(/\047/, "", v)
      print v
      exit
    }
  ' "${file}"
}

if [[ ! -f "${script}" ]]; then
  echo "coverage-threshold: FAIL (${script} not found)"
  exit 1
fi

default="$(extract_default "${script}")"

if [[ -z "${default}" ]]; then
  echo "coverage-threshold: FAIL (default_threshold not found in ${script})"
  exit 1
fi

awk -v d="${default}" -v r="${required}" 'BEGIN { exit !(d+0 >= r+0) }' || {
  echo "coverage-threshold: FAIL (default ${default}% < required ${required}%)"
  exit 1
}

for verifier in scripts/verify-typescript-coverage.sh scripts/verify-python-coverage.sh; do
  if [[ ! -f "${verifier}" ]]; then
    echo "coverage-threshold: FAIL (${verifier} not found)"
    exit 1
  fi
  d="$(extract_default "${verifier}")"
  if [[ -z "${d}" ]]; then
    echo "coverage-threshold: FAIL (default_threshold not found in ${verifier})"
    exit 1
  fi
  awk -v d="${d}" -v r="${required}" 'BEGIN { exit !(d+0 >= r+0) }' || {
    echo "coverage-threshold: FAIL (${verifier} default ${d}% < required ${required}%)"
    exit 1
  }
done

echo "coverage-threshold: ok (defaults >= ${required}%)"
