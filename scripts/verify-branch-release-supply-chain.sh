#!/usr/bin/env bash
set -euo pipefail

# Verifies the TableTheory release-lane scaffolding:
# - one lane: staging -> premain -> main -> staging
# - full gov-infra rubric only on staging PRs and workflow_dispatch
# - lightweight release hygiene on premain/main PRs and workflow_dispatch
# - direct protected-branch PRs with strict live-ref provenance; no merge queue
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

forbid_fixed() {
  local needle="$1"
  local path="$2"
  local message="$3"

  if grep -Fq -- "${needle}" "${path}"; then
    fail "${message}"
  fi
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
  ".release-please-manifest.json"
  "scripts/verify-go-semantic-import-version.sh"
  "scripts/prepare-release-package-versions.py"
  "scripts/verify-release-package-version-assets.py"
  "scripts/verify-release-package-version-build.sh"
  "scripts/verify-main-release-pr-postcondition.sh"
  "scripts/verify-prerelease-pr-postcondition.sh"
  "scripts/verify-premain-pending-stable-repair.sh"
  "scripts/verify-release-lane-provenance.sh"
  "scripts/verify-promotion-release-driver.sh"
  "scripts/verify-release-created-postcondition.sh"
  "scripts/create-stable-release-pr.py"
  "scripts/test-release-pr-tool-policy.sh"
  "scripts/watch-release-cycle.sh"
)

for path in "${required_files[@]}"; do
  require_file "${path}"
done

retired_files=(
  "scripts/prepare-stable-promotion.sh"
  "scripts/sync-post-stable-release-baselines.sh"
  "scripts/release-please-cli"
)

for path in "${retired_files[@]}"; do
  if [[ -e "${path}" ]]; then
    fail "${path} must remain retired; use deterministic stable Release PRs and normal main -> staging PR backmerges"
  fi
  grep_targets=(
    AGENTS.md
    docs/development/planning/theorydb-branch-release-policy.md
    docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md
    docs/development/planning/templates/high-risk-branch-release-policy.template.md
    .github/workflows
  )
  if [[ "${path}" != "scripts/release-please-cli" ]]; then
    grep_targets+=(scripts/test-release-*.sh)
  fi
  if grep -RInF "${path}" "${grep_targets[@]}" >/dev/null 2>&1; then
    fail "${path} must not be referenced by release docs or workflows after retirement"
  fi
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
  forbid_fixed "merge_group:" "${q}" \
    "quality-gates must not use the prohibited merge-queue event"
  require_fixed 'branches: ["staging"]' "${q}" \
    "quality-gates pull_request must target staging only"
  require_fixed "workflow_dispatch:" "${q}" \
    "quality-gates must support workflow_dispatch"
  require_fixed "run: make rubric" "${q}" \
    "quality-gates must run the full rubric"
  require_fixed "fetch-depth: 0" "${q}" \
    "quality-gates must checkout full history for in-flight premain RC repair verification"
  require_fixed "+refs/heads/premain:refs/remotes/origin/premain" "${q}" \
    "quality-gates must fetch origin/premain for in-flight premain RC repair verification"
  require_fixed "+refs/heads/staging:refs/remotes/origin/staging" "${q}" \
    "quality-gates must fetch origin/staging for in-flight premain RC repair verification"
  require_fixed "+refs/heads/main:refs/remotes/origin/main" "${q}" \
    "quality-gates must fetch origin/main for release-cycle state comparison"
  require_fixed 'python-version: "3.12"' "${q}" \
    "quality-gates must cover Python 3.12 before staging merge"
  require_fixed "Run Python 3.12 pre-merge compatibility" "${q}" \
    "quality-gates must name the Python 3.12 pre-merge compatibility step"
  require_fixed "uv --directory py run pytest -q tests/unit" "${q}" \
    "quality-gates Python 3.12 check must include unit tests"
  require_fixed "uv --directory py run pytest -q tests/integration" "${q}" \
    "quality-gates Python 3.12 check must include integration tests"
  require_fixed 'node-version: "20"' "${q}" \
    "quality-gates must cover Node 20 before staging merge"
  require_fixed "Run Node 20 pre-merge compatibility" "${q}" \
    "quality-gates must name the Node 20 pre-merge compatibility step"
  require_fixed "npm --prefix ts run test:integration" "${q}" \
    "quality-gates Node 20 check must include integration tests"
  require_fixed "Restore Python 3.14 for rubric" "${q}" \
    "quality-gates must restore Python 3.14 before make rubric"
  require_fixed "Restore Node 24 for rubric" "${q}" \
    "quality-gates must restore Node 24 before make rubric"
  require_fixed "gov-infra/evidence" "${q}" \
    "quality-gates must upload gov-infra evidence artifacts"
  legacy_evidence_path="hgm""-infra/evidence"
  if grep -Fq "${legacy_evidence_path}" "${q}"; then
    fail "quality-gates must not upload legacy governance evidence artifacts"
  fi
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

require_fixed "mode=premain-rc-repair" "scripts/lib/release-cycle-core.sh" \
  "release-cycle-state must expose verified in-flight premain RC repair mode"
require_fixed "origin/premain does not contain origin/staging" "scripts/lib/release-cycle-core.sh" \
  "release-cycle-state must prove premain contains staging before accepting RC repair into staging"
require_fixed "mode=staging-rc-followup-repair" "scripts/lib/release-cycle-core.sh" \
  "release-cycle-state must expose verified staging RC follow-up repair mode"
require_fixed "scripts/verify-go-semantic-import-version.sh" "scripts/verify-release-cycle-state.sh" \
  "release-cycle-state must verify Go semantic import path against manifest major"
require_fixed "scripts/verify-go-semantic-import-version.sh" "scripts/verify-builds.sh" \
  "build verification must include Go semantic import versioning"

if [[ -f ".github/workflows/release-hygiene.yml" ]]; then
  h=".github/workflows/release-hygiene.yml"
  require_fixed 'branches: ["premain", "main"]' "${h}" \
    "release-hygiene must target premain and main PRs"
  forbid_fixed "merge_group:" "${h}" \
    "release-hygiene must not use the prohibited merge-queue event"
  require_fixed "trusted-release/scripts/verify-release-lane-provenance.sh" "${h}" \
    "release-hygiene must verify release-lane same-repository provenance from trusted scripts"
  forbid_fixed "--queue-freshness" "${h}" \
    "release-hygiene must not bypass strict live-ref freshness"
  require_fixed "github.event.pull_request.base.sha" "${h}" \
    "release-hygiene must check out trusted release scripts from the PR base SHA"
  require_fixed "github.event.pull_request.head.sha" "${h}" \
    "release-hygiene must check out the PR head by exact SHA after provenance verification"
  require_fixed "persist-credentials: false" "${h}" \
    "release-hygiene checkouts must not persist GitHub credentials"
  require_fixed 'HEAD_REPOSITORY: ${{ github.event.pull_request.head.repo.full_name }}' "${h}" \
    "release-hygiene must pass PR head repository metadata to provenance guard"
  require_fixed 'BASE_REPOSITORY: ${{ github.event.pull_request.base.repo.full_name }}' "${h}" \
    "release-hygiene must pass PR base repository metadata to provenance guard"
  require_fixed 'release-please--branches--premain' "scripts/verify-release-lane-provenance.sh" \
    "release-hygiene must explicitly gate generated premain release-please PRs"
  require_fixed 'release-please--branches--main' "scripts/verify-release-lane-provenance.sh" \
    "release-hygiene must explicitly gate generated main release-please PRs"
  require_fixed "scripts/verify-release-cycle-state.sh" "${h}" \
    "release-hygiene must verify release-cycle state"
  require_fixed 'RELEASE_CYCLE_REPO_ROOT: ${{ github.workspace }}/pr' "${h}" \
    "release-hygiene must point trusted release-cycle verifier at the checked-out PR head"
  require_fixed "RELEASE_CYCLE_REPO_ROOT" "scripts/verify-release-cycle-state.sh" \
    "release-cycle-state verifier must support an explicit target repo root"
  require_fixed "scripts/verify-branch-release-supply-chain.sh" "${h}" \
    "release-hygiene must verify release supply-chain scaffolding"
  require_fixed "--forbid-rc-only" "${h}" \
    "release-hygiene must forbid RC-shaped main Release PRs"
  require_fixed "scripts/verify-promotion-release-driver.sh" "${h}" \
    "release-hygiene must verify human promotion release drivers"
  require_fixed "manifest-derived stable Release-As" "${h}" \
    "release-hygiene verifier selector must detect manifest-derived stable promotion driver support"
  require_fixed "resolved promotion-driver supply-chain marker" "${h}" \
    "release-hygiene verifier selector must detect resolved promotion-driver supply-chain support"
  require_fixed "deterministic stable Release PR marker" "${h}" \
    "release-hygiene verifier selector must detect deterministic stable Release PR support"
  require_fixed "direct protected-PR verifier marker" "${h}" \
    "release-hygiene verifier selector must detect direct protected-PR support"
  require_fixed "Go semantic import verifier marker" "${h}" \
    "release-hygiene verifier selector must detect Go semantic import verifier support"
  require_fixed "Go semantic import supply-chain marker" "${h}" \
    "release-hygiene verifier selector must detect Go semantic import supply-chain support"
  require_fixed "github.base_ref == 'premain' && github.head_ref == 'staging'" "${h}" \
    "release-hygiene must run the release driver guard on staging -> premain PRs"
  require_fixed "github.base_ref == 'main' && github.head_ref == 'premain'" "${h}" \
    "release-hygiene must run the release driver guard on premain -> main PRs"
  require_fixed 'bash "${VERIFIER_ROOT}/scripts/verify-promotion-release-driver.sh"' "${h}" \
    "release-hygiene must run the promotion driver from the resolved verifier source"
  require_fixed "promotion-release-driver: using \${VERIFIER_LABEL} verifier source" "${h}" \
    "release-hygiene must log the promotion-driver verifier source"
  require_fixed "github.event_name == 'workflow_dispatch'" "${h}" \
    "release-hygiene must retain read-only manual dispatch validation"
  if grep -Fq "secrets.RELEASE_PLEASE_TOKEN" "${h}"; then
    fail "release-hygiene must not expose release-please token"
  fi
  if grep -Eq 'make rubric|verify-rubric' "${h}"; then
    fail "release-hygiene must not run the full rubric"
  fi
fi

for workflow in \
  ".github/workflows/typescript.yml" \
  ".github/workflows/python.yml" \
  ".github/workflows/unit-cover.yml"; do
  if [[ -f "${workflow}" ]]; then
    require_fixed "paths-ignore:" "${workflow}" \
      "${workflow} pull_request trigger must suppress manifest/changelog-only release-please PR fan-out"
    require_fixed '".release-please-manifest.json"' "${workflow}" \
      "${workflow} must ignore release-please manifest-only PR changes"
    require_fixed '"CHANGELOG.md"' "${workflow}" \
      "${workflow} must ignore release-please changelog-only PR changes"
    if grep -Eq '^[[:space:]]*paths:' "${workflow}"; then
      fail "${workflow} must not replace normal PR validation with a paths allowlist"
    fi
  fi
done

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
  require_regex 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv5\b' "${p}" \
    "prerelease workflow must pin release-please v5 by commit SHA"
  require_regex 'contents:\s*write' "${p}" \
    "prerelease workflow must request contents: write"
  require_regex 'config-file:\s*release-please-config\.premain\.json' "${p}" \
    "prerelease workflow must reference release-please-config.premain.json"
  require_regex 'manifest-file:\s*\.release-please-manifest\.json' "${p}" \
    "prerelease workflow must reference the single release-please manifest"
  require_fixed "scripts/verify-release-cycle-state.sh" "${p}" \
    "prerelease workflow must verify release-cycle state before release-please"
  require_fixed "scripts/verify-branch-version-sync.sh" "${p}" \
    "prerelease workflow must verify branch version sync before release-please"
  require_fixed "scripts/verify-premain-pending-stable-repair.sh" "${p}" \
    "prerelease workflow must classify pending stable repair before release-please"
  require_fixed "pending_stable_repair != 'true'" "${p}" \
    "prerelease workflow must skip prerelease publication during pending stable repair"
  require_fixed "pending stable repair accepted; skipping prerelease publication" "${p}" \
    "prerelease workflow must log pending stable repair no-op"
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
  require_fixed "scripts/prepare-release-package-versions.py --tag-name" "${p}" \
    "prerelease workflow must stamp TS/Py versions from tag_name before packaging"
  require_fixed "scripts/verify-release-package-version-assets.py" "${p}" \
    "prerelease workflow must verify TS/Py asset metadata matches tag_name"
  require_regex 'npm pack --pack-destination \.\./release-assets' "${p}" \
    "prerelease workflow must attach TypeScript npm pack artifact"
  require_regex 'python -m build --outdir \.\./release-assets' "${p}" \
    "prerelease workflow must attach Python wheel/sdist artifacts"
  require_regex 'actions/setup-go@[0-9a-fA-F]{40}' "${p}" \
    "prerelease workflow must set up Go for the CLI release asset build"
  require_fixed './cmd/tabletheory' "${p}" \
    "prerelease workflow must build the tabletheory CLI as a release asset"
  require_fixed 'release-assets/tabletheory-${os}-${arch}' "${p}" \
    "prerelease workflow must attach tabletheory CLI binaries per os/arch"
  require_fixed 'tabletheory-SHA256SUMS.txt' "${p}" \
    "prerelease workflow must publish CLI binary checksums"
  require_regex 'gh release upload' "${p}" \
    "prerelease workflow must upload release assets"
fi

if [[ -f ".github/workflows/prerelease-pr.yml" ]]; then
  pp=".github/workflows/prerelease-pr.yml"
  require_regex 'branches:.*premain' "${pp}" \
    "prerelease-pr workflow must target premain"
  require_regex 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv5\b' "${pp}" \
    "prerelease-pr workflow must pin release-please v5 by commit SHA"
  require_regex 'config-file:\s*release-please-config\.premain\.json' "${pp}" \
    "prerelease-pr workflow must reference release-please-config.premain.json"
  require_regex 'manifest-file:\s*\.release-please-manifest\.json' "${pp}" \
    "prerelease-pr workflow must reference the single release-please manifest"
  require_fixed "scripts/verify-release-cycle-state.sh" "${pp}" \
    "prerelease-pr workflow must verify release-cycle state before release-please"
  require_fixed "scripts/verify-branch-version-sync.sh" "${pp}" \
    "prerelease-pr workflow must verify branch version sync before release-please"
  require_fixed "scripts/verify-premain-pending-stable-repair.sh" "${pp}" \
    "prerelease-pr workflow must classify pending stable repair before release-please"
  require_fixed "pending_stable_repair != 'true'" "${pp}" \
    "prerelease-pr workflow must skip RC PR generation during pending stable repair"
  require_fixed "pending stable repair accepted; skipping RC PR generation" "${pp}" \
    "prerelease-pr workflow must log pending stable repair no-op"
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
  require_regex 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv5\b' "${r}" \
    "release workflow must pin release-please v5 by commit SHA"
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
    "release workflow must gate stable release publication on strict release-cycle state"
  require_regex 'missing tag_name output' "${r}" \
    "release workflow must fail asset/publish steps when tag_name is missing"
  require_regex 'release_created' "${r}" \
    "release workflow must use release-please outputs"
  require_fixed "scripts/verify-release-created-postcondition.sh" "${r}" \
    "release workflow must require release_created/tag_name on generated stable release PR merges"
  require_fixed "generated stable release PR merge" "scripts/verify-release-created-postcondition.sh" \
    "release-created postcondition must classify generated stable release PR merges"
  require_fixed "release-pr.yml must require the deterministic stable Release PR" "scripts/verify-release-created-postcondition.sh" \
    "stable publish postcondition must classify plain premain -> main promotions as PR-generation setup"
  require_fixed "main stable publish workflow observed an RC-shaped release message" "scripts/verify-release-created-postcondition.sh" \
    "stable publish postcondition must forbid RC-shaped main releases"
  require_regex 'pushd ts' "${r}" \
    "release workflow must package TypeScript from ts/"
  require_fixed "scripts/prepare-release-package-versions.py --tag-name" "${r}" \
    "release workflow must stamp TS/Py versions from tag_name before packaging"
  require_fixed "scripts/verify-release-package-version-assets.py" "${r}" \
    "release workflow must verify TS/Py asset metadata matches tag_name"
  require_regex 'npm pack --pack-destination \.\./release-assets' "${r}" \
    "release workflow must attach TypeScript npm pack artifact"
  require_regex 'python -m build --outdir \.\./release-assets' "${r}" \
    "release workflow must attach Python wheel/sdist artifacts"
  require_regex 'actions/setup-go@[0-9a-fA-F]{40}' "${r}" \
    "release workflow must set up Go for the CLI release asset build"
  require_fixed './cmd/tabletheory' "${r}" \
    "release workflow must build the tabletheory CLI as a release asset"
  require_fixed 'release-assets/tabletheory-${os}-${arch}' "${r}" \
    "release workflow must attach tabletheory CLI binaries per os/arch"
  require_fixed 'tabletheory-SHA256SUMS.txt' "${r}" \
    "release workflow must publish CLI binary checksums"
  require_regex 'gh release upload' "${r}" \
    "release workflow must upload release assets"
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
  require_regex 'branches:.*main' "${rp}"     "release-pr workflow must target main"
  if grep -Fq "googleapis/release-please-action" "${rp}"; then
    fail "release-pr workflow must not use release-please-action for stable PR generation"
  fi
  if grep -Fq "scripts/release-please-cli" "${rp}"; then
    fail "release-pr workflow must not depend on release-please CLI for stable PR generation"
  fi
  if grep -Fq -- "--release-as" "${rp}"; then
    fail "release-pr workflow must not rely on release-please release-as for stable PR generation"
  fi
  if grep -Fq ".release-please-manifest.premain.json" "${rp}"; then
    fail "release-pr workflow must not reference retired .release-please-manifest.premain.json"
  fi
  require_fixed "persist-credentials: false" "${rp}"     "release-pr checkout must not persist GitHub credentials"
  require_fixed "scripts/create-stable-release-pr.py" "${rp}"     "release-pr workflow must create the stable Release PR deterministically"
  require_fixed "--head release-please--branches--main" "${rp}"     "release-pr workflow must use the canonical stable release branch"
  require_fixed "scripts/verify-release-cycle-state.sh" "${rp}"     "release-pr workflow must verify release-cycle state before stable PR generation"
  require_fixed "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true" "${rp}"     "release-pr workflow must explicitly allow pending stable promotion verification"
  require_fixed "pending stable promotion accepted for stable Release PR generation" "${rp}"     "release-pr workflow must document pending promotion PR generation"
  require_fixed "steps.cycle.outputs.pending_stable_promotion == 'true'" "${rp}"     "release-pr workflow must gate stable PR creation on pending promotion"
  require_fixed "single-manifest pending stable promotion did not compute a stable release-as" "${rp}"     "release-pr workflow must fail if pending promotion has no stable version"
  require_fixed "steps.version.outputs.release_as" "${rp}"     "release-pr workflow must use the stable version computed from the RC baseline"
  require_fixed "--forbid-rc-only" "${rp}"     "release-pr workflow must forbid RC-shaped main Release PRs"
  require_fixed "scripts/verify-main-release-pr-postcondition.sh" "${rp}"     "release-pr workflow must verify the stable Release PR postcondition"
fi

if [[ -f "scripts/create-stable-release-pr.py" ]]; then
  stable_pr_generator="scripts/create-stable-release-pr.py"
  require_fixed "stable_pull_request_body" "${stable_pr_generator}" \
    "stable Release PR generator must create a Release Please-compatible body"
  require_fixed "autorelease: pending" "${stable_pr_generator}" \
    "stable Release PR generator must apply the Release Please pending label"
  require_fixed "createCommitOnBranch" "${stable_pr_generator}" \
    "stable Release PR generator must create the release commit through GitHub's signed commit path"
  require_fixed "expectedHeadOid" "${stable_pr_generator}" \
    "stable Release PR generator must use GitHub branch-head lease protection"
  require_fixed "GitHub did not report a verified signature" "${stable_pr_generator}" \
    "stable Release PR generator must reject unsigned generated commits"
  require_fixed "create_or_reset_branch" "${stable_pr_generator}" \
    "stable Release PR generator must replace stale generated branch heads before creating the signed commit"
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
    "CHANGELOG.md"; do
    require_fixed "${path}" "${postcondition}" \
      "main Release PR postcondition must require ${path}"
  done
  require_fixed "autorelease: pending" "${postcondition}" \
    "main Release PR postcondition must require the Release Please pending label"
  require_fixed "release-note delimiters" "${postcondition}" \
    "main Release PR postcondition must require a Release Please-compatible body"
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
  require_fixed "X.Y.Z-rc or X.Y.Z-rc.N" "${driver}" \
    "promotion release driver guard must accept release-please first RC and numbered later RC syntax"
fi

if [[ -f "scripts/verify-release-lane-provenance.sh" ]]; then
  provenance="scripts/verify-release-lane-provenance.sh"
  require_fixed "release-lane PRs must be same-repository" "${provenance}" \
    "release-lane provenance guard must reject fork/name-spoofed PRs"
  require_fixed "does not match same-repository refs/heads" "${provenance}" \
    "release-lane provenance guard must require exact live branch SHAs"
  forbid_fixed "--queue-freshness" "${provenance}" \
    "release-lane provenance guard must not expose a merge-queue freshness bypass"
  require_fixed "-rc(\\.[0-9]+)?" "${provenance}" \
    "release-lane provenance guard must accept release-please first RC and numbered later RC PR titles"
fi

if [[ -f "scripts/verify-prerelease-pr-postcondition.sh" ]]; then
  prerelease_postcondition="scripts/verify-prerelease-pr-postcondition.sh"
  require_fixed "release-please--branches--premain" "${prerelease_postcondition}" \
    "prerelease PR postcondition must require the generated premain head branch"
  require_fixed "rc_title_re = re.compile" "${prerelease_postcondition}" \
    "prerelease PR postcondition must require RC-shaped release titles"
  require_fixed "-rc(?:\\.\\d+)?" "${prerelease_postcondition}" \
    "prerelease PR postcondition must accept release-please first RC and numbered later RC version syntax"
  require_fixed ".release-please-manifest.json" "${prerelease_postcondition}" \
    "prerelease PR postcondition must require the single manifest"
fi

if [[ -f "scripts/verify-premain-pending-stable-repair.sh" ]]; then
  premain_repair="scripts/verify-premain-pending-stable-repair.sh"
  require_fixed "origin/main" "${premain_repair}" \
    "premain pending-stable repair verifier must compare against origin/main"
  require_fixed "scripts/create-stable-release-pr.py" "${premain_repair}" \
    "premain pending-stable repair verifier must require deterministic stable PR support"
  require_fixed "premain != main" "${premain_repair}" \
    "premain pending-stable repair verifier must require exact RC equality"
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
    if grep -Fq '"extra-files"' "${cfg}"; then
      fail "${cfg}: must not use extra-files for tag-derived SDK package versions"
    fi
    if grep -Eq 'ts/package(-lock)?\.json|py/src/tabletheory_py/version\.json|\.release-please-manifest\.premain\.json' "${cfg}"; then
      fail "${cfg}: must not list SDK versions or retired prerelease manifest as release-please extra files"
    fi
  done
fi

for cfg in "release-please-config.json" "release-please-config.premain.json"; do
  [[ -f "${cfg}" ]] || continue
  if ! CFG="${cfg}" python3 - <<'PY'
import json
import os
from pathlib import Path

config = json.loads(Path(os.environ["CFG"]).read_text(encoding="utf-8"))
if config.get("packages", {}).get(".") != {}:
    raise SystemExit(1)
PY
  then
    fail "${cfg} must leave SDK/package versioning to tag-derived release-build scripts"
  fi
done

if [[ -f "py/pyproject.toml" ]]; then
  for cfg in "release-please-config.premain.json" "release-please-config.json"; do
    [[ -f "${cfg}" ]] || continue
    if grep -Fq "py/src/tabletheory_py/version.json" "${cfg}"; then
      fail "${cfg}: must not bump py/src/tabletheory_py/version.json through release-please"
    fi
  done
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "branch-release: FAIL (${failures} issue(s))"
  exit 1
fi

echo "branch-release: PASS"
