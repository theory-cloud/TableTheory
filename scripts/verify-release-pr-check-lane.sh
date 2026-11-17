#!/usr/bin/env bash
set -euo pipefail

# REL guard: every generated release pull request has, for every required context
# of its base, both paths the ruleset needs.
#
#   - an early, head-bound verdict the lane can reach without a credential; and
#   - an eligible event that makes the context count.
#
# The failure this prevents is a release lane that looks wired but cannot gate
# itself. release-please (and scripts/create-stable-release-pr.py) create their
# pull requests with the built-in `GITHUB_TOKEN`: the `pull_request` runs that
# token causes arrive in an approval-required state, and a `workflow_dispatch`
# run's check runs never appear in the pull request's checks section and never
# satisfy a required status check. So a generated release pull request has
# exactly one eligible event available to it without a credential — the
# code-owner review the premain/main rulesets already require — and the
# verifying workflow must run on it.
#
# The declaration in `.github/release-lane.json` names, per base branch, the
# required contexts, the workflow and job that report each one, and the eligible
# events that make it count. This guard proves that:
#
#   1. every workflow that creates a generated release pull request declares the
#      permission to create pull requests and to dispatch workflows;
#   2. it invokes scripts/sync-release-pr-checks.sh for the exact head branch the
#      declaration names;
#   3. each required context is a job id in a workflow that has a
#      workflow_dispatch path taking the pull request number, that binds the run
#      to the pull request head through scripts/resolve-release-pr-head.sh, and
#      that re-checks the checkout is that head before any check runs;
#   4. each declared eligible event triggers the workflow for that release base,
#      the `pull_request_review` trigger subscribes to submitted reviews only and
#      never to a staging pull request, and the job's `if:` does not exclude it;
#   5. the reporting job declares no `name:`, so the context is the same job id
#      on every path;
#   6. no workflow reintroduces a release-please token fallback: the lane must
#      run on the built-in token only; and
#   7. the lane never counts a skipped or neutral context as green and names the
#      eligible event that satisfies the ruleset.
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

Fails when a generated release pull request has no head-bound dispatch path or no
eligible-event path for one of its base branch's required contexts. --self-test
additionally proves the guard and the head-binding resolver reject hostile shapes.
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
lane="${root}/scripts/sync-release-pr-checks.sh"

if [[ ! -f "${config}" ]]; then
  echo "release-pr-check-lane: FAIL (missing ${config})" >&2
  exit 1
fi

# Emit one line per (head, base, context, workflow, job), one per declared
# eligible event, and one per producer. The parsing lives in one place so the
# guard and the lane script cannot read the declaration differently.
plan="$(python3 - "${config}" <<'PY'
import json
import sys

# GitHub evaluates a required status check only for a run triggered by one of
# these events. A workflow_dispatch run is not one of them, which is why the lane
# it starts is an early verdict and never proof for the ruleset.
ELIGIBLE = {
    "push",
    "pull_request",
    "pull_request_review",
    "pull_request_target",
    "deployment",
    "deployment_status",
}

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
        context = str(item["context"])
        verifier = str(item.get("workflow") or "")
        job = str(item.get("job") or "")
        print(f"context\t{base}\t{context}\t{verifier}\t{job}")
        events = item.get("eligibleEvents")
        if not isinstance(events, list) or not events:
            print(
                f"release-pr-check-lane: FAIL ({context} on {base} declares no eligibleEvents, so nothing states which run makes it count)",
                file=sys.stderr,
            )
            raise SystemExit(1)
        unknown = sorted({str(event) for event in events} - ELIGIBLE)
        if unknown:
            print(
                f"release-pr-check-lane: FAIL ({context} on {base} declares ineligible events {unknown}; a required check is evaluated only for {sorted(ELIGIBLE)})",
                file=sys.stderr,
            )
            raise SystemExit(1)
        if "pull_request_review" not in events:
            print(
                f"release-pr-check-lane: FAIL ({context} on {base} declares no pull_request_review eligible event, so no credential-free event can make it count)",
                file=sys.stderr,
            )
            raise SystemExit(1)
        for event in sorted({str(event) for event in events}):
            print(f"eligible\t{base}\t{context}\t{event}\t{verifier}")
PY
)" || exit 1

# The block of a top-level trigger, keyed by its name inside `on:`.
trigger_block() {
  awk -v key="$2" '
    $0 ~ "^  " key ":[ \t]*$" { inside = 1; next }
    inside && /^  [A-Za-z_]/ { inside = 0 }
    inside { print }
  ' "$1"
}

# The block of a job, keyed by its id.
job_block() {
  awk -v job="$2" '
    $0 ~ "^  " job ":[ \t]*$" { inside = 1; print; next }
    inside && /^  [A-Za-z_]/ { inside = 0 }
    inside { print }
  ' "$1"
}

# Whether a `branches:` list in the given block names the branch, in either the
# inline (`branches: [a, b]`) or the block (`branches:` then `- a`) form.
block_lists_branch() {
  awk -v want="$2" '
    function check(value,   parts, n, i) {
      gsub(/[][,"\047]/, " ", value)
      n = split(value, parts, /[ \t]+/)
      for (i = 1; i <= n; i++) {
        if (parts[i] == want) { found = 1 }
      }
    }
    /^[ \t]+branches:/ {
      value = $0
      sub(/^[ \t]+branches:[ \t]*/, "", value)
      if (value ~ /\[/) { check(value) } else { listing = 1 }
      next
    }
    listing && /^[ \t]*-[ \t]*/ {
      value = $0
      sub(/^[ \t]*-[ \t]*/, "", value)
      check(value)
      next
    }
    listing && !/^[ \t]*-[ \t]*/ { listing = 0 }
    END { exit(found ? 0 : 1) }
  ' <<<"$1"
}

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
        report "${producer_workflow} does not run the release check lane for its generated pull request"
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
        continue
      fi
      if ! grep -Eq '^[[:space:]]+workflow_dispatch:[[:space:]]*($|#)' "${path}"; then
        report "${verifier_workflow} reports ${context} but has no workflow_dispatch path"
      fi
      if ! grep -Eq '^[[:space:]]+release_pr_number:[[:space:]]*$' "${path}"; then
        report "${verifier_workflow} has no release_pr_number dispatch input to bind the head"
      fi
      if ! grep -Eq "^[[:space:]]{2}${job}:[[:space:]]*$" "${path}"; then
        report "${verifier_workflow} has no job named ${job}, so ${context} would never report"
        continue
      fi
      block="$(job_block "${path}" "${job}")"
      if grep -Eq '^[[:space:]]{4}name:' <<<"${block}"; then
        report "${verifier_workflow} job ${job} sets a name:, so it would not report the context ${context}"
      fi
      # A job-level `if:` decides whether the job reports at all. One that keeps
      # only the pull_request event would leave the eligible review run green and
      # jobless, so the context would never arrive.
      job_if="$(grep -E '^[[:space:]]{4}if:' <<<"${block}" || true)"
      if [[ -n "${job_if}" ]] && grep -Fq "github.event_name" <<<"${job_if}" &&
        ! grep -Fq "pull_request_review" <<<"${job_if}"; then
        report "${verifier_workflow} job ${job} excludes the pull_request_review event through its if:"
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
    eligible)
      base="${a}"
      context="${b}"
      event="${c}"
      verifier_workflow="${d}"
      path="${workflows}/${verifier_workflow}"
      if [[ ! -f "${path}" ]]; then
        report "required context ${context} names a missing workflow ${verifier_workflow}"
        continue
      fi
      if ! grep -Eq "^  ${event}:[[:space:]]*($|#)" "${path}"; then
        report "${verifier_workflow} has no ${event} trigger, so a ${event} run can never make ${context} count on ${base}"
        continue
      fi
      block="$(trigger_block "${path}" "${event}")"
      if ! block_lists_branch "${block}" "${base}"; then
        report "${verifier_workflow} does not run ${event} for ${base}, so ${context} can never count there"
      fi
      if [[ "${event}" == "pull_request_review" ]]; then
        if ! grep -Eq '^[[:space:]]+types:.*\bsubmitted\b' <<<"${block}"; then
          report "${verifier_workflow} does not subscribe the pull_request_review trigger to submitted"
        fi
        # A review on a staging pull request must not become a release-lane run.
        if block_lists_branch "${block}" "staging"; then
          report "${verifier_workflow} also runs pull_request_review for staging, which gains no release-lane run"
        fi
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

# The lane's own claims are part of the wiring: it must never count a skipped or
# neutral context as proof, and it must name the eligible event that does satisfy
# the ruleset rather than imply its dispatch does.
if [[ -f "${lane}" ]]; then
  if grep -Eq 'PASS = \{[^}]*"(skipped|neutral)"' "${lane}"; then
    report "the release check lane counts a skipped or neutral context as green"
  fi
  if ! grep -Fq "pull_request_review" "${lane}"; then
    report "the release check lane does not name the pull_request_review event that satisfies the ruleset"
  fi
else
  report "missing ${lane}; the lane script is part of the release wiring"
fi

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
    cp "${lane}" "${dir}/scripts/sync-release-pr-checks.sh"
    printf '%s\n' "${dir}"
  }

  # Set the branches: list of one top-level trigger, leaving every other trigger
  # untouched.
  set_trigger_branches() {
    local path="$1"
    local trigger="$2"
    local replacement="$3"
    python3 - "${path}" "${trigger}" "${replacement}" <<'PY'
import re
import sys

path, trigger, replacement = sys.argv[1], sys.argv[2], sys.argv[3]
lines = open(path, encoding="utf-8").read().splitlines(keepends=True)
out = []
inside = False
for line in lines:
    if re.match(r"^  " + re.escape(trigger) + r":\s*$", line):
        inside = True
        out.append(line)
        continue
    if inside and re.match(r"^  \S", line):
        inside = False
    if inside and re.match(r"^    branches:", line):
        out.append(f"    branches: {replacement}\n")
        continue
    out.append(line)
open(path, "w", encoding="utf-8").write("".join(out))
PY
  }

  # The unmodified copy passes: the guard reads only the declaration, the
  # workflows and the lane, so a faithful copy must be accepted before mutations
  # mean anything.
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
  expect_fixture_failure "a producer that stops running the lane fails" \
    "does not run the release check lane" "${fixture}"

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

  # 7. The eligible event removed: the code-owner review would report nothing,
  #    and a dispatched run would never satisfy the ruleset.
  fixture="$(new_fixture no-review-trigger)"
  python3 - "${fixture}/.github/workflows/release-hygiene.yml" <<'PY'
import re
import sys

path = sys.argv[1]
lines = open(path, encoding="utf-8").read().splitlines(keepends=True)
out = []
skipping = False
for line in lines:
    if line.rstrip("\n") == "  pull_request_review:":
        skipping = True
        continue
    if skipping:
        if line.startswith("    "):
            continue
        skipping = False
    out.append(line)
open(path, "w", encoding="utf-8").write("".join(out))
PY
  expect_fixture_failure "a verifying workflow without the eligible-event trigger fails" \
    "has no pull_request_review trigger" "${fixture}"

  # 8. The eligible trigger narrowed away from submitted reviews, so the approval
  #    that the ruleset already requires would run no check.
  fixture="$(new_fixture review-not-submitted)"
  sed -i.bak 's|types: \[submitted\]|types: [edited]|' \
    "${fixture}/.github/workflows/release-hygiene.yml"
  rm -f "${fixture}/.github/workflows/release-hygiene.yml.bak"
  expect_fixture_failure "a review trigger that drops submitted fails" \
    "does not subscribe the pull_request_review trigger to submitted" "${fixture}"

  # 9. The eligible trigger that does not cover the declared release base.
  fixture="$(new_fixture review-other-base)"
  set_trigger_branches "${fixture}/.github/workflows/release-hygiene.yml" pull_request_review '["staging"]'
  expect_fixture_failure "a review trigger outside the release base fails" \
    "does not run pull_request_review for premain" "${fixture}"

  # 10. The eligible trigger that also fires for staging pull requests, which
  #     must gain no release-lane run.
  fixture="$(new_fixture review-on-staging)"
  set_trigger_branches "${fixture}/.github/workflows/release-hygiene.yml" pull_request_review '["staging", "premain", "main"]'
  expect_fixture_failure "a review trigger that also covers staging fails" \
    "also runs pull_request_review for staging" "${fixture}"

  # 11. A job-level condition that keeps only the pull_request event: the review
  #     run would start and the job would skip, so the context never arrives.
  fixture="$(new_fixture job-if-excludes-review)"
  python3 - "${fixture}/.github/workflows/release-hygiene.yml" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
path.write_text(
    text.replace(
        "    runs-on: ubuntu-latest\n",
        "    if: github.event_name == 'pull_request'\n    runs-on: ubuntu-latest\n",
        1,
    ),
    encoding="utf-8",
)
PY
  expect_fixture_failure "a job condition that excludes the review event fails" \
    "excludes the pull_request_review event" "${fixture}"

  # 12. A job that renames its context, so the ruleset's context name would no
  #     longer be what the job reports.
  fixture="$(new_fixture renamed-context)"
  python3 - "${fixture}/.github/workflows/release-hygiene.yml" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
path.write_text(
    text.replace(
        "    runs-on: ubuntu-latest\n",
        "    name: Release hygiene\n    runs-on: ubuntu-latest\n",
        1,
    ),
    encoding="utf-8",
)
PY
  expect_fixture_failure "a job that renames its context fails" \
    "sets a name:" "${fixture}"

  # 13. The lane counting a skipped context as green: a skipped check proves
  #     nothing about the tree.
  fixture="$(new_fixture lane-skip-as-pass)"
  python3 - "${fixture}/scripts/sync-release-pr-checks.sh" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
path.write_text(
    text.replace('PASS = {"success"}', 'PASS = {"success", "neutral", "skipped"}', 1),
    encoding="utf-8",
)
PY
  expect_fixture_failure "a lane that counts a skipped context as green fails" \
    "counts a skipped or neutral context as green" "${fixture}"

  # 14. The lane that stops naming the eligible event, which is how a lane drifts
  #     back into implying its dispatch satisfies the ruleset.
  fixture="$(new_fixture lane-without-eligible-event)"
  python3 - "${fixture}/scripts/sync-release-pr-checks.sh" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
path.write_text(text.replace("pull_request_review", "review-event"), encoding="utf-8")
PY
  expect_fixture_failure "a lane that stops naming the eligible event fails" \
    "does not name the pull_request_review event" "${fixture}"

  # 15. The head-binding resolver itself. A stub `gh` answers the pull request
  #     read, so every hostile answer runs through the real validation.
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
  echo "release-pr-check-lane: PASS (every generated release PR has a head-bound dispatch path and an eligible-event path for each required context)"
fi
