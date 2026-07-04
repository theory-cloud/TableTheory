#!/usr/bin/env bash
set -euo pipefail

# Canonical release-version verifier.
#
# Default mode preserves the branch-version sync guard used on premain/main
# release-lane gates. --alignment preserves the former verify-version-alignment
# behavior for local build/package version checks.

mode="branch-sync"
repo_root=""

usage() {
  cat <<'USAGE'
Usage:
  bash scripts/verify-branch-version-sync.sh [--alignment] [--repo-root PATH]

Modes:
  default       Verify premain/main branch release-version sync.
  --alignment  Verify checked-out SDK/package versions align with the selected
               release-please manifest. Replaces verify-version-alignment.sh.
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
  git show "${ref}:${path}" | python3 - "${expr}" <<'PY'
import json
import sys

expr = sys.argv[1]
data = json.load(sys.stdin)
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

run_alignment_check() {
  local base_ref="${GITHUB_BASE_REF:-}"
  local ref_name="${GITHUB_REF_NAME:-}"
  local branch="${base_ref:-${ref_name:-}}"
  if [[ -z "${branch}" ]]; then
    branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  fi

  local ts_version=""
  if [[ -f "ts/package.json" ]]; then
    ts_version="$(json_file_value "ts/package.json" "version")"
  fi

  local py_version=""
  if [[ -f "py/src/theorydb_py/version.json" ]]; then
    py_version="$(json_file_value "py/src/theorydb_py/version.json" "version")"
  fi

  if [[ -z "${ts_version}" && -z "${py_version}" ]]; then
    echo "version-alignment: SKIP (no versioned packages found)"
    return 0
  fi

  local observed_version="${ts_version:-${py_version}}"
  local manifest=""

  case "${branch}" in
    main)
      manifest=".release-please-manifest.json"
      ;;
    premain)
      manifest=".release-please-manifest.premain.json"
      ;;
    *)
      # Local runs won't have PR context (no `GITHUB_BASE_REF`). Infer intent from the observed package version:
      # - prereleases (e.g., `-rc` or `-rc.N`) validate against the premain manifest
      # - stable versions validate against the main manifest
      if [[ "${observed_version}" == *"-rc"* && -f ".release-please-manifest.premain.json" ]]; then
        manifest=".release-please-manifest.premain.json"
      else
        manifest=".release-please-manifest.json"
      fi
      ;;
  esac

  if [[ ! -f "${manifest}" ]]; then
    echo "version-alignment: FAIL (missing ${manifest})"
    return 1
  fi

  local expected
  expected="$(json_file_value "${manifest}" ".")"
  if [[ -z "${expected}" ]]; then
    echo "version-alignment: FAIL (missing '.' version in ${manifest})"
    return 1
  fi

  if [[ "${observed_version}" != "${expected}" ]]; then
    if [[ "${branch}" == "main" && "${observed_version}" == *"-rc"* && -f ".release-please-manifest.premain.json" ]]; then
      # Promotion PRs (premain -> main) and immediate post-merge pushes may still carry prerelease versions.
      # Allow alignment against the premain prerelease manifest; the subsequent release PR on `main` will
      # enforce stable alignment.
      expected="$(json_file_value ".release-please-manifest.premain.json" ".")"
      manifest=".release-please-manifest.premain.json"
    elif [[ "${branch}" == "premain" && "${observed_version}" != *"-rc"* && -f ".release-please-manifest.json" ]]; then
      # Promotion PRs (staging -> premain) and immediate post-merge pushes start from the latest stable
      # baseline. Allow alignment against the stable manifest; the subsequent prerelease PR on `premain`
      # will bump versions and enforce prerelease alignment.
      expected="$(json_file_value ".release-please-manifest.json" ".")"
      manifest=".release-please-manifest.json"
    fi
  fi

  if [[ -n "${ts_version}" ]]; then
    if [[ "${ts_version}" != "${expected}" ]]; then
      echo "version-alignment: FAIL (ts/package.json ${ts_version} != ${expected} from ${manifest})"
      return 1
    fi

    local lock_version pkg_lock_version
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

    if [[ "${lock_version}" != "${expected}" ]]; then
      echo "version-alignment: FAIL (ts/package-lock.json ${lock_version} != ${expected})"
      return 1
    fi

    if [[ "${pkg_lock_version}" != "${expected}" ]]; then
      echo "version-alignment: FAIL (ts/package-lock.json packages[''].version ${pkg_lock_version} != ${expected})"
      return 1
    fi
  fi

  if [[ -n "${py_version}" && "${py_version}" != "${expected}" ]]; then
    echo "version-alignment: FAIL (py/src/theorydb_py/version.json ${py_version} != ${expected} from ${manifest})"
    return 1
  fi

  echo "version-alignment: PASS (${expected})"
}

run_branch_sync_check() {
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

  for f in ".release-please-manifest.json" ".release-please-manifest.premain.json"; do
    if [[ ! -f "${f}" ]]; then
      echo "branch-version-sync: FAIL (missing ${f})"
      return 1
    fi
  done

  git_fetch_retry origin main

  local main_stable
  main_stable="$(git_ref_json_value origin/main .release-please-manifest.json '.')"
  if [[ -z "${main_stable}" ]]; then
    echo "branch-version-sync: FAIL (could not read origin/main stable version)"
    return 1
  fi

  local premain_stable=""
  local premain_version=""

  if [[ "${sync_mode}" == "premain" ]]; then
    premain_stable="$(json_file_value ".release-please-manifest.json" ".")"
    premain_version="$(json_file_value ".release-please-manifest.premain.json" ".")"
  else
    git_fetch_retry origin premain
    premain_stable="$(git_ref_json_value origin/premain .release-please-manifest.json '.')"
    premain_version="$(git_ref_json_value origin/premain .release-please-manifest.premain.json '.')"
  fi

  if [[ -z "${premain_stable}" ]]; then
    echo "branch-version-sync: FAIL (missing premain stable manifest version)"
    return 1
  fi

  if [[ -z "${premain_version}" ]]; then
    echo "branch-version-sync: FAIL (missing premain prerelease manifest version)"
    return 1
  fi

  if [[ "${premain_stable}" != "${main_stable}" ]]; then
    echo "branch-version-sync: FAIL (premain .release-please-manifest.json ${premain_stable} != origin/main ${main_stable})"
    echo "branch-version-sync: hint: inspect the release.yml post-stable baseline sync for premain/staging"
    return 1
  fi

  MAIN_STABLE="${main_stable}" PREMAIN_VERSION="${premain_version}" python3 - <<'PY'
import os
import sys

main_stable = os.environ["MAIN_STABLE"]
premain_version = os.environ["PREMAIN_VERSION"]


def parse_base(v: str) -> tuple[int, int, int]:
    v = v.strip()
    if v.startswith("v"):
        v = v[1:]
    v = v.split("+", 1)[0]
    base = v.split("-", 1)[0]
    parts = base.split(".")
    if len(parts) != 3:
        raise ValueError(f"invalid semver base: {v}")
    return (int(parts[0]), int(parts[1]), int(parts[2]))


try:
    main_tuple = parse_base(main_stable)
    premain_tuple = parse_base(premain_version)
except Exception as exc:
    print(f"branch-version-sync: FAIL ({exc})")
    sys.exit(1)

if premain_tuple < main_tuple:
    print(
        "branch-version-sync: FAIL "
        f"(premain prerelease track {premain_version} is behind main {main_stable})"
    )
    print(
        "branch-version-sync: hint: release.yml should reset "
        ".release-please-manifest.premain.json after cutting a release on main"
    )
    sys.exit(1)
PY

  echo "branch-version-sync: PASS (main=${main_stable}, premain=${premain_version})"
}

case "${mode}" in
  alignment)
    run_alignment_check
    ;;
  branch-sync)
    run_branch_sync_check
    ;;
esac
