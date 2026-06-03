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
