#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
checker="${repo_root}/scripts/verify-release-lane-provenance.sh"
base_sha="1111111111111111111111111111111111111111"
head_sha="2222222222222222222222222222222222222222"
advanced_base_sha="3333333333333333333333333333333333333333"
repo="theory-cloud/TableTheory"
tmpdirs=()

cleanup() {
  local tmpdir
  for tmpdir in "${tmpdirs[@]}"; do
    rm -rf "${tmpdir}"
  done
}
trap cleanup EXIT

expect_success_contains() {
  local expected="$1"
  shift

  local output
  if ! output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected command to succeed"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected output to contain: ${expected}"
    exit 1
  fi
}

expect_failure_contains() {
  local expected="$1"
  shift

  local output
  if output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected command to fail"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "release-hygiene-policy-test: expected failure output to contain: ${expected}"
    exit 1
  fi
}

common_args=(
  --repo "${repo}"
  --base premain
  --head staging
  --base-repo "${repo}"
  --head-repo "${repo}"
  --base-sha "${base_sha}"
  --head-sha "${head_sha}"
  --title "Promote staging to premain"
  --ref "refs/heads/premain=${base_sha}"
  --ref "refs/heads/staging=${head_sha}"
)

expect_success_contains \
  "release-lane-provenance: PASS" \
  bash "${checker}" "${common_args[@]}"

expect_failure_contains \
  "same-repository" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "attacker/TableTheory" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/staging=${head_sha}"

expect_failure_contains \
  "head SHA" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "3333333333333333333333333333333333333333" \
    --title "Promote staging to premain" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/staging=${head_sha}"

expect_failure_contains \
  "base SHA" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/staging=${head_sha}"

expect_success_contains \
  "live ref freshness covered by merge queue" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --queue-freshness \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/staging=4444444444444444444444444444444444444444"

expect_failure_contains \
  "same-repository" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head staging \
    --base-repo "${repo}" \
    --head-repo "attacker/TableTheory" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "Promote staging to premain" \
    --queue-freshness

expect_failure_contains \
  "numbered RC version" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --title "chore(premain): release 1.9.3-rc" \
    --ref "refs/heads/premain=${base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

expect_success_contains \
  "stale merged premain release-please RC PR" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --pr-state MERGED \
    --pr-merged true \
    --title "chore(premain): release 1.10.1-rc.1" \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

expect_failure_contains \
  "base SHA" \
  bash "${checker}" \
    --repo "${repo}" \
    --base premain \
    --head release-please--branches--premain \
    --base-repo "${repo}" \
    --head-repo "${repo}" \
    --base-sha "${base_sha}" \
    --head-sha "${head_sha}" \
    --pr-state OPEN \
    --pr-merged false \
    --title "chore(premain): release 1.10.1-rc.1" \
    --ref "refs/heads/premain=${advanced_base_sha}" \
    --ref "refs/heads/release-please--branches--premain=${head_sha}"

expect_failure_contains \
  "X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3-rc" \
    --dry-run

expect_success_contains \
  "RC Release-As 1.9.3-rc.1" \
  bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3-rc.1" \
    --dry-run

pending_fixture="$(mktemp -d)"
tmpdirs+=("${pending_fixture}")
mkdir -p \
  "${pending_fixture}/.github/workflows" \
  "${pending_fixture}/scripts" \
  "${pending_fixture}/ts" \
  "${pending_fixture}/py/src/tabletheory_py"
cat >"${pending_fixture}/.release-please-manifest.json" <<'JSON'
{".":"1.10.1-rc.1"}
JSON
cat >"${pending_fixture}/ts/package.json" <<'JSON'
{"version":"1.10.0"}
JSON
cat >"${pending_fixture}/ts/package-lock.json" <<'JSON'
{"version":"1.10.0","packages":{"":{"version":"1.10.0"}}}
JSON
cat >"${pending_fixture}/py/src/tabletheory_py/version.json" <<'JSON'
{"version":"1.10.0"}
JSON
touch \
  "${pending_fixture}/.github/workflows/release-hygiene.yml" \
  "${pending_fixture}/scripts/prepare-release-package-versions.py" \
  "${pending_fixture}/scripts/verify-release-package-version-assets.py" \
  "${pending_fixture}/scripts/watch-release-cycle.sh"

run_in_pending_fixture() (
  cd "${pending_fixture}"
  "$@"
)

expect_success_contains \
  "rc=1.10.1-rc.1" \
  env \
    GITHUB_BASE_REF=main \
    GITHUB_HEAD_REF=premain \
    RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true \
    RELEASE_CYCLE_REPO_ROOT="${pending_fixture}" \
    bash "${repo_root}/scripts/verify-release-cycle-state.sh"

expect_failure_contains \
  "must be an absolute path" \
  env \
    RELEASE_CYCLE_REPO_ROOT=relative-target \
    bash "${repo_root}/scripts/verify-release-cycle-state.sh"

expect_success_contains \
  "premain -> main pending stable promotion 1.10.1-rc.1 -> 1.10.1" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1" \
      --commit-message $'fix(security): recover release cycle\n\nRelease-As: 1.10.1-rc.1' \
      --dry-run

expect_failure_contains \
  "premain -> main Release-As footers must be stable X.Y.Z" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.1-rc.1" \
      --dry-run

expect_failure_contains \
  "premain -> main PR title must not be RC-shaped" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote 1.10.1-rc.1 to main" \
      --body "Release-As: 1.10.1" \
      --dry-run

expect_failure_contains \
  "pending RC base 1.10.1" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "Release-As: 1.10.2" \
      --dry-run

expect_failure_contains \
  "requires a stable Release-As footer" \
  run_in_pending_fixture \
    bash "${repo_root}/scripts/verify-promotion-release-driver.sh" \
      --base main \
      --head premain \
      --title "Promote premain to main" \
      --body "" \
      --dry-run

expect_failure_contains \
  "numbered RC-shaped X.Y.Z-rc.N" \
  bash "${repo_root}/scripts/verify-prerelease-pr-postcondition.sh" \
    --expected-version 1.9.3-rc \
    --dry-run

expect_failure_contains \
  "non-numbered-RC tag_name" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc \
    --commit-message "Merge pull request from release-please--branches--premain"

expect_success_contains \
  "published v1.9.3-rc.1" \
  bash "${repo_root}/scripts/verify-release-created-postcondition.sh" \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc.1 \
    --commit-message "chore(premain): release 1.9.3-rc.1"

if grep -Fq "secrets.RELEASE_PLEASE_TOKEN" "${repo_root}/.github/workflows/release-hygiene.yml"; then
  echo "release-hygiene-policy-test: release hygiene must not expose release-please token"
  exit 1
fi

grep -Fq "persist-credentials: false" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene checkouts must disable persisted credentials"
  exit 1
}

grep -Fq "merge_group:" "${repo_root}/.github/workflows/quality-gates.yml" || {
  echo "release-hygiene-policy-test: quality gates must support merge_group for staging queue"
  exit 1
}

grep -Fq "merge_group:" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must support merge_group for release queue"
  exit 1
}

grep -Fq -- "--queue-freshness" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene PR provenance must delegate live ref freshness to merge queue"
  exit 1
}

grep -Fq "pending stable promotion accepted on queued main merge group" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release hygiene must allow queued main pending stable promotion"
  exit 1
}

grep -Fq "../trusted-release/scripts/verify-promotion-release-driver.sh" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: promotion driver must run from trusted checkout"
  exit 1
}

grep -Fq "Verify release-hygiene bootstrap scope" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: main bootstrap PRs must have a scoped hygiene guard"
  exit 1
}

grep -Fq "fix/release-hygiene-main-bootstrap-" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: main bootstrap guard must be branch-scoped"
  exit 1
}

grep -Fq "pulls/\${PR_NUMBER}/files" "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: main bootstrap guard must inspect PR changed files"
  exit 1
}

bootstrap_scope_section="$(sed -n '/Verify release-hygiene bootstrap scope/,/Verify release-lane same-repository provenance/p' "${repo_root}/.github/workflows/release-hygiene.yml")"
grep -Fq "scripts/verify-release-cycle-state.sh" <<<"${bootstrap_scope_section}" || {
  echo "release-hygiene-policy-test: main bootstrap guard must allow verify-release-cycle-state bootstrap repairs"
  exit 1
}

grep -Fq 'RELEASE_CYCLE_REPO_ROOT: ${{ github.workspace }}/pr' "${repo_root}/.github/workflows/release-hygiene.yml" || {
  echo "release-hygiene-policy-test: release cycle state must validate the checked-out PR head"
  exit 1
}

echo "release-hygiene-policy-test: PASS"
