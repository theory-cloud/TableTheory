#!/usr/bin/env bash
set -euo pipefail

base_ref="${GITHUB_BASE_REF:-}"
ref_name="${GITHUB_REF_NAME:-}"
branch="${base_ref:-${ref_name:-}}"
if [[ -z "${branch}" ]]; then
  branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
fi

ts_version=""
if [[ -f "ts/package.json" ]]; then
  ts_version="$(
    python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("ts/package.json").read_text(encoding="utf-8"))
print(data.get("version", ""))
PY
  )"
fi

py_version=""
if [[ -f "py/src/theorydb_py/version.json" ]]; then
  py_version="$(
    python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("py/src/theorydb_py/version.json").read_text(encoding="utf-8"))
print(data.get("version", ""))
PY
  )"
fi

if [[ -z "${ts_version}" && -z "${py_version}" ]]; then
  echo "version-alignment: SKIP (no versioned packages found)"
  exit 0
fi

observed_version="${ts_version:-${py_version}}"

manifest=""

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
  exit 1
fi

expected="$(
  python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("${manifest}").read_text(encoding="utf-8"))
print(data.get(".", ""))
PY
)"

if [[ -z "${expected}" ]]; then
  echo "version-alignment: FAIL (missing '.' version in ${manifest})"
  exit 1
fi

if [[ "${observed_version}" != "${expected}" ]]; then
  if [[ "${branch}" == "main" && "${observed_version}" == *"-rc"* && -f ".release-please-manifest.premain.json" ]]; then
    # Promotion PRs (premain -> main) and immediate post-merge pushes may still carry prerelease versions.
    # Allow alignment against the premain prerelease manifest; the subsequent release PR on `main` will
    # enforce stable alignment.
    expected="$(
      python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path(".release-please-manifest.premain.json").read_text(encoding="utf-8"))
print(data.get(".", ""))
PY
)"
    manifest=".release-please-manifest.premain.json"
  elif [[ "${branch}" == "premain" && "${observed_version}" != *"-rc"* && -f ".release-please-manifest.json" ]]; then
    # Promotion PRs (staging -> premain) and immediate post-merge pushes start from the latest stable
    # baseline. Allow alignment against the stable manifest; the subsequent prerelease PR on `premain`
    # will bump versions and enforce prerelease alignment.
    expected="$(
      python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path(".release-please-manifest.json").read_text(encoding="utf-8"))
print(data.get(".", ""))
PY
)"
    manifest=".release-please-manifest.json"
  fi
fi

if [[ -n "${ts_version}" ]]; then
  if [[ "${ts_version}" != "${expected}" ]]; then
    echo "version-alignment: FAIL (ts/package.json ${ts_version} != ${expected} from ${manifest})"
    exit 1
  fi

  lock_version="$(
    python3 - <<PY
import json
from pathlib import Path

data = json.loads(Path("ts/package-lock.json").read_text(encoding="utf-8"))
print(data.get("version", ""))
PY
  )"

  pkg_lock_version="$(
    python3 - <<PY
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
    exit 1
  fi

  if [[ "${pkg_lock_version}" != "${expected}" ]]; then
    echo "version-alignment: FAIL (ts/package-lock.json packages[''].version ${pkg_lock_version} != ${expected})"
    exit 1
  fi
fi

if [[ -n "${py_version}" ]]; then
  if [[ "${py_version}" != "${expected}" ]]; then
    echo "version-alignment: FAIL (py/src/theorydb_py/version.json ${py_version} != ${expected} from ${manifest})"
    exit 1
  fi
fi

echo "version-alignment: PASS (${expected})"
