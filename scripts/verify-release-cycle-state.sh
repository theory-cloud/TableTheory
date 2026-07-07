#!/usr/bin/env bash
set -euo pipefail

# CI-safe local release-cycle checks. By default this verifier validates the
# checkout that contains the trusted script. Release Hygiene may pass an explicit
# target checkout root after same-repository provenance verification so trusted
# base scripts inspect PR-head files without executing PR-head scripts. Remote
# branch drift is reported by scripts/watch-release-cycle.sh.

failures=0

fail() {
  echo "release-cycle-state: FAIL ($1)"
  failures=$((failures + 1))
}

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-release-cycle-state.sh [--repo-root ABSOLUTE_PATH]

Options:
  --repo-root ABSOLUTE_PATH  Validate this checkout root instead of the script checkout.
  -h, --help                Show this help.
USAGE
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
script_repo_root="$(cd "${script_dir}/.." && pwd -P)"
source "${script_dir}/lib/release-cycle-core.sh"

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
  "go.mod"
  "scripts/prepare-release-package-versions.py"
  "scripts/verify-go-semantic-import-version.sh"
  "scripts/verify-release-package-version-assets.py"
  "scripts/watch-release-cycle.sh"
  ".release-please-manifest.json"
  "ts/package.json"
  "ts/package-lock.json"
  "py/src/tabletheory_py/version.json"
)

for path in "${required_files[@]}"; do
  require_file "${path}"
done

if [[ "${failures}" -eq 0 ]]; then
  if ! bash "${script_dir}/verify-go-semantic-import-version.sh" --repo-root "${repo_root}"; then
    failures=$((failures + 1))
  fi
fi

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
