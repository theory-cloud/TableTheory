#!/usr/bin/env bash
set -euo pipefail

# Read-only release-cycle watch helper.
#
# Default mode reports PASS/WARN/FAIL findings and exits 0 so it can be used
# during incident review without masking all findings behind the first failure.
# Use --strict in CI or before merge/release to exit non-zero on FAIL findings.

usage() {
  cat <<'USAGE'
Usage: bash scripts/watch-release-cycle.sh [--strict] [--tag vX.Y.Z] [--repo-root PATH] [--skip-github]

Options:
  --strict   Exit non-zero if any FAIL finding is reported.
  --tag TAG  Check that an existing GitHub release is published, tagged, and has non-source assets.
  --repo-root PATH
             Evaluate a different checkout root (used by policy self-tests).
  --skip-github
             Skip live GitHub PR/release reads (used by policy self-tests).
  -h, --help Show this help.

This command reads local refs and, when gh is authenticated, open PR/release
metadata. It does not fetch, push, merge, tag, publish, edit, or delete anything.
USAGE
}

strict=0
tag_name=""
repo_root=""
skip_github=0
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
    --repo-root)
      if [[ $# -lt 2 ]]; then
        echo "watch-release-cycle: FAIL (--repo-root requires a value)" >&2
        exit 2
      fi
      repo_root="$2"
      shift 2
      ;;
    --skip-github)
      skip_github=1
      shift
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

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${repo_root}" ]]; then
  repo_root="$(cd "${script_dir}/.." && pwd)"
fi
source "${script_dir}/lib/release-cycle-core.sh"
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

watch_json_value_at_ref() {
  local __target="$1"
  local ref="$2"
  local path="$3"
  local expr="$4"
  local output status

  set +e
  output="$(release_cycle_json_value_at_ref "${ref}" "${path}" "${expr}" 2>&1)"
  status=$?
  set -e

  if [[ "${status}" -ne 0 ]]; then
    if [[ -z "${output}" ]]; then
      output="${ref}:${path} could not be parsed as JSON"
    fi
    while IFS= read -r line; do
      if [[ -n "${line}" ]]; then
        fail "${line}"
      fi
    done <<<"${output}"
    printf -v "${__target}" '%s' ""
    return 1
  fi

  printf -v "${__target}" '%s' "${output}"
  return 0
}

watch_python_version_at_ref() {
  local __version_target="$1"
  local __label_target="$2"
  local ref="$3"
  local canonical_path="py/src/tabletheory_py/version.json"
  local legacy_path="py/src/theorydb_py/version.json"
  local path="${canonical_path}"
  local label="${canonical_path}"

  if git cat-file -e "${ref}:${canonical_path}" 2>/dev/null; then
    path="${canonical_path}"
  elif git cat-file -e "${ref}:${legacy_path}" 2>/dev/null; then
    path="${legacy_path}"
    label="${canonical_path} (fallback ${legacy_path})"
  fi

  printf -v "${__label_target}" '%s' "${label}"
  watch_json_value_at_ref "${__version_target}" "${ref}" "${path}" version
}

refs=(origin/main origin/premain origin/staging)
for ref in "${refs[@]}"; do
  if ! git rev-parse --verify --quiet "${ref}" >/dev/null; then
    warn "${ref} is not available locally; run git fetch origin before relying on this report"
  fi
done

main_stable=""
premain_version=""
staging_stable=""
main_pending_promotion=0
main_pending_version=""

check_source_version() {
  local ref="$1"
  local label="$2"
  local version="$3"
  if [[ -z "${version}" ]]; then
    fail "${ref} ${label} version is missing"
  elif [[ "${version}" == *-rc* ]]; then
    fail "${ref} ${label} source version remains prerelease (${version}); release assets must be tag-derived instead"
  elif [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    pass "${ref} ${label} source version is stable (${version})"
  else
    fail "${ref} ${label} source version is invalid (${version})"
  fi
}

if git rev-parse --verify --quiet origin/main >/dev/null; then
  watch_json_value_at_ref main_stable origin/main .release-please-manifest.json '.' || true
  watch_json_value_at_ref main_ts origin/main ts/package.json version || true
  watch_json_value_at_ref main_ts_lock origin/main ts/package-lock.json version || true
  watch_json_value_at_ref main_ts_lock_pkg origin/main ts/package-lock.json 'packages..version' || true
  watch_python_version_at_ref main_py main_py_label origin/main || true

  if git show origin/main:.release-please-manifest.premain.json >/dev/null 2>&1; then
    fail "origin/main still contains retired .release-please-manifest.premain.json"
  fi

  if [[ "${main_stable}" == *-rc* ]]; then
    main_pending_promotion=1
    main_pending_version="$(release_cycle_semver_base "${main_stable}" || true)"
    warn "origin/main pending stable promotion: single manifest ${main_stable}, expected stable PR ${main_pending_version:-<unknown>}"
  else
    pass "origin/main single manifest is stable (${main_stable:-<missing>})"
  fi

  for item in \
    "ts/package.json:${main_ts}" \
    "ts/package-lock.json:${main_ts_lock}" \
    "ts/package-lock.json packages['']:${main_ts_lock_pkg}" \
    "${main_py_label}:${main_py}"; do
    check_source_version "origin/main" "${item%%:*}" "${item#*:}"
  done

  main_go="$(release_cycle_toolchain_at_ref origin/main go.mod)"
  main_example_go="$(release_cycle_toolchain_at_ref origin/main examples/multi-tenant/go.mod)"
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
  watch_json_value_at_ref premain_version origin/premain .release-please-manifest.json '.' || true

  if git show origin/premain:.release-please-manifest.premain.json >/dev/null 2>&1; then
    fail "origin/premain still contains retired .release-please-manifest.premain.json"
  fi

  if [[ -n "${main_stable}" && -n "${premain_version}" ]]; then
    premain_base="$(release_cycle_semver_base "${premain_version}" || true)"
    main_base="$(release_cycle_semver_base "${main_stable}" || true)"
    if [[ -n "${premain_base}" && -n "${main_base}" ]] && release_cycle_semver_lt "${premain_base}" "${main_base}"; then
      fail "origin/premain release track ${premain_version} is behind origin/main ${main_stable}"
    else
      pass "origin/premain release track is ${premain_version}"
    fi
  else
    fail "origin/premain single manifest is missing"
  fi

  premain_go="$(release_cycle_toolchain_at_ref origin/premain go.mod)"
  if [[ "${premain_go}" == "go1.26.3" ]]; then
    fail "origin/premain go.mod still observes vulnerable toolchain ${premain_go}"
  elif [[ -n "${premain_go}" ]]; then
    pass "origin/premain go.mod toolchain is ${premain_go}"
  else
    fail "origin/premain go.mod has no toolchain line"
  fi
fi

if git rev-parse --verify --quiet origin/staging >/dev/null; then
  watch_json_value_at_ref staging_stable origin/staging .release-please-manifest.json '.' || true

  if git show origin/staging:.release-please-manifest.premain.json >/dev/null 2>&1; then
    fail "origin/staging still contains retired .release-please-manifest.premain.json"
  fi

  if [[ "${staging_stable}" == *-rc* ]]; then
    fail "origin/staging single manifest is prerelease (${staging_stable})"
  elif [[ "${main_pending_promotion}" -eq 1 ]]; then
    warn "origin/staging remains at ${staging_stable:-<missing>} while origin/main awaits stable Release PR ${main_pending_version:-<unknown>}"
  elif [[ -n "${main_stable}" && -n "${staging_stable}" && "${staging_stable}" != "${main_stable}" ]]; then
    fail "origin/staging single manifest ${staging_stable} != origin/main ${main_stable}"
  else
    pass "origin/staging single manifest matches origin/main (${staging_stable:-<missing>})"
  fi

  staging_go="$(release_cycle_toolchain_at_ref origin/staging go.mod)"
  staging_example_go="$(release_cycle_toolchain_at_ref origin/staging examples/multi-tenant/go.mod)"
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

if [[ "${skip_github}" -eq 0 ]] && command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
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
      fail "origin/main pending stable promotion ${main_pending_version} has no open stable Release PR; pause and investigate"
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
      release_tag="$(release_cycle_json_string_value "${release_json}" tagName)"
      release_is_draft="$(release_cycle_json_string_value "${release_json}" isDraft)"
      release_is_prerelease="$(release_cycle_json_string_value "${release_json}" isPrerelease)"
      release_published_at="$(release_cycle_json_string_value "${release_json}" publishedAt)"
      release_target_commitish="$(release_cycle_json_string_value "${release_json}" targetCommitish)"
      release_url="$(release_cycle_json_string_value "${release_json}" url)"
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
      tag_ref_status="$(release_cycle_json_string_value "${tag_ref_json:-{}}" status)"
      tag_ref_message="$(release_cycle_json_string_value "${tag_ref_json:-{}}" message)"
      if [[ -z "${tag_ref_json}" || "${tag_ref_status}" == "404" || "${tag_ref_message}" == "Not Found" ]]; then
        fail "git tag ref refs/tags/${tag_name} is missing"
      else
        tag_ref_type="$(release_cycle_json_string_value "${tag_ref_json}" object.type)"
        tag_ref_sha="$(release_cycle_json_string_value "${tag_ref_json}" object.sha)"
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
              annotated_target_type="$(release_cycle_json_string_value "${annotated_tag_json}" object.type)"
              annotated_target_sha="$(release_cycle_json_string_value "${annotated_tag_json}" object.sha)"
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
    if [[ "${skip_github}" -eq 1 ]]; then
      warn "origin/main pending stable promotion requires an open stable release PR check, but GitHub reads were skipped"
    else
      warn "origin/main pending stable promotion requires an open stable release PR check, but gh is unavailable or unauthenticated"
    fi
  fi
  if [[ "${skip_github}" -eq 1 ]]; then
    warn "GitHub PR and release asset watchpoints skipped by --skip-github"
  else
    warn "gh is unavailable or unauthenticated; skipped PR and release asset watchpoints"
  fi
fi

if [[ "${strict}" -eq 1 && "${fail_count}" -ne 0 ]]; then
  echo "watch-release-cycle: SUMMARY fail=${fail_count} warn=${warn_count} strict=1"
  exit 1
fi

echo "watch-release-cycle: SUMMARY fail=${fail_count} warn=${warn_count} strict=${strict}"
if [[ "${strict}" -eq 0 && "${fail_count}" -ne 0 ]]; then
  echo "watch-release-cycle: observation mode exits 0; rerun with --strict before merge/release gates"
fi
