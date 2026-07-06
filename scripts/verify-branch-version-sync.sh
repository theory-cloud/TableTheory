#!/usr/bin/env bash
set -euo pipefail

# Canonical release-version verifier.
#
# v2 release lane keeps one release-please manifest. Branch sync validates that
# premain/main promotion candidates are not behind origin/main. Alignment mode is
# retained for existing rubric callers, but TS/Py source package versions are no
# longer release-cycle state; release assets are stamped from tag_name at build
# time by scripts/prepare-release-package-versions.py.

mode="branch-sync"
repo_root=""

usage() {
  cat <<'USAGE'
Usage:
  bash scripts/verify-branch-version-sync.sh [--alignment] [--repo-root PATH]

Modes:
  default       Verify premain/main branch release-version sync from the single manifest.
  --alignment  Verify local source/package version files are valid while noting
               that release asset versions are tag-derived, not committed state.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --alignment)
      mode="alignment"
      shift
      ;;
    --repo-root)
      repo_root="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "branch-version-sync: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${repo_root}" ]]; then
  repo_root="$(cd "${script_dir}/.." && pwd)"
fi
cd "${repo_root}"

json_file_value() {
  local path="$1"
  local expr="$2"
  python3 - "${path}" "${expr}" <<'PY'
import json
import sys
from pathlib import Path

path, expr = sys.argv[1], sys.argv[2]
data = json.loads(Path(path).read_text(encoding="utf-8"))
if expr == ".":
    print(data.get(".", "") if isinstance(data, dict) else "")
    raise SystemExit(0)
value = data
for part in expr.split("."):
    if not part:
        continue
    if isinstance(value, dict):
        value = value.get(part, "")
    else:
        value = ""
print(value if isinstance(value, str) else "")
PY
}

git_ref_json_value() {
  local ref="$1"
  local path="$2"
  local expr="$3"
  git show "${ref}:${path}" | python3 -c '
import json
import sys

ref, path, expr = sys.argv[1], sys.argv[2], sys.argv[3]
try:
    data = json.load(sys.stdin)
except json.JSONDecodeError as exc:
    print(f"branch-version-sync: FAIL ({ref}:{path} is not valid JSON: {exc})", file=sys.stderr)
    raise SystemExit(1)

if expr == ".":
    print(data.get(".", "") if isinstance(data, dict) else "")
    raise SystemExit(0)
value = data
for part in expr.split("."):
    if not part:
        continue
    if isinstance(value, dict):
        value = value.get(part, "")
    else:
        value = ""
print(value if isinstance(value, str) else "")
' "${ref}" "${path}" "${expr}"
}

git_fetch_retry() {
  local remote="$1"
  shift

  local -a refspecs=("$@")
  local attempts="${GIT_FETCH_RETRIES:-5}"
  local base_sleep="${GIT_FETCH_RETRY_SLEEP_SECS:-2}"

  local i=1
  while true; do
    if git fetch --quiet --depth=1 "${remote}" "${refspecs[@]}"; then
      return 0
    fi

    if [[ "${i}" -ge "${attempts}" ]]; then
      echo "branch-version-sync: FAIL (git fetch failed after ${attempts} attempts)" >&2
      return 1
    fi

    sleep_for=$((base_sleep * i))
    echo "branch-version-sync: retrying git fetch in ${sleep_for}s (${i}/${attempts})..." >&2
    sleep "${sleep_for}"
    i=$((i + 1))
  done
}

semver_check() {
  local label="$1"
  local version="$2"
  VERSION_LABEL="${label}" VERSION_VALUE="${version}" python3 - <<'PY'
import os
import re
import sys

label = os.environ["VERSION_LABEL"]
version = os.environ["VERSION_VALUE"]
if not re.fullmatch(r"v?\d+\.\d+\.\d+(?:-rc(?:\.\d+)?)?", version):
    print(f"branch-version-sync: FAIL ({label} has invalid version {version!r})")
    sys.exit(1)
PY
}

run_alignment_check() {
  [[ ! -f ".release-please-manifest.premain.json" ]] || {
    echo "version-alignment: FAIL (.release-please-manifest.premain.json must be retired)"
    return 1
  }

  local manifest
  manifest="$(json_file_value ".release-please-manifest.json" ".")"
  if [[ -z "${manifest}" ]]; then
    echo "version-alignment: FAIL (missing single manifest version)"
    return 1
  fi
  semver_check ".release-please-manifest.json" "${manifest}"

  local ts_version py_version lock_version pkg_lock_version
  ts_version="$(json_file_value "ts/package.json" "version")"
  lock_version="$(json_file_value "ts/package-lock.json" "version")"
  pkg_lock_version="$(python3 - <<'PY'
import json
from pathlib import Path

data = json.loads(Path("ts/package-lock.json").read_text(encoding="utf-8"))
packages = data.get("packages", {})
root = packages.get("", {}) if isinstance(packages, dict) else {}
print(root.get("version", ""))
PY
)"
  py_version="$(json_file_value "py/src/tabletheory_py/version.json" "version")"

  for item in \
    "ts/package.json:${ts_version}" \
    "ts/package-lock.json:${lock_version}" \
    "ts/package-lock.json packages['']:${pkg_lock_version}" \
    "py/src/tabletheory_py/version.json:${py_version}"; do
    label="${item%%:*}"
    version="${item#*:}"
    if [[ -z "${version}" ]]; then
      echo "version-alignment: FAIL (${label} is missing a source package version)"
      return 1
    fi
    semver_check "${label}" "${version}"
  done

  echo "version-alignment: PASS (single-manifest=${manifest}; release asset versions are tag-derived)"
}

run_branch_sync_check() {
  [[ ! -f ".release-please-manifest.premain.json" ]] || {
    echo "branch-version-sync: FAIL (.release-please-manifest.premain.json must be retired)"
    return 1
  }

  local base_ref="${GITHUB_BASE_REF:-}"
  local head_ref="${GITHUB_HEAD_REF:-}"
  local ref_name="${GITHUB_REF_NAME:-}"
  local branch="${base_ref:-${ref_name:-}}"
  if [[ -z "${branch}" ]]; then
    branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  fi

  local sync_mode="skip"
  if [[ "${branch}" == "premain" ]]; then
    sync_mode="premain"
  elif [[ "${branch}" == "main" && "${head_ref}" == "premain" ]]; then
    sync_mode="promotion"
  fi

  if [[ "${sync_mode}" == "skip" ]]; then
    echo "branch-version-sync: SKIP"
    return 0
  fi

  git_fetch_retry origin main

  local main_stable candidate
  main_stable="$(git_ref_json_value origin/main .release-please-manifest.json '.')"
  candidate="$(json_file_value .release-please-manifest.json '.')"

  if [[ -z "${main_stable}" ]]; then
    echo "branch-version-sync: FAIL (could not read origin/main single manifest version)"
    return 1
  fi
  if [[ -z "${candidate}" ]]; then
    echo "branch-version-sync: FAIL (missing checked-out single manifest version)"
    return 1
  fi

  semver_check "origin/main .release-please-manifest.json" "${main_stable}"
  semver_check "checked-out .release-please-manifest.json" "${candidate}"

  MAIN_STABLE="${main_stable}" CANDIDATE_VERSION="${candidate}" python3 - <<'PY'
import os
import sys

main_stable = os.environ["MAIN_STABLE"]
candidate = os.environ["CANDIDATE_VERSION"]


def parse_base(v: str) -> tuple[int, int, int]:
    v = v.strip()
    if v.startswith("v"):
        v = v[1:]
    base = v.split("+", 1)[0].split("-", 1)[0]
    parts = base.split(".")
    if len(parts) != 3:
        raise ValueError(f"invalid semver base: {v}")
    return (int(parts[0]), int(parts[1]), int(parts[2]))

try:
    main_tuple = parse_base(main_stable)
    candidate_tuple = parse_base(candidate)
except Exception as exc:
    print(f"branch-version-sync: FAIL ({exc})")
    sys.exit(1)

if "-" in main_stable:
    print(
        "branch-version-sync: FAIL "
        f"(origin/main single manifest must be stable, got {main_stable})"
    )
    sys.exit(1)

if candidate_tuple < main_tuple:
    print(
        "branch-version-sync: FAIL "
        f"(checked-out release track {candidate} is behind origin/main {main_stable})"
    )
    print(
        "branch-version-sync: hint: backmerge main to staging/premain before advancing the release lane"
    )
    sys.exit(1)
PY

  echo "branch-version-sync: PASS (main=${main_stable}, candidate=${candidate}, mode=${sync_mode})"
}

case "${mode}" in
  alignment)
    run_alignment_check
    ;;
  branch-sync)
    run_branch_sync_check
    ;;
esac
