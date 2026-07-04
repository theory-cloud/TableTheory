#!/usr/bin/env bash
set -euo pipefail

# Canonical coverage verifier and coverage utility surface.
#
# Default mode preserves the QUA-3 gate: Go library coverage first, then
# TypeScript and Python parity gates. Parameterized modes replace the former
# per-language coverage helpers, threshold-config checker, package-floor helper,
# and terminal dashboard while keeping one source of truth for thresholds and
# measurement surfaces.

default_threshold="90.0"
ts_default_threshold="90.0"
py_default_threshold="90.0"
required_threshold="90.0"

language="all"
suite=""
profile="${COVER_PROFILE:-coverage_lib.out}"
record_only="false"
check_threshold_config="false"
dashboard="false"
package_targets=""

usage() {
  cat <<'USAGE'
Usage:
  bash scripts/verify-coverage.sh [options]

Default:
  Runs the full rubric coverage gate (Go + TypeScript + Python) with raise-only
  thresholds (default 90%).

Options:
  --language go|typescript|python|all
      Select the coverage surface. Default: all.
  --suite unit|all
      Select TypeScript/Python suite. Defaults to TS_COVERAGE_SUITE,
      PY_COVERAGE_SUITE, or unit.
  --profile <cover.out>
      Go coverage profile path. Default: COVER_PROFILE or coverage_lib.out.
  --record-only
      Measure/write coverage artifacts without enforcing thresholds. Replaces
      the former coverage.sh, coverage-ts.sh, and coverage-py.sh helper paths.
  --check-threshold-config
      Verify default coverage thresholds are not diluted. Replaces the former
      verify-coverage-threshold.sh.
  --package-targets <targets.tsv>
      Verify Go package coverage floors against a target TSV. Replaces the
      former verify-coverage-packages.sh. Requires --profile if not using the
      default profile.
  --dashboard
      Print the terminal package dashboard. Replaces the former
      coverage-dashboard.sh.
  -h, --help
      Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --language)
      language="${2:-}"
      shift 2
      ;;
    --suite)
      suite="${2:-}"
      shift 2
      ;;
    --profile)
      profile="${2:-}"
      shift 2
      ;;
    --record-only)
      record_only="true"
      shift
      ;;
    --check-threshold-config)
      check_threshold_config="true"
      shift
      ;;
    --package-targets)
      package_targets="${2:-}"
      shift 2
      ;;
    --dashboard)
      dashboard="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "coverage: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "${language}" in
  go|typescript|python|all) ;;
  *)
    echo "coverage: FAIL (--language must be go, typescript, python, or all; got ${language})"
    exit 2
    ;;
esac

coverage_threshold_ok() {
  local actual="$1"
  local minimum="$2"
  awk -v a="${actual}" -v m="${minimum}" 'BEGIN { exit !(a+0 >= m+0) }'
}

check_raise_only_threshold() {
  local label="$1"
  local threshold="$2"
  local default="$3"
  if ! coverage_threshold_ok "${threshold}" "${default}"; then
    echo "${label}: ${label^^}_COVERAGE_THRESHOLD (${threshold}) must be >= default (${default})"
    return 1
  fi
}

run_threshold_config_check() {
  local failures=0

  coverage_threshold_ok "${default_threshold}" "${required_threshold}" || {
    echo "coverage-threshold: FAIL (go default ${default_threshold}% < required ${required_threshold}%)"
    failures=$((failures + 1))
  }
  coverage_threshold_ok "${ts_default_threshold}" "${required_threshold}" || {
    echo "coverage-threshold: FAIL (typescript default ${ts_default_threshold}% < required ${required_threshold}%)"
    failures=$((failures + 1))
  }
  coverage_threshold_ok "${py_default_threshold}" "${required_threshold}" || {
    echo "coverage-threshold: FAIL (python default ${py_default_threshold}% < required ${required_threshold}%)"
    failures=$((failures + 1))
  }

  if [[ "${failures}" -ne 0 ]]; then
    exit 1
  fi

  echo "coverage-threshold: ok (defaults >= ${required_threshold}%)"
}

measure_go_coverage() {
  local out_profile="$1"

  # Measure "library coverage" (exclude repo-local examples, tests, tool harness
  # packages, and third-party dependency trees materialized inside the repo).
  # This avoids a low-signal denominator dominated by non-library modules.
  local pkgs
  pkgs="$(go list ./... | grep -Ev '/examples($|/)|/tests($|/)|/scripts($|/)|/node_modules($|/)|/vendor($|/)')"
  if [[ -z "${pkgs}" ]]; then
    echo "no packages found"
    return 1
  fi

  local coverpkgs
  coverpkgs="$(echo "${pkgs}" | paste -sd, -)"

  go test -short -count=1 -coverpkg="${coverpkgs}" -coverprofile="${out_profile}" ${pkgs} >/dev/null
  go tool cover -func="${out_profile}" | tail -n 1
}

verify_go_coverage() {
  local threshold="${COVERAGE_THRESHOLD:-${default_threshold}}"

  if ! coverage_threshold_ok "${threshold}" "${default_threshold}"; then
    echo "COVERAGE_THRESHOLD (${threshold}) must be >= default (${default_threshold})"
    return 1
  fi

  local total_line total_pct
  total_line="$(measure_go_coverage "${profile}")"
  total_pct="$(echo "${total_line}" | awk '{print $NF}' | sed 's/%$//')"

  if ! coverage_threshold_ok "${total_pct}" "${threshold}"; then
    echo "coverage: FAIL (${total_pct}% < ${threshold}%)"
    return 1
  fi

  echo "coverage: PASS (${total_pct}% >= ${threshold}%)"
}

validate_suite() {
  local label="$1"
  local selected="$2"
  if [[ "${selected}" != "unit" && "${selected}" != "all" ]]; then
    echo "${label}: FAIL (${label^^}_COVERAGE_SUITE must be 'unit' or 'all'; got ${selected})"
    return 1
  fi
}

measure_typescript_coverage() {
  local selected_suite="$1"

  if [[ ! -f "ts/package.json" ]]; then
    echo "coverage-ts: SKIP (ts/package.json not found)"
    return 0
  fi

  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi

  local outdir="ts/coverage"
  mkdir -p "${outdir}"

  export AWS_REGION="${AWS_REGION:-us-east-1}"
  export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
  export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
  export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
  export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

  local tests=(test/unit/*.test.ts)
  if [[ "${selected_suite}" == "all" ]]; then
    tests+=(test/integration/*.test.ts)
  else
    validate_suite "coverage-ts" "${selected_suite}"
  fi

  local log="${outdir}/coverage-${selected_suite}.txt"
  local summary_json="${outdir}/coverage-${selected_suite}.json"
  local output status

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
    return "${status}"
  fi

  local summary_line
  summary_line="$(printf "%s\n" "${output}" | grep -F 'all files' | tail -n 1 || true)"
  if [[ -z "${summary_line}" ]]; then
    echo "coverage-ts: FAIL (missing 'all files' summary line in coverage output)"
    return 1
  fi

  local lines branches functions
  lines="$(printf "%s\n" "${summary_line}" | awk -F'|' '{gsub(/[[:space:]]/, "", $2); print $2}')"
  branches="$(printf "%s\n" "${summary_line}" | awk -F'|' '{gsub(/[[:space:]]/, "", $3); print $3}')"
  functions="$(printf "%s\n" "${summary_line}" | awk -F'|' '{gsub(/[[:space:]]/, "", $4); print $4}')"

  if [[ -z "${lines}" || -z "${branches}" || -z "${functions}" ]]; then
    echo "coverage-ts: FAIL (unable to parse coverage percentages from: ${summary_line})"
    return 1
  fi

  cat >"${summary_json}" <<JSON
{
  "suite": "${selected_suite}",
  "lines": ${lines},
  "branches": ${branches},
  "functions": ${functions}
}
JSON

  echo "coverage-ts: PASS (${selected_suite}; lines ${lines}%, branches ${branches}%, functions ${functions}%)"
}

verify_typescript_coverage() {
  local selected_suite="${suite:-${TS_COVERAGE_SUITE:-unit}}"
  validate_suite "ts-coverage" "${selected_suite}"

  local threshold="${TS_COVERAGE_THRESHOLD:-${ts_default_threshold}}"
  if ! coverage_threshold_ok "${threshold}" "${ts_default_threshold}"; then
    echo "ts-coverage: TS_COVERAGE_THRESHOLD (${threshold}) must be >= default (${ts_default_threshold})"
    return 1
  fi

  measure_typescript_coverage "${selected_suite}" >/dev/null

  local summary="ts/coverage/coverage-${selected_suite}.json"
  if [[ ! -f "${summary}" ]]; then
    echo "ts-coverage: FAIL (missing coverage summary: ${summary})"
    return 1
  fi

  local lines
  lines="$(
    python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("${summary}").read_text(encoding="utf-8"))
val = data.get("lines", None)
if val is None:
    raise SystemExit(2)
print(val)
PY
  )"

  if ! coverage_threshold_ok "${lines}" "${threshold}"; then
    echo "ts-coverage: FAIL (${lines}% < ${threshold}%)"
    return 1
  fi

  echo "ts-coverage: PASS (${lines}% >= ${threshold}%)"
}

measure_python_coverage() {
  local selected_suite="$1"

  if [[ ! -f "py/pyproject.toml" ]]; then
    echo "coverage-py: SKIP (py/pyproject.toml not found)"
    return 0
  fi

  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi

  export AWS_REGION="${AWS_REGION:-us-east-1}"
  export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
  export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
  export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
  export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

  local tests=(tests/unit)
  if [[ "${selected_suite}" == "all" ]]; then
    local skip="${SKIP_INTEGRATION:-}"
    if [[ "${skip}" == "1" || "${skip}" == "true" ]]; then
      echo "coverage-py: SKIP (SKIP_INTEGRATION=${skip})"
      return 0
    fi
    tests=(tests/unit tests/integration)
  else
    validate_suite "coverage-py" "${selected_suite}"
  fi

  local log="py/coverage-${selected_suite}.txt"
  local summary_json="py/coverage-${selected_suite}.json"

  uv --directory py run pytest -q "${tests[@]}" \
    --cov=theorydb_py \
    --cov-report=term \
    --cov-report=xml:coverage.xml | tee "${log}"

  if [[ ! -f "py/coverage.xml" ]]; then
    echo "coverage-py: FAIL (missing py/coverage.xml)"
    return 1
  fi

  local total
  total="$(
    python3 - <<'PY'
import xml.etree.ElementTree as ET
from pathlib import Path

root = ET.fromstring(Path("py/coverage.xml").read_text(encoding="utf-8", errors="replace"))
rate = root.attrib.get("line-rate")
if not rate:
    raise SystemExit(2)
print(f"{float(rate) * 100.0:.2f}")
PY
  )"

  cat >"${summary_json}" <<JSON
{
  "suite": "${selected_suite}",
  "lines": ${total}
}
JSON

  echo "coverage-py: PASS (${selected_suite}; lines ${total}%)"
}

verify_python_coverage() {
  local selected_suite="${suite:-${PY_COVERAGE_SUITE:-unit}}"
  validate_suite "py-coverage" "${selected_suite}"

  local threshold="${PY_COVERAGE_THRESHOLD:-${py_default_threshold}}"
  if ! coverage_threshold_ok "${threshold}" "${py_default_threshold}"; then
    echo "py-coverage: PY_COVERAGE_THRESHOLD (${threshold}) must be >= default (${py_default_threshold})"
    return 1
  fi

  measure_python_coverage "${selected_suite}" >/dev/null

  local summary="py/coverage-${selected_suite}.json"
  if [[ ! -f "${summary}" ]]; then
    echo "py-coverage: FAIL (missing coverage summary: ${summary})"
    return 1
  fi

  local lines
  lines="$(
    python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("${summary}").read_text(encoding="utf-8"))
val = data.get("lines", None)
if val is None:
    raise SystemExit(2)
print(val)
PY
  )"

  if ! coverage_threshold_ok "${lines}" "${threshold}"; then
    echo "py-coverage: FAIL (${lines}% < ${threshold}%)"
    return 1
  fi

  echo "py-coverage: PASS (${lines}% >= ${threshold}%)"
}

verify_package_targets() {
  local targets="$1"

  if [[ -z "${targets}" ]]; then
    echo "coverage-packages: FAIL (missing --package-targets value)"
    return 2
  fi
  if [[ ! -f "${profile}" ]]; then
    echo "missing coverage profile: ${profile}"
    echo "run: bash scripts/verify-coverage.sh --language go --record-only --profile ${profile}"
    return 1
  fi
  if [[ ! -f "${targets}" ]]; then
    echo "missing targets file: ${targets}"
    return 1
  fi

  python3 - "${profile}" "${targets}" <<'PY'
from __future__ import annotations

import sys
from pathlib import Path

profile = Path(sys.argv[1])
targets = Path(sys.argv[2])

min_pct: dict[str, float] = {}
for raw in targets.read_text(encoding="utf-8").splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    parts = line.split()
    if len(parts) != 2:
        print(f"invalid targets line (expected: <package> <min_percent>): {raw}")
        raise SystemExit(2)
    min_pct[parts[0]] = float(parts[1])

covered: dict[str, int] = {}
total: dict[str, int] = {}
for raw in profile.read_text(encoding="utf-8", errors="replace").splitlines()[1:]:
    parts = raw.split()
    if len(parts) < 3:
        continue
    location, stmts_s, count_s = parts[:3]
    file_path = location.split(":", 1)[0]
    pkg = str(Path(file_path).parent)
    stmts = int(float(stmts_s))
    count = int(float(count_s))
    total[pkg] = total.get(pkg, 0) + stmts
    if count > 0:
        covered[pkg] = covered.get(pkg, 0) + stmts

failures = 0
for pkg in sorted(total):
    if pkg not in min_pct:
        print(f"targets missing package: {pkg}")
        failures += 1
for pkg in sorted(min_pct):
    if pkg not in total:
        print(f"coverage missing package: {pkg}")
        failures += 1
for pkg in sorted(total):
    if pkg not in min_pct:
        continue
    pct = (100.0 * covered.get(pkg, 0) / total[pkg]) if total[pkg] else 0.0
    if pct < min_pct[pkg]:
        print(f"coverage too low: {pkg} ({pct:.2f}% < {min_pct[pkg]}%)")
        failures += 1

if failures:
    print(f"coverage-packages: FAIL ({failures} issue(s))")
    raise SystemExit(1)
print("coverage-packages: PASS")
PY
}

coverage_for_package() {
  local pkg="$1"
  local coverage
  coverage="$(go test -cover "./${pkg}" 2>&1 | grep -oE '[0-9]+\.[0-9]%' | head -1 || echo "0.0%")"
  echo "${coverage:-0.0%}"
}

lines_for_package() {
  local pkg="$1"
  find "./${pkg}" -name "*.go" -not -name "*_test.go" 2>/dev/null | xargs -r wc -l 2>/dev/null | tail -1 | awk '{print $1}' || echo "0"
}

print_dashboard() {
  local red=$'\033[0;31m'
  local green=$'\033[0;32m'
  local yellow=$'\033[1;33m'
  local blue=$'\033[0;34m'
  local nc=$'\033[0m'

  display_coverage() {
    local current="$1"
    local target="$2"
    local coverage_num="${current%%%}"
    local coverage_int="${coverage_num%.*}"
    if [[ "${coverage_int}" -eq 0 ]]; then
      echo -e "${red}${current}${nc}"
    elif [[ "${coverage_int}" -ge "${target}" ]]; then
      echo -e "${green}${current}${nc}"
    elif [[ "${coverage_int}" -ge 50 ]]; then
      echo -e "${yellow}${current}${nc}"
    else
      echo -e "${red}${current}${nc}"
    fi
  }

  echo -e "${blue}======================================"
  echo -e "   TableTheory Test Coverage Dashboard   "
  echo -e "======================================${nc}"
  echo ""
  date
  echo ""
  echo "Running tests..."
  go test -coverprofile=coverage.out ./pkg/... ./internal/... 2>/dev/null || true

  local overall="0.0%"
  if [[ -f coverage.out ]]; then
    overall="$(go tool cover -func=coverage.out 2>/dev/null | grep "total:" | awk '{print $3}' || echo "0.0%")"
  fi
  echo -e "${blue}Overall Coverage:${nc} $(display_coverage "${overall}" 75)"
  echo ""
  echo -e "${blue}Package Coverage Status:${nc}"
  echo "┌─────────────────────┬──────────┬──────────┬────────┬──────────┐"
  echo "│ Package             │ Current  │ Target   │ Lines  │ Status   │"
  echo "├─────────────────────┼──────────┼──────────┼────────┼──────────┤"

  local packages=(pkg/types pkg/marshal pkg/core pkg/errors pkg/index pkg/session pkg/model pkg/transaction pkg/query internal/expr)
  local targets=(80 80 85 90 80 80 76 74 70 70)
  local total_lines=0 tested_lines=0 packages_done=0 packages_partial=0 packages_todo=0

  for i in "${!packages[@]}"; do
    local pkg="${packages[$i]}"
    local target="${targets[$i]}"
    local coverage lines coverage_num coverage_int tested status pkg_display
    coverage="$(coverage_for_package "${pkg}")"
    lines="$(lines_for_package "${pkg}")"
    total_lines=$((total_lines + lines))
    coverage_num="${coverage%%%}"
    coverage_int="${coverage_num%.*}"
    if [[ "${coverage_int}" -gt 0 ]]; then
      tested=$((lines * coverage_int / 100))
      tested_lines=$((tested_lines + tested))
      status="✓"
      if [[ "${coverage_int}" -lt "${target}" ]]; then
        status="⚠"
        packages_partial=$((packages_partial + 1))
      else
        packages_done=$((packages_done + 1))
      fi
    else
      status="✗"
      packages_todo=$((packages_todo + 1))
    fi
    pkg_display="$(printf "%-19s" "${pkg}")"
    echo -n "│ ${pkg_display} │ "
    printf "%8s" "$(display_coverage "${coverage}" "${target}")"
    echo -n " │ "
    printf "%8s" "${target}%"
    echo -n " │ "
    printf "%6s" "${lines}"
    echo -n " │ "
    if [[ "${status}" == "✓" ]]; then
      echo -e "   ${green}${status}${nc}      │"
    elif [[ "${status}" == "⚠" ]]; then
      echo -e "   ${yellow}${status}${nc}      │"
    else
      echo -e "   ${red}${status}${nc}      │"
    fi
  done

  echo "└─────────────────────┴──────────┴──────────┴────────┴──────────┘"
  echo ""
  echo -e "${blue}Summary:${nc}"
  echo "• Total lines of code: ${total_lines}"
  echo "• Estimated tested lines: ${tested_lines}"
  echo "• Untested lines: $((total_lines - tested_lines))"
  echo ""
  echo -e "${blue}Progress Tracking:${nc}"
  echo "• Packages at target: ${packages_done}/10"
  echo "• Packages in progress: ${packages_partial}/10"
  echo "• Packages not started: ${packages_todo}/10"
  echo ""
  echo -e "${blue}Priority Actions:${nc}"
  for i in 0 1 3 2; do
    local pkg="${packages[$i]}"
    local target="${targets[$i]}"
    local coverage coverage_num coverage_int needed
    coverage="$(coverage_for_package "${pkg}")"
    coverage_num="${coverage%%%}"
    coverage_int="${coverage_num%.*}"
    if [[ "${coverage_int}" -lt "${target}" ]]; then
      needed=$((target - coverage_int))
      echo "• ${pkg}: needs ${needed}% more coverage"
    fi
  done
  echo ""
  echo -e "${blue}Test Health:${nc}"
  local failing_tests
  failing_tests="$(go test ./... 2>&1 | grep -E "FAIL:|--- FAIL:" | wc -l || echo "0")"
  if [[ "${failing_tests}" -gt 0 ]]; then
    echo -e "${red}• ${failing_tests} failing tests detected${nc}"
  else
    echo -e "${green}• All tests passing${nc}"
  fi
  echo ""
  echo "Run 'make coverage' to see detailed HTML report"
  echo ""
}

if [[ "${check_threshold_config}" == "true" ]]; then
  run_threshold_config_check
  exit 0
fi

if [[ -n "${package_targets}" ]]; then
  verify_package_targets "${package_targets}"
  exit $?
fi

if [[ "${dashboard}" == "true" ]]; then
  print_dashboard
  exit 0
fi

if [[ "${record_only}" == "true" ]]; then
  case "${language}" in
    go)
      measure_go_coverage "${profile}"
      ;;
    typescript)
      measure_typescript_coverage "${suite:-unit}"
      ;;
    python)
      measure_python_coverage "${suite:-unit}"
      ;;
    all)
      measure_go_coverage "${profile}"
      measure_typescript_coverage "${suite:-unit}"
      measure_python_coverage "${suite:-unit}"
      ;;
  esac
  exit 0
fi

case "${language}" in
  go)
    verify_go_coverage
    ;;
  typescript)
    verify_typescript_coverage
    ;;
  python)
    verify_python_coverage
    ;;
  all)
    verify_go_coverage
    export TS_COVERAGE_THRESHOLD="${TS_COVERAGE_THRESHOLD:-${COVERAGE_THRESHOLD:-${default_threshold}}}"
    export PY_COVERAGE_THRESHOLD="${PY_COVERAGE_THRESHOLD:-${COVERAGE_THRESHOLD:-${default_threshold}}}"
    verify_typescript_coverage
    verify_python_coverage
    ;;
esac
