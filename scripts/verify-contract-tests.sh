#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d "contract-tests" ]]; then
  echo "contract-tests: SKIP (contract-tests/ not found)"
  exit 0
fi

skip="${SKIP_INTEGRATION:-}"
if [[ "${skip}" == "1" || "${skip}" == "true" ]]; then
  echo "contract-tests: SKIP (SKIP_INTEGRATION=${skip})"
  exit 0
fi

export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

if [[ -f "contract-tests/runners/go/go.mod" ]]; then
  echo "contract-tests: go"
  (cd contract-tests/runners/go && go test ./... -v)
fi

if [[ -f "contract-tests/runners/ts/package.json" ]]; then
  echo "contract-tests: ts"
  if [[ ! -d "contract-tests/runners/ts/node_modules" ]]; then
    npm --prefix contract-tests/runners/ts ci
  fi
  npm --prefix contract-tests/runners/ts test
fi

if [[ -d "contract-tests/runners/py" ]]; then
  echo "contract-tests: py"
  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi
  uv --directory py run pytest -q ../contract-tests/runners/py
fi

echo "contract-tests: PASS"

