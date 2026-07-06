#!/usr/bin/env bash
set -euo pipefail

# Read-only provenance guard for release-lane pull requests. It rejects branch
# name spoofing by requiring the PR head/base repositories to be this repository.
# By default it also verifies the PR head/base SHAs against live same-repository
# branch refs for the manual-freeze fallback. In the normal v2 path,
# --queue-freshness delegates that live-ref freshness guard to the protected
# branch merge queue while retaining same-repository and release-branch checks.

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-release-lane-provenance.sh [options]

Options:
  --repo OWNER/REPO        Expected repository. Defaults to $GITHUB_REPOSITORY or theory-cloud/TableTheory.
  --base BRANCH           PR base branch. Defaults to $GITHUB_BASE_REF.
  --head BRANCH           PR head branch. Defaults to $GITHUB_HEAD_REF.
  --base-repo OWNER/REPO  PR base repository full name.
  --head-repo OWNER/REPO  PR head repository full name.
  --base-sha SHA          PR base SHA.
  --head-sha SHA          PR head SHA.
  --pr NUMBER             PR number for current lifecycle lookup.
  --pr-state STATE        PR state from the event payload or test fixture.
  --pr-merged BOOL        Whether the PR is known merged from the event payload or test fixture.
  --title TITLE           PR title for generated release-please PR validation.
  --ref REF=SHA           Test-only ref override, e.g. refs/heads/staging=<sha>.
  --queue-freshness       Treat live ref freshness as covered by a required GitHub merge queue.
  -h, --help              Show this help.

This command uses read-only GitHub ref lookups unless --ref supplies all needed
refs. It never creates, updates, merges, tags, publishes, uploads, deletes, or
pushes anything.
USAGE
}

repo="${GITHUB_REPOSITORY:-theory-cloud/TableTheory}"
base="${GITHUB_BASE_REF:-}"
head="${GITHUB_HEAD_REF:-}"
base_repo="${BASE_REPOSITORY:-}"
head_repo="${HEAD_REPOSITORY:-}"
base_sha="${BASE_SHA:-}"
head_sha="${HEAD_SHA:-}"
pr_number="${PR_NUMBER:-}"
pr_state="${PR_STATE:-}"
pr_merged="${PR_MERGED:-}"
title="${PR_TITLE:-}"
queue_freshness=0
ref_overrides=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--repo requires a value)" >&2; exit 2; }
      repo="$2"
      shift 2
      ;;
    --base)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--base requires a value)" >&2; exit 2; }
      base="$2"
      shift 2
      ;;
    --head)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--head requires a value)" >&2; exit 2; }
      head="$2"
      shift 2
      ;;
    --base-repo)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--base-repo requires a value)" >&2; exit 2; }
      base_repo="$2"
      shift 2
      ;;
    --head-repo)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--head-repo requires a value)" >&2; exit 2; }
      head_repo="$2"
      shift 2
      ;;
    --base-sha)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--base-sha requires a value)" >&2; exit 2; }
      base_sha="$2"
      shift 2
      ;;
    --head-sha)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--head-sha requires a value)" >&2; exit 2; }
      head_sha="$2"
      shift 2
      ;;
    --pr)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--pr requires a value)" >&2; exit 2; }
      pr_number="$2"
      shift 2
      ;;
    --pr-state)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--pr-state requires a value)" >&2; exit 2; }
      pr_state="$2"
      shift 2
      ;;
    --pr-merged)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--pr-merged requires a value)" >&2; exit 2; }
      pr_merged="$2"
      shift 2
      ;;
    --title)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--title requires a value)" >&2; exit 2; }
      title="$2"
      shift 2
      ;;
    --ref)
      [[ $# -ge 2 ]] || { echo "release-lane-provenance: FAIL (--ref requires a value)" >&2; exit 2; }
      ref_overrides+=("$2")
      shift 2
      ;;
    --queue-freshness)
      queue_freshness=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "release-lane-provenance: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

fail() {
  echo "release-lane-provenance: FAIL ($1)" >&2
  exit 1
}

require_sha() {
  local label="$1"
  local value="$2"

  if [[ ! "${value}" =~ ^[0-9a-f]{40}$ ]]; then
    fail "${label} must be a 40-character lowercase git SHA, got ${value:-<empty>}"
  fi
}

ref_override_sha() {
  local ref="$1"
  local entry

  for entry in "${ref_overrides[@]}"; do
    if [[ "${entry}" == "${ref}="* ]]; then
      printf '%s\n' "${entry#*=}"
      return 0
    fi
  done
  return 1
}

lookup_ref_sha() {
  local branch="$1"
  local ref="refs/heads/${branch}"
  local override

  if override="$(ref_override_sha "${ref}")"; then
    printf '%s\n' "${override}"
    return 0
  fi

  command -v gh >/dev/null 2>&1 || fail "gh is required for ${ref} lookup"
  gh auth status >/dev/null 2>&1 || fail "gh authentication is required for ${ref} lookup"

  local refs_json
  refs_json="$(gh api "repos/${repo}/git/matching-refs/heads/${branch}")"
  python3 -c '
import json
import sys

want = sys.argv[1]
refs = json.load(sys.stdin)
for entry in refs:
    if entry.get("ref") == want:
        print(entry.get("object", {}).get("sha", ""))
        raise SystemExit(0)
raise SystemExit(1)
' "${ref}" <<<"${refs_json}"
}

refresh_pr_lifecycle() {
  if [[ -z "${pr_number}" ]]; then
    return 0
  fi
  command -v gh >/dev/null 2>&1 || return 0
  gh auth status >/dev/null 2>&1 || return 0

  local pr_json
  if ! pr_json="$(gh pr view "${pr_number}" --repo "${repo}" --json state,mergedAt,closed 2>/dev/null)"; then
    return 0
  fi

  local parsed live_state live_merged
  if ! parsed="$(python3 -c '
import json
import sys

data = json.load(sys.stdin)
state = str(data.get("state") or "")
merged_at = data.get("mergedAt")
print(state, "true" if merged_at else "false")
' <<<"${pr_json}")"; then
    return 0
  fi
  read -r live_state live_merged <<<"${parsed}"
  if [[ -n "${live_state}" ]]; then
    pr_state="${live_state}"
  fi
  if [[ -n "${live_merged}" ]]; then
    pr_merged="${live_merged}"
  fi
}

pr_is_merged() {
  local state="${pr_state,,}"
  local merged="${pr_merged,,}"

  [[ "${state}" == "merged" ]] || [[ "${merged}" == "true" ]]
}

is_premain_release_please_rc_pr() {
  [[ "${base}" == "premain" ]] &&
    [[ "${head}" == "release-please--branches--premain" ]] &&
    [[ "${title}" =~ ^chore\(premain\):[[:space:]]release[[:space:]][0-9]+\.[0-9]+\.[0-9]+-rc(\.[0-9]+)?$ ]]
}

stale_merged_rc_pr_allowed=0
allow_stale_merged_rc_pr() {
  if [[ "${stale_merged_rc_pr_allowed}" -eq 1 ]]; then
    return 0
  fi
  return 1
}

pass_stale_merged_rc_pr() {
  local reason="$1"

  echo "release-lane-provenance: PASS (stale merged premain release-please RC PR; ${reason})"
  exit 0
}

if [[ -z "${repo}" || -z "${base}" || -z "${head}" ]]; then
  fail "missing repository or PR base/head branch"
fi
if [[ -z "${base_repo}" || -z "${head_repo}" ]]; then
  fail "missing PR base/head repository metadata"
fi
if [[ "${base_repo}" != "${repo}" || "${head_repo}" != "${repo}" ]]; then
  fail "release-lane PRs must be same-repository (${head_repo} -> ${base_repo}, expected ${repo})"
fi

require_sha "base SHA" "${base_sha}"
require_sha "head SHA" "${head_sha}"

if [[ "${base}" == "premain" ]]; then
  if [[ "${head}" == "staging" ]]; then
    :
  elif [[ "${head}" == "release-please--branches--premain" ]]; then
    [[ "${title}" =~ ^chore\(premain\):[[:space:]]release[[:space:]][0-9]+\.[0-9]+\.[0-9]+-rc(\.[0-9]+)?$ ]] ||
      fail "premain release-please PR must advertise an RC version (X.Y.Z-rc or X.Y.Z-rc.N), got ${title@Q}"
  else
    fail "premain PR head must be staging or release-please--branches--premain, got ${head@Q}"
  fi
elif [[ "${base}" == "main" ]]; then
  if [[ "${title}" =~ [0-9]+\.[0-9]+\.[0-9]+-rc([.\-[:alnum:]])* ]]; then
    fail "main PR must not advertise an RC version, got ${title@Q}"
  fi
  if [[ "${head}" == "premain" ]]; then
    :
  elif [[ "${head}" == "release-please--branches--main" ]]; then
    [[ "${title}" =~ ^chore\(main\):[[:space:]]release[[:space:]][0-9]+\.[0-9]+\.[0-9]+$ ]] ||
      fail "main release-please PR must advertise a stable version, got ${title@Q}"
  else
    fail "main PR head must be premain or release-please--branches--main, got ${head@Q}"
  fi
else
  fail "unsupported release-hygiene base branch ${base@Q}"
fi

refresh_pr_lifecycle
if is_premain_release_please_rc_pr && pr_is_merged; then
  stale_merged_rc_pr_allowed=1
fi

if [[ "${queue_freshness}" -eq 1 ]]; then
  echo "release-lane-provenance: PASS (${head}@${head_sha} -> ${base}@${base_sha}; live ref freshness covered by merge queue)"
  exit 0
fi

expected_base_sha="$(lookup_ref_sha "${base}")" || {
  if allow_stale_merged_rc_pr; then
    pass_stale_merged_rc_pr "could not resolve live refs/heads/${base} after merge"
  fi
  fail "could not resolve refs/heads/${base}"
}
expected_head_sha="$(lookup_ref_sha "${head}")" || {
  if allow_stale_merged_rc_pr; then
    pass_stale_merged_rc_pr "could not resolve live refs/heads/${head} after merge"
  fi
  fail "could not resolve refs/heads/${head}"
}

require_sha "resolved base ref SHA" "${expected_base_sha}"
require_sha "resolved head ref SHA" "${expected_head_sha}"

if [[ "${base_sha}" != "${expected_base_sha}" ]]; then
  if allow_stale_merged_rc_pr; then
    pass_stale_merged_rc_pr "base ref advanced from ${base_sha} to ${expected_base_sha}"
  fi
  fail "base SHA ${base_sha} does not match same-repository refs/heads/${base} ${expected_base_sha}"
fi
if [[ "${head_sha}" != "${expected_head_sha}" ]]; then
  if allow_stale_merged_rc_pr; then
    pass_stale_merged_rc_pr "head ref advanced from ${head_sha} to ${expected_head_sha}"
  fi
  fail "head SHA ${head_sha} does not match same-repository refs/heads/${head} ${expected_head_sha}"
fi

echo "release-lane-provenance: PASS (${head}@${head_sha} -> ${base}@${base_sha})"
