#!/usr/bin/env bash
set -euo pipefail

app_dir="examples/cdk-multilang"
python_app_dir="${app_dir}/lambdas/python"

if [[ ! -f "${app_dir}/package.json" ]]; then
  echo "cdk-synth: FAIL (missing ${app_dir}/package.json)"
  exit 1
fi

if [[ ! -f "${python_app_dir}/pyproject.toml" || ! -f "${python_app_dir}/uv.lock" ]]; then
  echo "cdk-synth: FAIL (Python Lambda must provide pyproject.toml and uv.lock for dependency graph submission)"
  exit 1
fi

if [[ -f "${python_app_dir}/requirements.txt" ]]; then
  echo "cdk-synth: FAIL (requirements.txt would split the Python Lambda dependency source from its uv project)"
  exit 1
fi

command -v uv >/dev/null 2>&1 || {
  echo "cdk-synth: FAIL (uv not found)"
  exit 1
}

if ! uv lock --directory "${python_app_dir}" --check >"/dev/null" 2>&1; then
  echo "cdk-synth: FAIL (Python Lambda uv.lock is stale)"
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
