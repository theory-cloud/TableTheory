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
#   - R-F1 trigger parity: a job that runs on a push to `staging`, or on a
#     promotion pull request (base `premain`/`main`), must also be exercised on
#     pull requests to `staging`
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
  mkdir -p "${case_dir}/.github/workflows"
  printf 'module example.com/fixture\n\ngo 1.26\n\ntoolchain go1.26.6\n' >"${case_dir}/go.mod"
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

# 5. R-F1 rule A: push to staging without any staging pull-request coverage.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  push:
    branches: ["staging", "premain", "main"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status FAIL "push-to-staging without staging PR coverage"
expect_message "runs on push to staging but is never exercised" "push-to-staging without staging PR coverage"

# 6. R-F1 rule B: a promotion pull-request gate that staging PRs never run.
begin_case
write_workflow promotion.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["premain", "main"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status FAIL "promotion PR gate missing staging PR coverage"
expect_message "gates pull requests to premain/main but not pull requests to staging" "promotion PR gate missing staging PR coverage"

# 7. Inline branch lists satisfy rule A.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  push:
    branches: [staging]
  pull_request:
    branches: [staging]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status PASS "inline branch lists"

# 8. Block-form branch lists satisfy rule B.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches:
      - "staging"
      - "premain"
      - "main"
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status PASS "block-form branch lists"

# 9. The documented promotion-lane exemption (release-hygiene.yml) is honoured.
begin_case
write_workflow release-hygiene.yml <<'YAML'
name: fixture
on:
  pull_request:
    branches: ["premain", "main"]
jobs:
  hygiene:
    runs-on: ubuntu-latest
    steps:
      - run: echo hygiene
YAML
run_verifier
expect_status PASS "release-hygiene exemption"

# 10. An event declared without a branch filter means "every branch", so a
#     `pull_request: {}` event already covers staging.
begin_case
write_workflow ci.yml <<'YAML'
name: fixture
on:
  push:
    branches: [staging]
  pull_request: {}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
run_verifier
expect_status PASS "unfiltered pull_request event"

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-toolchain-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-toolchain-policy: PASS (pinning, lockfile-install and trigger-parity guards are live)"
