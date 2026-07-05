#!/usr/bin/env bash
set -euo pipefail

# Fast contributor gate for local development.
#
# Scope: formatting/lint, unit tests, and documentation gates across the
# Go/TypeScript/Python repo without starting DynamoDB Local or running the full
# security/release/subtree rubric.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

export SKIP_INTEGRATION="${SKIP_INTEGRATION:-true}"

GO_TOOL_BIN="$(command -v go 2>/dev/null || true)"
if [[ -n "${GO_TOOL_BIN}" ]]; then
  GO_TOOL_DIR="$(dirname "${GO_TOOL_BIN}")"
  export PATH="${GO_TOOL_DIR}:${PATH}"
fi

if command -v go >/dev/null 2>&1; then
  GO_BIN_DIR="$(go env GOBIN)"
  if [[ -z "${GO_BIN_DIR}" ]]; then
    GO_BIN_DIR="$(go env GOPATH)/bin"
  fi
  export PATH="${GO_BIN_DIR}:${PATH}"
fi

check_file_exists() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    echo "rubric-fast: FAIL (missing ${path})" >&2
    return 1
  fi
}

check_gov_threat_controls_parity() {
  local threat_model="gov-infra/planning/theorydb-threat-model.md"
  local controls_matrix="gov-infra/planning/theorydb-controls-matrix.md"

  check_file_exists "${threat_model}"
  check_file_exists "${controls_matrix}"

  local threats mapped missing unknown
  threats="$(grep -oE 'THR-[0-9]+' "${threat_model}" | sort -u || true)"
  mapped="$(grep -oE 'THR-[0-9]+' "${controls_matrix}" | sort -u || true)"

  if [[ -z "${threats}" ]]; then
    echo "rubric-fast: FAIL (no THR-* IDs found in ${threat_model})" >&2
    return 1
  fi
  if [[ -z "${mapped}" ]]; then
    echo "rubric-fast: FAIL (no THR-* IDs found in ${controls_matrix})" >&2
    return 1
  fi

  missing="$(comm -23 <(printf '%s\n' "${threats}") <(printf '%s\n' "${mapped}") || true)"
  unknown="$(comm -13 <(printf '%s\n' "${threats}") <(printf '%s\n' "${mapped}") || true)"

  if [[ -n "${missing}" || -n "${unknown}" ]]; then
    if [[ -n "${missing}" ]]; then
      printf 'rubric-fast: missing gov threat mappings:\n%s\n' "${missing}" >&2
    fi
    if [[ -n "${unknown}" ]]; then
      printf 'rubric-fast: unknown gov threat mappings:\n%s\n' "${unknown}" >&2
    fi
    return 1
  fi

  echo "gov-threat-parity: PASS"
}

echo "=== TableTheory fast contributor rubric ==="
echo "SKIP_INTEGRATION=${SKIP_INTEGRATION}"
echo ""

bash scripts/verify-formatting.sh
bash scripts/verify-lint.sh
bash scripts/verify-unit-tests.sh
bash scripts/verify-planning-docs.sh

check_file_exists "gov-infra/planning/theorydb-threat-model.md"
check_file_exists "gov-infra/planning/theorydb-evidence-plan.md"
check_file_exists "gov-infra/planning/theorydb-10of10-rubric.md"
bash scripts/verify-doc-integrity.sh
check_gov_threat_controls_parity
bash scripts/verify-threat-controls-parity.sh

echo "rubric-fast: PASS"
