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
  --expected-version VER   Optional RC version the PR must advertise, e.g. 2.0.0-rc or 2.0.0-rc.1.
  --open-prs-file PATH     Test-only gh pr list JSON fixture.
  --details-file PATH      Test-only gh pr view JSON fixture for the selected PR.
  --manifest-file PATH     Test-only .release-please-manifest.json fixture for the selected PR head.
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
open_prs_fixture=""
details_fixture=""
manifest_fixture=""

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
    --open-prs-file)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--open-prs-file requires a value)" >&2
        exit 2
      fi
      open_prs_fixture="$2"
      shift 2
      ;;
    --details-file)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--details-file requires a value)" >&2
        exit 2
      fi
      details_fixture="$2"
      shift 2
      ;;
    --manifest-file)
      if [[ $# -lt 2 ]]; then
        echo "prerelease-pr-postcondition: FAIL (--manifest-file requires a value)" >&2
        exit 2
      fi
      manifest_fixture="$2"
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

if [[ -n "${expected_version}" && ! "${expected_version}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+-rc(\.[0-9]+)?$ ]]; then
  fail "expected version must be RC-shaped X.Y.Z-rc or X.Y.Z-rc.N, got ${expected_version}"
fi

fixture_count=0
[[ -z "${open_prs_fixture}" ]] || fixture_count=$((fixture_count + 1))
[[ -z "${details_fixture}" ]] || fixture_count=$((fixture_count + 1))
[[ -z "${manifest_fixture}" ]] || fixture_count=$((fixture_count + 1))
if [[ "${fixture_count}" -ne 0 && "${fixture_count}" -ne 3 ]]; then
  fail "--open-prs-file, --details-file, and --manifest-file must be supplied together"
fi

if [[ "${fixture_count}" -eq 3 ]]; then
  for fixture in "${open_prs_fixture}" "${details_fixture}" "${manifest_fixture}"; do
    [[ -f "${fixture}" ]] || fail "fixture file not found: ${fixture}"
  done
else
  if ! command -v gh >/dev/null 2>&1; then
    fail "gh is required"
  fi

  if ! gh auth status >/dev/null 2>&1; then
    fail "gh is not authenticated"
  fi
fi

if [[ "${dry_run}" -eq 1 ]]; then
  echo "prerelease-pr-postcondition: dry-run/read-only mode"
fi

open_prs="$(mktemp)"
candidate_number_file="$(mktemp)"
details="$(mktemp)"
manifest="$(mktemp)"
cleanup() {
  rm -f "${open_prs}" "${candidate_number_file}" "${details}" "${manifest}"
}
trap cleanup EXIT

if [[ "${fixture_count}" -eq 3 ]]; then
  cp "${open_prs_fixture}" "${open_prs}"
else
  gh pr list \
    --repo "${repo}" \
    --base "${base}" \
    --state open \
    --json number,title,headRefName,url >"${open_prs}"
fi

python3 - "${open_prs}" "${expected_version}" "${base}" "${head}" >"${candidate_number_file}" <<'PY'
import json
import re
import sys
from pathlib import Path

path, expected_version, base, head = sys.argv[1:5]
prs = json.loads(Path(path).read_text(encoding="utf-8"))
rc_title_re = re.compile(rf"^chore\({re.escape(base)}\): release \d+\.\d+\.\d+-rc(?:\.\d+)?$")


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
    fail(f"generated premain release-please PR is not RC-shaped: {details}")

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

if [[ "${fixture_count}" -eq 3 ]]; then
  cp "${details_fixture}" "${details}"
  cp "${manifest_fixture}" "${manifest}"
else
  gh pr view "${candidate_number}" \
    --repo "${repo}" \
    --json number,title,headRefName,baseRefName,headRefOid,url,files >"${details}"

  manifest_ref="$(
    python3 - "${details}" "${head}" <<'PY'
import json
import sys
from pathlib import Path

path, head = sys.argv[1:3]
pr = json.loads(Path(path).read_text(encoding="utf-8"))
print(pr.get("headRefOid") or pr.get("headRefName") or head)
PY
  )"

  gh api -X GET "repos/${repo}/contents/.release-please-manifest.json" \
    -f "ref=${manifest_ref}" \
    --jq '.content' | base64 --decode >"${manifest}"
fi

python3 - "${details}" "${manifest}" "${base}" "${head}" <<'PY'
import json
import re
import sys
from pathlib import Path

details_path, manifest_path, base, head = sys.argv[1:5]
pr = json.loads(Path(details_path).read_text(encoding="utf-8"))
manifest = json.loads(Path(manifest_path).read_text(encoding="utf-8"))
rc_version_re = re.compile(r"^\d+\.\d+\.\d+-rc(?:\.\d+)?$")
rc_title_re = re.compile(rf"^chore\({re.escape(base)}\): release (\d+\.\d+\.\d+-rc(?:\.\d+)?)$")
required_paths = {
    ".release-please-manifest.json",
    "CHANGELOG.md",
}


def fail(message: str) -> None:
    print(f"prerelease-pr-postcondition: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


if pr.get("baseRefName") != base:
    fail(f"PR #{pr.get('number')} base {pr.get('baseRefName')!r} != {base!r}")

if pr.get("headRefName") != head:
    fail(f"PR #{pr.get('number')} head {pr.get('headRefName')!r} != {head!r}")

title_match = rc_title_re.fullmatch(pr.get("title", ""))
if not title_match:
    fail(f"PR #{pr.get('number')} title is not RC-shaped: {pr.get('title')!r}")

title_version = title_match.group(1)
manifest_version = manifest.get(".") if isinstance(manifest, dict) else None
if not isinstance(manifest_version, str) or not rc_version_re.fullmatch(manifest_version):
    fail(
        f"PR #{pr.get('number')} manifest version is not RC-shaped "
        f"X.Y.Z-rc or X.Y.Z-rc.N: {manifest_version!r}"
    )
if manifest_version != title_version:
    fail(
        f"PR #{pr.get('number')} manifest version {manifest_version!r} "
        f"does not match title release {title_version!r}"
    )

paths = {entry.get("path", "") for entry in pr.get("files", [])}
missing = sorted(required_paths - paths)
if missing:
    fail(f"PR #{pr.get('number')} missing single-manifest prerelease files: {', '.join(missing)}")

print(
    "prerelease-pr-postcondition: PASS "
    f"(#{pr.get('number')} {pr.get('title')}; manifest {manifest_version}; "
    "single-manifest prerelease files present)"
)
PY
