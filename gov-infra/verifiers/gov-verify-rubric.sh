#!/usr/bin/env bash
# gov-infra Rubric Verifier (Single Entrypoint)
# Generated from pack version: 816465a1618d
# Project: theorydb (theorydb)
#
# Usage (from repo root; scripts may be non-executable by default):
#   bash gov-infra/verifiers/gov-verify-rubric.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GOV_INFRA="${REPO_ROOT}/gov-infra"
PLANNING_DIR="${GOV_INFRA}/planning"
EVIDENCE_DIR="${GOV_INFRA}/evidence"
REPORT_PATH="${EVIDENCE_DIR}/gov-rubric-report.json"

cd "${REPO_ROOT}"

# Local-only tool install dir (do not commit binaries)
GOV_TOOLS_DIR="${GOV_INFRA}/.tools"
GOV_TOOLS_BIN="${GOV_TOOLS_DIR}/bin"
mkdir -p "${GOV_TOOLS_BIN}"

# Prefer the executable Go toolchain path discovered by the shell before adding
# repo-local tools. This avoids non-executable PATH shadows on developer hosts
# without changing CI's pinned setup-go behavior.
GO_TOOL_BIN="$(command -v go 2>/dev/null || true)"
if [[ -n "${GO_TOOL_BIN}" ]]; then
  GO_TOOL_DIR="$(dirname "${GO_TOOL_BIN}")"
  export PATH="${GO_TOOL_DIR}:${PATH}"
fi
export PATH="${GOV_TOOLS_BIN}:${PATH}"

# Tool pins (derived from repo CI and go.mod)
PIN_GOLANGCI_LINT_VERSION="v2.5.0"
PIN_GOVULNCHECK_VERSION="v1.1.4"
PIN_GOSEC_VERSION="v2.22.11"

# Optional feature flags (opt-in pack features)
FEATURE_OSS_RELEASE="false"

mkdir -p "${EVIDENCE_DIR}"

PREVIOUS_REPORT_PATH=""
if [[ -f "${REPORT_PATH}" ]]; then
  PREVIOUS_REPORT_PATH="$(mktemp)"
  cp "${REPORT_PATH}" "${PREVIOUS_REPORT_PATH}"
fi

cleanup_previous_report() {
  if [[ -n "${PREVIOUS_REPORT_PATH}" && -f "${PREVIOUS_REPORT_PATH}" ]]; then
    rm -f "${PREVIOUS_REPORT_PATH}"
  fi
}
trap cleanup_previous_report EXIT

rm -f \
  "${REPORT_PATH}" \
  "${EVIDENCE_DIR}/"*-output.log \
  "${EVIDENCE_DIR}/DOC-5-parity.log"

REPORT_SCHEMA_URI="theorymcp://namespaces/theorycloud/governance-profiles/theorycloud_governance_profile.v0.1/schemas/gov_rubric_report.v1"
REPORT_SCHEMA_VERSION="gov_rubric_report.v1"
REPORT_TIMESTAMP_CURRENT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PASS_COUNT=0
FAIL_COUNT=0
BLOCKED_COUNT=0

declare -a RESULTS=()

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/\\r}"
  printf '%s' "$s"
}

repo_relative_path() {
  local path="$1"
  if [[ -z "${path}" ]]; then
    printf '%s' ""
    return 0
  fi
  if [[ "${path}" == "${REPO_ROOT}" ]]; then
    printf '%s' "."
    return 0
  fi
  if [[ "${path}" == "${REPO_ROOT}/"* ]]; then
    printf '%s' "${path#"${REPO_ROOT}/"}"
    return 0
  fi
  printf '%s' "${path}"
}

record_result() {
  local id="$1"
  local category="$2"
  local status="$3"
  local message="$4"
  local evidence_path="$5"
  local evidence_path_report
  evidence_path_report="$(repo_relative_path "${evidence_path}")"

  case "$status" in
    PASS) ((PASS_COUNT++)) || true ;;
    FAIL) ((FAIL_COUNT++)) || true ;;
    BLOCKED) ((BLOCKED_COUNT++)) || true ;;
    *)
      echo "Internal error: invalid status '${status}'" >&2
      exit 2
      ;;
  esac

  RESULTS+=(
    "{\"id\":\"$(json_escape "$id")\",\"category\":\"$(json_escape "$category")\",\"status\":\"$(json_escape "$status")\",\"message\":\"$(json_escape "$message")\",\"evidencePath\":\"$(json_escape "$evidence_path_report")\"}"
  )
}

is_unset_token() {
  local v="$1"
  [[ -z "${v//[[:space:]]/}" ]] && return 0
  [[ "$v" == "TODO:"* ]] && return 0
  # Avoid embedding double-curly literal sequences in this verifier; treat any token-like
  # values as unset by checking for a leading '{' character.
  [[ "$v" == "{"* ]] && return 0
  return 1
}

ensure_go_tool_pinned() {
  # Installs a Go tool into ${GOV_TOOLS_BIN} at a pinned version.
  # Returns:
  #   0 success
  #   2 BLOCKED (missing go toolchain / install failed)
  local tool_name="$1"      # e.g., golangci-lint
  local module_at_ver="$2"  # e.g., github.com/...@vX.Y.Z
  local expected_substr="$3" # substring expected in '<tool> --version' output

  local expected_mod_ver="${module_at_ver##*@}"

  tool_matches_pinned_version() {
    local tool="$1"
    local expected_ver="$2"
    local expected_version_substr="$3"

    local tool_path
    tool_path="$(command -v "${tool}" 2>/dev/null || true)"
    [[ -n "${tool_path}" ]] || return 1

    # Prefer Go's module metadata for verification (works even if the tool doesn't
    # embed a human-readable version string).
    if command -v go >/dev/null 2>&1; then
      local installed_mod_ver
      installed_mod_ver="$(go version -m "${tool_path}" 2>/dev/null | awk '$1 == "mod" { print $3; exit }')"
      if [[ -n "${installed_mod_ver}" && "${installed_mod_ver}" == "${expected_ver}" ]]; then
        return 0
      fi
    fi

    # Fallback: best-effort string check.
    if [[ -n "${expected_version_substr}" ]]; then
      if "${tool}" --version 2>/dev/null | grep -Fq "${expected_version_substr}"; then
        return 0
      fi
    fi

    return 1
  }

  golangci_lint_build_is_compatible() {
    local tool="$1"
    local output

    if [[ ! -f "${REPO_ROOT}/.golangci-v2.yml" ]]; then
      return 0
    fi

    if output="$("${tool}" config verify -c "${REPO_ROOT}/.golangci-v2.yml" 2>&1 >/dev/null)"; then
      return 0
    fi

    if grep -Fq "used to build golangci-lint is lower than the targeted Go version" <<<"${output}"; then
      return 1
    fi

    return 0
  }

  if ! command -v go >/dev/null 2>&1; then
    echo "BLOCKED: go toolchain is required to install ${tool_name}" >&2
    return 2
  fi

  # If present and matches expected version, keep.
  if command -v "${tool_name}" >/dev/null 2>&1; then
    if tool_matches_pinned_version "${tool_name}" "${expected_mod_ver}" "${expected_substr}"; then
      if [[ "${tool_name}" != "golangci-lint" ]] || golangci_lint_build_is_compatible "${tool_name}"; then
        return 0
      fi
      echo "Refreshing ${tool_name} to match repo Go target..." >&2
    fi
  fi

  echo "Installing ${tool_name} (${module_at_ver}) into ${GOV_TOOLS_BIN}..." >&2
  if ! GOBIN="${GOV_TOOLS_BIN}" go install "${module_at_ver}"; then
    echo "BLOCKED: failed to install pinned ${tool_name} (${module_at_ver})" >&2
    return 2
  fi

  if ! command -v "${tool_name}" >/dev/null 2>&1; then
    echo "BLOCKED: ${tool_name} not found after installation" >&2
    return 2
  fi

  if ! tool_matches_pinned_version "${tool_name}" "${expected_mod_ver}" "${expected_substr}"; then
    echo "FAIL: installed ${tool_name} does not match expected pinned version (${expected_mod_ver})" >&2
    "${tool_name}" --version 2>/dev/null || true
    go version -m "$(command -v "${tool_name}")" 2>/dev/null || true
    return 1
  fi

  return 0
}

prepare_check_env() {
  local id="$1"
  local cmd="$2"

  # Only attempt installs if this repo is a Go module.
  [[ -f "${REPO_ROOT}/go.mod" ]] || return 0

  case "$id" in
    CON-2|COM-4)
      # Lint and config validation depend on golangci-lint.
      ensure_go_tool_pinned \
        "golangci-lint" \
        "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${PIN_GOLANGCI_LINT_VERSION}" \
        "${PIN_GOLANGCI_LINT_VERSION#v}"
      ;;
    SEC-1)
      # gosec is invoked by scripts/sec-gosec.sh
      ensure_go_tool_pinned \
        "gosec" \
        "github.com/securego/gosec/v2/cmd/gosec@${PIN_GOSEC_VERSION}" \
        "${PIN_GOSEC_VERSION#v}"
      ;;
    SEC-2)
      # govulncheck is invoked by scripts/sec-govulncheck.sh (transitively via sec-dependency-scans.sh)
      ensure_go_tool_pinned \
        "govulncheck" \
        "golang.org/x/vuln/cmd/govulncheck@${PIN_GOVULNCHECK_VERSION}" \
        "govulncheck@${PIN_GOVULNCHECK_VERSION}"
      ;;
    *)
      return 0
      ;;
  esac
}

run_check() {
  local id="$1"
  local category="$2"
  local cmd="$3"

  local output_file="${EVIDENCE_DIR}/${id}-output.log"

  if [[ -z "${cmd//[[:space:]]/}" ]] || [[ "${cmd}" == "TODO:"* ]] || [[ "${cmd}" == "{"* ]]; then
    printf '%s\n' "Verifier command not configured: ${cmd}" >"${output_file}"
    record_result "$id" "$category" "BLOCKED" "Verifier command not configured" "$output_file"
    return 0
  fi

  set +e
  (
    set -euo pipefail
    prepare_check_env "$id" "$cmd"
    eval "${cmd}"
  ) >"${output_file}" 2>&1
  local ec=$?
  set -e

  if [[ $ec -eq 0 ]]; then
    record_result "$id" "$category" "PASS" "Command succeeded" "$output_file"
  elif [[ $ec -eq 2 || $ec -eq 126 || $ec -eq 127 ]]; then
    record_result "$id" "$category" "BLOCKED" "Command reported BLOCKED (exit code ${ec})" "$output_file"
  else
    record_result "$id" "$category" "FAIL" "Command failed with exit code ${ec}" "$output_file"
  fi
}

check_file_exists() {
  local id="$1"
  local category="$2"
  local file_path="$3"

  if [[ -f "${file_path}" ]]; then
    record_result "$id" "$category" "PASS" "File exists" "$file_path"
  else
    record_result "$id" "$category" "FAIL" "Required file missing" "$file_path"
  fi
}

check_threat_controls_parity_gov() {
  local threat_model="${PLANNING_DIR}/theorydb-threat-model.md"
  local controls_matrix="${PLANNING_DIR}/theorydb-controls-matrix.md"
  local evidence_path="${EVIDENCE_DIR}/DOC-5-parity.log"

  if [[ ! -f "${threat_model}" ]] || [[ ! -f "${controls_matrix}" ]]; then
    printf '%s\n' "Threat model or controls matrix missing (gov-infra planning)" >"${evidence_path}"
    echo "threat-parity(gov): BLOCKED (missing threat model or controls matrix)" >&2
    return 2
  fi

  local threats
  threats="$(grep -oE 'THR-[0-9]+' "${threat_model}" | sort -u || true)"
  if [[ -z "${threats}" ]]; then
    printf '%s\n' "No THR-* IDs found in gov-infra threat model" >"${evidence_path}"
    echo "threat-parity(gov): FAIL (no THR-* IDs found in threat model)" >&2
    return 1
  fi

  local mapped
  mapped="$(grep -oE 'THR-[0-9]+' "${controls_matrix}" | sort -u || true)"
  if [[ -z "${mapped}" ]]; then
    printf '%s\n' "No THR-* IDs found in gov-infra controls matrix" >"${evidence_path}"
    echo "threat-parity(gov): FAIL (no THR-* IDs found in controls matrix)" >&2
    return 1
  fi

  local missing_list
  missing_list="$(comm -23 <(printf '%s\n' "${threats}") <(printf '%s\n' "${mapped}") || true)"
  local unknown_list
  unknown_list="$(comm -13 <(printf '%s\n' "${threats}") <(printf '%s\n' "${mapped}") || true)"

  {
    echo "Threat IDs found (gov-infra threat model): ${threats}"
    echo "Threat IDs referenced (gov-infra controls matrix): ${mapped}"
    echo "Missing from controls:${missing_list:- none}"
    echo "Unknown threats referenced in controls:${unknown_list:- none}"
  } >"${evidence_path}"

  if [[ -n "${missing_list}" || -n "${unknown_list}" ]]; then
    echo "threat-parity(gov): FAIL" >&2
    return 1
  fi

  echo "threat-parity(gov): PASS"
  return 0
}

check_threat_controls_parity_full() {
  check_threat_controls_parity_gov
  bash scripts/verify-threat-controls-parity.sh
}

check_security_config_not_diluted() {
  # Security config not diluted (minimal deterministic check).
  # This is intentionally strict and opinionated for this repo:
  # - Require `gosec` linter enabled in .golangci-v2.yml
  # - Allow only exclude G104 (handled by errcheck), and no others.

  local cfg="${REPO_ROOT}/.golangci-v2.yml"
  if [[ ! -f "${cfg}" ]]; then
    echo "BLOCKED: missing .golangci-v2.yml" >&2
    return 2
  fi

  grep -Eq '^[[:space:]]*-[[:space:]]*gosec\b' "${cfg}" || {
    echo "FAIL: .golangci-v2.yml does not enable gosec" >&2
    return 1
  }

  # Extract gosec excludes (YAML is parsed loosely to avoid extra dependencies).
  local excludes
  excludes="$(
    awk '
      function indent(line) { match(line, /^[[:space:]]*/); return RLENGTH }
      function trim(s) { sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s); return s }
      BEGIN {
        in_gosec = 0
        in_excludes = 0
        gosec_indent = -1
        excludes_indent = -1
      }
      /^[[:space:]]*gosec:[[:space:]]*$/ {
        in_gosec = 1
        in_excludes = 0
        gosec_indent = indent($0)
        next
      }
      in_gosec {
        if ($0 ~ /^[[:space:]]*#/ || $0 ~ /^[[:space:]]*$/) next
        if (indent($0) <= gosec_indent && $0 ~ /^[[:space:]]*[A-Za-z0-9_-]+:[[:space:]]*/) {
          in_gosec = 0
          in_excludes = 0
          next
        }
        if ($0 ~ /^[[:space:]]*excludes:[[:space:]]*$/) {
          in_excludes = 1
          excludes_indent = indent($0)
          next
        }
      }
      in_excludes {
        if ($0 ~ /^[[:space:]]*#/ || $0 ~ /^[[:space:]]*$/) next
        if (indent($0) <= excludes_indent && $0 ~ /^[[:space:]]*[A-Za-z0-9_-]+:[[:space:]]*/) {
          in_excludes = 0
          next
        }
        if ($0 ~ /^[[:space:]]*-[[:space:]]*/) {
          line = $0
          sub(/^[[:space:]]*-[[:space:]]*/, "", line)
          sub(/[[:space:]]*#.*/, "", line)
          line = trim(line)
          if (line != "") print line
        }
      }
    ' "${cfg}" | sort -u
  )"

  if [[ -z "${excludes}" ]]; then
    echo "security-config: PASS (no gosec excludes)"
    return 0
  fi

  local allowed="G104"

  local bad=0
  local ex
  while IFS= read -r ex; do
    [[ -z "${ex}" ]] && continue
    if [[ "${ex}" != "${allowed}" ]]; then
      echo "FAIL: gosec exclude not allowed (dilution risk): ${ex}" >&2
      bad=1
    fi
  done <<<"${excludes}"

  if [[ "${bad}" -ne 0 ]]; then
    return 1
  fi

  echo "security-config: PASS (only allowed gosec exclude: ${allowed})"
  return 0
}

check_logging_ops_standards() {
  # COM-6: logging/operational standards enforced (repo-specific; deterministic).
  #
  # This repo is primarily a library. We treat "stdout printing" and "process
  # termination" as high-risk behaviors in library code. We allow limited
  # operational warnings via log.Printf where returning an error isn't possible.

  local standards="${PLANNING_DIR}/theorydb-logging-ops-standards.md"
  if [[ ! -f "${standards}" ]]; then
    echo "BLOCKED: missing logging/ops standards doc: ${standards}" >&2
    return 2
  fi

  local missing=0
  for heading in \
    "## Scope" \
    "## Allowed patterns (library code)" \
    "## Prohibited patterns (library code)" \
    "## Examples vs library code"; do
    if ! grep -Fq "${heading}" "${standards}"; then
      echo "FAIL: logging/ops standards missing required heading: ${heading}" >&2
      missing=1
    fi
  done

  if [[ "${missing}" -ne 0 ]]; then
    return 1
  fi

  if ! command -v git >/dev/null 2>&1; then
    echo "BLOCKED: git is required to deterministically enumerate in-scope files" >&2
    return 2
  fi

  local files
  files="$(
    git ls-files '*.go' \
      | grep -vE '^(examples/|tests/|scripts/|contract-tests/)' \
      | grep -vE '_test[.]go$' \
      || true
  )"

  if [[ -z "${files}" ]]; then
    echo "FAIL: no in-scope Go files found for logging/ops scan" >&2
    return 1
  fi

  # Disallow stdout printing and process-terminating log calls in library code.
  local hits
  hits="$(
    printf '%s\n' "${files}" \
      | xargs -r grep -nE \
        '\\bfmt\\.(Print|Printf|Println)\\b|\\bprint(ln)?\\s*\\(|\\blog\\.Print(ln)?\\b|\\blog\\.(Fatal|Fatalln|Fatalf|Panic|Panicln|Panicf)\\b' \
      || true
  )"

  if [[ -n "${hits}" ]]; then
    echo "FAIL: prohibited logging/printing patterns found in in-scope library code:" >&2
    echo "${hits}" >&2
    return 1
  fi

  echo "logging-ops: PASS"
  return 0
}

check_maintainability_roadmap() {
  # MAI-2: maintainability roadmap current (deterministic doc check).

  local roadmap="${PLANNING_DIR}/theorydb-maintainability-roadmap.md"
  if [[ ! -f "${roadmap}" ]]; then
    echo "BLOCKED: missing maintainability roadmap: ${roadmap}" >&2
    return 2
  fi

  if grep -Fq "$(printf '{%s' '{')" "${roadmap}"; then
    echo "FAIL: maintainability roadmap contains unrendered template token markers" >&2
    return 1
  fi

  local missing=0
  for heading in \
    "## Baseline (start of MAI work)" \
    "## Hotspots" \
    "## Workstreams" \
    "## MAI rubric mapping"; do
    if ! grep -Fq "${heading}" "${roadmap}"; then
      echo "FAIL: maintainability roadmap missing required section: ${heading}" >&2
      missing=1
    fi
  done

  for rubric_id in "MAI-1" "MAI-2" "MAI-3"; do
    if ! grep -Fq "${rubric_id}" "${roadmap}"; then
      echo "FAIL: maintainability roadmap missing required rubric reference: ${rubric_id}" >&2
      missing=1
    fi
  done

  if [[ "${missing}" -ne 0 ]]; then
    return 1
  fi

  echo "maintainability-roadmap: PASS"
  return 0
}

check_gov_doc_integrity() {
  # DOC-4: doc integrity for gov-infra only.
  # Checks:
  # - No unrendered template token markers (double-curly style)
  # - All relative markdown links resolve to existing files (fragment anchors are not validated)

  if ! command -v python3 >/dev/null 2>&1; then
    echo "BLOCKED: python3 required for doc integrity check" >&2
    return 2
  fi

  python3 - <<'PY'
from __future__ import annotations

import os
import re
from pathlib import Path

repo_root = Path(os.getcwd())
gov = repo_root / "gov-infra"

if not gov.exists():
    raise SystemExit("FAIL: gov-infra directory missing")

md_files = []
for p in [gov / "README.md", gov / "AGENTS.md"]:
    if p.exists():
        md_files.append(p)

planning_dir = gov / "planning"
if planning_dir.exists():
    md_files.extend(sorted(planning_dir.glob("*.md")))

if not md_files:
    raise SystemExit("FAIL: no markdown files found under gov-infra")

link_re = re.compile(r"\[[^\]]*\]\(([^)]+)\)")

problems: list[str] = []

for md in md_files:
    text = md.read_text(encoding="utf-8", errors="replace")

    # We treat any double-curly markers as an error because it indicates template tokens
    # were not rendered.
    if "{" + "{" in text:
        problems.append(f"{md.relative_to(repo_root)}: contains unrendered template token markers")

    for m in link_re.finditer(text):
        raw = m.group(1).strip()
        if not raw:
            continue
        if raw.startswith("http://") or raw.startswith("https://") or raw.startswith("mailto:"):
            continue

        # drop query and fragment
        path_part = raw.split("#", 1)[0].split("?", 1)[0].strip()
        if not path_part:
            continue

        # Support absolute repo links: (/path)
        if path_part.startswith("/"):
            target = (repo_root / path_part.lstrip("/")).resolve()
        else:
            target = (md.parent / path_part).resolve()

        try:
            target.relative_to(repo_root.resolve())
        except ValueError:
            continue

        if not target.exists():
            problems.append(
                f"{md.relative_to(repo_root)}: broken link '{raw}' -> '{target.relative_to(repo_root)}'"
            )

if problems:
    print("doc-integrity: FAIL")
    for p in problems:
        print(f"- {p}")
    raise SystemExit(1)

print("doc-integrity: PASS")
PY
}

check_doc_integrity() {
  check_gov_doc_integrity
  bash scripts/verify-doc-integrity.sh
}

echo "=== gov-infra Rubric Verifier ==="
echo "Project: theorydb"
echo "Timestamp: ${REPORT_TIMESTAMP_CURRENT}"
echo ""

# Commands are centralized here so the rubric docs and verifier stay aligned.
CMD_UNIT="bash scripts/verify-unit-tests.sh"
CMD_INTEGRATION="bash scripts/verify-integration-tests.sh"
CMD_COVERAGE="bash scripts/verify-coverage.sh"
CMD_VALIDATION="bash scripts/verify-validation-parity.sh"
CMD_FUZZ="bash scripts/fuzz-smoke.sh"

CMD_FMT="bash scripts/verify-formatting.sh"
CMD_LINT="bash scripts/verify-lint.sh"
CMD_CONTRACT="bash scripts/verify-public-api-contracts.sh && bash scripts/verify-dms-first-workflow.sh && bash scripts/verify-generated-models.sh --check"

CMD_BUILDS="bash scripts/verify-typescript-deps.sh && bash scripts/verify-python-deps.sh && bash scripts/verify-builds.sh"
CMD_TOOLCHAIN="bash scripts/verify-ci-toolchain.sh"
CMD_PLANNING_DOCS="bash scripts/verify-planning-docs.sh"
CMD_LINT_CONFIG="golangci-lint config verify -c .golangci-v2.yml"
CMD_COV_THRESHOLD="bash scripts/verify-coverage-threshold.sh"
CMD_CI_RUBRIC="bash scripts/verify-ci-rubric-enforced.sh"
CMD_DYNAMODB_PIN="bash scripts/verify-dynamodb-local-pin.sh"
CMD_BRANCH_RELEASE="bash scripts/verify-branch-release-supply-chain.sh && bash scripts/verify-branch-version-sync.sh && bash scripts/verify-release-cycle-state.sh"

CMD_SAST="check_security_config_not_diluted && bash scripts/sec-gosec.sh"
CMD_VULN="bash scripts/sec-dependency-scans.sh"
CMD_SUPPLY="go mod verify"
CMD_NO_PANICS="bash scripts/verify-no-panics.sh"
CMD_SAFE_DEFAULTS="bash scripts/verify-safe-defaults.sh"
CMD_EXPR_HARDEN="bash scripts/verify-expression-hardening.sh"
CMD_NET_HYGIENE="bash scripts/verify-network-hygiene.sh"
CMD_ENCRYPTED_TAG="bash scripts/verify-encrypted-tag-implemented.sh"
CMD_LOGGING_OPS="check_logging_ops_standards"

CMD_FILE_BUDGET="bash scripts/verify-file-size.sh"
CMD_MAINTAINABILITY="bash scripts/verify-maintainability-roadmap.sh"
CMD_SINGLETON="bash scripts/verify-query-singleton.sh"

CMD_DOC_INTEGRITY="check_doc_integrity"
CMD_DOC_PARITY="check_threat_controls_parity_full"

# === Quality (QUA) ===
run_check "QUA-1" "Quality" "$CMD_UNIT"
run_check "QUA-2" "Quality" "$CMD_INTEGRATION"
run_check "QUA-3" "Quality" "$CMD_COVERAGE"
run_check "QUA-4" "Quality" "$CMD_VALIDATION"
run_check "QUA-5" "Quality" "$CMD_FUZZ"

# === Consistency (CON) ===
run_check "CON-1" "Consistency" "$CMD_FMT"
run_check "CON-2" "Consistency" "$CMD_LINT"
run_check "CON-3" "Consistency" "$CMD_CONTRACT"

# === Completeness (COM) ===
run_check "COM-1" "Completeness" "$CMD_BUILDS"
run_check "COM-2" "Completeness" "$CMD_TOOLCHAIN"
run_check "COM-3" "Completeness" "$CMD_PLANNING_DOCS"
run_check "COM-4" "Completeness" "$CMD_LINT_CONFIG"
run_check "COM-5" "Completeness" "$CMD_COV_THRESHOLD"
run_check "COM-6" "Completeness" "$CMD_CI_RUBRIC"
run_check "COM-7" "Completeness" "$CMD_DYNAMODB_PIN"
run_check "COM-8" "Completeness" "$CMD_BRANCH_RELEASE"

# === Security (SEC) ===
run_check "SEC-1" "Security" "$CMD_SAST"
run_check "SEC-2" "Security" "$CMD_VULN"
run_check "SEC-3" "Security" "$CMD_SUPPLY"
run_check "SEC-4" "Security" "$CMD_NO_PANICS"
run_check "SEC-5" "Security" "$CMD_SAFE_DEFAULTS"
run_check "SEC-6" "Security" "$CMD_EXPR_HARDEN"
run_check "SEC-7" "Security" "$CMD_NET_HYGIENE"
run_check "SEC-8" "Security" "$CMD_ENCRYPTED_TAG"
run_check "SEC-9" "Security" "$CMD_LOGGING_OPS"

# === Compliance Readiness (CMP) ===
check_file_exists "CMP-1" "Compliance" "${PLANNING_DIR}/theorydb-controls-matrix.md"
check_file_exists "CMP-2" "Compliance" "${PLANNING_DIR}/theorydb-evidence-plan.md"
check_file_exists "CMP-3" "Compliance" "${PLANNING_DIR}/theorydb-threat-model.md"

# === Maintainability (MAI) ===
run_check "MAI-1" "Maintainability" "$CMD_FILE_BUDGET"
run_check "MAI-2" "Maintainability" "$CMD_MAINTAINABILITY"
run_check "MAI-3" "Maintainability" "$CMD_SINGLETON"

# === Docs (DOC) ===
check_file_exists "DOC-1" "Docs" "${PLANNING_DIR}/theorydb-threat-model.md"
check_file_exists "DOC-2" "Docs" "${PLANNING_DIR}/theorydb-evidence-plan.md"
check_file_exists "DOC-3" "Docs" "${PLANNING_DIR}/theorydb-10of10-rubric.md"
run_check "DOC-4" "Docs" "$CMD_DOC_INTEGRITY"
run_check "DOC-5" "Docs" "$CMD_DOC_PARITY"

# === Generate Report ===
RESULTS_JSON=$(printf "%s," "${RESULTS[@]}")
RESULTS_JSON="[${RESULTS_JSON%,}]"

OVERALL_STATUS="PASS"
if [[ ${FAIL_COUNT} -gt 0 ]]; then
  OVERALL_STATUS="FAIL"
elif [[ ${BLOCKED_COUNT} -gt 0 ]]; then
  OVERALL_STATUS="BLOCKED"
fi

write_report() {
  local output_path="$1"
  local report_timestamp="$2"

  cat >"${output_path}" <<EOF
{
  "\$schema": "${REPORT_SCHEMA_URI}",
  "schemaVersion": "${REPORT_SCHEMA_VERSION}",
  "timestamp": "${report_timestamp}",
  "pack": {
    "version": "816465a1618d",
    "digest": "896aed16549928f21626fb4effe9bb6236fc60292a8f50bae8ce77e873ac775b"
  },
  "project": {
    "name": "theorydb",
    "slug": "theorydb"
  },
  "summary": {
    "status": "${OVERALL_STATUS}",
    "pass": ${PASS_COUNT},
    "fail": ${FAIL_COUNT},
    "blocked": ${BLOCKED_COUNT}
  },
  "results": ${RESULTS_JSON}
}
EOF
}

previous_report_timestamp() {
  local report_path="$1"
  python3 - "${report_path}" <<'PY'
import json
import sys
from pathlib import Path

try:
    report = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
except Exception:
    raise SystemExit(1)

timestamp = report.get("timestamp")
if not isinstance(timestamp, str) or not timestamp:
    raise SystemExit(1)
print(timestamp)
PY
}

reports_match_except_timestamp() {
  local previous_path="$1"
  local current_path="$2"

  python3 - "${previous_path}" "${current_path}" <<'PY'
import json
import sys
from pathlib import Path

previous = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
current = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))
previous["timestamp"] = None
current["timestamp"] = None
raise SystemExit(0 if previous == current else 1)
PY
}

NEW_REPORT_PATH="$(mktemp)"
write_report "${NEW_REPORT_PATH}" "${REPORT_TIMESTAMP_CURRENT}"

if [[ -n "${PREVIOUS_REPORT_PATH}" ]] && reports_match_except_timestamp "${PREVIOUS_REPORT_PATH}" "${NEW_REPORT_PATH}"; then
  if previous_timestamp="$(previous_report_timestamp "${PREVIOUS_REPORT_PATH}")"; then
    write_report "${REPORT_PATH}" "${previous_timestamp}"
    rm -f "${NEW_REPORT_PATH}"
  else
    mv "${NEW_REPORT_PATH}" "${REPORT_PATH}"
  fi
else
  mv "${NEW_REPORT_PATH}" "${REPORT_PATH}"
fi

echo "Report written to: ${REPORT_PATH}"
echo "Status: ${OVERALL_STATUS} (pass=${PASS_COUNT} fail=${FAIL_COUNT} blocked=${BLOCKED_COUNT})"

if [[ "${OVERALL_STATUS}" == "PASS" ]]; then
  exit 0
fi
exit 1
