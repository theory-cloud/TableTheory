#!/usr/bin/env bash
set -euo pipefail

# Read-only postcondition for the premain Release PR workflow. It verifies that
# release-please opened an RC-shaped PR targeting premain after a promotion push.

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-prerelease-pr-postcondition.sh [options]

Options:
  --repo OWNER/REPO         GitHub repository. Defaults to $GITHUB_REPOSITORY or theory-cloud/TableTheory.
  --base BRANCH            Release PR base branch. Defaults to premain.
  --head BRANCH            Release-please PR head branch. Defaults to release-please--branches--premain.
  --expected-version VER   Optional numbered RC version the PR must advertise, e.g. 1.9.4-rc.1.
  --dry-run                Read-only local validation mode; no GitHub state is changed.
  -h, --help               Show this help.

This command uses only read-only gh queries. It never creates, updates, closes,
merges, tags, publishes, uploads, or deletes anything.
USAGE
}

repo="${GITHUB_REPOSITORY:-theory-cloud/TableTheory}"
base="premain"
head="release-please--branches--premain"
expected_version="${PRERELEASE_PR_EXPECTED_VERSION:-}"
dry_run=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--repo requires a value)" >&2
        exit 2
      fi
      repo="$2"
      shift 2
      ;;
    --base)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--base requires a value)" >&2
        exit 2
      fi
      base="$2"
      shift 2
      ;;
    --head)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--head requires a value)" >&2
        exit 2
      fi
      head="$2"
      shift 2
      ;;
    --expected-version)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--expected-version requires a value)" >&2
        exit 2
      fi
      expected_version="$2"
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
      echo "prerelease-pr-postcondition: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

fail() {
  echo "prerelease-pr-postcondition: FAIL ($1)" >&2
  exit 1
}

if [[ -n "${expected_version}" && ! "${expected_version}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+$ ]]; then
  fail "expected version must be numbered RC-shaped X.Y.Z-rc.N, got ${expected_version}"
fi

if ! command -v gh >/dev/null 2>&1; then
  fail "gh is required"
fi

if ! gh auth status >/dev/null 2>&1; then
  fail "gh is not authenticated"
fi

if [[ "${dry_run}" -eq 1 ]]; then
  echo "prerelease-pr-postcondition: dry-run/read-only mode"
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

python3 - "${open_prs}" "${expected_version}" "${base}" "${head}" >"${candidate_number_file}" <<'PY'
import json
import re
import sys
from pathlib import Path

path, expected_version, base, head = sys.argv[1:5]
prs = json.loads(Path(path).read_text(encoding="utf-8"))
rc_title_re = re.compile(rf"^chore\({re.escape(base)}\): release \d+\.\d+\.\d+-rc\.\d+$")


def fail(message: str) -> None:
    print(f"prerelease-pr-postcondition: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


generated = [pr for pr in prs if pr.get("headRefName") == head]
if not generated:
    fail(
        "release-please did not leave an open RC PR targeting premain; "
        "'No user facing commits' is a failed gate, not a successful no-op"
    )

bad = [pr for pr in generated if not rc_title_re.fullmatch(pr.get("title", ""))]
if bad:
    details = ", ".join(
        f"#{pr.get('number')} {pr.get('title')} {pr.get('url')}" for pr in bad
    )
    fail(f"generated premain release-please PR is not numbered RC-shaped: {details}")

if expected_version:
    expected = expected_version[1:] if expected_version.startswith("v") else expected_version
    generated = [
        pr
        for pr in generated
        if pr.get("title") == f"chore({base}): release {expected}"
    ]
    if not generated:
        fail(f"no generated premain RC PR advertises expected version {expected}")

if len(generated) > 1:
    details = ", ".join(
        f"#{pr.get('number')} {pr.get('title')} {pr.get('url')}" for pr in generated
    )
    fail(f"multiple generated premain RC PRs found: {details}")

candidate = generated[0]
print(candidate["number"])
print(
    "prerelease-pr-postcondition: candidate "
    f"#{candidate.get('number')} {candidate.get('title')} {candidate.get('url')}",
    file=sys.stderr,
)
PY

candidate_number="$(cat "${candidate_number_file}")"

gh pr view "${candidate_number}" \
  --repo "${repo}" \
  --json number,title,headRefName,baseRefName,url,files >"${details}"

python3 - "${details}" "${base}" "${head}" <<'PY'
import json
import re
import sys
from pathlib import Path

path, base, head = sys.argv[1:4]
pr = json.loads(Path(path).read_text(encoding="utf-8"))
rc_title_re = re.compile(rf"^chore\({re.escape(base)}\): release \d+\.\d+\.\d+-rc\.\d+$")
required_paths = {
    ".release-please-manifest.premain.json",
    "py/src/theorydb_py/version.json",
    "ts/package.json",
    "ts/package-lock.json",
    "CHANGELOG.md",
}


def fail(message: str) -> None:
    print(f"prerelease-pr-postcondition: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


if pr.get("baseRefName") != base:
    fail(f"PR #{pr.get('number')} base {pr.get('baseRefName')!r} != {base!r}")

if pr.get("headRefName") != head:
    fail(f"PR #{pr.get('number')} head {pr.get('headRefName')!r} != {head!r}")

if not rc_title_re.fullmatch(pr.get("title", "")):
    fail(f"PR #{pr.get('number')} title is not numbered RC-shaped: {pr.get('title')!r}")

paths = {entry.get("path", "") for entry in pr.get("files", [])}
missing = sorted(required_paths - paths)
if missing:
    fail(f"PR #{pr.get('number')} missing prerelease files: {', '.join(missing)}")

print(
    "prerelease-pr-postcondition: PASS "
    f"(#{pr.get('number')} {pr.get('title')}; prerelease files present)"
)
PY
