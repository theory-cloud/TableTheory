#!/usr/bin/env bash
set -euo pipefail

repo="${GITHUB_REPOSITORY:-theory-cloud/TableTheory}"
sha="${GITHUB_SHA:-}"
base="premain"
json=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) repo="$2"; shift 2 ;;
    --sha) sha="$2"; shift 2 ;;
    --base) base="$2"; shift 2 ;;
    --prs-json) json="$2"; shift 2 ;;
    *) echo "resolve-prerelease-release-as: unknown argument $1" >&2; exit 2 ;;
  esac
done
[[ -n "${sha}" ]] || { echo "resolve-prerelease-release-as: --sha is required" >&2; exit 2; }
[[ -n "${json}" ]] || json="$(gh api "repos/${repo}/commits/${sha}/pulls")"
printf '%s' "${json}" | python3 -c '
import json, re, sys
base = sys.argv[1]
rc = re.compile(r"(?im)^[ \t]*Release-As:[ \t]*v?([0-9]+\.[0-9]+\.[0-9]+-rc(?:\.[0-9]+)?)[ \t]*$")
versions = set()
for pr in json.load(sys.stdin):
    if pr.get("base", {}).get("ref") == base and pr.get("merged_at"):
        versions.update(rc.findall("\n".join((pr.get("title") or "", pr.get("body") or ""))))
if len(versions) > 1:
    raise SystemExit("resolve-prerelease-release-as: multiple RC Release-As footers found")
print(next(iter(versions), ""))
' "${base}"
