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

if ! npm --prefix "${app_dir}" ci --ignore-scripts >"${log_file}" 2>&1; then
  cat "${log_file}"
  echo "cdk-synth: FAIL (npm ci)"
  exit 1
fi

if ! npm --prefix "${app_dir}" run synth >"${log_file}" 2>&1; then
  cat "${log_file}"
  echo "cdk-synth: FAIL (cdk synth)"
  exit 1
fi

# Read the synthesized template locally: assert every Lambda function's
# effective runtime is supported. This never deploys the stack. The assertion's
# own self-test proves the read path fails closed on a deprecated override, a
# dynamic runtime, and an empty template.
node scripts/verify-cdk-synth-runtimes.mjs --self-test >"${log_file}" 2>&1 || {
  cat "${log_file}"
  echo "cdk-synth: FAIL (synthesized runtime assertion self-test)"
  exit 1
}
node scripts/verify-cdk-synth-runtimes.mjs "${app_dir}/cdk.out" >"${log_file}" 2>&1 || {
  cat "${log_file}"
  echo "cdk-synth: FAIL (synthesized runtime assertion)"
  exit 1
}

echo "cdk-synth: PASS"
