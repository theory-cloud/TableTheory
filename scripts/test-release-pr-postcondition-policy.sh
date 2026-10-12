#!/usr/bin/env bash
set -euo pipefail

# Policy test for scripts/verify-main-release-pr-postcondition.sh candidate
# selection (finding 2f17dfd4). GitHub PR listings for a base repository include
# cross-repository PRs, and `gh pr list --head` cannot be owner-qualified, so an
# external fork can open a PR to `main` from a branch named like the generated
# release branch. Every case runs the real postcondition against a stub `gh`
# that replays fixture JSON, so a regression in same-repository head ownership
# makes a real assert fail rather than a vacuous grep.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
postcondition="${repo_root}/scripts/verify-main-release-pr-postcondition.sh"

if [[ ! -f "${postcondition}" ]]; then
  echo "release-pr-postcondition-policy: FAIL (missing ${postcondition})"
  exit 1
fi

scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

failures=0

same_repo_list='[{"number":12,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","url":"https://github.com/theory-cloud/TableTheory/pull/12","headRepositoryOwner":{"login":"theory-cloud"},"headRepository":{"name":"TableTheory"},"isCrossRepository":false}]'
fork_list='[{"number":7,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","url":"https://github.com/attacker/TableTheory/pull/7","headRepositoryOwner":{"login":"attacker"},"headRepository":{"name":"TableTheory"},"isCrossRepository":true}]'
spoofed_list='[{"number":8,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","url":"https://github.com/attacker/TableTheory/pull/8","headRepositoryOwner":{"login":"attacker"},"headRepository":{"name":"TableTheory"},"isCrossRepository":false}]'
mixed_list='[{"number":7,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","url":"https://github.com/attacker/TableTheory/pull/7","headRepositoryOwner":{"login":"attacker"},"headRepository":{"name":"TableTheory"},"isCrossRepository":true},{"number":12,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","url":"https://github.com/theory-cloud/TableTheory/pull/12","headRepositoryOwner":{"login":"theory-cloud"},"headRepository":{"name":"TableTheory"},"isCrossRepository":false}]'

valid_view='{"number":12,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","baseRefName":"main","url":"https://github.com/theory-cloud/TableTheory/pull/12","files":[{"path":".release-please-manifest.json"},{"path":"CHANGELOG.md"}],"labels":[{"name":"autorelease: pending"}],"body":"---\n\n## [2.0.1](https://github.com/theory-cloud/TableTheory/compare/v2.0.1-rc.1...v2.0.1) (2026-10-09)\n\n---\n","headRepositoryOwner":{"login":"theory-cloud"},"headRepository":{"name":"TableTheory"},"isCrossRepository":false}'

run_case() {
  local name="$1" list_json="$2" view_json="$3" expected_status="$4" expected_text="$5"
  local bin="${scratch}/${name}/bin"
  mkdir -p "${bin}"
  cat > "${bin}/gh" <<STUB
#!/usr/bin/env bash
set -euo pipefail
case "\$1 \$2" in
  "auth status") exit 0 ;;
  "pr list") printf '%s' '${list_json}'; exit 0 ;;
  "pr view") printf '%s' '${view_json}'; exit 0 ;;
  *) echo "stub gh: unexpected args: \$*" >&2; exit 2 ;;
esac
STUB
  chmod +x "${bin}/gh"

  local status output
  set +e
  output="$(PATH="${bin}:${PATH}" bash "${postcondition}" \
    --repo theory-cloud/TableTheory --base main --expected-version 2.0.1 2>&1)"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf '%s\n' "${output}"
    echo "release-pr-postcondition-policy: ${name}: expected exit ${expected_status}, got ${status}"
    failures=$((failures + 1))
    return
  fi
  if [[ -n "${expected_text}" ]] && ! grep -Fq "${expected_text}" <<< "${output}"; then
    printf '%s\n' "${output}"
    echo "release-pr-postcondition-policy: ${name}: missing message: ${expected_text}"
    failures=$((failures + 1))
  fi
}

# 1. A same-repository generated release PR is attested.
run_case same-repo "${same_repo_list}" "${valid_view}" 0 "release-pr-postcondition: PASS"

# 2. A fork PR from the release branch name is not selected as the release PR.
run_case fork-only "${fork_list}" "${valid_view}" 1 "no open main release PR found"

# 3. A name-spoofed same-repository flag with a foreign head owner is rejected.
run_case spoofed-owner "${spoofed_list}" "${valid_view}" 1 "no open main release PR found"

# 4. When a fork PR and the real same-repository PR both exist, the same-repository
#    PR is the one selected.
run_case fork-plus-same-repo "${mixed_list}" "${valid_view}" 0 "release-pr-postcondition: PASS"

# 5. A cross-repository candidate that reaches the detail stage fails closed.
run_case cross-repo-detail "${same_repo_list}" \
  '{"number":7,"title":"chore(main): release 2.0.1","headRefName":"release-please--branches--main","baseRefName":"main","url":"https://github.com/attacker/TableTheory/pull/7","files":[{"path":".release-please-manifest.json"},{"path":"CHANGELOG.md"}],"labels":[{"name":"autorelease: pending"}],"body":"---\n\n## [2.0.1](https://x) (2026-10-09)\n\n---\n","headRepositoryOwner":{"login":"attacker"},"headRepository":{"name":"TableTheory"},"isCrossRepository":true}' \
  1 "head is cross-repository"

if [[ "${failures}" -ne 0 ]]; then
  echo "release-pr-postcondition-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "release-pr-postcondition-policy: PASS (same-repository head ownership is enforced)"
