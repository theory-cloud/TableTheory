#!/usr/bin/env bash
set -euo pipefail

# Verifies Go semantic import versioning for TableTheory's GitHub-release tag
# lane. Go modules at v2+ must carry the major version suffix in go.mod.

repo_root=""

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-go-semantic-import-version.sh [--repo-root PATH]

Checks .release-please-manifest.json against the root go.mod module path.
For manifest major 0/1, the module path must be github.com/theory-cloud/tabletheory.
For manifest major N>=2, it must be github.com/theory-cloud/tabletheory/vN.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      if [[ $# -lt 2 ]]; then
        echo "go-semantic-import: FAIL (--repo-root requires a value)" >&2
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
      echo "go-semantic-import: FAIL (unknown argument: $1)" >&2
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

fail() {
  echo "go-semantic-import: FAIL ($1)"
  exit 1
}

[[ -f ".release-please-manifest.json" ]] || fail "missing .release-please-manifest.json"
[[ -f "go.mod" ]] || fail "missing go.mod"

manifest="$(
  python3 - <<'PY'
import json
from pathlib import Path

data = json.loads(Path(".release-please-manifest.json").read_text(encoding="utf-8"))
value = data.get(".", "") if isinstance(data, dict) else ""
print(value if isinstance(value, str) else "")
PY
)"
[[ -n "${manifest}" ]] || fail "missing single manifest version"

module_path="$(awk '$1 == "module" { print $2; exit }' go.mod)"
[[ -n "${module_path}" ]] || fail "go.mod is missing a module directive"

expected="$(
  MANIFEST_VERSION="${manifest}" python3 - <<'PY'
import os
import re
import sys

version = os.environ["MANIFEST_VERSION"].strip()
match = re.fullmatch(r"v?([0-9]+)\.[0-9]+\.[0-9]+(?:-rc(?:\.[0-9]+)?)?", version)
if not match:
    print(f"go-semantic-import: FAIL (manifest has invalid release version {version!r})", file=sys.stderr)
    raise SystemExit(2)

major = int(match.group(1))
base = "github.com/theory-cloud/tabletheory"
print(f"{base}/v{major}" if major >= 2 else base)
PY
)"
case "${expected}" in
  go-semantic-import:*)
    echo "${expected}"
    exit 1
    ;;
esac

if [[ "${module_path}" != "${expected}" ]]; then
  fail "manifest ${manifest} requires module ${expected}, got ${module_path}"
fi

echo "go-semantic-import: PASS (manifest=${manifest}, module=${module_path})"
