#!/usr/bin/env bash
set -euo pipefail

# Read-only postcondition for the main Release PR workflow. It verifies that
# pending stable promotion produced a stable release-please PR, not an RC PR,
# and that the PR includes the files that normalize the single manifest.

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-main-release-pr-postcondition.sh --expected-version X.Y.Z [options]

Options:
  --expected-version X.Y.Z  Stable version the main release PR must advertise.
  --forbid-rc-only          Only verify no open main release PR advertises an RC version.
  --repo OWNER/REPO         GitHub repository. Defaults to $GITHUB_REPOSITORY or theory-cloud/TableTheory.
  --base BRANCH            Release PR base branch. Defaults to main.
  --head BRANCH            Release-please PR head branch. Defaults to release-please--branches--main.
  --dry-run                Read-only local validation mode; no GitHub state is changed.
  -h, --help               Show this help.

This command uses only read-only gh queries. It never creates, updates, closes,
merges, tags, publishes, uploads, or deletes anything.
USAGE
}

repo="${GITHUB_REPOSITORY:-theory-cloud/TableTheory}"
base="main"
head="release-please--branches--main"
expected_version="${RELEASE_PR_EXPECTED_VERSION:-}"
forbid_rc_only=0
dry_run=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --expected-version)
      if [[ $# -lt 2 ]]; then
        echo "release-pr-postcondition: FAIL (--expected-version requires a value)" >&2
        exit 2
      fi
      expected_version="$2"
      shift 2
      ;;
    --forbid-rc-only)
      forbid_rc_only=1
      shift
      ;;
    --repo)
      if [[ $# -lt 2 ]]; then
        echo "release-pr-postcondition: FAIL (--repo requires a value)" >&2
        exit 2
      fi
      repo="$2"
      shift 2
      ;;
    --base)
      if [[ $# -lt 2 ]]; then
        echo "release-pr-postcondition: FAIL (--base requires a value)" >&2
        exit 2
      fi
      base="$2"
      shift 2
      ;;
    --head)
      if [[ $# -lt 2 ]]; then
        echo "release-pr-postcondition: FAIL (--head requires a value)" >&2
        exit 2
      fi
      head="$2"
      shift 2
      ;;
    --dry-run)
      dry_run=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "release-pr-postcondition: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

fail() {
  echo "release-pr-postcondition: FAIL ($1)" >&2
  exit 1
}

if [[ "${forbid_rc_only}" -ne 1 && -z "${expected_version}" ]]; then
  fail "missing --expected-version"
fi

if [[ "${forbid_rc_only}" -ne 1 && ! "${expected_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  fail "expected version must be stable X.Y.Z, got ${expected_version}"
fi

if ! command -v gh >/dev/null 2>&1; then
  fail "gh is required"
fi

if ! gh auth status >/dev/null 2>&1; then
  fail "gh is not authenticated"
fi

if [[ "${dry_run}" -eq 1 ]]; then
  echo "release-pr-postcondition: dry-run/read-only mode"
fi

open_prs="$(mktemp)"
candidate_number_file="$(mktemp)"
details="$(mktemp)"
cleanup() {
  rm -f "${open_prs}" "${candidate_number_file}" "${details}"
}
trap cleanup EXIT

gh pr list \
  --repo "${repo}" \
  --base "${base}" \
  --state open \
  --json number,title,headRefName,url >"${open_prs}"

if [[ "${forbid_rc_only}" -eq 1 ]]; then
  python3 - "${open_prs}" "${base}" <<'PY'
import json
import re
import sys
from pathlib import Path

path, base = sys.argv[1:3]
prs = json.loads(Path(path).read_text(encoding="utf-8"))


def fail(message: str) -> None:
    print(f"release-pr-postcondition: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


rc_release_prs = [
    pr
    for pr in prs
    if "release" in pr.get("title", "").lower()
    and re.search(r"\d+\.\d+\.\d+-rc(?:[.\-\w]*)?", pr.get("title", ""))
]
if rc_release_prs:
    details = ", ".join(
        f"#{pr.get('number')} {pr.get('title')} {pr.get('url')}"
        for pr in rc_release_prs
    )
    fail(f"open {base} release PR advertises an RC version: {details}")

print(f"release-pr-postcondition: PASS (no open {base} RC-shaped release PRs)")
PY
  exit 0
fi

python3 - "${open_prs}" "${expected_version}" "${base}" "${head}" >"${candidate_number_file}" <<'PY'
import json
import re
import sys
from pathlib import Path

path, expected_version, base, head = sys.argv[1:5]
prs = json.loads(Path(path).read_text(encoding="utf-8"))
expected_title = f"chore({base}): release {expected_version}"


def fail(message: str) -> None:
    print(f"release-pr-postcondition: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


rc_release_prs = [
    pr
    for pr in prs
    if "release" in pr.get("title", "").lower()
    and re.search(r"\d+\.\d+\.\d+-rc(?:[.\-\w]*)?", pr.get("title", ""))
]
if rc_release_prs:
    details = ", ".join(
        f"#{pr.get('number')} {pr.get('title')} {pr.get('url')}"
        for pr in rc_release_prs
    )
    fail(f"open {base} release PR advertises an RC version: {details}")

head_matches = [pr for pr in prs if pr.get("headRefName") == head]
title_matches = [pr for pr in prs if pr.get("title") == expected_title]
candidates = head_matches or title_matches
if not candidates:
    fail(
        "no open main release PR found for "
        f"{expected_title!r} or head {head!r}"
    )

if len(candidates) > 1:
    details = ", ".join(
        f"#{pr.get('number')} {pr.get('title')} {pr.get('url')}"
        for pr in candidates
    )
    fail(f"multiple candidate release PRs found: {details}")

candidate = candidates[0]
if candidate.get("title") != expected_title:
    fail(
        f"release PR #{candidate.get('number')} title "
        f"{candidate.get('title')!r} != {expected_title!r}"
    )

print(candidate["number"])
print(
    "release-pr-postcondition: candidate "
    f"#{candidate.get('number')} {candidate.get('title')} {candidate.get('url')}",
    file=sys.stderr,
)
PY

candidate_number="$(cat "${candidate_number_file}")"

gh pr view "${candidate_number}" \
  --repo "${repo}" \
  --json number,title,headRefName,baseRefName,url,files >"${details}"

python3 - "${details}" "${expected_version}" "${base}" "${head}" <<'PY'
import json
import sys
from pathlib import Path

path, expected_version, base, head = sys.argv[1:5]
pr = json.loads(Path(path).read_text(encoding="utf-8"))
expected_title = f"chore({base}): release {expected_version}"
required_paths = {
    ".release-please-manifest.json",
    "CHANGELOG.md",
}


def fail(message: str) -> None:
    print(f"release-pr-postcondition: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


if pr.get("baseRefName") != base:
    fail(f"PR #{pr.get('number')} base {pr.get('baseRefName')!r} != {base!r}")

if pr.get("headRefName") != head:
    fail(f"PR #{pr.get('number')} head {pr.get('headRefName')!r} != {head!r}")

if pr.get("title") != expected_title:
    fail(f"PR #{pr.get('number')} title {pr.get('title')!r} != {expected_title!r}")

if "-rc" in pr.get("title", ""):
    fail(f"PR #{pr.get('number')} title is prerelease-shaped: {pr.get('title')}")

paths = {entry.get("path", "") for entry in pr.get("files", [])}
missing = sorted(required_paths - paths)
if missing:
    fail(f"PR #{pr.get('number')} missing single-manifest normalization files: {', '.join(missing)}")

print(
    "release-pr-postcondition: PASS "
    f"(#{pr.get('number')} {expected_title}; single-manifest normalization files present)"
)
PY
