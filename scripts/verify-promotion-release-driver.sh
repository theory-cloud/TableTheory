#!/usr/bin/env bash
set -euo pipefail

# Read-only release-intent guard for human promotion PRs.
# It rejects premain/main promotion PRs that cannot drive the required
# release-please RC/stable path.

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-promotion-release-driver.sh [options]

Options:
  --repo OWNER/REPO      GitHub repository. Defaults to $GITHUB_REPOSITORY or theory-cloud/TableTheory.
  --base BRANCH         PR base branch. Defaults to $GITHUB_BASE_REF.
  --head BRANCH         PR head branch. Defaults to $GITHUB_HEAD_REF.
  --pr NUMBER           Pull request number for read-only GitHub metadata lookup.
  --title TITLE         PR title fallback when --pr/gh is unavailable.
  --body BODY           PR body fallback when --pr/gh is unavailable.
  --commit-message MSG  Local/test fallback commit message; repeatable.
  --dry-run             Print that local read-only mode is being used.
  -h, --help            Show this help.

This command reads PR metadata and checked-out version files only. It never
creates, updates, merges, tags, publishes, uploads, deletes, resets, or pushes.
USAGE
}

repo="${GITHUB_REPOSITORY:-theory-cloud/TableTheory}"
base="${GITHUB_BASE_REF:-}"
head="${GITHUB_HEAD_REF:-}"
pr_number="${PR_NUMBER:-}"
title="${PR_TITLE:-}"
body="${PR_BODY:-}"
dry_run=0
commit_messages=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--repo requires a value)" >&2
        exit 2
      fi
      repo="$2"
      shift 2
      ;;
    --base)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--base requires a value)" >&2
        exit 2
      fi
      base="$2"
      shift 2
      ;;
    --head)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--head requires a value)" >&2
        exit 2
      fi
      head="$2"
      shift 2
      ;;
    --pr)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--pr requires a value)" >&2
        exit 2
      fi
      pr_number="$2"
      shift 2
      ;;
    --title)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--title requires a value)" >&2
        exit 2
      fi
      title="$2"
      shift 2
      ;;
    --body)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--body requires a value)" >&2
        exit 2
      fi
      body="$2"
      shift 2
      ;;
    --commit-message)
      if [[ $# -lt 2 ]]; then
        echo "promotion-release-driver: FAIL (--commit-message requires a value)" >&2
        exit 2
      fi
      commit_messages+=("$2")
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
      echo "promotion-release-driver: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

fail() {
  echo "promotion-release-driver: FAIL ($1)" >&2
  echo "promotion-release-driver: remediation: fix staging/premain content or the promotion PR squash title/body/footer through normal PR flow; do not use tags, resets, manual manifests, package-version edits, or GitHub release mutations." >&2
  exit 1
}

if [[ -z "${base}" || -z "${head}" ]]; then
  fail "missing PR base/head branch"
fi

if [[ "${dry_run}" -eq 1 ]]; then
  echo "promotion-release-driver: dry-run/read-only mode"
fi

metadata_file=""
if [[ -n "${pr_number}" ]]; then
  if ! command -v gh >/dev/null 2>&1; then
    if [[ "${dry_run}" -ne 1 ]]; then
      fail "gh is required when --pr is provided"
    fi
  elif ! gh auth status >/dev/null 2>&1; then
    if [[ "${dry_run}" -ne 1 ]]; then
      fail "gh authentication is required when --pr is provided"
    fi
  else
    metadata_file="$(mktemp)"
    if ! gh pr view "${pr_number}" \
      --repo "${repo}" \
      --json title,body,commits >"${metadata_file}"; then
      rm -f "${metadata_file}"
      metadata_file=""
      if [[ "${dry_run}" -ne 1 ]]; then
        fail "could not read PR #${pr_number} metadata"
      fi
    fi
  fi
fi

fallback_file="$(mktemp)"
cleanup() {
  [[ -z "${metadata_file}" ]] || rm -f "${metadata_file}"
  rm -f "${fallback_file}"
}
trap cleanup EXIT

python3 - "${fallback_file}" "${title}" "${body}" "${commit_messages[@]}" <<'PY'
import json
import subprocess
import sys
from pathlib import Path

path, title, body, *commits = sys.argv[1:]
if not commits and not title.strip() and not body.strip():
    try:
        out = subprocess.check_output(
            ["git", "log", "--format=%s%n%b%x1e", "--max-count=50"],
            stderr=subprocess.DEVNULL,
            text=True,
        )
        commits = [chunk.strip() for chunk in out.split("\x1e") if chunk.strip()]
    except Exception:
        commits = []

Path(path).write_text(
    json.dumps({"title": title, "body": body, "commits": commits}),
    encoding="utf-8",
)
PY

metadata_arg="${metadata_file:-${fallback_file}}"

BASE_REF="${base}" HEAD_REF="${head}" python3 - "${metadata_arg}" <<'PY'
import json
import os
import re
import sys
from pathlib import Path

metadata_path = Path(sys.argv[1])
base = os.environ["BASE_REF"]
head = os.environ["HEAD_REF"]

rc_version_re = re.compile(r"^v?\d+\.\d+\.\d+-rc\.\d+$")
stable_version_re = re.compile(r"^v?\d+\.\d+\.\d+$")
rc_title_re = re.compile(r"^chore\(premain\): release \d+\.\d+\.\d+-rc\.\d+$")
stable_title_re = re.compile(r"^chore\(main\): release \d+\.\d+\.\d+$")
any_rc_re = re.compile(r"\d+\.\d+\.\d+-rc(?:[.\-\w]*)?")


def fail(message: str) -> None:
    print(f"promotion-release-driver: FAIL ({message})", file=sys.stderr)
    print(
        "promotion-release-driver: remediation: fix staging/premain content "
        "or the promotion PR squash title/body/footer through normal PR flow; "
        "do not use tags, resets, manual manifests, package-version edits, "
        "or GitHub release mutations.",
        file=sys.stderr,
    )
    raise SystemExit(1)


def read_metadata() -> tuple[str, str, list[str]]:
    data = json.loads(metadata_path.read_text(encoding="utf-8"))
    title = data.get("title") or ""
    body = data.get("body") or ""
    commits = []
    for entry in data.get("commits") or []:
        if isinstance(entry, str):
            commits.append(entry)
            continue
        if isinstance(entry, dict):
            commits.append(
                "\n".join(
                    part
                    for part in [
                        entry.get("messageHeadline") or entry.get("headline") or "",
                        entry.get("messageBody") or entry.get("body") or "",
                    ]
                    if part
                )
            )
    return title, body, commits


def release_as_versions(text: str) -> list[str]:
    return [
        match.group(1)
        for match in re.finditer(
            r"(?im)^[ \t]*Release-As:[ \t]*v?([0-9]+\.[0-9]+\.[0-9]+(?:-rc(?:\.[0-9]+)?)?)[ \t]*$",
            text,
        )
    ]


def release_eligible_driver(text: str) -> str | None:
    release_types = {"feat", "fix", "perf"}
    for line in text.splitlines():
        stripped = line.strip()
        match = re.match(r"^([a-z][a-z0-9-]*)(?:\([^)]+\))?(!)?:\s+\S", stripped)
        if not match:
            continue
        commit_type, bang = match.group(1), match.group(2)
        if commit_type in release_types or bang:
            return stripped
    breaking = re.search(r"(?im)^[ \t]*BREAKING[ -]CHANGE:[ \t]*\S", text)
    if breaking:
        return "BREAKING CHANGE footer"
    return None


def parse_base(version: str) -> tuple[int, int, int]:
    version = version.strip()
    if version.startswith("v"):
        version = version[1:]
    version = version.split("+", 1)[0].split("-", 1)[0]
    parts = version.split(".")
    if len(parts) != 3:
        fail(f"invalid semver base {version!r}")
    return tuple(int(part) for part in parts)


def read_json(path: str) -> object:
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except FileNotFoundError:
        fail(f"missing {path}")


def pending_stable_promotion_version() -> str:
    stable = read_json(".release-please-manifest.json").get(".", "")
    premain = read_json(".release-please-manifest.premain.json").get(".", "")
    ts_package = read_json("ts/package.json").get("version", "")
    ts_lock = read_json("ts/package-lock.json")
    ts_lock_root = ts_lock.get("version", "")
    ts_lock_pkg = ts_lock.get("packages", {}).get("", {}).get("version", "")
    py_version = read_json("py/src/theorydb_py/version.json").get("version", "")

    if not stable or not isinstance(stable, str):
        fail(".release-please-manifest.json is missing a stable version")
    if "-rc" in stable:
        fail(f"main stable manifest must not be RC-shaped ({stable})")

    pending_files = {
        ".release-please-manifest.premain.json": premain,
        "ts/package.json": ts_package,
        "ts/package-lock.json": ts_lock_root,
        "ts/package-lock.json packages['']": ts_lock_pkg,
        "py/src/theorydb_py/version.json": py_version,
    }
    first_label, first_version = next(iter(pending_files.items()))
    if not isinstance(first_version, str) or not rc_version_re.match(first_version):
        fail(
            "premain -> main promotion must carry an RC pending stable "
            f"promotion state with numbered -rc.N syntax; {first_label} is {first_version!r}"
        )
    for label, version in pending_files.items():
        if version != first_version:
            fail(
                "pending stable promotion files are inconsistent "
                f"({label} {version!r} != {first_version!r})"
            )
    if parse_base(first_version) <= parse_base(stable):
        fail(
            "pending stable promotion RC must be ahead of the stable manifest "
            f"({first_version} <= {stable})"
        )
    return first_version


title, body, commits = read_metadata()
aggregate = "\n\n".join([title, body, *commits])
pr_text = "\n\n".join([title, body])

if base == "premain" and head == "release-please--branches--premain":
    if not rc_title_re.fullmatch(title):
        fail(f"generated premain release-please PR must be numbered RC-shaped, got {title!r}")
    print(f"promotion-release-driver: PASS (generated premain RC PR {title!r})")
    raise SystemExit(0)

if base == "main" and head == "release-please--branches--main":
    if not stable_title_re.fullmatch(title) or any_rc_re.search(title):
        fail(f"generated main release-please PR must be stable-shaped, got {title!r}")
    print(f"promotion-release-driver: PASS (generated main stable PR {title!r})")
    raise SystemExit(0)

if base == "premain":
    if head != "staging":
        fail(f"premain promotion PR head must be staging, got {head!r}")
    versions = release_as_versions(aggregate)
    invalid = [version for version in versions if not rc_version_re.match(version)]
    if invalid:
        fail(
            "staging -> premain Release-As footers must be RC-shaped "
            f"X.Y.Z-rc.N, got {', '.join(invalid)}"
        )
    if versions:
        print(
            "promotion-release-driver: PASS "
            f"(staging -> premain RC Release-As {', '.join(versions)})"
        )
        raise SystemExit(0)
    driver = release_eligible_driver(aggregate)
    if driver:
        print(
            "promotion-release-driver: PASS "
            f"(staging -> premain release-eligible driver {driver!r})"
        )
        raise SystemExit(0)
    fail(
        "staging -> premain promotion lacks a release-eligible conventional "
        "commit or RC-shaped Release-As footer; release-please would be "
        "allowed to report 'No user facing commits'"
    )

if base == "main":
    if head != "premain":
        fail(f"main promotion PR head must be premain, got {head!r}")
    if any_rc_re.search(title):
        fail(f"premain -> main PR title must not be RC-shaped, got {title!r}")
    versions = release_as_versions(pr_text)
    rc_versions = [version for version in versions if rc_version_re.match(version)]
    if rc_versions:
        fail(
            "premain -> main Release-As footers must be stable X.Y.Z, "
            f"not RC-shaped ({', '.join(rc_versions)})"
        )
    if any_rc_re.search(pr_text):
        fail("premain -> main PR title/body must not be RC-shaped")
    invalid = [
        version
        for version in versions
        if not stable_version_re.match(version) and not rc_version_re.match(version)
    ]
    if invalid:
        fail(f"invalid Release-As footer(s): {', '.join(invalid)}")
    pending_rc = pending_stable_promotion_version()
    pending_stable = pending_rc.split("-", 1)[0]
    if not versions:
        fail(
            "premain -> main promotion requires a stable Release-As footer "
            f"matching the pending RC base {pending_stable}"
        )
    mismatched = [version for version in versions if version != pending_stable]
    if mismatched:
        fail(
            "premain -> main stable Release-As footer must match the pending "
            f"RC base {pending_stable}, got {', '.join(mismatched)}"
        )
    print(
        "promotion-release-driver: PASS "
        f"(premain -> main pending stable promotion {pending_rc} -> {pending_stable})"
    )
    raise SystemExit(0)

fail(f"unsupported promotion base branch {base!r}")
PY
