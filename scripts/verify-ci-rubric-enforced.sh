#!/usr/bin/env bash
set -euo pipefail

# COM-6: the repository's CI shape, and the rubric surfaces it depends on but
# does not read directly.
#
# The rule this enforces (docs/140, TMS MAI-4/REL-1 semantics): an item passes
# when its verifier result sits in a locally produced, input-bound evidence
# record that CI verifies on pull requests to staging. CI verifies the record,
# executes no gate, provisions no execution toolchain and runs no gate on a
# push. A missing or stale record fails with the command that produces one.
#
# This is intentionally a deterministic, text-based check; it is not a full YAML
# parser. scripts/verify-ci-toolchain.sh (COM-2) covers trigger classification
# and gate-execution needles across every workflow file.

wf=".github/workflows/quality-gates.yml"
registry="scripts/evidence/validation-gates.json"

if [[ ! -f "${wf}" ]]; then
  echo "ci-rubric: FAIL (missing ${wf})"
  exit 1
fi
if [[ ! -f "${registry}" ]]; then
  echo "ci-rubric: FAIL (missing ${registry})"
  exit 1
fi

failures=0

workflow_trigger_block() {
  local trigger="$1"

  awk -v trigger="${trigger}" '
    /^[^[:space:]][^:]*:[[:space:]]*/ {
      if ($0 ~ /^on:[[:space:]]*($|#)/) {
        in_on = 1
      } else if (in_on) {
        in_on = 0
      }
      in_trigger = 0
    }

    in_on && $0 ~ "^[[:space:]]{2}" trigger ":[[:space:]]*" {
      in_trigger = 1
      print
      next
    }

    in_trigger && $0 ~ "^[[:space:]]{2}[^[:space:]][^:]*:" {
      in_trigger = 0
      next
    }

    in_trigger {
      print
    }
  ' "${wf}"
}

pull_request_limited_to_staging() {
  local block="$1"

  if grep -Eq '^[[:space:]]{4}branches:[[:space:]]*\[[[:space:]]*"?staging"?[[:space:]]*\][[:space:]]*(#.*)?$' <<< "${block}"; then
    return 0
  fi

  awk '
    /^[[:space:]]{4}branches:[[:space:]]*($|#)/ {
      in_branches = 1
      found = 1
      next
    }

    in_branches && /^[[:space:]]{4}[^[:space:]][^:]*:/ {
      in_branches = 0
    }

    in_branches && /^[[:space:]]{6}-[[:space:]]*"?staging"?[[:space:]]*(#.*)?$/ {
      count++
      next
    }

    in_branches && /^[[:space:]]{6}-[[:space:]]*/ {
      bad = 1
    }

    END {
      exit (found && count == 1 && !bad) ? 0 : 1
    }
  ' <<< "${block}"
}

# The workflow is one verify job per tracked proof profile. The name is pinned
# because it is the required context on staging.
grep -Eq '^name:[[:space:]]*Quality Gates [(]10/10 Rubric[)][[:space:]]*$' "${wf}" || {
  echo "ci-rubric: ${wf}: missing expected workflow name"
  failures=$((failures + 1))
}

pull_request_block="$(workflow_trigger_block pull_request)"
if [[ -z "${pull_request_block}" ]]; then
  echo "ci-rubric: ${wf}: missing pull_request trigger"
  failures=$((failures + 1))
elif ! pull_request_limited_to_staging "${pull_request_block}"; then
  echo "ci-rubric: ${wf}: pull_request trigger must target only staging"
  failures=$((failures + 1))
fi

# A merge into staging re-verifies nothing: strict up-to-date checks make the
# merged tree the verified tree, so a push trigger here would be a second gate
# run over the same inputs.
if workflow_trigger_block push | grep -q .; then
  echo "ci-rubric: ${wf}: must not define a push trigger"
  failures=$((failures + 1))
fi

# Verification reads a committed record and needs only the shell and the Python
# interpreter the runner image provides. A provisioned toolchain here would be
# the first step of the execution path this policy removes.
for unprovisioned in \
  "actions/setup-go@" \
  "actions/setup-node@" \
  "actions/setup-python@" \
  "actions/upload-artifact@" \
  "github.com/golangci/golangci-lint"; do
  if grep -Fq -- "${unprovisioned}" "${wf}"; then
    echo "ci-rubric: ${wf}: verification must not provision or install ${unprovisioned}"
    failures=$((failures + 1))
  fi
done

# CI verifies; it never executes the rubric or a suite.
if grep -Eq 'run:\s*make rubric' "${wf}"; then
  echo "ci-rubric: ${wf}: must not execute 'make rubric'; verify the record instead"
  failures=$((failures + 1))
fi

for verify_line in \
  'bash scripts/evidence/verify-gate.sh --gate rubric --profile default' \
  'bash scripts/evidence/verify-gate.sh --gate ts --profile default' \
  'bash scripts/evidence/verify-gate.sh --gate py --profile default' \
  'bash scripts/evidence/verify-gate.sh --gate examples-local --profile default' \
  'bash scripts/evidence/verify-gate.sh --gate ts --profile floor-node22' \
  'bash scripts/evidence/verify-gate.sh --gate py --profile floor-py3.12' \
  'bash scripts/evidence/verify-gate.sh --gate ts --profile floor-node26' \
  'bash scripts/evidence/verify-gate.sh --gate py --profile floor-py3.15' \
  'bash scripts/evidence/verify-gate.sh --gate go-integration --profile linux-arm64'; do
  grep -Fq -- "${verify_line}" "${wf}" || {
    echo "ci-rubric: ${wf}: missing evidence verification step: ${verify_line}"
    failures=$((failures + 1))
  }
done

# Every declared gate/profile pair must be read by one of those steps or by
# composition, and each pinned step must name a pair the registry declares. This
# is the shape check that turns "a record exists" into "a record is verified".
if ! python3 scripts/evidence/check-policy.py --coverage-gaps; then
  echo "ci-rubric: a producible gate profile is not verified by ${wf} or composed by the rubric"
  failures=$((failures + 1))
fi

# The coverage rule must stay non-vacuous: the policy test feeds it an unverified
# profile, a removed composing dependency and unknown verify steps, and proves
# each one fails.
if ! bash scripts/test-evidence-record-coverage-policy.sh; then
  echo "ci-rubric: scripts/test-evidence-record-coverage-policy.sh failed"
  failures=$((failures + 1))
fi

# Release history must not be able to stale a record, and the class that says so
# must stay the reviewed allowlist. The controls above prove the coverage rule;
# this one proves the release-notes projection and its policy boundary.
if ! bash scripts/test-validation-evidence.sh; then
  echo "ci-rubric: scripts/test-validation-evidence.sh failed"
  failures=$((failures + 1))
fi

# A generated release pull request is opened with GITHUB_TOKEN, so its
# pull_request runs arrive in an approval-required state and a dispatched run
# never satisfies the ruleset; the eligible run is the code-owner review. The
# guard proves every required context of every generated release pull request has
# a head-bound dispatch path for the early verdict and the eligible
# pull_request_review trigger the ruleset evaluates, and its self-test proves the
# guard and the head binding reject hostile shapes.
if ! bash scripts/verify-release-pr-check-lane.sh --self-test; then
  echo "ci-rubric: scripts/verify-release-pr-check-lane.sh --self-test failed"
  failures=$((failures + 1))
fi

# The catch-all gate is what makes a tooling edit stale the record: the rubric
# record's scope is every path, so a change under any excluded directory still
# invalidates the record the staging pull request verifies.
python3 - "${registry}" <<'PY' || failures=$((failures + 1))
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
registry = json.loads(path.read_text(encoding="utf-8"))
problems = []

gates = registry.get("gates") or {}
catch_all = [gate_id for gate_id, gate in gates.items() if "**" in (gate.get("paths") or [])]
if catch_all != ["rubric"]:
    problems.append(f"exactly the rubric gate must carry the catch-all ** scope, found {sorted(catch_all)}")

rubric = gates.get("rubric") or {}
commands = rubric.get("commands") or []
if [command.get("argv") for command in commands] != [["make", "rubric"]]:
    problems.append("the tracked rubric gate must keep the single approved 'make rubric' command")
reuse = ""
for command in commands:
    reuse = (command.get("env") or {}).get("TABLE_THEORY_EVIDENCE_REUSE", "")
if not {"unit", "integration", "coverage", "format", "lint", "builds"} <= {item for item in reuse.split(",") if item}:
    problems.append("the rubric gate must compose the recorded product gates instead of re-running them")

for gate_id in ("ts", "py", "go-integration"):
    gate = gates.get(gate_id) or {}
    profiles = gate.get("profiles") or []
    if not profiles:
        problems.append(f"gate {gate_id} must declare its proof profiles")

if problems:
    for problem in problems:
        print(f"ci-rubric: {path}: {problem}")
    raise SystemExit(1)
PY

# The retired workflows must stay retired: each one re-executed a suite on a
# push or on a promotion pull request, which is exactly the duplication this
# change removes. Their coverage lives in the evidence record.
for retired in python.yml typescript.yml unit-cover.yml codeql.yml examples-local.yml; do
  if [[ -e ".github/workflows/${retired}" ]]; then
    echo "ci-rubric: .github/workflows/${retired}: must stay deleted; its coverage lives in the evidence record"
    failures=$((failures + 1))
  fi
done

# The SEC-2 dependency-scan aggregator is what carries the security gates into
# the rubric, so a gate dropped from it stops running without any other signal.
scan_aggregator="scripts/sec-dependency-scans.sh"
if [[ ! -f "${scan_aggregator}" ]]; then
  echo "ci-rubric: FAIL (missing ${scan_aggregator})"
  failures=$((failures + 1))
else
  for needle in \
    "bash scripts/sec-npm-audit.sh" \
    "bash scripts/test-npm-engines-floor-policy.sh" \
    "bash scripts/verify-npm-engines-floor.sh" \
    "bash scripts/test-lambda-runtime-deprecations-policy.sh" \
    "bash scripts/verify-lambda-runtime-deprecations.sh"; do
    grep -Fq -- "${needle}" "${scan_aggregator}" || {
      echo "ci-rubric: ${scan_aggregator}: must chain ${needle}"
      failures=$((failures + 1))
    }
  done
fi

# The Lambda-runtime gate's scanned scope is pinned on the QUOTED ARRAY ELEMENT,
# not on the bare path. The bare paths also appear in the checker's header
# comment, so an unanchored needle would still match that prose after the arrays
# were emptied; anchoring on the quoted element pins the constant instead of the
# comment. Each of the three surface classes is pinned, so the template and
# snippet classes cannot be dropped from the scope without failing here.
runtime_checker="scripts/check-lambda-runtime-deprecations.mjs"
if [[ ! -f "${runtime_checker}" ]]; then
  echo "ci-rubric: FAIL (missing ${runtime_checker})"
  failures=$((failures + 1))
else
  for needle in \
    '"examples/cdk-multilang/lib/multilang-demo-stack.ts",' \
    '"examples/cdk-multilang/lib/tabletheory-ttl-archive.ts",' \
    '"examples/blog/deployment/sam-template.yaml",' \
    '"examples/multi-tenant/deployment/template.yaml",' \
    '"examples/lambda/README.md"' \
    '"examples/cdk-multilang/lib"]'; do
    grep -Fq -- "${needle}" "${runtime_checker}" || {
      echo "ci-rubric: ${runtime_checker}: must pin the scanned scope entry ${needle}"
      failures=$((failures + 1))
    }
  done
fi

# The COM-2 toolchain verifier carries the governance guards (action pinning,
# lockfile-install hygiene, and the verify-only trigger shape). Each guard name
# must stay both defined and called, so deleting a call cannot silently retire a
# guard while the verifier still reports clean.
toolchain_verifier="scripts/verify-ci-toolchain.sh"
if [[ ! -f "${toolchain_verifier}" ]]; then
  echo "ci-rubric: FAIL (missing ${toolchain_verifier})"
  failures=$((failures + 1))
else
  for guard in check_action_pins check_npm_ci_across_repo check_verify_only_trigger_shape; do
    occurrences="$(grep -c -F -- "${guard}" "${toolchain_verifier}" || true)"
    if [[ "${occurrences}" -lt 2 ]]; then
      echo "ci-rubric: ${toolchain_verifier}: guard ${guard} must stay defined and called"
      failures=$((failures + 1))
    fi
  done
  grep -Fq -- '--ignore-scripts' "${toolchain_verifier}" || {
    echo "ci-rubric: ${toolchain_verifier}: must reject npm ci without --ignore-scripts"
    failures=$((failures + 1))
  }
fi

# The DynamoDB Local image is part of the integration and floor proof: a mutable
# tag would let a gate execute a different binary than the record was produced
# against. COM-6 pins the digest rule and runs the guard's own policy test, so the
# rule cannot be weakened back into a tag check without failing here.
pin_checker="scripts/verify-dynamodb-local-pin.sh"
if [[ ! -f "${pin_checker}" ]]; then
  echo "ci-rubric: FAIL (missing ${pin_checker})"
  failures=$((failures + 1))
else
  grep -Fq -- '@sha256:[0-9a-f]{64}' "${pin_checker}" || {
    echo "ci-rubric: ${pin_checker}: must require an image digest, not only a tag"
    failures=$((failures + 1))
  }
  if ! bash "${pin_checker}"; then
    echo "ci-rubric: ${pin_checker}: this tree names an unpinned DynamoDB Local image"
    failures=$((failures + 1))
  fi
  if ! bash scripts/test-dynamodb-local-pin-policy.sh; then
    echo "ci-rubric: ${pin_checker}: policy test failed"
    failures=$((failures + 1))
  fi
fi

# The evidence policy and its human documentation must stay in step, and the
# records must stay trackable while their local logs stay ignored.
if ! python3 scripts/evidence/check-policy.py; then
  echo "ci-rubric: scripts/evidence/check-policy.py failed"
  failures=$((failures + 1))
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "ci-rubric: FAIL (${failures} issue(s))"
  exit 1
fi

echo "ci-rubric: enforced (CI verifies the evidence record; it executes no gate)"
