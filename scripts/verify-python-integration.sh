#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "python-integration: SKIP (py/pyproject.toml not found)"
  exit 0
fi

skip="${SKIP_INTEGRATION:-}"
if [[ "${skip}" == "1" || "${skip}" == "true" ]]; then
  echo "python-integration: SKIP (SKIP_INTEGRATION=${skip})"
  exit 0
fi

if [[ ! -d "py/.venv" ]]; then
  bash scripts/verify-python-deps.sh
fi

export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

uv --directory py run pytest -q tests/integration

echo "python-integration: PASS"

