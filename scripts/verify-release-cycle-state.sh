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
import re
import sys
from pathlib import Path

semver_re = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$")


def read(path: str) -> object:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def version_info(label: str, version: str) -> tuple[tuple[int, int, int], bool]:
    match = semver_re.match(version.strip())
    if not match:
        print(f"release-cycle-state: FAIL ({label} has invalid semver {version!r})")
        raise SystemExit(1)
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

for label, version in {**stable_files, ".release-please-manifest.premain.json": premain}.items():
    if not isinstance(version, str) or not version.strip():
        print(f"release-cycle-state: FAIL ({label} is missing a version)")
        raise SystemExit(1)

stable_base, stable_is_prerelease = version_info(".release-please-manifest.json", stable)
if stable_is_prerelease:
    print(f"release-cycle-state: FAIL (.release-please-manifest.json is prerelease {stable})")
    raise SystemExit(1)

current_branch = (
    __import__("os").environ.get("GITHUB_BASE_REF")
    or __import__("os").environ.get("GITHUB_REF_NAME")
    or ""
)
if not current_branch:
    import subprocess

    current_branch = subprocess.check_output(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"], text=True
    ).strip()

if current_branch in {"main", "staging"}:
    for label, version in stable_files.items():
        base, is_prerelease = version_info(label, version)
        if is_prerelease:
            print(f"release-cycle-state: FAIL ({label} is prerelease {version} on {current_branch})")
            raise SystemExit(1)
        if base != stable_base:
            print(
                "release-cycle-state: FAIL "
                f"({label} {version} does not match stable manifest {stable})"
            )
            raise SystemExit(1)

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
