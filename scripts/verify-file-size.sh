#!/usr/bin/env bash
set -euo pipefail

# Canonical file-size verifier. Parameterized language modes replace the former
# verify-go-file-size.sh, verify-ts-file-size.sh, and verify-python-file-size.sh
# wrappers while preserving the rubric MAI-1 output identity.

language="all"

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-file-size.sh [--language go|typescript|python|all]
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --language)
      language="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "file-size: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "${language}" in
  go|typescript|python|all) ;;
  *)
    echo "file-size: FAIL (--language must be go, typescript, python, or all; got ${language})"
    exit 2
    ;;
esac

verify_language_file_size() {
  local label="$1"
  local max_lines="$2"
  shift 2
  local files
  files="$@"

  local failures=0

  if [[ -z "${files}" ]]; then
    case "${label}" in
      go-file-size)
        echo "${label}: FAIL (no production Go files found)"
        return 1
        ;;
      ts-file-size)
        echo "${label}: SKIP (no production TS files found)"
        return 0
        ;;
      py-file-size)
        echo "${label}: SKIP (no production Python files found)"
        return 0
        ;;
    esac
  fi

  while IFS= read -r f; do
    if [[ -z "${f}" || ! -f "${f}" ]]; then
      continue
    fi
    lines="$(wc -l <"${f}" | tr -d ' ')"
    if [[ "${lines}" -gt "${max_lines}" ]]; then
      echo "${label}: ${f}: ${lines} lines (max ${max_lines})"
      failures=$((failures + 1))
    fi
  done <<<"${files}"

  if [[ "${failures}" -ne 0 ]]; then
    echo "${label}: FAIL (${failures} file(s) exceed ${max_lines} lines)"
    echo "${label}: see docs/development/planning/theorydb-maintainability-roadmap.md"
    return 1
  fi

  echo "${label}: PASS (max ${max_lines})"
}

verify_go() {
  local files
  files="$(git ls-files '*.go' | grep -Ev '(^|/)examples/|(^|/)tests/|(^|/)scripts/|(^|/)pkg/(mocks|testing)/|_test\.go$' || true)"
  verify_language_file_size "go-file-size" 2500 "${files}"
}

verify_typescript() {
  local files
  files="$(git ls-files 'ts/src/**/*.ts' 'ts/src/*.ts' 2>/dev/null || true)"
  verify_language_file_size "ts-file-size" 1500 "${files}"
}

verify_python() {
  local files
  files="$(git ls-files 'py/src/**/*.py' 'py/src/*.py' 2>/dev/null || true)"
  verify_language_file_size "py-file-size" 1500 "${files}"
}

case "${language}" in
  go)
    verify_go
    ;;
  typescript)
    verify_typescript
    ;;
  python)
    verify_python
    ;;
  all)
    verify_go
    verify_typescript
    verify_python
    echo "file-size: PASS"
    ;;
esac
