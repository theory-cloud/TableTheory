#!/usr/bin/env bash
set -euo pipefail

# Read-only release-cycle watch helper.
#
# Default mode reports PASS/WARN/FAIL findings and exits 0 so it can be used
# during incident review without masking all findings behind the first failure.
# Use --strict in CI or before merge/release to exit non-zero on FAIL findings.

usage() {
  cat <<'USAGE'
Usage: bash scripts/watch-release-cycle.sh [--strict] [--tag vX.Y.Z]

Options:
  --strict   Exit non-zero if any FAIL finding is reported.
  --tag TAG  Check that an existing GitHub release has non-source assets.
  -h, --help Show this help.

This command reads local refs and, when gh is authenticated, open PR/release
metadata. It does not fetch, push, merge, tag, publish, edit, or delete anything.
USAGE
}

strict=0
tag_name=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict)
      strict=1
      shift
      ;;
    --tag)
      if [[ $# -lt 2 ]]; then
        echo "watch-release-cycle: FAIL (--tag requires a value)" >&2
        exit 2
      fi
      tag_name="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "watch-release-cycle: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

fail_count=0
warn_count=0

pass() {
  echo "watch-release-cycle: PASS $1"
}

warn() {
  echo "watch-release-cycle: WARN $1"
  warn_count=$((warn_count + 1))
}

fail() {
  echo "watch-release-cycle: FAIL $1"
  fail_count=$((fail_count + 1))
}

json_value_at_ref() {
  local ref="$1"
  local path="$2"
  local expr="$3"

  git show "${ref}:${path}" 2>/dev/null | python3 -c '
import json
import sys

expr = sys.argv[1]
data = json.load(sys.stdin)
if expr == ".":
    print(data.get(".", "") if isinstance(data, dict) else "")
    raise SystemExit(0)
value = data
for part in expr.split("."):
    if not part:
        continue
    if isinstance(value, dict):
        value = value.get(part, "")
    else:
        value = ""
print(value if isinstance(value, str) else "")
' "${expr}"
}

toolchain_at_ref() {
  local ref="$1"
  local path="$2"
  git show "${ref}:${path}" 2>/dev/null | awk '$1 == "toolchain" { print $2; exit }'
}

semver_base() {
  python3 - "$1" <<'PY'
import re
import sys

v = sys.argv[1].strip()
m = re.match(r"^v?(\d+)\.(\d+)\.(\d+)", v)
if not m:
    raise SystemExit(1)
print(".".join(m.group(i) for i in range(1, 4)))
PY
}

semver_lt() {
  python3 - "$1" "$2" <<'PY'
import re
import sys


def parse(v: str) -> tuple[int, int, int]:
    m = re.match(r"^v?(\d+)\.(\d+)\.(\d+)", v.strip())
    if not m:
        raise SystemExit(2)
    return tuple(int(m.group(i)) for i in range(1, 4))

raise SystemExit(0 if parse(sys.argv[1]) < parse(sys.argv[2]) else 1)
PY
}

refs=(origin/main origin/premain origin/staging)
for ref in "${refs[@]}"; do
  if ! git rev-parse --verify --quiet "${ref}" >/dev/null; then
    warn "${ref} is not available locally; run git fetch origin before relying on this report"
  fi
done

main_stable=""
premain_stable=""
premain_prerelease=""
staging_stable=""
main_pending_promotion=0
main_pending_version=""

if git rev-parse --verify --quiet origin/main >/dev/null; then
  main_stable="$(json_value_at_ref origin/main .release-please-manifest.json '.')"
  main_premain="$(json_value_at_ref origin/main .release-please-manifest.premain.json '.')"
  main_ts="$(json_value_at_ref origin/main ts/package.json version)"
  main_ts_lock="$(json_value_at_ref origin/main ts/package-lock.json version)"
  main_ts_lock_pkg="$(git show origin/main:ts/package-lock.json 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("packages", {}).get("", {}).get("version", ""))')"
  main_py="$(json_value_at_ref origin/main py/src/theorydb_py/version.json version)"

  main_pending_candidate=1
  main_pending_version="${main_premain}"
  for version in "${main_premain}" "${main_ts}" "${main_ts_lock}" "${main_ts_lock_pkg}" "${main_py}"; do
    if [[ -z "${version}" || "${version}" == *-* || "${version}" != "${main_pending_version}" ]]; then
      main_pending_candidate=0
    fi
  done

  if [[ "${main_pending_candidate}" -eq 1 && -n "${main_stable}" && "${main_stable}" != *-* ]] && semver_lt "${main_stable}" "${main_pending_version}"; then
    main_pending_promotion=1
    warn "origin/main pending stable promotion: stable manifest ${main_stable}, normalized files ${main_pending_version}"
  fi

  if [[ "${main_stable}" == *-rc* ]]; then
    fail "origin/main stable manifest is prerelease (${main_stable})"
  else
    pass "origin/main stable manifest is ${main_stable:-<missing>}"
  fi

  if [[ -z "${main_premain}" ]]; then
    fail "origin/main .release-please-manifest.premain.json version is missing"
  elif [[ "${main_pending_promotion}" -eq 1 && "${main_premain}" == "${main_pending_version}" ]]; then
    warn "origin/main .release-please-manifest.premain.json is pending stable promotion (${main_premain}; stable manifest ${main_stable})"
  elif [[ "${main_premain}" == *-rc* ]]; then
    fail "origin/main .release-please-manifest.premain.json remains prerelease (${main_premain})"
  elif [[ -n "${main_stable}" && "${main_premain}" != "${main_stable}" ]]; then
    fail "origin/main .release-please-manifest.premain.json ${main_premain} != stable manifest ${main_stable}"
  else
    pass "origin/main .release-please-manifest.premain.json is stable (${main_premain})"
  fi

  for item in \
    "ts/package.json:${main_ts}" \
    "ts/package-lock.json:${main_ts_lock}" \
    "ts/package-lock.json packages['']:${main_ts_lock_pkg}" \
    "py/src/theorydb_py/version.json:${main_py}"; do
    label="${item%%:*}"
    version="${item#*:}"
    if [[ -z "${version}" ]]; then
      fail "origin/main ${label} version is missing"
    elif [[ "${version}" == *-rc* ]]; then
      fail "origin/main ${label} remains prerelease (${version})"
    elif [[ "${main_pending_promotion}" -eq 1 && "${version}" == "${main_pending_version}" ]]; then
      warn "origin/main ${label} is pending stable promotion (${version}; stable manifest ${main_stable})"
    elif [[ -n "${main_stable}" && "${version}" != "${main_stable}" ]]; then
      fail "origin/main ${label} ${version} != stable manifest ${main_stable}"
    else
      pass "origin/main ${label} is stable (${version})"
    fi
  done

  main_go="$(toolchain_at_ref origin/main go.mod)"
  main_example_go="$(toolchain_at_ref origin/main examples/multi-tenant/go.mod)"
  for item in "go.mod:${main_go}" "examples/multi-tenant/go.mod:${main_example_go}"; do
    label="${item%%:*}"
    version="${item#*:}"
    if [[ "${version}" == "go1.26.3" ]]; then
      fail "origin/main ${label} still observes vulnerable toolchain ${version}"
    elif [[ -z "${version}" ]]; then
      fail "origin/main ${label} has no toolchain line"
    else
      pass "origin/main ${label} toolchain is ${version}"
    fi
  done
fi

if git rev-parse --verify --quiet origin/premain >/dev/null; then
  premain_stable="$(json_value_at_ref origin/premain .release-please-manifest.json '.')"
  premain_prerelease="$(json_value_at_ref origin/premain .release-please-manifest.premain.json '.')"

  if [[ -n "${main_stable}" && -n "${premain_stable}" && "${premain_stable}" != "${main_stable}" ]]; then
    fail "origin/premain stable manifest ${premain_stable} != origin/main ${main_stable}"
  else
    pass "origin/premain stable manifest matches origin/main (${premain_stable:-<missing>})"
  fi

  if [[ -n "${main_stable}" && -n "${premain_prerelease}" ]]; then
    premain_base="$(semver_base "${premain_prerelease}" || true)"
    main_base="$(semver_base "${main_stable}" || true)"
    if [[ -n "${premain_base}" && -n "${main_base}" ]] && semver_lt "${premain_base}" "${main_base}"; then
      fail "origin/premain prerelease track ${premain_prerelease} is behind origin/main ${main_stable}"
    else
      pass "origin/premain prerelease track is ${premain_prerelease}"
    fi
  fi

  premain_go="$(toolchain_at_ref origin/premain go.mod)"
  if [[ "${premain_go}" == "go1.26.3" ]]; then
    fail "origin/premain go.mod still observes vulnerable toolchain ${premain_go}"
  elif [[ -n "${premain_go}" ]]; then
    pass "origin/premain go.mod toolchain is ${premain_go}"
  else
    fail "origin/premain go.mod has no toolchain line"
  fi
fi

if git rev-parse --verify --quiet origin/staging >/dev/null; then
  staging_stable="$(json_value_at_ref origin/staging .release-please-manifest.json '.')"
  if [[ -n "${main_stable}" && -n "${staging_stable}" && "${staging_stable}" != "${main_stable}" ]]; then
    fail "origin/staging stable manifest ${staging_stable} != origin/main ${main_stable}"
  else
    pass "origin/staging stable manifest matches origin/main (${staging_stable:-<missing>})"
  fi

  staging_go="$(toolchain_at_ref origin/staging go.mod)"
  staging_example_go="$(toolchain_at_ref origin/staging examples/multi-tenant/go.mod)"
  for item in "go.mod:${staging_go}" "examples/multi-tenant/go.mod:${staging_example_go}"; do
    label="${item%%:*}"
    version="${item#*:}"
    if [[ "${version}" == "go1.26.3" ]]; then
      fail "origin/staging ${label} still observes vulnerable toolchain ${version}"
    elif [[ -z "${version}" ]]; then
      fail "origin/staging ${label} has no toolchain line"
    else
      pass "origin/staging ${label} toolchain is ${version}"
    fi
  done
fi

if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  stable_rc_prs="$(
    gh pr list \
      --repo theory-cloud/TableTheory \
      --base main \
      --state open \
      --json number,title,headRefName,url \
      --jq '.[] | select(.title | test("-rc\\.")) | "\(.number) \(.title) \(.url)"' \
      2>/dev/null || true
  )"
  if [[ -n "${stable_rc_prs}" ]]; then
    while IFS= read -r pr; do
      fail "stable release PR requires review for RC shape: ${pr}"
    done <<<"${stable_rc_prs}"
  else
    pass "no open main release PR advertises an RC version"
  fi

  if [[ "${main_pending_promotion}" -eq 1 ]]; then
    pending_prs="$(
      VERSION="${main_pending_version}" gh pr list \
        --repo theory-cloud/TableTheory \
        --base main \
        --state open \
        --json number,title,headRefName,url \
        --jq '.[] | select((.headRefName == "release-please--branches--main") or ((.title | test("release"; "i")) and (.title | test(env.VERSION)))) | "\(.number) \(.title) \(.url)"' \
        2>/dev/null || true
    )"
    if [[ -z "${pending_prs}" ]]; then
      fail "origin/main pending stable promotion ${main_pending_version} has no open stable release-please PR; pause and investigate"
    else
      while IFS= read -r pr; do
        pass "pending stable promotion has open stable release PR: ${pr}"
      done <<<"${pending_prs}"
    fi
  fi

  if [[ -n "${tag_name}" ]]; then
    release_json="$(gh release view "${tag_name}" --repo theory-cloud/TableTheory --json assets,isDraft,isPrerelease,tagName 2>/dev/null || true)"
    if [[ -z "${release_json}" ]]; then
      fail "GitHub release ${tag_name} does not exist"
    else
      asset_count="$(python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("assets", [])))' <<<"${release_json}")"
      if [[ "${asset_count}" -eq 0 ]]; then
        fail "GitHub release ${tag_name} exists but has no uploaded assets"
      else
        pass "GitHub release ${tag_name} has ${asset_count} uploaded asset(s)"
      fi
    fi
  else
    warn "no --tag supplied; skipped GitHub release asset check"
  fi
else
  if [[ "${main_pending_promotion}" -eq 1 ]]; then
    warn "origin/main pending stable promotion requires an open stable release PR check, but gh is unavailable or unauthenticated"
  fi
  warn "gh is unavailable or unauthenticated; skipped PR and release asset watchpoints"
fi

if [[ "${strict}" -eq 1 && "${fail_count}" -ne 0 ]]; then
  echo "watch-release-cycle: SUMMARY fail=${fail_count} warn=${warn_count} strict=1"
  exit 1
fi

echo "watch-release-cycle: SUMMARY fail=${fail_count} warn=${warn_count} strict=${strict}"
if [[ "${strict}" -eq 0 && "${fail_count}" -ne 0 ]]; then
  echo "watch-release-cycle: observation mode exits 0; rerun with --strict before merge/release gates"
fi
