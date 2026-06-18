#!/usr/bin/env bash
set -euo pipefail

checker="scripts/verify-release-lane-provenance.sh"
base_sha="1111111111111111111111111111111111111111"
head_sha="2222222222222222222222222222222222222222"
repo="theory-cloud/TableTheory"

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

expect_failure_contains \
  "X.Y.Z-rc.N" \
  bash scripts/verify-promotion-release-driver.sh \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3-rc" \
    --dry-run

expect_success_contains \
  "RC Release-As 1.9.3-rc.1" \
  bash scripts/verify-promotion-release-driver.sh \
    --base premain \
    --head staging \
    --title "Promote staging to premain" \
    --body "Release-As: 1.9.3-rc.1" \
    --dry-run

expect_failure_contains \
  "numbered RC-shaped X.Y.Z-rc.N" \
  bash scripts/verify-prerelease-pr-postcondition.sh \
    --expected-version 1.9.3-rc \
    --dry-run

expect_failure_contains \
  "non-numbered-RC tag_name" \
  bash scripts/verify-release-created-postcondition.sh \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc \
    --commit-message "Merge pull request from release-please--branches--premain"

expect_success_contains \
  "published v1.9.3-rc.1" \
  bash scripts/verify-release-created-postcondition.sh \
    --kind prerelease \
    --branch premain \
    --release-created true \
    --tag-name v1.9.3-rc.1 \
    --commit-message "chore(premain): release 1.9.3-rc.1"

if grep -Fq "secrets.RELEASE_PLEASE_TOKEN" .github/workflows/release-hygiene.yml; then
  echo "release-hygiene-policy-test: release hygiene must not expose release-please token"
  exit 1
fi

grep -Fq "persist-credentials: false" .github/workflows/release-hygiene.yml || {
  echo "release-hygiene-policy-test: release hygiene checkouts must disable persisted credentials"
  exit 1
}

grep -Fq "../trusted-release/scripts/verify-promotion-release-driver.sh" .github/workflows/release-hygiene.yml || {
  echo "release-hygiene-policy-test: promotion driver must run from trusted checkout"
  exit 1
}

echo "release-hygiene-policy-test: PASS"
