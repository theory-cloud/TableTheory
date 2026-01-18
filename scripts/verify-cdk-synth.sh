#!/usr/bin/env bash
set -euo pipefail

app_dir="examples/cdk-multilang"

if [[ ! -f "${app_dir}/package.json" ]]; then
  echo "cdk-synth: FAIL (missing ${app_dir}/package.json)"
  exit 1
fi

log_file="$(mktemp)"
cleanup() { rm -f "${log_file}"; }
trap cleanup EXIT

if ! npm --prefix "${app_dir}" ci >"${log_file}" 2>&1; then
  cat "${log_file}"
  echo "cdk-synth: FAIL (npm ci)"
  exit 1
fi

if ! npm --prefix "${app_dir}" run synth >"${log_file}" 2>&1; then
  cat "${log_file}"
  echo "cdk-synth: FAIL (cdk synth)"
  exit 1
fi

echo "cdk-synth: PASS"
