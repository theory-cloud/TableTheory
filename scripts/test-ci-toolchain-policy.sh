#!/usr/bin/env bash
set -euo pipefail

# ci-toolchain-guard: file-owns-npm-ci-fixtures
#
# The `npm ci` strings below are fixtures this test feeds to the guard, not
# installs it performs, so the guard skips this file. A regression here can
# still never turn the guard vacuous: cases 1 and 2 assert that a lockfile
# install without `--ignore-scripts` is rejected and that the same install with
# the flag passes.
#
# Policy test for the scripts/verify-ci-toolchain.sh guards added by the
# governance-conformance wave (rubric COM-2):
#
#   - every third-party action pinned to a full commit SHA
#   - every `npm ci` runs with `--ignore-scripts`
#   - the verify-only CI shape: a push trigger is a publish side effect only, no
#     workflow executes a gate from the tracked evidence registry, and the record
#     verifier runs on pull requests to staging and nowhere else
#   - the lint toolchain pin lives in the repository, because CI installs no
#     linter in a verify-only repository
#
# Each case builds a throwaway workflow tree, runs the real verifier inside it,
# and asserts both the exit status and the emitted message, so a guard cannot
# silently become vacuous or start rejecting conforming input.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
verifier="${repo_root}/scripts/verify-ci-toolchain.sh"

if [[ ! -f "${verifier}" ]]; then
  echo "ci-toolchain-policy: FAIL (missing ${verifier})"
  exit 1
fi

failures=0

scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

case_dir=""
output=""
status=0

begin_case() {
  case_dir="$(mktemp -d "${scratch}/case.XXXXXX")"
  mkdir -p "${case_dir}/.github/workflows" "${case_dir}/scripts/evidence"
  printf 'module example.com/fixture\n\ngo 1.26\n\ntoolchain go1.26.6\n' >"${case_dir}/go.mod"
  # The guard reads the gate command list from the tracked registry, so every
  # case carries a minimal one. Its content only needs the one gate command the
  # gate-execution cases exercise.
  cat >"${case_dir}/scripts/evidence/validation-gates.json" <<'JSON'
{"schemaVersion": 1, "gates": {"unit": {"kind": "source", "commands": [{"id": "unit", "argv": ["bash", "scripts/verify-unit-tests.sh"]}]}}}
JSON
  # The local lint pin the guard requires of a verify-only repository.
  printf 'GOLANGCI_LINT_VERSION ?= v2.14.0\n' >"${case_dir}/Makefile"
  # Reads GOLANGCI_LINT_VERSION from the Makefile; never restates a version.
  printf '%s\n' 'pin="$(awk -F= /^GOLANGCI_LINT_VERSION/ {print $2} Makefile)"' \
    >"${case_dir}/scripts/ensure-go-lint.sh"
}

write_workflow() {
  cat >"${case_dir}/.github/workflows/$1"
}

run_verifier() {
  set +e
  output="$(cd "${case_dir}" && bash "${verifier}" 2>&1)"
  status=$?
  set -e
}

report_failure() {
  echo "ci-toolchain-policy: FAIL ($1)"
  printf '%s\n' "${output}" | sed 's/^/    /'
  failures=$((failures + 1))
}

expect_status() {
  local expected="$1" label="$2"
  if [[ "${expected}" == PASS && "${status}" -ne 0 ]]; then
    report_failure "${label}: expected PASS, verifier exited ${status}"
  elif [[ "${expected}" == FAIL && "${status}" -eq 0 ]]; then
    report_failure "${label}: expected FAIL, verifier passed"
  fi
}

expect_message() {
  local needle="$1" label="$2"
  grep -Fq -- "${needle}" <<< "${output}" || report_failure "${label}: missing message '${needle}'"
}

# 1. A lockfile install without --ignore-scripts is rejected.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
YAML
run_verifier
expect_status FAIL "npm ci without --ignore-scripts"
expect_message "must pass --ignore-scripts" "npm ci without --ignore-scripts"

# 2. The same install with --ignore-scripts passes.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm --prefix ts ci --ignore-scripts
YAML
run_verifier
expect_status PASS "npm ci --ignore-scripts"

# 3. A mutable action tag is rejected.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
YAML
run_verifier
expect_status FAIL "unpinned action"
expect_message "is not pinned to a full commit SHA" "unpinned action"

# 4. A commit-SHA-pinned action passes.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
YAML
run_verifier
expect_status PASS "SHA-pinned action"

# 4b. A SHA hidden in a YAML comment does not pin the action: the executable ref
#     is the mutable tag, so the guard must reject it.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7 # @0123456789abcdef0123456789abcdef01234567
YAML
run_verifier
expect_status FAIL "comment-only SHA pin"
expect_message "is not pinned to a full commit SHA" "comment-only SHA pin"

# 5. A push trigger on a workflow that is not a publish workflow fails closed.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  push:
    branches: ["staging"]
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status FAIL "gate-class push trigger"
expect_message "declares a push trigger but is not a publish workflow" "gate-class push trigger"

# 6. The documented publish workflows keep their push trigger.
begin_case
write_workflow pages.yml <<'YAML'
name: fixture
on:
  push:
    branches: ["staging"]
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status PASS "publish workflow push trigger"

# 7. Executing a gate command from the registry in CI fails closed.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: bash scripts/verify-unit-tests.sh
YAML
run_verifier
expect_status FAIL "gate executed in CI"
expect_message "executes a recorded gate in CI" "gate executed in CI"

# 8. A record verifier on a promotion pull request fails closed.
begin_case
write_workflow verify.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["premain", "main"]
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: bash scripts/evidence/verify-gate.sh --gate unit
YAML
run_verifier
expect_status FAIL "record verification on a promotion pull request"
expect_message "verifies the evidence record on promotion pull requests" "record verification on a promotion pull request"

# 9. The same verifier on a pull request to staging passes, in both branch-list
#    forms.
begin_case
write_workflow verify.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: [staging]
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: bash scripts/evidence/verify-gate.sh --gate unit
YAML
run_verifier
expect_status PASS "record verification on staging, inline branch list"

begin_case
write_workflow verify.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches:
      - "staging"
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: bash scripts/evidence/verify-gate.sh --gate unit
YAML
run_verifier
expect_status PASS "record verification on staging, block branch list"

# 10. The local lint pin must be tracked, and CI must not install a linter.
begin_case
write_workflow verify.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: bash scripts/evidence/verify-gate.sh --gate unit
YAML
printf 'lint:\n\t@echo lint\n' >"${case_dir}/Makefile"
run_verifier
expect_status FAIL "missing local lint pin"
expect_message "must declare GOLANGCI_LINT_VERSION" "missing local lint pin"

begin_case
write_workflow lint.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["staging"]
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
YAML
run_verifier
expect_status FAIL "CI installs a linter"
expect_message "must not install a linter" "CI installs a linter"

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-toolchain-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-toolchain-policy: PASS (pinning, lockfile-install, verify-only shape and local lint pin guards are live)"
