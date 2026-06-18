#!/usr/bin/env bash
set -euo pipefail

failures=0

fail() {
  echo "release-cycle-state: FAIL ($1)"
  failures=$((failures + 1))
}

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-release-cycle-state.sh [--repo-root ABSOLUTE_PATH]

CI-safe local release-cycle checks. By default this verifier validates the
checkout that contains the trusted script. Release Hygiene may pass an explicit
target checkout root after same-repository provenance verification so trusted
base scripts inspect the PR head files without executing PR-head scripts.

Options:
  --repo-root ABSOLUTE_PATH  Validate this checkout root instead of the script checkout.
  -h, --help                Show this help.
USAGE
}

script_repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
repo_root="${script_repo_root}"
explicit_repo_root="${RELEASE_CYCLE_REPO_ROOT:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      if [[ $# -lt 2 ]]; then
        echo "release-cycle-state: FAIL (--repo-root requires a value)" >&2
        exit 2
      fi
      explicit_repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "release-cycle-state: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -n "${explicit_repo_root}" ]]; then
  if [[ "${explicit_repo_root}" != /* ]]; then
    echo "release-cycle-state: FAIL (--repo-root/RELEASE_CYCLE_REPO_ROOT must be an absolute path)" >&2
    exit 2
  fi
  if [[ ! -d "${explicit_repo_root}" ]]; then
    echo "release-cycle-state: FAIL (--repo-root/RELEASE_CYCLE_REPO_ROOT does not exist: ${explicit_repo_root})" >&2
    exit 2
  fi
  repo_root="$(cd "${explicit_repo_root}" && pwd -P)"
fi

cd "${repo_root}"

require_file() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    fail "missing ${path}"
  fi
}

required_files=(
  "scripts/prepare-stable-promotion.sh"
  "scripts/watch-release-cycle.sh"
  ".release-please-manifest.json"
  ".release-please-manifest.premain.json"
  "ts/package.json"
  "ts/package-lock.json"
  "py/src/theorydb_py/version.json"
)

for path in "${required_files[@]}"; do
  require_file "${path}"
done

if [[ "${failures}" -eq 0 ]]; then
  if ! python3 - <<'PY'
import json
import os
import re
import subprocess
import sys
from pathlib import Path

semver_re = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$")


def read(path: str) -> object:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def fail(message: str) -> None:
    print(f"release-cycle-state: FAIL ({message})")
    raise SystemExit(1)


def version_info(label: str, version: str) -> tuple[tuple[int, int, int], bool]:
    match = semver_re.match(version.strip())
    if not match:
        fail(f"{label} has invalid semver {version!r}")
    base = tuple(int(part) for part in match.group(1, 2, 3))
    return base, bool(match.group(4))


stable = read(".release-please-manifest.json").get(".", "")
premain = read(".release-please-manifest.premain.json").get(".", "")
ts_package = read("ts/package.json").get("version", "")
ts_lock = read("ts/package-lock.json")
ts_lock_root = ts_lock.get("version", "")
ts_lock_pkg = ts_lock.get("packages", {}).get("", {}).get("version", "")
py_version = read("py/src/theorydb_py/version.json").get("version", "")

promotion_files = {
    ".release-please-manifest.premain.json": premain,
    "ts/package.json": ts_package,
    "ts/package-lock.json": ts_lock_root,
    "ts/package-lock.json packages['']": ts_lock_pkg,
    "py/src/theorydb_py/version.json": py_version,
}

for label, version in {".release-please-manifest.json": stable, **promotion_files}.items():
    if not isinstance(version, str) or not version.strip():
        fail(f"{label} is missing a version")

stable_base, stable_is_prerelease = version_info(".release-please-manifest.json", stable)
if stable_is_prerelease:
    fail(f".release-please-manifest.json is prerelease {stable}")

current_branch = (
    os.environ.get("GITHUB_BASE_REF")
    or os.environ.get("GITHUB_REF_NAME")
    or ""
)
if not current_branch:
    current_branch = subprocess.check_output(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"], text=True
    ).strip()

pending_mode_raw = os.environ.get("RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION", "")
pending_mode = pending_mode_raw == "true"
if pending_mode_raw and pending_mode_raw not in {"true", "false"}:
    fail("RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION must be exactly true or false")


def consistent_version(
    files: dict[str, str],
    mode_label: str,
) -> tuple[tuple[int, int, int], str, bool]:
    expected_base = None
    expected_version = None
    expected_is_prerelease = None
    for label, version in files.items():
        base, is_prerelease = version_info(label, version)
        if expected_version is None:
            expected_base = base
            expected_version = version
            expected_is_prerelease = is_prerelease
            continue
        if version != expected_version or base != expected_base:
            fail(
                f"{mode_label} files are inconsistent "
                f"({label} {version} != {expected_version})"
            )
    if (
        expected_base is None
        or expected_version is None
        or expected_is_prerelease is None
    ):
        fail(f"{mode_label} has no version files")
    return expected_base, expected_version, expected_is_prerelease


def stable_file_mismatch(branch: str) -> str | None:
    for label, version in promotion_files.items():
        base, is_prerelease = version_info(label, version)
        if is_prerelease:
            return f"{label} is prerelease {version} on {branch}"
        if base != stable_base or version != stable:
            return f"{label} {version} does not match stable manifest {stable}"
    return None


if pending_mode:
    if current_branch != "main":
        fail(
            "pending stable promotion mode is only allowed on main "
            f"(current branch: {current_branch})"
        )
    if os.environ.get("GITHUB_HEAD_REF", "") not in {"", "premain"}:
        fail(
            "pending stable promotion mode is only allowed for premain -> main "
            f"(head branch: {os.environ.get('GITHUB_HEAD_REF')})"
        )

    pending_base, pending_version, pending_is_prerelease = consistent_version(
        promotion_files,
        "pending stable promotion",
    )
    if not pending_is_prerelease:
        fail(
            "pending stable promotion files must carry the promoted RC version "
            f"(got {pending_version})"
        )
    if pending_base <= stable_base:
        fail(
            "pending stable promotion version must be ahead of the stable manifest "
            f"({pending_version} <= {stable})"
        )

    print(
        "release-cycle-state: PASS "
        f"(branch={current_branch}, mode=pending-stable-promotion, "
        f"stable={stable}, pending={pending_version})"
    )
    raise SystemExit(0)

if current_branch == "main":
    mismatch = stable_file_mismatch(current_branch)
    if mismatch:
        fail(mismatch)

if current_branch == "staging":
    mismatch = stable_file_mismatch(current_branch)
    if mismatch:
        (
            reconciliation_base,
            reconciliation_version,
            reconciliation_is_prerelease,
        ) = consistent_version(
            promotion_files,
            "staging RC reconciliation",
        )
        if not reconciliation_is_prerelease:
            fail(mismatch)
        if reconciliation_base <= stable_base:
            fail(
                "staging RC reconciliation version must be ahead of the stable manifest "
                f"({reconciliation_version} <= {stable})"
            )
        print(
            "release-cycle-state: PASS "
            f"(branch={current_branch}, mode=staging-rc-reconciliation, "
            f"stable={stable}, rc={reconciliation_version})"
        )
        raise SystemExit(0)

print(
    "release-cycle-state: PASS "
    f"(branch={current_branch}, stable={stable}, premain={premain})"
)
PY
  then
    failures=$((failures + 1))
  fi
fi

# Do not execute scripts from an explicit target checkout: release hygiene runs
# this verifier from the trusted base checkout while inspecting PR-head files.
# Keep the stable-promotion dry-run as trusted verifier logic over target data.
if ! python3 - <<'PY' >/tmp/tabletheory-stable-promotion-check.$$ 2>&1; then
import json
import re
from pathlib import Path

semver_re = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$")


def fail(message: str) -> None:
    print(f"stable-promotion: FAIL ({message})")
    raise SystemExit(1)


def load_json(path: str) -> object:
    p = Path(path)
    if not p.is_file():
        fail(f"missing {path}")
    return json.loads(p.read_text(encoding="utf-8"))


def semver_info(version: str) -> tuple[tuple[int, int, int], str, bool]:
    match = semver_re.match(version.strip())
    if not match:
        fail(f"invalid semver: {version}")
    base_tuple = tuple(int(part) for part in match.group(1, 2, 3))
    base = ".".join(str(part) for part in base_tuple)
    prerelease = match.group(4) or ""
    return base_tuple, base, bool(prerelease)


stable_manifest = load_json(".release-please-manifest.json")
premain_manifest = load_json(".release-please-manifest.premain.json")
ts_package = load_json("ts/package.json")
ts_lock = load_json("ts/package-lock.json")
py_version = load_json("py/src/theorydb_py/version.json")

versions = {
    ".release-please-manifest.json": stable_manifest.get(".", ""),
    ".release-please-manifest.premain.json": premain_manifest.get(".", ""),
    "ts/package.json": ts_package.get("version", ""),
    "ts/package-lock.json": ts_lock.get("version", ""),
    "ts/package-lock.json packages['']": ts_lock.get("packages", {}).get("", {}).get("version", ""),
    "py/src/theorydb_py/version.json": py_version.get("version", ""),
}

for path, version in versions.items():
    if not isinstance(version, str) or not version.strip():
        fail(f"missing version in {path}")

parsed = {path: semver_info(version) for path, version in versions.items()}

stable_version = versions[".release-please-manifest.json"]
if parsed[".release-please-manifest.json"][2]:
    fail(f"stable manifest is a prerelease: {stable_version}")

target_tuple, target_version = max((info[0], info[1]) for info in parsed.values())

if parsed[".release-please-manifest.json"][0] > target_tuple:
    fail(
        "stable manifest is ahead of derived promotion baseline "
        f"({stable_version} > {target_version})"
    )

changes: list[tuple[str, str, str]] = []


def plan(path: str, current: str, desired: str) -> None:
    if current != desired:
        changes.append((path, current, desired))


plan(".release-please-manifest.premain.json", versions[".release-please-manifest.premain.json"], target_version)
plan("ts/package.json", versions["ts/package.json"], target_version)
plan("ts/package-lock.json", versions["ts/package-lock.json"], target_version)
plan("ts/package-lock.json packages['']", versions["ts/package-lock.json packages['']"], target_version)
plan("py/src/theorydb_py/version.json", versions["py/src/theorydb_py/version.json"], target_version)

print(f"stable-promotion: target={target_version}")
print(f"stable-promotion: stable-manifest={stable_version} (validated, not advanced)")

for path, current, desired in changes:
    print(f"stable-promotion: PLAN {path}: {current} -> {desired}")

print("stable-promotion: PASS (dry-run)")
PY
  cat /tmp/tabletheory-stable-promotion-check.$$
  rm -f /tmp/tabletheory-stable-promotion-check.$$
  fail "stable promotion dry-run failed"
else
  rm -f /tmp/tabletheory-stable-promotion-check.$$
fi

if grep -RInE 'git push +origin +(main|premain|staging)|git push +[^[:space:]]+ +(main|premain|staging)' .github/workflows scripts | grep -v 'verify-release-cycle-state.sh'; then
  fail "release automation contains direct branch mutation"
fi

if grep -RInE 'gh api .*git/refs/(heads/)?(main|premain|staging)|gh api .*contents/.*(main|premain|staging)' .github/workflows scripts | grep -v 'verify-release-cycle-state.sh'; then
  fail "release automation appears to mutate protected branch refs through GitHub API"
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "release-cycle-state: FAIL (${failures} issue(s))"
  exit 1
fi

echo "release-cycle-state: PASS"
