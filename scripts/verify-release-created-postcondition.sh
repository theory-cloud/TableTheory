#!/usr/bin/env bash
set -euo pipefail

# Read-only publish postcondition for release-please release workflows. It
# prevents generated release PR merges from passing green when release-please
# reports no release or omits tag_name.

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-release-created-postcondition.sh --kind prerelease|stable [options]

Options:
  --kind KIND                    prerelease or stable.
  --branch BRANCH                Current branch. Defaults to $GITHUB_REF_NAME.
  --release-created true|false   release-please release_created output.
  --tag-name TAG                 release-please tag_name output.
  --commit-message MESSAGE       Pushed head commit message.
  --pending-stable-promotion BOOL  Stable workflow pending-promotion classifier output.
  -h, --help                     Show this help.

This command validates workflow outputs only. It never creates, updates, tags,
publishes, uploads, edits, deletes, resets, or pushes.
USAGE
}

kind=""
branch="${GITHUB_REF_NAME:-}"
release_created="${RELEASE_CREATED:-}"
tag_name="${TAG_NAME:-}"
commit_message="${HEAD_COMMIT_MESSAGE:-}"
pending_stable_promotion="${PENDING_STABLE_PROMOTION:-false}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --kind)
      if [[ $# -lt 2 ]]; then
        echo "release-created-postcondition: FAIL (--kind requires a value)" >&2
        exit 2
      fi
      kind="$2"
      shift 2
      ;;
    --branch)
      if [[ $# -lt 2 ]]; then
        echo "release-created-postcondition: FAIL (--branch requires a value)" >&2
        exit 2
      fi
      branch="$2"
      shift 2
      ;;
    --release-created)
      if [[ $# -lt 2 ]]; then
        echo "release-created-postcondition: FAIL (--release-created requires a value)" >&2
        exit 2
      fi
      release_created="$2"
      shift 2
      ;;
    --tag-name)
      if [[ $# -lt 2 ]]; then
        echo "release-created-postcondition: FAIL (--tag-name requires a value)" >&2
        exit 2
      fi
      tag_name="$2"
      shift 2
      ;;
    --commit-message)
      if [[ $# -lt 2 ]]; then
        echo "release-created-postcondition: FAIL (--commit-message requires a value)" >&2
        exit 2
      fi
      commit_message="$2"
      shift 2
      ;;
    --pending-stable-promotion)
      if [[ $# -lt 2 ]]; then
        echo "release-created-postcondition: FAIL (--pending-stable-promotion requires a value)" >&2
        exit 2
      fi
      pending_stable_promotion="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "release-created-postcondition: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

fail() {
  echo "release-created-postcondition: FAIL ($1)" >&2
  exit 1
}

if [[ -z "${kind}" ]]; then
  fail "missing --kind"
fi

if [[ -z "${branch}" ]]; then
  branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
fi

case "${kind}" in
  prerelease|stable)
    ;;
  *)
    fail "--kind must be prerelease or stable, got ${kind}"
    ;;
esac

generated=0
release_version=""

if [[ "${kind}" == "prerelease" ]]; then
  [[ "${branch}" == "premain" ]] || fail "prerelease publish workflow must run on premain, got ${branch}"
  if [[ "${commit_message}" == *"release-please--branches--premain"* ]]; then
    generated=1
  fi
  if [[ "${commit_message}" =~ chore\(premain\):[[:space:]]release[[:space:]]([0-9]+\.[0-9]+\.[0-9]+-rc(\.[0-9]+)?)([^[:alnum:].-]|$) ]]; then
    generated=1
    release_version="${BASH_REMATCH[1]}"
  fi

  if [[ "${generated}" -ne 1 ]]; then
    if [[ "${release_created}" == "true" ]]; then
      fail "prerelease workflow created a release on a non-generated RC release PR merge"
    fi
    echo "release-created-postcondition: PASS (premain push is not the publish step; prerelease-pr.yml must require the generated RC release-please PR)"
    exit 0
  fi

  if [[ "${release_created}" != "true" ]]; then
    fail "generated RC release PR merge reported release_created=false; release-please 'No user facing commits' is a failed publish gate"
  fi

  if [[ -z "${tag_name}" ]]; then
    fail "generated RC release PR merge created a release without tag_name"
  fi

  if [[ ! "${tag_name}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+-rc(\.[0-9]+)?$ ]]; then
    fail "generated RC release PR merge produced non-RC tag_name ${tag_name}; expected X.Y.Z-rc or X.Y.Z-rc.N"
  fi

  if [[ -n "${release_version}" ]]; then
    normalized_tag="${tag_name#v}"
    if [[ "${normalized_tag}" != "${release_version}" ]]; then
      fail "tag_name ${tag_name} does not match generated RC release ${release_version}"
    fi
  fi

  echo "release-created-postcondition: PASS (generated RC release PR merge published ${tag_name})"
  exit 0
fi

[[ "${branch}" == "main" ]] || fail "stable publish workflow must run on main, got ${branch}"

if [[ "${pending_stable_promotion}" == "true" ]]; then
  if [[ "${release_created}" == "true" ]]; then
    fail "stable workflow created a release during pending stable promotion"
  fi
  if [[ -n "${tag_name}" ]]; then
    fail "stable workflow produced tag_name ${tag_name} during pending stable promotion"
  fi
  echo "release-created-postcondition: PASS (plain premain -> main promotion is pending stable release PR generation; release-pr.yml must require the generated stable release-please PR)"
  exit 0
fi

if [[ "${commit_message}" == *"release-please--branches--main"* ]]; then
  generated=1
fi
if [[ "${commit_message}" =~ chore\(main\):[[:space:]]release[[:space:]]([0-9]+\.[0-9]+\.[0-9]+) ]]; then
  generated=1
  release_version="${BASH_REMATCH[1]}"
fi

if [[ "${commit_message}" =~ chore\(main\):[[:space:]]release[[:space:]][0-9]+\.[0-9]+\.[0-9]+-rc(\.|-|[[:alnum:]])* ]]; then
  fail "main stable publish workflow observed an RC-shaped release message"
fi

if [[ "${generated}" -ne 1 ]]; then
  if [[ "${release_created}" == "true" ]]; then
    fail "stable workflow created a release on a non-generated stable release PR merge"
  fi
  echo "release-created-postcondition: PASS (main push is not a generated stable release PR merge)"
  exit 0
fi

if [[ "${release_created}" != "true" ]]; then
  fail "generated stable release PR merge reported release_created=false; release-please 'No user facing commits' is a failed publish gate"
fi

if [[ -z "${tag_name}" ]]; then
  fail "generated stable release PR merge created a release without tag_name"
fi

if [[ ! "${tag_name}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  fail "generated stable release PR merge produced non-stable tag_name ${tag_name}"
fi

if [[ -n "${release_version}" ]]; then
  normalized_tag="${tag_name#v}"
  if [[ "${normalized_tag}" != "${release_version}" ]]; then
    fail "tag_name ${tag_name} does not match generated stable release ${release_version}"
  fi
fi

echo "release-created-postcondition: PASS (generated stable release PR merge published ${tag_name})"
