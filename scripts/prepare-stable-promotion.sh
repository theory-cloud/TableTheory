#!/usr/bin/env bash
set -euo pipefail

# DEPRECATED / DIAGNOSTIC-ONLY DRY RUN.
# Release-lane v2 uses one release-please manifest. A premain -> main
# promotion may briefly put an RC in .release-please-manifest.json on main;
# release-pr.yml then opens the stable release-please PR that normalizes the
# manifest and changelog. This helper remains only as a read-only diagnostic
# until the release-lane-v2 soak completes.

usage() {
  cat <<'USAGE'
Usage: bash scripts/prepare-stable-promotion.sh [--check] [--write] [--expected-version X.Y.Z]

Deprecated diagnostic/fallback helper only. The normal stable release path leaves
version and changelog edits to release-please. In the single-manifest lane this
script is dry-run only; --write is rejected.

Options:
  --check                 Print the normalization target only. This is the default.
  --write                 Rejected in release-lane v2; use release-please PRs.
  --expected-version VER  Require the derived stable baseline to equal VER.
  -h, --help              Show this help.
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

if [[ "${mode}" == "write" ]]; then
  echo "stable-promotion: FAIL (--write is retired in release-lane v2; use the generated stable release-please PR)"
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

python3 - "${expected_version}" <<'PY'
import json
import re
import sys
from pathlib import Path

expected_version = sys.argv[1].strip()
semver_re = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$")


def fail(message: str) -> None:
    print(f"stable-promotion: FAIL ({message})")
    raise SystemExit(1)


def semver_base(version: str) -> tuple[tuple[int, int, int], str, bool]:
    match = semver_re.match(version.strip())
    if not match:
        fail(f"invalid semver: {version}")
    base_tuple = tuple(int(part) for part in match.group(1, 2, 3))
    base = ".".join(str(part) for part in base_tuple)
    return base_tuple, base, bool(match.group(4))

if Path(".release-please-manifest.premain.json").exists():
    fail("retired .release-please-manifest.premain.json is still present")

manifest = json.loads(Path(".release-please-manifest.json").read_text(encoding="utf-8")).get(".", "")
if not isinstance(manifest, str) or not manifest:
    fail("missing version in .release-please-manifest.json")

_, target_version, is_prerelease = semver_base(manifest)

if expected_version:
    _, expected_base, expected_is_prerelease = semver_base(expected_version)
    if expected_is_prerelease:
        fail(f"--expected-version must be stable, got {expected_version}")
    if expected_base != target_version:
        fail(f"derived stable baseline {target_version} != expected {expected_base}")

print("stable-promotion: DEPRECATED dry-run only")
if is_prerelease:
    print(f"stable-promotion: single-manifest RC {manifest} would normalize to {target_version}")
else:
    print(f"stable-promotion: single manifest already stable at {manifest}")
print("stable-promotion: PASS")
PY
