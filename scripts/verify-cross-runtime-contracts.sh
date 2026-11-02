#!/usr/bin/env bash
# Cross-runtime contract gate for the two shared, language-neutral fixtures.
#
# The nil/empty serialization table and the transaction-cancellation fixture are
# the single source of truth every runtime must agree on. Each runtime already
# asserts its own column inside its own suite, but those assertions are reached
# through the per-runtime unit suites and the Docker-gated integration path, so
# the shared surface itself had no dedicated, always-on gate. This script is
# that gate: it runs both fixtures in Go, TypeScript and Python, and it is
# deliberately independent of SKIP_INTEGRATION so it cannot be silently skipped.
#
# It needs no Docker and no DynamoDB Local: every fixture is driven through each
# runtime's in-memory stateful seam. A divergence in any one runtime fails here.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

if [[ ! -d "contract-tests" ]]; then
  echo "cross-runtime-contracts: SKIP (contract-tests/ not found)"
  exit 0
fi

have_go=0
command -v go >/dev/null 2>&1 && have_go=1
have_node=0
command -v npm >/dev/null 2>&1 && have_node=1
have_py=0
[[ -f "py/pyproject.toml" ]] && command -v uv >/dev/null 2>&1 && have_py=1

# Fixture integrity first: a dropped row/case or a runtime that stopped reading
# its fixture must fail before the per-runtime assertions are trusted.
if [[ "${have_py}" -eq 1 ]]; then
  echo "cross-runtime-contracts: fixture integrity"
  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi
  # The Python venv carries PyYAML, which the fixture validator needs.
  uv --directory py run python ../scripts/verify-cross-runtime-fixtures.py
else
  echo "cross-runtime-contracts: fixture integrity SKIP (python venv unavailable)"
fi

if [[ "${have_go}" -eq 1 ]]; then
  echo "cross-runtime-contracts: go"
  # Serialization table, Go column (writer-path preflight conformance).
  go test ./pkg/marshal/ -run 'TestSharedConformanceTableGoColumn' -count=1
  # Lifecycle-clock table, Go column (lifecycle stamping on both writer paths).
  go test ./pkg/marshal/ -run 'TestSharedLifecycleClockTableGoColumn' -count=1
  if [[ -f "contract-tests/runners/go/go.mod" ]]; then
    # Transaction-cancellation fixture, Go column.
    (cd contract-tests/runners/go && go test . -run 'TestContract_TransactionCancellation' -count=1)
  fi
else
  echo "cross-runtime-contracts: go SKIP (go unavailable)"
fi

if [[ "${have_node}" -eq 1 ]]; then
  echo "cross-runtime-contracts: typescript"
  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  # Serialization table, TypeScript column.
  (cd ts && npm exec -- tsx test/unit/preflight-conformance.test.ts)
  # Lifecycle-clock table, TypeScript column.
  (cd ts && npm exec -- tsx test/unit/lifecycle-clock-conformance.test.ts)
  if [[ -f "contract-tests/runners/ts/package.json" ]]; then
    if [[ ! -d "contract-tests/runners/ts/node_modules" ]]; then
      npm --prefix contract-tests/runners/ts ci --ignore-scripts
    fi
    # Transaction-cancellation fixture, TypeScript column.
    npm --prefix contract-tests/runners/ts run test:contract:cancellation
  fi
else
  echo "cross-runtime-contracts: typescript SKIP (npm unavailable)"
fi

if [[ "${have_py}" -eq 1 ]]; then
  echo "cross-runtime-contracts: python"
  # Serialization table, Python column.
  uv --directory py run pytest -q tests/unit/test_preflight_conformance.py
  # Lifecycle-clock table, Python column.
  uv --directory py run pytest -q tests/unit/test_lifecycle_clock_conformance.py
  # Transaction-cancellation fixture, Python column.
  uv --directory py run pytest -q ../contract-tests/runners/py/test_contract_cancellation.py
fi

echo "cross-runtime-contracts: PASS"
