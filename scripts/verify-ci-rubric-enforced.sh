#!/usr/bin/env bash
set -euo pipefail

# Verifies that CI runs the repo's rubric via `make rubric` with pinned tooling and uploads key artifacts.
#
# This is intentionally a deterministic, grep-based check. It is not a full YAML parser.

wf=".github/workflows/quality-gates.yml"

if [[ ! -f "${wf}" ]]; then
  echo "ci-rubric: FAIL (missing ${wf})"
  exit 1
fi

failures=0

grep -Eq 'name:\s*Quality Gates' "${wf}" || {
  echo "ci-rubric: ${wf}: missing expected workflow name"
  failures=$((failures + 1))
}

grep -Eq 'pull_request:' "${wf}" || {
  echo "ci-rubric: ${wf}: missing pull_request trigger"
  failures=$((failures + 1))
}

grep -Eq 'push:' "${wf}" || {
  echo "ci-rubric: ${wf}: missing push trigger"
  failures=$((failures + 1))
}

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

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-rubric: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-rubric: enforced"
