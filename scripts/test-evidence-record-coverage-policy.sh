#!/usr/bin/env bash
set -euo pipefail

# Policy test for the record-coverage half of scripts/evidence/check-policy.py
# (rubric COM-6):
#
#   - a gate/profile pair that nothing verifies fails the policy
#   - composition by a verified gate counts as verification
#   - a verification step that names an undeclared gate or profile fails the
#     policy instead of landing as a red required check after the push
#
# The committed fixtures are minimal on purpose: they prove the check is not
# vacuous without mirroring the real registry, which would go stale. The derived
# cases then mutate a copy of the real workflow, so the real verify steps are
# proven load-bearing rather than merely present.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
checker="${repo_root}/scripts/evidence/check-policy.py"
fixtures="${repo_root}/scripts/fixtures/evidence-record-coverage"
registry="${repo_root}/scripts/evidence/validation-gates.json"
workflow="${repo_root}/.github/workflows/quality-gates.yml"

failures=0
scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

output=""
status=0

run_checker() {
  set +e
  output="$(cd "${repo_root}" && python3 "${checker}" --coverage-gaps "$@" 2>&1)"
  status=$?
  set -e
}

report_failure() {
  echo "evidence-record-coverage-policy: FAIL ($1)"
  printf '%s\n' "${output}" | sed 's/^/    /'
  failures=$((failures + 1))
}

expect_status() {
  local expected="$1" label="$2"
  if [[ "${expected}" == PASS && "${status}" -ne 0 ]]; then
    report_failure "${label}"
  elif [[ "${expected}" == FAIL && "${status}" -eq 0 ]]; then
    report_failure "${label}"
  fi
}

expect_message() {
  local needle="$1" label="$2"
  if ! grep -Fq -- "${needle}" <<< "${output}"; then
    report_failure "${label} (missing message: ${needle})"
  fi
}

# 1. A declared profile with no verification step and no composing gate fails.
run_checker --registry "${fixtures}/unverified/registry.json" --workflow "${fixtures}/unverified/quality-gates.yml"
expect_status FAIL "unverified declared profile"
expect_message "gate 'examples-local' profile 'default' is producible but unverified" "unverified declared profile"
expect_message "verify-gate.sh --gate examples-local --profile default" "unverified declared profile names the fix"

# 2. The same registry passes once that profile is verified explicitly.
run_checker --registry "${fixtures}/complete/registry.json" --workflow "${fixtures}/complete/quality-gates.yml"
expect_status PASS "explicitly verified declared profile"
expect_message "covered (3 recorded gate/profile pairs" "explicitly verified declared profile"

# 3. The real tree is fully covered: every declared pair is verified directly or
#    composed by the verified catch-all rubric.
run_checker --registry "${registry}" --workflow "${workflow}"
expect_status PASS "real registry and workflow"
expect_message "recorded gate/profile pairs are verified" "real registry and workflow"

# 4. The workflow's explicit steps are load-bearing: removing the examples
#    harness step from a copy of the real workflow makes the real registry fail.
cp "${workflow}" "${scratch}/without-examples.yml"
python3 - "${scratch}/without-examples.yml" <<'PY'
import sys
from pathlib import Path

path = Path(sys.argv[1])
lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
out = []
for line in lines:
    if "--gate examples-local --profile default" in line:
        out.pop()  # the step name
        continue
    out.append(line)
path.write_text("".join(out), encoding="utf-8")
PY
run_checker --registry "${registry}" --workflow "${scratch}/without-examples.yml"
expect_status FAIL "real workflow without the examples harness step"
expect_message "gate 'examples-local' profile 'default' is producible but unverified" "real workflow without the examples harness step"

# 5. Composition is what covers the rubric's dependencies: dropping the rubric
#    dependency on a recorded gate exposes that gate's record.
cp "${fixtures}/complete/registry.json" "${scratch}/no-dependency.json"
python3 - "${scratch}/no-dependency.json" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
registry = json.loads(path.read_text(encoding="utf-8"))
registry["gates"]["rubric"]["dependsOn"] = []
path.write_text(json.dumps(registry), encoding="utf-8")
PY
run_checker --registry "${scratch}/no-dependency.json" --workflow "${fixtures}/complete/quality-gates.yml"
expect_status FAIL "composed dependency removed"
expect_message "gate 'unit' profile 'default' is producible but unverified" "composed dependency removed"

# 6. A step that names a profile the gate does not declare fails the policy.
cp "${workflow}" "${scratch}/unknown-profile.yml"
sed 's/--gate ts --profile floor-node22/--gate ts --profile floor-node99/' "${workflow}" >"${scratch}/unknown-profile.yml"
run_checker --registry "${registry}" --workflow "${scratch}/unknown-profile.yml"
expect_status FAIL "verification step names an undeclared profile"
expect_message "names profile 'floor-node99' that gate 'ts' does not declare" "verification step names an undeclared profile"

# 7. A step that names a gate the registry does not declare fails the policy.
sed 's/--gate examples-local --profile default/--gate examples-locl --profile default/' "${workflow}" >"${scratch}/unknown-gate.yml"
run_checker --registry "${registry}" --workflow "${scratch}/unknown-gate.yml"
expect_status FAIL "verification step names an unknown gate"
expect_message "names unknown gate 'examples-locl'" "verification step names an unknown gate"

if [[ "${failures}" -ne 0 ]]; then
  echo "evidence-record-coverage-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "evidence-record-coverage-policy: PASS (every declared gate profile must be verified or composed; unknown steps fail closed)"
