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

grep -Fq "bootstrap fallback only supports staging -> premain" .github/workflows/release-hygiene.yml || {
  echo "release-hygiene-policy-test: premain bootstrap fallback must be tightly scoped"
  exit 1
}

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

python3 - <<'PY' >"${tmpdir}/provenance-step.sh"
from pathlib import Path

workflow = Path(".github/workflows/release-hygiene.yml").read_text(encoding="utf-8").splitlines()
in_step = False
for index, line in enumerate(workflow):
    if "name: Verify release-lane same-repository provenance" in line:
        in_step = True
        continue
    if in_step and line.strip() == "run: |":
        run_indent = len(line) - len(line.lstrip())
        block_indent = None
        for block_line in workflow[index + 1:]:
            if block_line.strip() and len(block_line) - len(block_line.lstrip()) <= run_indent:
                raise SystemExit(0)
            if not block_line.strip():
                print()
                continue
            if block_indent is None:
                block_indent = len(block_line) - len(block_line.lstrip())
            print(block_line[block_indent:])
        raise SystemExit(0)
raise SystemExit("could not extract release-lane provenance workflow step")
PY

mkdir -p "${tmpdir}/bin" "${tmpdir}/trusted-release/scripts"
cat >"${tmpdir}/bin/gh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

if [[ "$1" == "auth" && "$2" == "status" ]]; then
  exit 0
fi

if [[ "$1" == "api" ]]; then
  case "$2" in
    repos/theory-cloud/TableTheory/git/matching-refs/heads/premain)
      printf '%s\n' '[{"ref":"refs/heads/premain","object":{"sha":"1111111111111111111111111111111111111111"}}]'
      exit 0
      ;;
    repos/theory-cloud/TableTheory/git/matching-refs/heads/staging)
      printf '%s\n' '[{"ref":"refs/heads/staging","object":{"sha":"2222222222222222222222222222222222222222"}}]'
      exit 0
      ;;
  esac
fi

echo "unexpected gh invocation: $*" >&2
exit 1
SH
chmod +x "${tmpdir}/bin/gh"

workflow_env=(
  env
  "PATH=${tmpdir}/bin:${PATH}"
  GITHUB_REPOSITORY="${repo}"
  BASE_REF=premain
  HEAD_REF=staging
  BASE_REPOSITORY="${repo}"
  HEAD_REPOSITORY="${repo}"
  BASE_SHA="${base_sha}"
  HEAD_SHA="${head_sha}"
  PR_TITLE="Promote staging to premain"
)

expect_success_contains \
  "one-time staging -> premain bootstrap verifier" \
  bash -c 'cd "$1" && shift && "$@"' _ "${tmpdir}" "${workflow_env[@]}" bash "${tmpdir}/provenance-step.sh"

expect_failure_contains \
  "same-repository" \
  bash -c 'cd "$1" && shift && "$@"' _ "${tmpdir}" \
    env \
      "PATH=${tmpdir}/bin:${PATH}" \
      GITHUB_REPOSITORY="${repo}" \
      BASE_REF=premain \
      HEAD_REF=staging \
      BASE_REPOSITORY="${repo}" \
      HEAD_REPOSITORY="attacker/TableTheory" \
      BASE_SHA="${base_sha}" \
      HEAD_SHA="${head_sha}" \
      PR_TITLE="Promote staging to premain" \
      bash "${tmpdir}/provenance-step.sh"

expect_failure_contains \
  "head SHA" \
  bash -c 'cd "$1" && shift && "$@"' _ "${tmpdir}" \
    env \
      "PATH=${tmpdir}/bin:${PATH}" \
      GITHUB_REPOSITORY="${repo}" \
      BASE_REF=premain \
      HEAD_REF=staging \
      BASE_REPOSITORY="${repo}" \
      HEAD_REPOSITORY="${repo}" \
      BASE_SHA="${base_sha}" \
      HEAD_SHA="3333333333333333333333333333333333333333" \
      PR_TITLE="Promote staging to premain" \
      bash "${tmpdir}/provenance-step.sh"

expect_failure_contains \
  "bootstrap fallback only supports staging -> premain" \
  bash -c 'cd "$1" && shift && "$@"' _ "${tmpdir}" \
    env \
      "PATH=${tmpdir}/bin:${PATH}" \
      GITHUB_REPOSITORY="${repo}" \
      BASE_REF=premain \
      HEAD_REF=release-please--branches--premain \
      BASE_REPOSITORY="${repo}" \
      HEAD_REPOSITORY="${repo}" \
      BASE_SHA="${base_sha}" \
      HEAD_SHA="${head_sha}" \
      PR_TITLE="chore(premain): release 1.9.3-rc.1" \
      bash "${tmpdir}/provenance-step.sh"

echo "release-hygiene-policy-test: PASS"
