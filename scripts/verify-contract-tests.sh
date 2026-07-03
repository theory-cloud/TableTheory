#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d "contract-tests" ]]; then
  echo "contract-tests: SKIP (contract-tests/ not found)"
  exit 0
fi

echo "contract-tests: generated-ts"
bash scripts/verify-generated-ts-key-contract.sh

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

if [[ -d "contract-tests/scenarios/interop" ]]; then
  echo "contract-tests: cross-runtime interop"
  if [[ -f "contract-tests/runners/go/go.mod" ]]; then
    (cd contract-tests/runners/go && CONTRACT_RUN_INTEROP=1 go test . -run TestContract_Interop -count=1 -v)
  fi
  if [[ -f "contract-tests/runners/ts/package.json" ]]; then
    CONTRACT_RUN_INTEROP=1 npm --prefix contract-tests/runners/ts run test:contract:interop
  fi
  if [[ -d "contract-tests/runners/py" ]]; then
    CONTRACT_RUN_INTEROP=1 uv --directory py run pytest -q ../contract-tests/runners/py/test_contract_p0.py -k interop
  fi
fi

echo "contract-tests: PASS"
