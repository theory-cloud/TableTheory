#!/usr/bin/env bash
set -euo pipefail

# Verifies the TableTheory release-lane scaffolding:
# - one lane: staging -> premain -> main -> staging
# - full Hypergenium rubric only on staging PRs and workflow_dispatch
# - lightweight release hygiene on premain/main PRs
# - premain cuts RCs; main cuts stable releases only
# - no post-stable CI direct-push sync to protected branches
#
# This is a deterministic grep-based check, not a full YAML parser.

failures=0

fail() {
  echo "branch-release: $1"
  failures=$((failures + 1))
}

require_file() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    fail "missing ${path}"
  fi
}

require_fixed() {
  local needle="$1"
  local path="$2"
  local message="$3"

  grep -Fq -- "${needle}" "${path}" || fail "${message}"
}

require_regex() {
  local pattern="$1"
  local path="$2"
  local message="$3"

  grep -Eq -- "${pattern}" "${path}" || fail "${message}"
}

required_files=(
  "AGENTS.md"
  "docs/development/planning/theorydb-branch-release-policy.md"
  "docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md"
  "docs/development/planning/templates/high-risk-branch-release-policy.template.md"
  ".github/workflows/quality-gates.yml"
  ".github/workflows/release-hygiene.yml"
  ".github/workflows/prerelease.yml"
  ".github/workflows/prerelease-pr.yml"
  ".github/workflows/release.yml"
  ".github/workflows/release-pr.yml"
  "release-please-config.premain.json"
  "release-please-config.json"
  ".release-please-manifest.premain.json"
  ".release-please-manifest.json"
  "scripts/sync-post-stable-release-baselines.sh"
  "scripts/verify-main-release-pr-postcondition.sh"
  "scripts/verify-prerelease-pr-postcondition.sh"
  "scripts/verify-promotion-release-driver.sh"
  "scripts/verify-release-created-postcondition.sh"
  "scripts/watch-release-cycle.sh"
)

for path in "${required_files[@]}"; do
  require_file "${path}"
done

if [[ -f "scripts/watch-release-cycle.sh" ]]; then
  require_fixed "isDraft" "scripts/watch-release-cycle.sh" \
    "watch-release-cycle must check GitHub release draft state for --tag"
  require_fixed "publishedAt" "scripts/watch-release-cycle.sh" \
    "watch-release-cycle must check GitHub release publishedAt for --tag"
  require_fixed "git/ref/tags" "scripts/watch-release-cycle.sh" \
    "watch-release-cycle must check git tag refs for --tag"
  require_fixed "untagged-" "scripts/watch-release-cycle.sh" \
    "watch-release-cycle must reject untagged draft release URLs for --tag"
  require_fixed "targetCommitish" "scripts/watch-release-cycle.sh" \
    "watch-release-cycle must compare release targetCommitish with the tag ref"
fi

if [[ -f ".github/workflows/quality-gates.yml" ]]; then
  q=".github/workflows/quality-gates.yml"
  require_fixed "pull_request:" "${q}" \
    "quality-gates must run on pull_request"
  require_fixed 'branches: ["staging"]' "${q}" \
    "quality-gates pull_request must target staging only"
  require_fixed "workflow_dispatch:" "${q}" \
    "quality-gates must support workflow_dispatch"
  require_fixed "run: make rubric" "${q}" \
    "quality-gates must run the full rubric"
  if grep -Eq '^[[:space:]]*push:' "${q}"; then
    fail "quality-gates must not run on push"
  fi
  if grep -Eq 'branches:.*premain|branches:.*main' "${q}"; then
    fail "quality-gates must not target premain/main PRs"
  fi
  if grep -Fq "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION" "${q}"; then
    fail "quality-gates must not carry premain/main pending-promotion logic"
  fi
fi

if [[ -f ".github/workflows/release-hygiene.yml" ]]; then
  h=".github/workflows/release-hygiene.yml"
  require_fixed 'branches: ["premain", "main"]' "${h}" \
    "release-hygiene must target premain and main PRs"
  require_fixed 'head == "staging"' "${h}" \
    "release-hygiene must validate staging -> premain promotion PRs"
  require_fixed 'head == "premain"' "${h}" \
    "release-hygiene must validate premain -> main promotion PRs"
  require_fixed 'release-please--branches--premain' "${h}" \
    "release-hygiene must explicitly gate generated premain release-please PRs"
  require_fixed 'release-please--branches--main' "${h}" \
    "release-hygiene must explicitly gate generated main release-please PRs"
  require_fixed "scripts/verify-release-cycle-state.sh" "${h}" \
    "release-hygiene must verify release-cycle state"
  require_fixed "scripts/verify-branch-release-supply-chain.sh" "${h}" \
    "release-hygiene must verify release supply-chain scaffolding"
  require_fixed "--forbid-rc-only" "${h}" \
    "release-hygiene must forbid RC-shaped main Release PRs"
  require_fixed "scripts/verify-promotion-release-driver.sh" "${h}" \
    "release-hygiene must verify human promotion release drivers"
  require_fixed "github.base_ref == 'premain' && github.head_ref == 'staging'" "${h}" \
    "release-hygiene must run the release driver guard on staging -> premain PRs"
  require_fixed "github.base_ref == 'main' && github.head_ref == 'premain'" "${h}" \
    "release-hygiene must run the release driver guard on premain -> main PRs"
  if grep -Eq 'make rubric|verify-rubric' "${h}"; then
    fail "release-hygiene must not run the full rubric"
  fi
fi

for doc in \
  "AGENTS.md" \
  "docs/development/planning/theorydb-branch-release-policy.md" \
  "docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md" \
  "docs/development/planning/templates/high-risk-branch-release-policy.template.md"; do
  [[ -f "${doc}" ]] || continue
  require_fixed "staging -> premain -> main -> staging" "${doc}" \
    "${doc} must describe the one release lane"
done

require_fixed "tag_name was used by an immutable release" \
  "docs/development/planning/theorydb-branch-release-policy.md" \
  "branch release policy must document immutable release tag-name reuse recovery"
require_fixed "one-time-use" \
  "docs/development/planning/theorydb-branch-release-policy.md" \
  "branch release policy must name one-time-use immutable release versions"
require_fixed "Do not manually recreate tags" "AGENTS.md" \
  "AGENTS.md must prohibit manual tag recreation during release recovery"
require_fixed "Release-As: 1.9.3-rc.1" "AGENTS.md" \
  "AGENTS.md must document the THE-1869 1.9.3 recovery footer"
require_fixed "tag_name was used by an immutable release" \
  "docs/development/planning/templates/high-risk-branch-release-policy.template.md" \
  "high-risk branch policy template must include immutable release reuse recovery"
require_fixed "Release-As:" \
  "docs/development/planning/templates/high-risk-branch-release-policy.template.md" \
  "high-risk branch policy template must document release-please Release-As version recovery"

if grep -RInE 'two release lanes|separate release lanes' \
  AGENTS.md docs/development/planning/theorydb-branch-release-policy.md \
  docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md \
  docs/development/planning/templates/high-risk-branch-release-policy.template.md; then
  fail "docs must not describe TableTheory as having two release lanes"
fi

if [[ -f ".github/workflows/prerelease.yml" ]]; then
  p=".github/workflows/prerelease.yml"
  require_regex 'branches:.*premain' "${p}" \
    "prerelease workflow must target premain"
  require_regex 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' "${p}" \
    "prerelease workflow must pin release-please v4 by commit SHA"
  require_regex 'contents:\s*write' "${p}" \
    "prerelease workflow must request contents: write"
  require_regex 'config-file:\s*release-please-config\.premain\.json' "${p}" \
    "prerelease workflow must reference release-please-config.premain.json"
  require_regex 'manifest-file:\s*\.release-please-manifest\.premain\.json' "${p}" \
    "prerelease workflow must reference .release-please-manifest.premain.json"
  require_fixed "scripts/verify-release-cycle-state.sh" "${p}" \
    "prerelease workflow must verify release-cycle state before release-please"
  require_fixed "scripts/verify-branch-version-sync.sh" "${p}" \
    "prerelease workflow must verify branch version sync before release-please"
  require_regex 'release_created' "${p}" \
    "prerelease workflow must use release-please outputs"
  require_fixed "scripts/verify-release-created-postcondition.sh" "${p}" \
    "prerelease workflow must require release_created/tag_name on generated RC release PR merges"
  require_fixed "generated RC release PR merge" "scripts/verify-release-created-postcondition.sh" \
    "release-created postcondition must classify generated RC release PR merges"
  require_fixed "release_created=false" "scripts/verify-release-created-postcondition.sh" \
    "release-created postcondition must fail release_created=false for generated release PR merges"
  require_fixed "prerelease-pr.yml must require the generated RC release-please PR" "scripts/verify-release-created-postcondition.sh" \
    "prerelease publish postcondition must classify plain staging -> premain merges as PR-generation setup"
  require_regex 'pushd ts' "${p}" \
    "prerelease workflow must package TypeScript from ts/"
  require_regex 'npm pack --pack-destination \.\./release-assets' "${p}" \
    "prerelease workflow must attach TypeScript npm pack artifact"
  require_regex 'python -m build --outdir \.\./release-assets' "${p}" \
    "prerelease workflow must attach Python wheel/sdist artifacts"
  require_regex 'gh release upload' "${p}" \
    "prerelease workflow must upload release assets"
fi

if [[ -f ".github/workflows/prerelease-pr.yml" ]]; then
  pp=".github/workflows/prerelease-pr.yml"
  require_regex 'branches:.*premain' "${pp}" \
    "prerelease-pr workflow must target premain"
  require_regex 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' "${pp}" \
    "prerelease-pr workflow must pin release-please v4 by commit SHA"
  require_regex 'config-file:\s*release-please-config\.premain\.json' "${pp}" \
    "prerelease-pr workflow must reference release-please-config.premain.json"
  require_regex 'manifest-file:\s*\.release-please-manifest\.premain\.json' "${pp}" \
    "prerelease-pr workflow must reference .release-please-manifest.premain.json"
  require_fixed "scripts/verify-release-cycle-state.sh" "${pp}" \
    "prerelease-pr workflow must verify release-cycle state before release-please"
  require_fixed "scripts/verify-branch-version-sync.sh" "${pp}" \
    "prerelease-pr workflow must verify branch version sync before release-please"
  require_regex 'skip-github-release:\s*true' "${pp}" \
    "prerelease-pr workflow must set skip-github-release: true"
  require_fixed "scripts/verify-prerelease-pr-postcondition.sh" "${pp}" \
    "prerelease-pr workflow must require an open generated RC release-please PR"
  require_fixed "No user facing commits" "scripts/verify-prerelease-pr-postcondition.sh" \
    "prerelease PR postcondition must treat release-please no-op as a failed gate"
fi

if [[ -f ".github/workflows/release.yml" ]]; then
  r=".github/workflows/release.yml"
  require_regex 'branches:.*main' "${r}" \
    "release workflow must target main"
  require_regex 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' "${r}" \
    "release workflow must pin release-please v4 by commit SHA"
  require_regex 'contents:\s*write' "${r}" \
    "release workflow must request contents: write"
  require_regex 'config-file:\s*release-please-config\.json' "${r}" \
    "release workflow must reference release-please-config.json"
  require_regex 'manifest-file:\s*\.release-please-manifest\.json' "${r}" \
    "release workflow must reference .release-please-manifest.json"
  require_fixed "scripts/verify-release-cycle-state.sh" "${r}" \
    "release workflow must verify release-cycle state before release-please"
  require_fixed "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true" "${r}" \
    "release workflow must explicitly verify pending stable promotion mode"
  require_fixed "pending_stable_promotion" "${r}" \
    "release workflow must classify pending stable promotion"
  require_fixed "stable release creation is skipped" "${r}" \
    "release workflow must skip stable release creation during pending promotion"
  require_fixed "steps.cycle.outputs.pending_stable_promotion != 'true'" "${r}" \
    "release workflow must gate stable release-please on strict release-cycle state"
  require_regex 'missing tag_name output' "${r}" \
    "release workflow must fail asset/publish steps when tag_name is missing"
  require_regex 'release_created' "${r}" \
    "release workflow must use release-please outputs"
  require_fixed "scripts/verify-release-created-postcondition.sh" "${r}" \
    "release workflow must require release_created/tag_name on generated stable release PR merges"
  require_fixed "generated stable release PR merge" "scripts/verify-release-created-postcondition.sh" \
    "release-created postcondition must classify generated stable release PR merges"
  require_fixed "release-pr.yml must require the generated stable release-please PR" "scripts/verify-release-created-postcondition.sh" \
    "stable publish postcondition must classify plain premain -> main promotions as PR-generation setup"
  require_fixed "main stable publish workflow observed an RC-shaped release message" "scripts/verify-release-created-postcondition.sh" \
    "stable publish postcondition must forbid RC-shaped main releases"
  require_regex 'pushd ts' "${r}" \
    "release workflow must package TypeScript from ts/"
  require_regex 'npm pack --pack-destination \.\./release-assets' "${r}" \
    "release workflow must attach TypeScript npm pack artifact"
  require_regex 'python -m build --outdir \.\./release-assets' "${r}" \
    "release workflow must attach Python wheel/sdist artifacts"
  require_regex 'gh release upload' "${r}" \
    "release workflow must upload release assets"
  if grep -Fq "scripts/sync-post-stable-release-baselines.sh" "${r}"; then
    fail "release workflow must not call post-stable baseline sync"
  fi
  if grep -Fq "SYNC_RELEASE_BASELINE" "${r}"; then
    fail "release workflow must not configure post-stable direct-push sync"
  fi
  if grep -Fq "gh release create" "${r}"; then
    fail "release workflow must not hand-create releases for existing tags"
  fi
  if grep -Fq "inputs.tag_name" "${r}"; then
    fail "release workflow must not expose manual tag-name release mutation"
  fi
fi

if [[ -f ".github/workflows/release-pr.yml" ]]; then
  rp=".github/workflows/release-pr.yml"
  require_regex 'branches:.*main' "${rp}" \
    "release-pr workflow must target main"
  if grep -Fq "googleapis/release-please-action" "${rp}"; then
    fail "release-pr workflow must use pinned release-please CLI, not release-please-action"
  fi
  require_fixed '.release-please-manifest.premain.json' "${rp}" \
    "release-pr paths-ignore must include .release-please-manifest.premain.json and compute RC baseline"
  require_fixed 'RELEASE_PLEASE_CLI_VERSION: "17.3.0"' "${rp}" \
    "release-pr workflow must pin release-please CLI to v17.3.0"
  require_fixed 'npx --yes "release-please@${RELEASE_PLEASE_CLI_VERSION}"' "${rp}" \
    "release-pr workflow must invoke the pinned release-please CLI"
  require_fixed "release-pr" "${rp}" \
    "release-pr workflow must run the release-pr CLI command"
  require_regex '--target-branch[[:space:]]+main' "${rp}" \
    "release-pr workflow must target main through the CLI"
  require_regex '--config-file[[:space:]]+release-please-config\.json' "${rp}" \
    "release-pr workflow must reference release-please-config.json"
  require_regex '--manifest-file[[:space:]]+\.release-please-manifest\.json' "${rp}" \
    "release-pr workflow must reference .release-please-manifest.json"
  require_fixed "scripts/verify-release-cycle-state.sh" "${rp}" \
    "release-pr workflow must verify release-cycle state before release-please"
  require_fixed "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true" "${rp}" \
    "release-pr workflow must explicitly allow pending stable promotion verification"
  require_fixed "pending stable promotion accepted for stable Release PR generation" "${rp}" \
    "release-pr workflow must document pending promotion PR generation"
  require_fixed "PENDING_STABLE_PROMOTION" "${rp}" \
    "release-pr workflow must gate release-please on pending promotion"
  require_fixed "strict stable state; no release-as computed and no release-please call needed" "${rp}" \
    "release-pr workflow must no-op when main is already strict/stable"
  require_fixed "pending stable promotion did not compute a stable release-as" "${rp}" \
    "release-pr workflow must fail if pending promotion has no stable release-as"
  require_fixed "--release-as" "${rp}" \
    "release-pr workflow must pass release-as to the pinned CLI"
  require_fixed "steps.version.outputs.release_as" "${rp}" \
    "release-pr workflow must pass release-as from computed premain RC baseline"
  require_fixed "--forbid-rc-only" "${rp}" \
    "release-pr workflow must forbid RC-shaped main Release PRs"
  require_fixed "scripts/verify-main-release-pr-postcondition.sh" "${rp}" \
    "release-pr workflow must verify the stable Release PR postcondition"
  require_fixed "steps.cycle.outputs.pending_stable_promotion == 'true'" "${rp}" \
    "release-pr workflow must require stable Release PR postcondition whenever pending promotion is detected"
fi

if [[ -f "scripts/verify-main-release-pr-postcondition.sh" ]]; then
  postcondition="scripts/verify-main-release-pr-postcondition.sh"
  require_fixed "expected version must be stable X.Y.Z" "${postcondition}" \
    "main Release PR postcondition must reject RC-valued expected versions"
  require_fixed "advertises an RC version" "${postcondition}" \
    "main Release PR postcondition must reject open RC-valued main release PRs"
  require_fixed "--forbid-rc-only" "${postcondition}" \
    "main Release PR postcondition must support read-only RC-only checks"
  for path in \
    ".release-please-manifest.json" \
    ".release-please-manifest.premain.json" \
    "py/src/theorydb_py/version.json" \
    "ts/package.json" \
    "ts/package-lock.json" \
    "CHANGELOG.md"; do
    require_fixed "${path}" "${postcondition}" \
      "main Release PR postcondition must require ${path}"
  done
fi

if [[ -f "scripts/verify-promotion-release-driver.sh" ]]; then
  driver="scripts/verify-promotion-release-driver.sh"
  require_fixed "staging -> premain promotion lacks a release-eligible conventional" "${driver}" \
    "promotion release driver guard must fail staging -> premain no-driver PRs"
  require_fixed "premain -> main pending stable promotion" "${driver}" \
    "promotion release driver guard must validate premain -> main pending stable promotion"
  require_fixed "Release-As footers must be RC-shaped" "${driver}" \
    "promotion release driver guard must require RC-shaped Release-As for premain promotions"
  require_fixed "Release-As footers must be stable X.Y.Z" "${driver}" \
    "promotion release driver guard must reject RC-shaped Release-As for main promotions"
  require_fixed "No user facing commits" "${driver}" \
    "promotion release driver guard must name release-please no-op as a failed precondition"
  require_fixed "do not use tags, resets, manual manifests" "${driver}" \
    "promotion release driver guard must instruct normal PR-flow remediation"
fi

if [[ -f "scripts/verify-prerelease-pr-postcondition.sh" ]]; then
  prerelease_postcondition="scripts/verify-prerelease-pr-postcondition.sh"
  require_fixed "release-please--branches--premain" "${prerelease_postcondition}" \
    "prerelease PR postcondition must require the generated premain head branch"
  require_fixed "rc_title_re = re.compile" "${prerelease_postcondition}" \
    "prerelease PR postcondition must require RC-shaped release titles"
  require_fixed "-rc(?:\\.\\d+)?" "${prerelease_postcondition}" \
    "prerelease PR postcondition must accept bare and numbered RC version syntax"
  require_fixed ".release-please-manifest.premain.json" "${prerelease_postcondition}" \
    "prerelease PR postcondition must require the prerelease manifest"
fi

if [[ -f "scripts/sync-post-stable-release-baselines.sh" ]]; then
  sync_script="scripts/sync-post-stable-release-baselines.sh"
  require_fixed "DEPRECATED" "${sync_script}" \
    "post-stable sync helper must be marked deprecated"
  require_fixed "dry-run only" "${sync_script}" \
    "post-stable sync helper must be dry-run only"
  require_fixed "normal PR backmerge from main to staging" "${sync_script}" \
    "post-stable sync helper must point operators to main -> staging PR backmerge"
  require_fixed ".release-please-manifest.premain.json" "${sync_script}" \
    "post-stable sync helper must still name the prerelease manifest"
  if grep -Eq 'git[[:space:]].*push|gh[[:space:]]+api[[:space:]].*(git/refs|contents)' "${sync_script}"; then
    fail "deprecated post-stable sync helper must not contain branch mutation commands"
  fi
fi

if grep -RInF "scripts/sync-post-stable-release-baselines.sh" .github/workflows; then
  fail "workflows must not call deprecated post-stable baseline sync"
fi

if grep -RInF "SYNC_RELEASE_BASELINE" .github/workflows; then
  fail "workflows must not configure post-stable baseline sync"
fi

if grep -RInE 'git[[:space:]].*push.*(refs/heads/)?(main|premain|staging)|gh[[:space:]]+api[[:space:]].*(git/refs|contents).*(main|premain|staging)' \
  .github/workflows scripts | grep -v 'verify-release-cycle-state.sh'; then
  fail "release automation contains direct protected branch mutation"
fi

if [[ -f "ts/package.json" ]]; then
  require_regex '"private"\s*:\s*true' "ts/package.json" \
    "ts/package.json must remain private (no npm publishing)"

  for cfg in "release-please-config.premain.json" "release-please-config.json"; do
    [[ -f "${cfg}" ]] || continue
    require_regex '"extra-files"\s*:' "${cfg}" \
      "${cfg}: must define extra-files for multi-language versioning"
    require_regex '"path"\s*:\s*"ts/package\.json"' "${cfg}" \
      "${cfg}: must bump ts/package.json version"
    require_regex '"path"\s*:\s*"ts/package-lock\.json"' "${cfg}" \
      "${cfg}: must bump ts/package-lock.json version"
    require_regex "\\$\\.packages\\[''\\]\\.version" "${cfg}" \
      "${cfg}: must bump ts/package-lock.json packages[''].version"
  done
fi

if [[ -f "release-please-config.json" ]]; then
  if ! python3 - <<'PY'
import json
from pathlib import Path

config = json.loads(Path("release-please-config.json").read_text(encoding="utf-8"))
extra_files = config.get("packages", {}).get(".", {}).get("extra-files", [])

for entry in extra_files:
    if (
        isinstance(entry, dict)
        and entry.get("type") == "json"
        and entry.get("path") == ".release-please-manifest.premain.json"
        and entry.get("jsonpath") == "$['.']"
    ):
        raise SystemExit(0)

raise SystemExit(1)
PY
  then
    fail "release-please-config.json must normalize .release-please-manifest.premain.json through stable release-please"
  fi
fi

if [[ -f "py/pyproject.toml" ]]; then
  for cfg in "release-please-config.premain.json" "release-please-config.json"; do
    [[ -f "${cfg}" ]] || continue
    require_regex '"extra-files"\s*:' "${cfg}" \
      "${cfg}: must define extra-files for multi-language versioning"
    require_regex '"path"\s*:\s*"py/src/theorydb_py/version\.json"' "${cfg}" \
      "${cfg}: must bump py/src/theorydb_py/version.json version"
  done
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "branch-release: FAIL (${failures} issue(s))"
  exit 1
fi

echo "branch-release: PASS"
