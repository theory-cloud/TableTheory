#!/usr/bin/env bash
set -euo pipefail

# Verifies that CI runs the repo's rubric via `make rubric` with pinned tooling and uploads key artifacts.
#
# This is intentionally a deterministic, text-based check. It is not a full YAML parser.

wf=".github/workflows/quality-gates.yml"

if [[ ! -f "${wf}" ]]; then
  echo "ci-rubric: FAIL (missing ${wf})"
  exit 1
fi

failures=0

workflow_trigger_block() {
  local trigger="$1"

  awk -v trigger="${trigger}" '
    /^[^[:space:]][^:]*:[[:space:]]*/ {
      if ($0 ~ /^on:[[:space:]]*($|#)/) {
        in_on = 1
      } else if (in_on) {
        in_on = 0
      }
      in_trigger = 0
    }

    in_on && $0 ~ "^[[:space:]]{2}" trigger ":[[:space:]]*" {
      in_trigger = 1
      print
      next
    }

    in_trigger && $0 ~ "^[[:space:]]{2}[^[:space:]][^:]*:" {
      in_trigger = 0
      next
    }

    in_trigger {
      print
    }
  ' "${wf}"
}

pull_request_limited_to_staging() {
  local block="$1"

  if grep -Eq '^[[:space:]]{4}branches:[[:space:]]*\[[[:space:]]*"?staging"?[[:space:]]*\][[:space:]]*(#.*)?$' <<< "${block}"; then
    return 0
  fi

  awk '
    /^[[:space:]]{4}branches:[[:space:]]*($|#)/ {
      in_branches = 1
      found = 1
      next
    }

    in_branches && /^[[:space:]]{4}[^[:space:]][^:]*:/ {
      in_branches = 0
    }

    in_branches && /^[[:space:]]{6}-[[:space:]]*"?staging"?[[:space:]]*(#.*)?$/ {
      count++
      next
    }

    in_branches && /^[[:space:]]{6}-[[:space:]]*/ {
      bad = 1
    }

    END {
      exit (found && count == 1 && !bad) ? 0 : 1
    }
  ' <<< "${block}"
}

grep -Eq '^name:[[:space:]]*Quality Gates [(]10/10 Rubric[)][[:space:]]*$' "${wf}" || {
  echo "ci-rubric: ${wf}: missing expected workflow name"
  failures=$((failures + 1))
}

pull_request_block="$(workflow_trigger_block pull_request)"
if [[ -z "${pull_request_block}" ]]; then
  echo "ci-rubric: ${wf}: missing pull_request trigger"
  failures=$((failures + 1))
elif ! pull_request_limited_to_staging "${pull_request_block}"; then
  echo "ci-rubric: ${wf}: pull_request trigger must target only staging"
  failures=$((failures + 1))
fi

if workflow_trigger_block push | grep -q .; then
  echo "ci-rubric: ${wf}: must not define push trigger"
  failures=$((failures + 1))
fi

# Ensure the workflow uses the repo toolchain pin.
if grep -Eq '^[[:space:]]*uses:[[:space:]]*actions/setup-go@' "${wf}"; then
  grep -Ev '^[[:space:]]*#' "${wf}" | grep -q 'go-version-file: go.mod' || {
    echo "ci-rubric: ${wf}: setup-go must use go-version-file: go.mod"
    failures=$((failures + 1))
  }
else
  echo "ci-rubric: ${wf}: missing actions/setup-go step"
  failures=$((failures + 1))
fi

# Ensure we run the rubric surface as a single command (prevents CI drift when rubric changes).
grep -Eq 'run:\s*make rubric' "${wf}" || {
  echo "ci-rubric: ${wf}: must run 'make rubric'"
  failures=$((failures + 1))
}

# Rubric includes TypeScript checks; require Node setup (pinned).
if grep -Eq '^[[:space:]]*uses:[[:space:]]*actions/setup-node@' "${wf}"; then
  grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq 'node-version:[[:space:]]*["'"'"']?24(\\.x)?["'"'"']?' || {
    echo "ci-rubric: ${wf}: setup-node must pin node-version: 24"
    failures=$((failures + 1))
  }
else
  echo "ci-rubric: ${wf}: missing actions/setup-node step"
  failures=$((failures + 1))
fi

# Rubric includes Python checks; require Python setup (pinned).
if grep -Eq '^[[:space:]]*uses:[[:space:]]*actions/setup-python@' "${wf}"; then
  grep -Ev '^[[:space:]]*#' "${wf}" | grep -Eq 'python-version:[[:space:]]*"?3[.]14([.]x)?"?' || {
    echo "ci-rubric: ${wf}: setup-python must pin python-version: 3.14"
    failures=$((failures + 1))
  }
else
  echo "ci-rubric: ${wf}: missing actions/setup-python step"
  failures=$((failures + 1))
fi

# Ensure pinned tooling installs (no @latest; additional pinning is checked by scripts/verify-ci-toolchain.sh).
if grep -Eq 'go install .*@latest' "${wf}"; then
  echo "ci-rubric: ${wf}: contains @latest; pin versions"
  failures=$((failures + 1))
fi

# Ensure the workflow uploads the key artifacts we rely on for evidence.
grep -q 'coverage_lib.out' "${wf}" || {
  echo "ci-rubric: ${wf}: must upload coverage_lib.out"
  failures=$((failures + 1))
}
grep -q 'ts/coverage' "${wf}" || {
  echo "ci-rubric: ${wf}: must upload ts/coverage"
  failures=$((failures + 1))
}
grep -q 'py/coverage.xml' "${wf}" || {
  echo "ci-rubric: ${wf}: must upload py/coverage.xml"
  failures=$((failures + 1))
}
grep -q 'gosec.sarif' "${wf}" || {
  echo "ci-rubric: ${wf}: must upload gosec.sarif"
  failures=$((failures + 1))
}
grep -q 'gov-infra/evidence' "${wf}" || {
  echo "ci-rubric: ${wf}: must upload gov-infra/evidence"
  failures=$((failures + 1))
}
legacy_evidence_path="hgm""-infra/evidence"
if grep -q "${legacy_evidence_path}" "${wf}"; then
  echo "ci-rubric: ${wf}: must not upload legacy governance evidence"
  failures=$((failures + 1))
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-rubric: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-rubric: enforced"
