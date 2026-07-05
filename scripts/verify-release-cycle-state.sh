#!/usr/bin/env bash
set -euo pipefail

# CI-safe local release-cycle checks. This verifier is intentionally local: it
# fails on checked-out forbidden stable state and on missing guardrails, while
# remote branch drift is reported by scripts/watch-release-cycle.sh.

repo_root=""

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-release-cycle-state.sh [--repo-root PATH]

--repo-root PATH  Evaluate a different checkout root (used by policy self-tests).
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      if [[ $# -lt 2 ]]; then
        echo "release-cycle-state: FAIL (--repo-root requires a value)" >&2
        exit 2
      fi
      repo_root="$2"
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

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${repo_root}" ]]; then
  repo_root="$(cd "${script_dir}/.." && pwd)"
fi
source "${script_dir}/lib/release-cycle-core.sh"
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
  "scripts/prepare-release-package-versions.py"
  "scripts/verify-release-package-version-assets.py"
  "scripts/watch-release-cycle.sh"
  ".release-please-manifest.json"
  "ts/package.json"
  "ts/package-lock.json"
  "py/src/theorydb_py/version.json"
)

for path in "${required_files[@]}"; do
  require_file "${path}"
done

if [[ "${failures}" -eq 0 ]]; then
  if ! release_cycle_verify_local_state "${repo_root}"; then
    failures=$((failures + 1))
  fi
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
