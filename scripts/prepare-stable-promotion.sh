#!/usr/bin/env bash
set -euo pipefail

# Diagnostic/fallback helper for inspecting or normalizing RC-owned files during stable-promotion recovery.
#
# Normal stable releases are CI/release-please driven:
#   staging -> premain -> generated/published RC -> main -> generated/published stable release.
#
# Use this script only for diagnostics, or as an explicitly documented fallback when release automation is blocked.
#
# This script does not create branches, merge refs, push, tag, or publish releases.

usage() {
  cat <<'USAGE'
Usage: bash scripts/prepare-stable-promotion.sh [--check|--write] [--expected-version X.Y.Z]

Diagnostic/fallback helper only. The normal stable release path leaves version and changelog edits to release-please.

Options:
  --check                 Print the normalization plan only. This is the default.
  --write                 Rewrite RC-owned files to the stable baseline.
  --expected-version VER  Require the derived stable baseline to equal VER.
  -h, --help              Show this help.

Files normalized with --write:
  .release-please-manifest.premain.json
  ts/package.json
  ts/package-lock.json
  py/src/theorydb_py/version.json

The stable manifest (.release-please-manifest.json) is validated but not advanced here.
Release Please must advance it in the stable release PR so the immutable release remains
traceable to the release workflow.
USAGE
}

mode="check"
expected_version=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      mode="check"
      shift
      ;;
    --write)
      mode="write"
      shift
      ;;
    --expected-version)
      if [[ $# -lt 2 ]]; then
        echo "stable-promotion: FAIL (--expected-version requires a value)" >&2
        exit 2
      fi
      expected_version="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "stable-promotion: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${mode}" != "check" && "${mode}" != "write" ]]; then
  echo "stable-promotion: FAIL (invalid mode: ${mode})" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

python3 - "${mode}" "${expected_version}" <<'PY'
import json
import re
import sys
from pathlib import Path

mode = sys.argv[1]
expected_version = sys.argv[2].strip()

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

if expected_version:
    expected_tuple, expected_base, expected_is_prerelease = semver_info(expected_version)
    if expected_is_prerelease:
        fail(f"--expected-version must be stable, got {expected_version}")
    if expected_tuple != target_tuple:
        fail(f"derived stable baseline {target_version} != expected {expected_base}")
    target_version = expected_base

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

if not changes:
    print("stable-promotion: PASS (no normalization needed)")
    raise SystemExit(0)

for path, current, desired in changes:
    print(f"stable-promotion: PLAN {path}: {current} -> {desired}")

if mode == "check":
    print("stable-promotion: PASS (dry-run)")
    raise SystemExit(0)

premain_manifest["."] = target_version
ts_package["version"] = target_version
ts_lock["version"] = target_version
ts_lock.setdefault("packages", {}).setdefault("", {})["version"] = target_version
py_version["version"] = target_version

files = {
    ".release-please-manifest.premain.json": premain_manifest,
    "ts/package.json": ts_package,
    "ts/package-lock.json": ts_lock,
    "py/src/theorydb_py/version.json": py_version,
}

for path, data in files.items():
    Path(path).write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

print("stable-promotion: PASS (files normalized)")
PY
