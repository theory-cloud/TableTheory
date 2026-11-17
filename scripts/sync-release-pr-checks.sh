#!/usr/bin/env bash
set -euo pipefail

# Attach the required release check to a generated release pull request without
# a human action.
#
# release-please opens and updates its pull request with the repository's
# built-in `GITHUB_TOKEN`, and GitHub does not start `pull_request` workflows for
# events caused by that token. The generated release pull request therefore never
# gets its required `hygiene` context on its own, and the only repair used to be
# closing and reopening the pull request. `workflow_dispatch` is the documented
# exception, so this lane dispatches the verifying workflow against the release
# branch and holds it to the exact pull request head.
#
# The lane is deliberately conservative:
#
#   - idempotent: a green context at the head is reused and nothing is
#     re-dispatched; a red context stops the lane instead of being re-run until
#     it happens to pass;
#   - head-checked: every decision is taken against the pull request's current
#     head, and a head that moves during the wait sends the lane back to verify
#     the new head;
#   - draft-locked: the pull request stays a draft until the required contexts
#     are green at one head, and it is marked ready only after a final head read
#     proves that head has not moved.
#
# It never merges, never commits, never rewrites a branch, and never creates a
# credential. Reads and the dispatch use the built-in token.

usage() {
  cat <<'USAGE'
Usage: bash scripts/sync-release-pr-checks.sh --head-branch BRANCH [options]

Options:
  --repo OWNER/REPO        Repository. Defaults to $GITHUB_REPOSITORY or theory-cloud/TableTheory.
  --head-branch BRANCH     Generated release branch this run updated (required).
  --pr NUMBER              Pull request number; discovered from the branch when omitted.
  --config FILE            Release-lane declaration. Defaults to .github/release-lane.json.
  -h, --help               Show this help.

Environment:
  RELEASE_PR_CHECK_TIMEOUT_SECONDS   Overall wait budget (default 2700).
  RELEASE_PR_CHECK_INTERVAL_SECONDS  Poll interval (default 20).
  RELEASE_PR_DISPATCH_ATTEMPTS       Maximum dispatches per head (default 2).
  GH_BIN                             gh binary to use (default gh); a test seam.
USAGE
}

repo="${GITHUB_REPOSITORY:-theory-cloud/TableTheory}"
head_branch=""
pr_number=""
config=".github/release-lane.json"
gh_bin="${GH_BIN:-gh}"
timeout_seconds="${RELEASE_PR_CHECK_TIMEOUT_SECONDS:-2700}"
interval_seconds="${RELEASE_PR_CHECK_INTERVAL_SECONDS:-20}"
max_dispatch_attempts="${RELEASE_PR_DISPATCH_ATTEMPTS:-2}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      [[ $# -ge 2 ]] || { echo "sync-release-pr-checks: FAIL (--repo requires a value)" >&2; exit 2; }
      repo="$2"
      shift 2
      ;;
    --head-branch)
      [[ $# -ge 2 ]] || { echo "sync-release-pr-checks: FAIL (--head-branch requires a value)" >&2; exit 2; }
      head_branch="$2"
      shift 2
      ;;
    --pr)
      [[ $# -ge 2 ]] || { echo "sync-release-pr-checks: FAIL (--pr requires a value)" >&2; exit 2; }
      pr_number="$2"
      shift 2
      ;;
    --config)
      [[ $# -ge 2 ]] || { echo "sync-release-pr-checks: FAIL (--config requires a value)" >&2; exit 2; }
      config="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "sync-release-pr-checks: FAIL (unknown argument: $1)" >&2
      exit 2
      ;;
  esac
done

fail() {
  echo "sync-release-pr-checks: FAIL ($1)" >&2
  exit 1
}

[[ -n "${head_branch}" ]] || fail "--head-branch is required"

# The declaration is the single source of truth: the same file drives the static
# guard in scripts/verify-release-pr-check-lane.sh, so a workflow and its policy
# cannot drift apart.
lane="$(LANE_HEAD_BRANCH="${head_branch}" python3 - "${config}" <<'PY'
import json
import os
import sys

path = sys.argv[1]
head_branch = os.environ["LANE_HEAD_BRANCH"]
try:
    data = json.loads(open(path, encoding="utf-8").read())
except FileNotFoundError:
    print(f"sync-release-pr-checks: FAIL (missing release-lane declaration {path})", file=sys.stderr)
    raise SystemExit(1)

entries = [item for item in data.get("generatedReleasePullRequests") or [] if item.get("head") == head_branch]
if len(entries) != 1:
    print(
        f"sync-release-pr-checks: FAIL ({path} declares {len(entries)} generated release pull requests for {head_branch})",
        file=sys.stderr,
    )
    raise SystemExit(1)
entry = entries[0]
base = str(entry.get("base") or "")
if not base:
    print(f"sync-release-pr-checks: FAIL ({head_branch} entry declares no base)", file=sys.stderr)
    raise SystemExit(1)
required = [item for item in (data.get("requiredContexts") or {}).get(base) or [] if item.get("context")]
if not required:
    print(f"sync-release-pr-checks: FAIL ({path} declares no required context for {base})", file=sys.stderr)
    raise SystemExit(1)
print(f"base={base}")
for item in required:
    print(f"context\t{item['context']}\t{item['workflow']}\t{item.get('job') or ''}")
PY
)" || exit 1

base="$(printf '%s\n' "${lane}" | awk -F= '$1 == "base" { print $2 }')"
mapfile -t context_rows < <(printf '%s\n' "${lane}" | awk -F'\t' '$1 == "context"')

[[ -n "${base}" ]] || fail "could not resolve the release branch for ${head_branch}"
(( ${#context_rows[@]} > 0 )) || fail "could not resolve the required contexts for ${base}"

# One pull-request read, projected to the fields every decision below uses.
read_pr() {
  "${gh_bin}" api "repos/${repo}/pulls/${pr_number}" || fail "cannot read pull request #${pr_number}"
}

project_pr() {
  PR_JSON="${1}" python3 - <<'PY'
import json
import os

data = json.loads(os.environ["PR_JSON"])
head = data.get("head") or {}
print(f"state={str(data.get('state') or '')}")
print(f"draft={'true' if data.get('draft') else 'false'}")
print(f"head_sha={str(head.get('sha') or '')}")
PY
}

discover_pr() {
  local discovered count
  discovered="$("${gh_bin}" pr list \
    --repo "${repo}" \
    --state open \
    --base "${base}" \
    --head "${head_branch}" \
    --json number \
    --jq '.[].number' 2>/dev/null || true)"
  count="$(printf '%s' "${discovered}" | grep -c . || true)"
  case "${count}" in
    1)
      printf '%s\n' "${discovered}" | head -n 1
      ;;
    0)
      return 1
      ;;
    *)
      fail "more than one open ${head_branch} pull request targets ${base}"
      ;;
  esac
}

if [[ -z "${pr_number}" ]]; then
  pr_number="$(discover_pr)" || {
    echo "sync-release-pr-checks: no open ${head_branch} pull request targets ${base}; nothing to check" >&2
    exit 0
  }
fi
[[ "${pr_number}" =~ ^[0-9]+$ ]] || fail "pull request number must be numeric, got ${pr_number}"

ensure_draft() {
  local projected state draft
  projected="$(project_pr "$(read_pr)")"
  state="$(awk -F= '$1 == "state" { print $2 }' <<<"${projected}")"
  case "${state}" in
    open) ;;
    merged|closed)
      echo "sync-release-pr-checks: PR #${pr_number} is ${state}; the lane has nothing left to gate" >&2
      exit 0
      ;;
    *)
      fail "PR #${pr_number} is ${state:-unknown}"
      ;;
  esac
  draft="$(awk -F= '$1 == "draft" { print $2 }' <<<"${projected}")"
  if [[ "${draft}" != "true" ]]; then
    echo "sync-release-pr-checks: draft-locking PR #${pr_number} before the required checks run" >&2
    "${gh_bin}" pr ready "${pr_number}" --repo "${repo}" --undo >/dev/null
    draft="$(project_pr "$(read_pr)" | awk -F= '$1 == "draft" { print $2 }')"
    [[ "${draft}" == "true" ]] || fail "PR #${pr_number} could not be draft-locked"
  fi
}

# Report pass|fail|pending|none for every required context at one head. A
# context with no check run and no in-flight run of its workflow is "none": the
# dispatch is genuinely missing, not red and not queued.
classify_at_head() {
  local head_sha="$1"
  local checks_json runs_json
  checks_json="$("${gh_bin}" api "repos/${repo}/commits/${head_sha}/check-runs?filter=latest&per_page=100")" || {
    echo "unknown"
    return 1
  }
  runs_json="$("${gh_bin}" api "repos/${repo}/actions/runs?head_sha=${head_sha}&per_page=100" 2>/dev/null)" || runs_json='{"workflow_runs":[]}'

  CHECKS_JSON="${checks_json}" RUNS_JSON="${runs_json}" python3 - "${context_rows[@]}" <<'PY'
import json
import os
import sys

checks = json.loads(os.environ["CHECKS_JSON"]).get("check_runs") or []
runs = json.loads(os.environ["RUNS_JSON"]).get("workflow_runs") or []
PASS = {"success", "neutral", "skipped"}
FAIL = {"failure", "cancelled", "timed_out", "action_required", "stale", "startup_failure"}

by_name: dict[str, list[dict]] = {}
for check in checks:
    by_name.setdefault(str(check.get("name") or ""), []).append(check)

by_path: dict[str, list[dict]] = {}
for run in runs:
    by_path.setdefault(str(run.get("path") or ""), []).append(run)

failed: list[str] = []
pending: list[str] = []
missing: list[str] = []

for row in sys.argv[1:]:
    _, context, workflow, _job = row.split("\t")
    records = by_name.get(context) or []
    in_flight = [
        run
        for run in by_path.get(f".github/workflows/{workflow}", [])
        if str(run.get("status") or "") != "completed"
    ]

    if records:
        conclusions = {str(record.get("conclusion") or "") for record in records}
        statuses = {str(record.get("status") or "") for record in records}
        bad = {value for value in conclusions if value in FAIL}
        if bad:
            failed.append(f"{context}={'+'.join(sorted(bad))}")
            continue
        if not statuses <= {"completed"} or conclusions - PASS:
            pending.append(context)
            continue
        continue

    if in_flight:
        pending.append(context)
        continue

    missing.append(context)

if failed:
    print("fail")
    print("failed: " + ", ".join(failed))
elif missing:
    print("none")
    print("missing: " + ", ".join(missing))
elif pending:
    print("pending")
    print("pending: " + ", ".join(pending))
else:
    print("pass")
PY
}

dispatch_contexts() {
  local row context workflow
  for row in "${context_rows[@]}"; do
    IFS=$'\t' read -r _ context workflow _job <<<"${row}"
    echo "sync-release-pr-checks: dispatching ${workflow} for ${head_branch} to verify ${context} on PR #${pr_number}" >&2
    "${gh_bin}" workflow run "${workflow}" \
      --repo "${repo}" \
      --ref "${head_branch}" \
      -f "release_pr_number=${pr_number}" || {
      fail "could not dispatch ${workflow} for ${head_branch}"
    }
  done
}

rollup_reports_context() {
  local rollup_json
  rollup_json="$("${gh_bin}" pr view "${pr_number}" --repo "${repo}" --json statusCheckRollup 2>/dev/null || echo '{"statusCheckRollup":null}')"
  ROLLUP_JSON="${rollup_json}" python3 - "${context_rows[@]}" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["ROLLUP_JSON"]).get("statusCheckRollup")
if data is None:
    print("absent")
    raise SystemExit(0)
names = {str(item.get("name") or item.get("context") or "") for item in data}
wanted = [row.split("\t")[1] for row in sys.argv[1:]]
print("present" if all(name in names for name in wanted) else "absent")
PY
}

deadline=$((SECONDS + timeout_seconds))
dispatch_attempts=0
dispatched_head=""
ensure_draft

while :; do
  projected="$(project_pr "$(read_pr)")"
  state="$(awk -F= '$1 == "state" { print $2 }' <<<"${projected}")"
  case "${state}" in
    open) ;;
    merged|closed)
      echo "sync-release-pr-checks: PR #${pr_number} is ${state}; the lane has nothing left to gate" >&2
      exit 0
      ;;
    *)
      fail "PR #${pr_number} is ${state:-unknown}"
      ;;
  esac
  head_sha="$(awk -F= '$1 == "head_sha" { print $2 }' <<<"${projected}")"
  [[ "${head_sha}" =~ ^[0-9a-f]{40}$ ]] || fail "PR #${pr_number} head is not a commit SHA"

  status_file="$(mktemp)"
  classify_at_head "${head_sha}" >"${status_file}" || true
  status="$(head -n 1 "${status_file}")"
  details="$(tail -n +2 "${status_file}")"
  rm -f "${status_file}"

  if [[ "${head_sha}" != "${dispatched_head}" ]]; then
    dispatch_attempts=0
    dispatched_head="${head_sha}"
  fi

  case "${status}" in
    pass)
      # The head that passed is the head that gets marked ready. A head that
      # moved since the check ran is re-read here and verified again by the next
      # iteration instead of being certified on stale proof.
      if [[ "$(project_pr "$(read_pr)" | awk -F= '$1 == "head_sha" { print $2 }')" != "${head_sha}" ]]; then
        echo "sync-release-pr-checks: head moved while ${head_sha} was green; verifying the new head" >&2
        continue
      fi
      echo "sync-release-pr-checks: required checks are green at ${head_sha}"
      break
      ;;
    fail)
      echo "sync-release-pr-checks: FAIL (required checks already failed at ${head_sha}; the lane never re-dispatches around a red check)" >&2
      [[ -n "${details}" ]] && echo "${details}" >&2
      exit 1
      ;;
    none)
      if (( dispatch_attempts >= max_dispatch_attempts )); then
        echo "sync-release-pr-checks: FAIL (dispatched ${dispatch_attempts} times and still no required check at ${head_sha})" >&2
        [[ -n "${details}" ]] && echo "${details}" >&2
        exit 1
      fi
      dispatch_attempts=$((dispatch_attempts + 1))
      dispatch_contexts
      sleep "${interval_seconds}"
      ;;
    pending)
      if (( SECONDS >= deadline )); then
        echo "sync-release-pr-checks: FAIL (timed out waiting for required checks at ${head_sha})" >&2
        [[ -n "${details}" ]] && echo "${details}" >&2
        exit 1
      fi
      echo "sync-release-pr-checks: waiting for required checks at ${head_sha} (${details:-pending})"
      sleep "${interval_seconds}"
      ;;
    *)
      fail "could not read required check status at ${head_sha}"
      ;;
  esac
done

if [[ "$(project_pr "$(read_pr)" | awk -F= '$1 == "draft" { print $2 }')" != "true" ]]; then
  ensure_draft
fi

echo "sync-release-pr-checks: marking PR #${pr_number} ready at ${head_sha}"
"${gh_bin}" pr ready "${pr_number}" --repo "${repo}" >/dev/null

ready_head="$(project_pr "$(read_pr)" | awk -F= '$1 == "head_sha" { print $2 }')"
if [[ "${ready_head}" != "${head_sha}" ]]; then
  "${gh_bin}" pr ready "${pr_number}" --repo "${repo}" --undo >/dev/null || true
  fail "PR #${pr_number} head changed while marking ready; restored the draft lock"
fi
if [[ "$(project_pr "$(read_pr)" | awk -F= '$1 == "draft" { print $2 }')" != "false" ]]; then
  fail "PR #${pr_number} could not be marked ready"
fi

# The commit-level check runs are what the ruleset evaluates. The pull-request
# rollup is a GitHub-side projection of them, and it can lag or drop a context
# after a close/reopen, so a missing rollup entry is reported rather than
# treated as a missing check.
if [[ "$(rollup_reports_context)" != "present" ]]; then
  echo "sync-release-pr-checks: WARN (PR #${pr_number} rollup does not list every required context yet; the check runs exist at ${head_sha}, which is what the ruleset evaluates)" >&2
fi

echo "sync-release-pr-checks: PASS (PR #${pr_number} ready at ${head_sha} with green required checks)"
