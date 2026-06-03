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
  --tag TAG  Check that an existing GitHub release is published, tagged, and has non-source assets.
  -h, --help Show this help.

This command reads local refs and, when gh is authenticated, open PR/release
metadata. It does not fetch, push, merge, tag, publish, edit, or delete anything.
USAGE
}

strict=0
tag_name=""
github_repo="theory-cloud/TableTheory"

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

json_string_value() {
  local json="$1"
  local expr="$2"

  JSON_INPUT="${json}" python3 - "${expr}" <<'PY'
import json
import os
import sys

expr = sys.argv[1]
raw = os.environ["JSON_INPUT"]
try:
    data = json.loads(raw)
except json.JSONDecodeError:
    try:
        data, _ = json.JSONDecoder().raw_decode(raw)
    except json.JSONDecodeError:
        print("")
        raise SystemExit(0)
value = data
for part in expr.split("."):
    if not part:
        continue
    if isinstance(value, dict):
        value = value.get(part)
    else:
        value = None
if isinstance(value, bool):
    print("true" if value else "false")
elif value is None:
    print("")
elif isinstance(value, (str, int, float)):
    print(value)
else:
    print(json.dumps(value))
PY
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
  premain_bad_release_prs="$(
    gh pr list \
      --repo "${github_repo}" \
      --base premain \
      --state open \
      --json number,title,headRefName,url \
      --jq '.[] | select(.headRefName == "release-please--branches--premain") | select((.title | test("^chore\\(premain\\): release [0-9]+\\.[0-9]+\\.[0-9]+-rc\\.[0-9]+$")) | not) | "\(.number) \(.title) \(.url)"' \
      2>/dev/null || true
  )"
  if [[ -n "${premain_bad_release_prs}" ]]; then
    while IFS= read -r pr; do
      fail "premain release-please PR is not RC-shaped: ${pr}"
    done <<<"${premain_bad_release_prs}"
  fi

  premain_rc_prs="$(
    gh pr list \
      --repo "${github_repo}" \
      --base premain \
      --state open \
      --json number,title,headRefName,url \
      --jq '.[] | select(.headRefName == "release-please--branches--premain") | select(.title | test("^chore\\(premain\\): release [0-9]+\\.[0-9]+\\.[0-9]+-rc\\.[0-9]+$")) | "\(.number) \(.title) \(.url)"' \
      2>/dev/null || true
  )"
  if [[ -n "${premain_rc_prs}" ]]; then
    while IFS= read -r pr; do
      pass "open premain generated RC release PR: ${pr}"
    done <<<"${premain_rc_prs}"
  else
    pass "no open premain generated RC release PR"
  fi

  stable_rc_prs="$(
    gh pr list \
      --repo "${github_repo}" \
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
        --repo "${github_repo}" \
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
    release_json="$(
      gh release view "${tag_name}" \
        --repo "${github_repo}" \
        --json assets,isDraft,isPrerelease,publishedAt,tagName,targetCommitish,url \
        2>/dev/null || true
    )"
    if [[ -z "${release_json}" ]]; then
      fail "GitHub release ${tag_name} does not exist"
    else
      release_tag="$(json_string_value "${release_json}" tagName)"
      release_is_draft="$(json_string_value "${release_json}" isDraft)"
      release_is_prerelease="$(json_string_value "${release_json}" isPrerelease)"
      release_published_at="$(json_string_value "${release_json}" publishedAt)"
      release_target_commitish="$(json_string_value "${release_json}" targetCommitish)"
      release_url="$(json_string_value "${release_json}" url)"
      asset_count="$(python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("assets", [])))' <<<"${release_json}")"

      if [[ "${release_tag}" != "${tag_name}" ]]; then
        fail "GitHub release lookup for ${tag_name} returned tag ${release_tag:-<missing>}"
      else
        pass "GitHub release ${tag_name} reports matching tag name"
      fi

      if [[ "${release_is_draft}" == "true" ]]; then
        fail "GitHub release ${tag_name} is still draft"
      else
        pass "GitHub release ${tag_name} is not draft"
      fi

      if [[ -z "${release_published_at}" ]]; then
        fail "GitHub release ${tag_name} has no publishedAt timestamp"
      else
        pass "GitHub release ${tag_name} publishedAt is ${release_published_at}"
      fi

      if [[ "${release_url}" == *"/releases/tag/untagged-"* ]]; then
        fail "GitHub release ${tag_name} URL is an untagged draft URL (${release_url})"
      elif [[ -z "${release_url}" ]]; then
        warn "GitHub release ${tag_name} URL is missing from release metadata"
      else
        pass "GitHub release ${tag_name} URL is tag-addressed"
      fi

      if [[ "${tag_name}" == *"-rc"* ]]; then
        if [[ "${release_is_prerelease}" == "true" ]]; then
          pass "GitHub release ${tag_name} is marked prerelease"
        else
          fail "GitHub release ${tag_name} is not marked prerelease"
        fi
      elif [[ "${release_is_prerelease}" == "true" ]]; then
        fail "GitHub release ${tag_name} is marked prerelease for a non-RC tag"
      fi

      tag_ref_json="$(gh api "repos/${github_repo}/git/ref/tags/${tag_name}" 2>/dev/null || true)"
      tag_ref_target_sha=""
      tag_ref_status="$(json_string_value "${tag_ref_json:-{}}" status)"
      tag_ref_message="$(json_string_value "${tag_ref_json:-{}}" message)"
      if [[ -z "${tag_ref_json}" || "${tag_ref_status}" == "404" || "${tag_ref_message}" == "Not Found" ]]; then
        fail "git tag ref refs/tags/${tag_name} is missing"
      else
        tag_ref_type="$(json_string_value "${tag_ref_json}" object.type)"
        tag_ref_sha="$(json_string_value "${tag_ref_json}" object.sha)"
        case "${tag_ref_type}" in
          commit)
            tag_ref_target_sha="${tag_ref_sha}"
            pass "git tag ref refs/tags/${tag_name} points to commit ${tag_ref_target_sha}"
            ;;
          tag)
            annotated_tag_json="$(gh api "repos/${github_repo}/git/tags/${tag_ref_sha}" 2>/dev/null || true)"
            if [[ -z "${annotated_tag_json}" ]]; then
              fail "git tag ref refs/tags/${tag_name} points to an unreadable annotated tag ${tag_ref_sha}"
            else
              annotated_target_type="$(json_string_value "${annotated_tag_json}" object.type)"
              annotated_target_sha="$(json_string_value "${annotated_tag_json}" object.sha)"
              if [[ "${annotated_target_type}" == "commit" && -n "${annotated_target_sha}" ]]; then
                tag_ref_target_sha="${annotated_target_sha}"
                pass "git tag ref refs/tags/${tag_name} dereferences to commit ${tag_ref_target_sha}"
              else
                fail "git tag ref refs/tags/${tag_name} does not dereference to a commit"
              fi
            fi
            ;;
          "")
            fail "git tag ref refs/tags/${tag_name} has no observable target"
            ;;
          *)
            fail "git tag ref refs/tags/${tag_name} points to unsupported object type ${tag_ref_type}"
            ;;
        esac
      fi

      if [[ -n "${release_target_commitish}" && -n "${tag_ref_target_sha}" ]]; then
        release_target_sha="$(gh api "repos/${github_repo}/commits/${release_target_commitish}" --jq .sha 2>/dev/null || true)"
        if [[ -z "${release_target_sha}" ]]; then
          warn "GitHub release ${tag_name} targetCommitish ${release_target_commitish} could not be resolved"
        elif [[ "${release_target_sha}" != "${tag_ref_target_sha}" ]]; then
          fail "git tag ref ${tag_name} targets ${tag_ref_target_sha}, release targetCommitish resolves to ${release_target_sha}"
        else
          pass "git tag ref ${tag_name} matches release targetCommitish ${release_target_sha}"
        fi
      elif [[ -z "${release_target_commitish}" ]]; then
        warn "GitHub release ${tag_name} targetCommitish is missing from release metadata"
      fi

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
