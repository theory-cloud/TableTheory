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

Before a breaking release is generated, integration and promotion PRs may carry
the next semantic import major while the release-please manifest still records
the latest stable major. A premain push may accept that state only in the
workflow-fenced RC-PR generation or publication-skip contexts. The pending
transition is accepted only when:
- the module path advances by exactly one major,
- the event/branch context is explicitly allowed,
- CHANGELOG.md has an Unreleased Breaking Changes section, and
- docs/migration/vN.md exists for the pending major.
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

version_info="$(
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
expected = f"{base}/v{major}" if major >= 2 else base
print(f"{major}|{expected}")
PY
)"
case "${version_info}" in
  go-semantic-import:*)
    echo "${version_info}"
    exit 1
    ;;
esac

manifest_major="${version_info%%|*}"
expected="${version_info#*|}"

if [[ "${module_path}" == "${expected}" ]]; then
  echo "go-semantic-import: PASS (manifest=${manifest}, module=${module_path})"
  exit 0
fi

next_major="$((manifest_major + 1))"
next_module="github.com/theory-cloud/tabletheory"
if [[ "${next_major}" -ge 2 ]]; then
  next_module="${next_module}/v${next_major}"
fi
if [[ "${module_path}" != "${next_module}" ]]; then
  fail "manifest ${manifest} requires module ${expected}, got ${module_path}"
fi

target_branch="${GITHUB_BASE_REF:-${GITHUB_REF_NAME:-}}"
if [[ -z "${target_branch}" ]] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  target_branch="$(git branch --show-current)"
fi
if [[ -z "${target_branch}" ]]; then
  fail "manifest ${manifest} requires module ${expected}, got ${module_path}"
fi

pending_context="${TABLETHEORY_PENDING_MAJOR_PREMAIN_CONTEXT:-}"
event_context="local"
if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
  event_context="pr"
  case "${GITHUB_BASE_REF}" in
    staging|premain) ;;
    main)
      fail "pending v${next_major} semantic import transition is not allowed on main"
      ;;
    *)
      fail "pending v${next_major} semantic import transition is allowed only on PRs targeting staging or premain"
      ;;
  esac
elif [[ -n "${GITHUB_REF_NAME:-}" ]]; then
  event_context="push"
  case "${GITHUB_REF_NAME}" in
    staging) ;;
    main)
      fail "pending v${next_major} semantic import transition is not allowed on main"
      ;;
    premain)
      expected_workflow=""
      case "${pending_context}" in
        rc-pr-generation)
          expected_workflow="/.github/workflows/prerelease-pr.yml@"
          ;;
        publication-skip)
          expected_workflow="/.github/workflows/prerelease.yml@"
          ;;
        *)
          fail "pending v${next_major} premain push cannot publish until the manifest is v${next_major}-numbered"
          ;;
      esac
      if [[ "${GITHUB_WORKFLOW_REF:-}" != *"${expected_workflow}"* ]]; then
        fail "pending v${next_major} premain context ${pending_context} requires workflow ${expected_workflow}"
      fi
      ;;
    *)
      fail "pending v${next_major} semantic import transition is not allowed on push ref ${GITHUB_REF_NAME}"
      ;;
  esac
else
  case "${target_branch}" in
    main|premain)
      fail "pending v${next_major} semantic import transition is not allowed on local ${target_branch}"
      ;;
  esac
fi

[[ -f "CHANGELOG.md" ]] || fail "pending v${next_major} transition requires CHANGELOG.md"
python3 - "${next_major}" <<'PY' || exit 1
import re
import sys
from pathlib import Path

major = sys.argv[1]
text = Path("CHANGELOG.md").read_text(encoding="utf-8")
match = re.search(
    r"(?ms)^## \[?Unreleased\]?\s*$\s*(.*?)(?=^## |\Z)",
    text,
)
if not match or not re.search(
    r"(?ms)^### Breaking Changes\s*\n\s*\S",
    match.group(1),
):
    print(
        f"go-semantic-import: FAIL (pending v{major} transition requires a non-empty "
        "Unreleased Breaking Changes section)"
    )
    raise SystemExit(1)
PY

migration="docs/migration/v${next_major}.md"
[[ -f "${migration}" ]] || fail "pending v${next_major} transition requires ${migration}"

echo "go-semantic-import: PASS (pending-major-transition=${manifest_major}->${next_major}, manifest=${manifest}, module=${module_path}, target=${target_branch:-local}, event=${event_context}, context=${pending_context:-none})"
