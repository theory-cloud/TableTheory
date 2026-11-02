#!/usr/bin/env bash
# Pinned Node dependencies for the Lambda runtime scanner.
#
# The checker resolves the TypeScript compiler and the YAML parser from
# ts/package.json, so both must exist before SEC-2 runs the checker. A clean
# checkout has no ts/node_modules; install them from the committed lockfile.
# There is no host fallback and no unpinned package: `npm ci` reads
# ts/package-lock.json and ignores install scripts.
#
# Sourced by scripts/verify-lambda-runtime-deprecations.sh and
# scripts/test-lambda-runtime-deprecations-policy.sh.

ensure_node_scanner_deps() {
  local repo_root="${1:-}"
  if [[ -z "${repo_root}" ]]; then
    echo "lambda-scanner-deps: FAIL (missing repository root)" >&2
    return 1
  fi

  local typescript="${repo_root}/ts/node_modules/typescript/lib/typescript.js"
  local yaml="${repo_root}/ts/node_modules/yaml/dist/index.js"
  if [[ -f "${typescript}" && -f "${yaml}" ]]; then
    return 0
  fi

  if [[ ! -f "${repo_root}/ts/package.json" || ! -f "${repo_root}/ts/package-lock.json" ]]; then
    echo "lambda-scanner-deps: FAIL (missing ts/package.json or ts/package-lock.json)" >&2
    return 1
  fi

  if ! command -v npm >/dev/null 2>&1; then
    echo "lambda-scanner-deps: BLOCKED (npm not found; cannot install pinned scanner dependencies)" >&2
    return 1
  fi

  if ! npm --prefix "${repo_root}/ts" ci --ignore-scripts >/dev/null 2>&1; then
    echo "lambda-scanner-deps: FAIL (npm ci --prefix ts --ignore-scripts failed)" >&2
    return 1
  fi

  if [[ ! -f "${typescript}" || ! -f "${yaml}" ]]; then
    echo "lambda-scanner-deps: FAIL (pinned TypeScript/YAML packages missing after npm ci)" >&2
    return 1
  fi
}
