#!/usr/bin/env bash
# Purpose: prove a published artifact installs and works in a clean consumer.
#
# A package suite that imports the repository's own sources proves the sources
# compile; it does not prove what a consumer receives. This check builds the
# real artifact, installs it into an empty directory with nothing from this
# repository on the path, and runs one real call path against DynamoDB Local
# through the published entry points. The receipt is the gate record the
# collector binds: the log names the interpreter, the artifact and the package
# version it exercised, so a record cannot claim a runtime it did not run on.
#
# The call path is never skipped. A missing endpoint fails closed, because a
# skipped install would record a PASS for an artifact nobody installed.
#
# Usage: scripts/verify-consumer-install.sh --language ts|py
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

language=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --language)
      language="${2:-}"
      shift 2
      ;;
    --help|-h)
      cat <<'USAGE'
usage: scripts/verify-consumer-install.sh --language ts|py

Builds the TypeScript tarball or the Python wheel, installs it into an empty
consumer directory and runs one real call path against DynamoDB Local
(DYNAMODB_ENDPOINT, default http://localhost:8000).
USAGE
      exit 0
      ;;
    *)
      echo "consumer-install: FAIL (unknown argument: $1)" >&2
      exit 2
      ;;
  esac
done

case "${language}" in
  ts|py) ;;
  *)
    echo "consumer-install: FAIL (--language must be ts or py; got '${language}')" >&2
    exit 2
    ;;
esac

export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"

work="$(mktemp -d "${TMPDIR:-/tmp}/tabletheory-consumer-install.XXXXXX")"
# The consumer lives outside the checkout: nothing in the work tree may appear on
# the module or import path, and a leftover directory cannot be mistaken for an
# input by the next record.
trap 'rm -rf "${work}"' EXIT

install_ts() {
  command -v node >/dev/null 2>&1 || {
    echo "consumer-install(ts): FAIL (node not found)" >&2
    exit 1
  }
  command -v npm >/dev/null 2>&1 || {
    echo "consumer-install(ts): FAIL (npm not found)" >&2
    exit 1
  }
  if [[ ! -d ts/node_modules ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  # The tarball carries dist/, so the tree that is packed must be built first.
  # `npm run check` builds it in the ts gate, and a direct run builds it here.
  if [[ ! -d ts/dist ]]; then
    npm --prefix ts run build
  fi

  ( cd ts && npm pack --pack-destination "${work}" >/dev/null )
  tarball="$(find "${work}" -maxdepth 1 -name '*.tgz' -print -quit)"
  if [[ -z "${tarball}" ]]; then
    echo "consumer-install(ts): FAIL (npm pack produced no tarball)" >&2
    exit 1
  fi

  consumer="${work}/consumer-ts"
  mkdir -p "${consumer}"
  # A consumer of the release installs the asset, so peer dependencies resolve
  # exactly as they do for a new project. Lifecycle scripts stay disabled: this
  # check installs untrusted-by-default dependency code.
  cat >"${consumer}/package.json" <<'JSON'
{
  "name": "tabletheory-clean-consumer-ts",
  "private": true,
  "type": "module"
}
JSON
  ( cd "${consumer}" && npm install --no-audit --no-fund --ignore-scripts --save-exact "${tarball}" >/dev/null )
  cp scripts/fixtures/consumer-install/ts/consumer.mjs "${consumer}/consumer.mjs"

  echo "consumer-install(ts): node $(node --version), npm $(npm --version)"
  echo "consumer-install(ts): artifact $(basename "${tarball}")"
  ( cd "${consumer}" && node consumer.mjs )
}

install_py() {
  command -v python3 >/dev/null 2>&1 || {
    echo "consumer-install(py): FAIL (python3 not found)" >&2
    exit 1
  }
  command -v uv >/dev/null 2>&1 || {
    echo "consumer-install(py): FAIL (uv not found)" >&2
    exit 1
  }

  uv --directory py build --wheel --out-dir "${work}/dist" >/dev/null
  wheel="$(find "${work}/dist" -maxdepth 1 -name '*.whl' -print -quit)"
  if [[ -z "${wheel}" ]]; then
    echo "consumer-install(py): FAIL (build produced no wheel)" >&2
    exit 1
  fi

  consumer="${work}/consumer-py"
  mkdir -p "${consumer}"
  python3 -m venv "${consumer}/venv"
  "${consumer}/venv/bin/python" -m pip install --quiet --disable-pip-version-check "${wheel}"
  cp scripts/fixtures/consumer-install/py/consumer.py "${consumer}/consumer.py"

  echo "consumer-install(py): python $("${consumer}/venv/bin/python" -c 'import sys; print(sys.version.split()[0])')"
  echo "consumer-install(py): artifact $(basename "${wheel}")"
  ( cd "${consumer}" && ./venv/bin/python consumer.py )
}

case "${language}" in
  ts) install_ts ;;
  py) install_py ;;
esac

echo "consumer-install(${language}): PASS"
