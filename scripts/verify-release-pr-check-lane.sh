#!/usr/bin/env bash
set -euo pipefail

# REL guard: every generated release pull request has an automatic dispatch path
# for every required context of its base, bound to the pull request head.
#
# The failure this prevents is a release lane that looks wired but cannot gate
# itself. release-please (and scripts/create-stable-release-pr.py) create their
# pull requests with the built-in `GITHUB_TOKEN`, and GitHub starts no
# `pull_request` workflow for events that token causes, so the required context
# never reports and the only repair is closing and reopening the pull request.
# The declaration in `.github/release-lane.json` names, per base branch, the
# required contexts and the workflow that reports each one; this guard proves
# that:
#
#   1. every workflow that creates a generated release pull request declares the
#      permission to create pull requests and to dispatch workflows;
#   2. it invokes scripts/sync-release-pr-checks.sh for the exact head branch the
#      declaration names;
#   3. each required context is a job id in a workflow that has a
#      workflow_dispatch path taking the pull request number, that binds the run
#      to the pull request head through scripts/resolve-release-pr-head.sh, and
#      that re-checks the checkout is that head before any check runs;
#   4. no workflow reintroduces a release-please token fallback: the lane must
#      run on the built-in token only.
#
# The self-test proves the guard is not vacuous: it mutates copies of the real
# workflows and declaration and requires each mutation to fail, and it drives the
# head-binding resolver with hostile stub answers.
#
# Usage: bash scripts/verify-release-pr-check-lane.sh [--self-test] [--root DIR]

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
self_test=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --self-test)
      self_test=true
      shift
      ;;
    --root)
      [[ $# -ge 2 ]] || { echo "release-pr-check-lane: FAIL (--root requires a value)" >&2; exit 2; }
      root="$2"
      shift 2
      ;;
    -h|--help)
      cat <<'USAGE'
Usage: bash scripts/verify-release-pr-check-lane.sh [--self-test] [--root DIR]

Fails when a generated release pull request has no head-bound dispatch path for
one of its base branch's required contexts. --self-test additionally proves the
guard and the head-binding resolver reject hostile shapes.
USAGE
      exit 0
      ;;
    *)
      echo "release-pr-check-lane: FAIL (unknown argument: $1)" >&2
      exit 2
      ;;
  esac
done

failures=0
report() {
  echo "release-pr-check-lane: $1" >&2
  failures=$((failures + 1))
}

config="${root}/.github/release-lane.json"
workflows="${root}/.github/workflows"

if [[ ! -f "${config}" ]]; then
  echo "release-pr-check-lane: FAIL (missing ${config})" >&2
  exit 1
fi

# Emit one line per (head, base, context, workflow, job) and one per producer.
# The parsing lives in one place so the guard and the lane script cannot read the
# declaration differently.
plan="$(python3 - "${config}" <<'PY'
import json
import sys

try:
    data = json.loads(open(sys.argv[1], encoding="utf-8").read())
except json.JSONDecodeError as exc:
    print(f"release-pr-check-lane: FAIL (declaration is not JSON: {exc})", file=sys.stderr)
    raise SystemExit(1)

if data.get("schemaVersion") != 1:
    print("release-pr-check-lane: FAIL (declaration schemaVersion must be 1)", file=sys.stderr)
    raise SystemExit(1)

contexts = data.get("requiredContexts") or {}
producers = data.get("generatedReleasePullRequests") or []
if not producers:
    print("release-pr-check-lane: FAIL (declaration names no generated release pull request)", file=sys.stderr)
    raise SystemExit(1)

for entry in producers:
    head = str(entry.get("head") or "")
    base = str(entry.get("base") or "")
    workflow = str(entry.get("workflow") or "")
    if not head or not base or not workflow:
        print("release-pr-check-lane: FAIL (a producer entry is missing head, base or workflow)", file=sys.stderr)
        raise SystemExit(1)
    print(f"producer\t{head}\t{base}\t{workflow}")
    required = [item for item in contexts.get(base) or [] if item.get("context")]
    if not required:
        print(
            f"release-pr-check-lane: FAIL ({base} declares no required context, so {head} has nothing to satisfy)",
            file=sys.stderr,
        )
        raise SystemExit(1)
    for item in required:
        print(
            f"context\t{base}\t{item['context']}\t{str(item.get('workflow') or '')}\t{str(item.get('job') or '')}"
        )
PY
)" || exit 1

while IFS=$'\t' read -r kind a b c d; do
  case "${kind}" in
    producer)
      head="${a}"
      base="${b}"
      producer_workflow="${c}"
      path="${workflows}/${producer_workflow}"
      if [[ ! -f "${path}" ]]; then
        report "declared producer ${producer_workflow} does not exist"
        continue
      fi
      if ! grep -Eq '^[[:space:]]+pull-requests:[[:space:]]*write[[:space:]]*$' "${path}"; then
        report "${producer_workflow} creates a pull request but does not declare pull-requests: write"
      fi
      if ! grep -Eq '^[[:space:]]+actions:[[:space:]]*write[[:space:]]*$' "${path}"; then
        report "${producer_workflow} dispatches the verifying workflow but does not declare actions: write"
      fi
      if ! grep -Fq -- "--head-branch ${head}" "${path}"; then
        report "${producer_workflow} does not run scripts/sync-release-pr-checks.sh for ${head}"
      fi
      if ! grep -Fq "scripts/sync-release-pr-checks.sh" "${path}"; then
        report "${producer_workflow} does not attach the required checks to its generated pull request"
      fi
      ;;
    context)
      base="${a}"
      context="${b}"
      verifier_workflow="${c}"
      job="${d}"
      path="${workflows}/${verifier_workflow}"
      if [[ ! -f "${path}" ]]; then
        report "required context ${context} names a missing workflow ${verifier_workflow}"
        continue
      fi
      if [[ -z "${job}" || "${job}" != "${context}" ]]; then
        report "required context ${context} must be the reporting job id, found ${job:-<empty>}"
      fi
      if ! grep -Eq '^[[:space:]]+workflow_dispatch:[[:space:]]*($|#)' "${path}"; then
        report "${verifier_workflow} reports ${context} but has no workflow_dispatch path"
      fi
      if ! grep -Eq '^[[:space:]]+release_pr_number:[[:space:]]*$' "${path}"; then
        report "${verifier_workflow} has no release_pr_number dispatch input to bind the head"
      fi
      if ! grep -Eq "^[[:space:]]{2}${job}:[[:space:]]*$" "${path}"; then
        report "${verifier_workflow} has no job named ${job}, so ${context} would never report"
      fi
      if ! grep -Fq "scripts/resolve-release-pr-head.sh" "${path}"; then
        report "${verifier_workflow} does not bind a dispatched run to the pull request head"
      fi
      if ! grep -Fq "steps.context.outputs.head_sha" "${path}"; then
        report "${verifier_workflow} does not consume the resolved pull request head"
      fi
      if ! grep -Fq "git rev-parse HEAD" "${path}"; then
        report "${verifier_workflow} does not prove the checkout is the resolved pull request head"
      fi
      ;;
  esac
done < <(printf '%s\n' "${plan}")

# The lane must run on the built-in token. A release-please token fallback makes
# the lane depend on a credential that does not exist, and a token identity would
# let the pull request authenticate as a person.
for workflow in "${workflows}"/*.yml; do
  [[ -f "${workflow}" ]] || continue
  name="$(basename "${workflow}")"
  if grep -Fq "secrets.RELEASE_PLEASE_TOKEN" "${workflow}"; then
    report "${name} reintroduces the RELEASE_PLEASE_TOKEN identity"
  fi
  if grep -Eq 'secrets\.[A-Za-z0-9_]+[[:space:]]*\|\|[[:space:]]*secrets\.GITHUB_TOKEN' "${workflow}"; then
    report "${name} declares a secret-to-GITHUB_TOKEN fallback; only the built-in token may gate the lane"
  fi
done

run_self_test() {
  local scratch
  scratch="$(mktemp -d "${TMPDIR:-/tmp}/tabletheory-release-pr-check-lane.XXXXXX")"
  # shellcheck disable=SC2064 # the path is expanded now; the local is gone at EXIT
  trap "rm -rf '${scratch}'" EXIT

  local guard="${root}/scripts/verify-release-pr-check-lane.sh"
  local cases=0

  expect_fixture_failure() {
    local label="$1"
    local expected="$2"
    local fixture="$3"
    local output
    cases=$((cases + 1))
    if output="$(bash "${guard}" --root "${fixture}" 2>&1)"; then
      echo "release-pr-check-lane self-test: FAIL (${label}; guard accepted the mutation)" >&2
      printf '%s\n' "${output}" >&2
      exit 1
    fi
    if ! grep -Fq -- "${expected}" <<<"${output}"; then
      echo "release-pr-check-lane self-test: FAIL (${label}; missing ${expected})" >&2
      printf '%s\n' "${output}" >&2
      exit 1
    fi
    echo "release-pr-check-lane self-test: PASS ${label}"
  }

  new_fixture() {
    local name="$1"
    local dir="${scratch}/${name}"
    mkdir -p "${dir}/.github/workflows" "${dir}/scripts"
    cp "${config}" "${dir}/.github/release-lane.json"
    cp "${workflows}"/*.yml "${dir}/.github/workflows/"
    cp "${guard}" "${dir}/scripts/verify-release-pr-check-lane.sh"
    printf '%s\n' "${dir}"
  }

  # The unmodified copy passes: the guard reads only the declaration and the
  # workflows, so a faithful copy must be accepted before mutations mean anything.
  baseline="$(new_fixture baseline)"
  cases=$((cases + 1))
  if ! bash "${guard}" --root "${baseline}" >/dev/null 2>&1; then
    echo "release-pr-check-lane self-test: FAIL (the faithful copy of the real tree must pass)" >&2
    bash "${guard}" --root "${baseline}" >&2 || true
    exit 1
  fi
  echo "release-pr-check-lane self-test: PASS a faithful copy of the real tree passes"

  # 1. A producer that stops invoking the lane.
  fixture="$(new_fixture no-lane)"
  sed -i.bak '/scripts\/sync-release-pr-checks.sh/d' "${fixture}/.github/workflows/prerelease-pr.yml"
  rm -f "${fixture}/.github/workflows/prerelease-pr.yml.bak"
  expect_fixture_failure "a producer that stops attaching the lane fails" \
    "does not attach the required checks" "${fixture}"

  # 2. A producer that dispatches without the dispatch permission.
  fixture="$(new_fixture no-actions-write)"
  python3 - "${fixture}/.github/workflows/release-pr.yml" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
path.write_text(text.replace("  actions: write\n", "", 1), encoding="utf-8")
PY
  expect_fixture_failure "a producer without actions: write fails" \
    "does not declare actions: write" "${fixture}"

  # 3. A verifying workflow that loses its dispatch input, so nothing can bind a
  #    dispatched run to the pull request.
  fixture="$(new_fixture no-dispatch-input)"
  python3 - "${fixture}/.github/workflows/release-hygiene.yml" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
out = [line for line in lines if line.strip() != "release_pr_number:"]
path.write_text("".join(out), encoding="utf-8")
PY
  expect_fixture_failure "a verifying workflow without release_pr_number fails" \
    "has no release_pr_number dispatch input" "${fixture}"

  # 4. A verifying workflow that stops binding the run to the pull request head.
  fixture="$(new_fixture no-head-binding)"
  sed -i.bak 's|scripts/resolve-release-pr-head.sh|scripts/some-other-script.sh|g' \
    "${fixture}/.github/workflows/release-hygiene.yml"
  rm -f "${fixture}/.github/workflows/release-hygiene.yml.bak"
  expect_fixture_failure "a verifying workflow without the head binding fails" \
    "does not bind a dispatched run to the pull request head" "${fixture}"

  # 5. A required context whose job id no longer matches the context name would
  #    never report that context.
  fixture="$(new_fixture renamed-job)"
  sed -i.bak 's|^  hygiene:$|  release-hygiene:|' "${fixture}/.github/workflows/release-hygiene.yml"
  rm -f "${fixture}/.github/workflows/release-hygiene.yml.bak"
  expect_fixture_failure "a verifying job renamed away from its context fails" \
    "has no job named hygiene" "${fixture}"

  # 6. A credential fallback reintroduced anywhere in the workflow set.
  fixture="$(new_fixture token-fallback)"
  printf '\n# fallback for the record\n# token: ${{ secrets.RELEASE_PLEASE_TOKEN }}\n' \
    >>"${fixture}/.github/workflows/release.yml"
  expect_fixture_failure "a reintroduced release-please token fails" \
    "reintroduces the RELEASE_PLEASE_TOKEN identity" "${fixture}"

  # 7. The head-binding resolver itself. A stub `gh` answers the pull request
  #    read, so every hostile answer runs through the real validation.
  local resolver="${root}/scripts/resolve-release-pr-head.sh"
  local stub_dir="${scratch}/stub"
  mkdir -p "${stub_dir}"
  cat >"${stub_dir}/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "${GH_STUB_PAYLOAD}"
STUB
  chmod +x "${stub_dir}/gh"

  resolver_case() {
    local label="$1"
    local expected="$2"
    local expect_success="$3"
    local payload="$4"
    local pr="$5"
    local ref_name="$6"
    local ref_sha="$7"
    local output
    cases=$((cases + 1))
    output="$(GH_BIN="${stub_dir}/gh" GH_STUB_PAYLOAD="${payload}" \
      bash "${resolver}" --repo theory-cloud/TableTheory --pr "${pr}" \
      --ref-name "${ref_name}" --ref-sha "${ref_sha}" 2>&1)" && status=0 || status=$?
    if [[ "${expect_success}" == "true" ]]; then
      if [[ "${status}" -ne 0 ]]; then
        echo "release-pr-check-lane self-test: FAIL (${label}; resolver failed)" >&2
        printf '%s\n' "${output}" >&2
        exit 1
      fi
      if ! grep -Fq -- "${expected}" <<<"${output}"; then
        echo "release-pr-check-lane self-test: FAIL (${label}; missing ${expected})" >&2
        printf '%s\n' "${output}" >&2
        exit 1
      fi
    else
      if [[ "${status}" -eq 0 ]]; then
        echo "release-pr-check-lane self-test: FAIL (${label}; resolver accepted a hostile answer)" >&2
        printf '%s\n' "${output}" >&2
        exit 1
      fi
      if ! grep -Fq -- "${expected}" <<<"${output}"; then
        echo "release-pr-check-lane self-test: FAIL (${label}; missing ${expected})" >&2
        printf '%s\n' "${output}" >&2
        exit 1
      fi
    fi
    echo "release-pr-check-lane self-test: PASS ${label}"
  }

  local sha_head sha_other
  sha_head="1111111111111111111111111111111111111111"
  sha_other="2222222222222222222222222222222222222222"

  payload() {
    local state="$1"
    local base="$2"
    local head_repo="$3"
    local head_ref="$4"
    local head_sha="$5"
    local number="$6"
    local title="$7"
    printf '{"number": %s, "state": "%s", "title": "%s", "merged": false, "base": {"ref": "%s", "sha": "3333333333333333333333333333333333333333", "repo": {"full_name": "theory-cloud/TableTheory"}}, "head": {"ref": "%s", "sha": "%s", "repo": {"full_name": "%s"}}}' \
      "${number}" "${state}" "${title}" "${base}" "${head_ref}" "${head_sha}" "${head_repo}"
  }

  resolver_case "the resolver accepts the exact pull request head" \
    "head_sha=${sha_head}" true \
    "$(payload open premain theory-cloud/TableTheory release-please--branches--premain "${sha_head}" 1296 "chore(premain): release 7.5.0-rc")" \
    1296 release-please--branches--premain "${sha_head}"
  resolver_case "the resolver refuses a moved head" \
    "!= generated release PR head" false \
    "$(payload open premain theory-cloud/TableTheory release-please--branches--premain "${sha_other}" 1296 "chore(premain): release 7.5.0-rc")" \
    1296 release-please--branches--premain "${sha_head}"
  resolver_case "the resolver refuses a closed pull request" \
    "is closed, not open" false \
    "$(payload closed premain theory-cloud/TableTheory release-please--branches--premain "${sha_head}" 1296 "chore(premain): release 7.5.0-rc")" \
    1296 release-please--branches--premain "${sha_head}"
  resolver_case "the resolver refuses a foreign head repository" \
    "head repository" false \
    "$(payload open premain someone/TableTheory release-please--branches--premain "${sha_head}" 1296 "chore(premain): release 7.5.0-rc")" \
    1296 release-please--branches--premain "${sha_head}"
  resolver_case "the resolver refuses a non-release base" \
    "not a release branch" false \
    "$(payload open staging theory-cloud/TableTheory release-please--branches--premain "${sha_head}" 1296 "chore(premain): release 7.5.0-rc")" \
    1296 release-please--branches--premain "${sha_head}"
  resolver_case "the resolver refuses a different head branch" \
    "!= generated release PR head" false \
    "$(payload open premain theory-cloud/TableTheory release-please--branches--main "${sha_head}" 1296 "chore(premain): release 7.5.0-rc")" \
    1296 release-please--branches--premain "${sha_head}"
  resolver_case "the resolver refuses a non-numeric pull request" \
    "must be numeric" false \
    "$(payload open premain theory-cloud/TableTheory release-please--branches--premain "${sha_head}" 1296 "chore(premain): release 7.5.0-rc")" \
    release-please release-please--branches--premain "${sha_head}"

  echo "release-pr-check-lane self-test: PASS (${cases} cases)"
}

if (( failures > 0 )); then
  echo "release-pr-check-lane: FAIL (${failures} problem(s))" >&2
  exit 1
fi

if [[ "${self_test}" == "true" ]]; then
  run_self_test
else
  echo "release-pr-check-lane: PASS (every generated release PR has a head-bound dispatch path for each required context)"
fi
