#!/usr/bin/env bash
set -euo pipefail

# Canonical formatting verifier. The --language go mode replaces the former
# fmt-check.sh helper while the default preserves the rubric CON-1 gate.

language="all"

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-formatting.sh [--language go|typescript|python|all]
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
      echo "formatting: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "${language}" in
  go|typescript|python|all) ;;
  *)
    echo "formatting: FAIL (--language must be go, typescript, python, or all; got ${language})"
    exit 2
    ;;
esac

verify_go_formatting() {
  local files unformatted
  files="$(git ls-files '*.go')"
  if [[ -z "${files}" ]]; then
    return 0
  fi

  unformatted="$(gofmt -l ${files})"
  if [[ -n "${unformatted}" ]]; then
    echo "gofmt is required on the following files:"
    echo "${unformatted}"
    return 1
  fi

  echo "gofmt: clean"
}

verify_typescript_formatting() {
  if [[ -f "ts/package.json" ]]; then
    if [[ ! -d "ts/node_modules" ]]; then
      bash scripts/verify-typescript-deps.sh
    fi
    npm --prefix ts run format:check
  fi
}

verify_python_formatting() {
  if [[ -f "py/pyproject.toml" ]]; then
    if [[ ! -d "py/.venv" ]]; then
      bash scripts/verify-python-deps.sh
    fi
    uv --directory py run ruff format --check .
  fi
}

case "${language}" in
  go)
    verify_go_formatting
    ;;
  typescript)
    verify_typescript_formatting
    ;;
  python)
    verify_python_formatting
    ;;
  all)
    verify_go_formatting
    verify_typescript_formatting
    verify_python_formatting
    echo "formatting: PASS"
    ;;
esac
