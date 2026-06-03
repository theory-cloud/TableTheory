#!/usr/bin/env bash
set -euo pipefail

# CI-safe local release-cycle checks. This verifier is intentionally local: it
# fails on checked-out forbidden stable state and on missing guardrails, while
# remote branch drift is reported by scripts/watch-release-cycle.sh.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

failures=0

fail() {
  echo "release-cycle-state: FAIL ($1)"
  failures=$((failures + 1))
}

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

stable_files = {
    ".release-please-manifest.json": stable,
    "ts/package.json": ts_package,
    "ts/package-lock.json": ts_lock_root,
    "ts/package-lock.json packages['']": ts_lock_pkg,
    "py/src/theorydb_py/version.json": py_version,
}

normalized_promotion_files = {
    ".release-please-manifest.premain.json": premain,
    "ts/package.json": ts_package,
    "ts/package-lock.json": ts_lock_root,
    "ts/package-lock.json packages['']": ts_lock_pkg,
    "py/src/theorydb_py/version.json": py_version,
}

for label, version in {**stable_files, ".release-please-manifest.premain.json": premain}.items():
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

if pending_mode:
    if current_branch != "main":
        fail(
            "pending stable promotion mode is only allowed on main "
            f"(current branch: {current_branch})"
        )

    pending_base = None
    pending_version = None
    for label, version in normalized_promotion_files.items():
        base, is_prerelease = version_info(label, version)
        if is_prerelease:
            fail(f"{label} is prerelease {version} in pending stable promotion mode")
        if pending_base is None:
            pending_base = base
            pending_version = version
            continue
        if base != pending_base or version != pending_version:
            fail(
                "pending stable promotion files are inconsistent "
                f"({label} {version} != {pending_version})"
            )

    if pending_base is None or pending_version is None:
        fail("pending stable promotion has no normalized version files")
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

if current_branch in {"main", "staging"}:
    for label, version in stable_files.items():
        base, is_prerelease = version_info(label, version)
        if is_prerelease:
            fail(f"{label} is prerelease {version} on {current_branch}")
        if base != stable_base or version != stable:
            fail(f"{label} {version} does not match stable manifest {stable}")

print(
    "release-cycle-state: PASS "
    f"(branch={current_branch}, stable={stable}, premain={premain})"
)
PY
  then
    failures=$((failures + 1))
  fi
fi

if ! bash scripts/prepare-stable-promotion.sh --check >/tmp/tabletheory-stable-promotion-check.$$ 2>&1; then
  cat /tmp/tabletheory-stable-promotion-check.$$
  rm -f /tmp/tabletheory-stable-promotion-check.$$
  fail "stable promotion helper dry-run failed"
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
