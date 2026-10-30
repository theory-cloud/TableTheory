#!/usr/bin/env bash
set -euo pipefail

# Focused policy test for the TableTheory TheoryCloud publish lane.
#
# Findings:
#   9cc0c4ca  Publishing tools must be installed from an exact, hash-verified
#             lock BEFORE the stage-scoped AWS role is assumed, so no mutable
#             PyPI dependency executes with role credentials.
#   00ba12fe  The trigger helper must preserve awscurl's actual nonzero exit
#             status in diagnostics and fail closed.
#   R09       The publish target guard must require a branch ref type and an
#             exact full ref before checkout or OIDC. A same-named tag such as
#             refs/tags/premain must fail there, and stage/role must be derived
#             from the validated ref rather than the ref name alone.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workflow="${repo_root}/.github/workflows/theorycloud-tabletheory-publish.yml"
trigger="${repo_root}/scripts/trigger_theorycloud_publish.sh"
lock="${repo_root}/scripts/requirements/theorycloud-publish.txt"

failures=0
fail() {
  echo "theorycloud-publish-policy: FAIL ($1)"
  failures=$((failures + 1))
}

[[ -f "${workflow}" ]] || fail "missing ${workflow}"
[[ -f "${trigger}" ]] || fail "missing ${trigger}"
[[ -f "${lock}" ]] || fail "missing ${lock}"

scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

# --- 9cc0c4ca: dependency integrity + ordering --------------------------------
if [[ -f "${lock}" ]]; then
  grep -Eq '^awscurl==[0-9]' "${lock}" || fail "publish lock must pin an exact awscurl version"
  grep -q -- '--hash=sha256:' "${lock}" || fail "publish lock must carry sha256 hashes"
fi

if [[ -f "${workflow}" ]]; then
  grep -Fq -- "scripts/requirements/theorycloud-publish.txt" "${workflow}" \
    || fail "publish workflow must install the committed tooling lock"
  grep -Fq -- "--require-hashes" "${workflow}" \
    || fail "publish workflow must install with --require-hashes"
  grep -Fq -- "--only-binary=:all:" "${workflow}" \
    || fail "publish workflow must forbid sdist builds (--only-binary=:all:)"
  if grep -Fq -- "--upgrade pip" "${workflow}"; then
    fail "publish workflow must not upgrade pip unpinned"
  fi

  # R09: the guard is the first step, it reads the ref TYPE and the exact full
  # ref, and every credential or checked-out-script step comes after it.
  if ! python3 - "${workflow}" <<'PY'
import re
import sys
from pathlib import Path

lines = Path(sys.argv[1]).read_text(encoding="utf-8").splitlines()


def step_index(name):
    needle = f"- name: {name}"
    for i, line in enumerate(lines):
        if line.strip() == needle:
            return i
    raise SystemExit(f"missing step: {name}")


def step_end(start):
    for i in range(start + 1, len(lines)):
        if re.match(r"^      - name:", lines[i]):
            return i
    return len(lines)


guard = step_index("Resolve and validate publish target")
checkout = step_index("Checkout protected branch head")
validate = step_index("Validate publish workflow configuration")
install = step_index("Install pinned publish tooling (before role assumption)")
assume = step_index("Assume stage-scoped theorycloud publish role")

steps_at = next(i for i, line in enumerate(lines) if line.rstrip() == "    steps:")
first_step = next(i for i in range(steps_at + 1, len(lines)) if lines[i].lstrip().startswith("- name:"))
if guard != first_step:
    raise SystemExit("publish target guard must be the first job step")

if not guard < checkout < validate < install < assume:
    raise SystemExit("publish target guard must precede checkout, validation, install, and role assumption")

guard_body = "\n".join(lines[guard:step_end(guard)])
for needle in ("id: publish-target", "ref_type", "branch", "refs/heads/premain", "refs/heads/main"):
    if needle not in guard_body:
        raise SystemExit(f"publish target guard is missing {needle!r}")
if "uses:" in guard_body:
    raise SystemExit("publish target guard must run inline, not use an action")
if "GITHUB_REF_NAME" not in guard_body or "GITHUB_REF" not in guard_body:
    raise SystemExit("publish target guard must read both GITHUB_REF and GITHUB_REF_NAME")

checkout_body = "\n".join(lines[checkout:step_end(checkout)])
if "ref: ${{ steps.publish-target.outputs.ref }}" not in checkout_body:
    raise SystemExit("checkout must be pinned to the validated full ref")

assume_body = "\n".join(lines[assume:step_end(assume)])
if "role-to-assume: ${{ steps.publish-target.outputs.role_arn }}" not in assume_body:
    raise SystemExit("OIDC role must come from the validated ref outputs")

workflow_text = "\n".join(lines)
if "THEORYCLOUD_STAGE: ${{ github.ref_name" in workflow_text:
    raise SystemExit("stage must not be derived from the ref name alone")
if "AWS_ROLE_ARN: ${{ github.ref_name" in workflow_text:
    raise SystemExit("role must not be derived from the ref name alone")
if re.search(r"role-session-name:.*github\.ref_name", workflow_text):
    raise SystemExit("role session name must not be derived from the ref name alone")
if "THEORYCLOUD_PUBLISH_REASON: ${{ steps.publish-target.outputs.reason }}" not in workflow_text:
    raise SystemExit("publish reason must come from the validated ref outputs")
if workflow_text.count("THEORYCLOUD_STAGE: ${{ steps.publish-target.outputs.stage }}") < 2:
    raise SystemExit("stage output must feed every stage-scoped step")
PY
  then
    fail "publish workflow must validate branch ref type + exact full ref before checkout/OIDC"
  fi

  # Extract the actual guard script so the hostile cases run the shipped code,
  # not a copy of its logic.
  guard_script="${scratch}/publish-guard.sh"
  if ! python3 - "${workflow}" "${guard_script}" <<'PY'
import re
import sys
from pathlib import Path

lines = Path(sys.argv[1]).read_text(encoding="utf-8").splitlines()
start = None
for i, line in enumerate(lines):
    if line.strip() == "- name: Resolve and validate publish target":
        start = i
        break
if start is None:
    raise SystemExit("missing publish target guard")
run = None
for i in range(start + 1, len(lines)):
    if lines[i].strip() == "run: |":
        run = i
        break
    if re.match(r"^      - name:", lines[i]):
        break
if run is None:
    raise SystemExit("publish target guard has no run block")
body = []
for line in lines[run + 1:]:
    if re.match(r"^      - name:", line) or (line and not line.startswith("          ")):
        break
    body.append(line[10:] if line.startswith("          ") else "")
Path(sys.argv[2]).write_text("\n".join(body) + "\n", encoding="utf-8")
PY
  then
    fail "could not extract the publish target guard"
  fi
fi

run_guard() {
  local ref_type="$1" ref="$2" ref_name="$3" expected_status="$4" expected_text="$5"
  local output_file="${scratch}/guard.out"
  : >"${output_file}"

  local output status
  set +e
  output="$(
    GITHUB_REF_TYPE="${ref_type}" \
      GITHUB_REF="${ref}" \
      GITHUB_REF_NAME="${ref_name}" \
      GITHUB_REPOSITORY="theory-cloud/TableTheory" \
      GITHUB_OUTPUT="${output_file}" \
      bash "${scratch}/publish-guard.sh" 2>&1
  )"
  status=$?
  set -e

  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf '%s\n' "${output}"
    fail "publish guard for ${ref_type:-<empty>} ${ref:-<empty>}: expected exit ${expected_status}, got ${status}"
    return
  fi
  if [[ -n "${expected_text}" ]] && ! grep -Fq "${expected_text}" <<<"${output}"; then
    printf '%s\n' "${output}"
    fail "publish guard for ${ref_type:-<empty>} ${ref:-<empty>}: missing message '${expected_text}'"
  fi
  if [[ "${expected_status}" -ne 0 && -s "${output_file}" ]]; then
    fail "publish guard for ${ref} wrote stage/role outputs before failing"
  fi
}

# Protected branches keep their normal outcomes, and the stage/role are derived
# from the validated full ref.
run_guard branch refs/heads/premain premain 0 ""
grep -Fq "stage=lab" "${scratch}/guard.out" || fail "premain must resolve to the lab stage"
grep -Fq "role_arn=arn:aws:iam::787107040121:role/KnowledgeTheory-TheoryCloud-TableTheory-lab-Publisher" "${scratch}/guard.out" \
  || fail "premain must resolve to the lab publisher role"
grep -Fq "ref=refs/heads/premain" "${scratch}/guard.out" || fail "premain must pin the exact full ref"
grep -Fq "reason=github:theory-cloud/TableTheory:premain" "${scratch}/guard.out" \
  || fail "premain must derive the publish reason from the validated ref"

run_guard branch refs/heads/main main 0 ""
grep -Fq "stage=live" "${scratch}/guard.out" || fail "main must resolve to the live stage"
grep -Fq "role_arn=arn:aws:iam::787107040121:role/KnowledgeTheory-TheoryCloud-TableTheory-live-Publisher" "${scratch}/guard.out" \
  || fail "main must resolve to the live publisher role"

# The hostile that motivated the guard: a tag whose name is a protected branch.
run_guard tag refs/tags/premain premain 1 "publishing is limited to branch refs"
run_guard tag refs/tags/main main 1 "publishing is limited to branch refs"

# A tag cannot smuggle itself through by claiming a branch ref type, and a
# non-protected or nested ref cannot pass the exact-match check.
run_guard branch refs/tags/premain premain 1 "publishing is limited to refs/heads/premain and refs/heads/main"
run_guard branch refs/heads/feature/security-m7 feature/security-m7 1 "publishing is limited to refs/heads/premain and refs/heads/main"
run_guard branch refs/heads/premain/extra premain/extra 1 "publishing is limited to refs/heads/premain and refs/heads/main"
run_guard "" refs/heads/premain premain 1 "publishing is limited to branch refs"
run_guard branch refs/heads/premain main 1 "does not match the validated full ref"

# --- 00ba12fe: awscurl failure-status injection --------------------------------
mkdir -p "${scratch}/bin"
cat > "${scratch}/bin/awscurl" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -n "${out}" ]]; then
  printf '{"status":"error","message":"injected"}' > "${out}"
fi
exit "${STUB_AWSCURL_EXIT:-0}"
STUB
chmod +x "${scratch}/bin/awscurl"

run_trigger() {
  local injected="$1" expected_status="$2" expected_text="$3"
  local output status
  set +e
  output="$(STUB_AWSCURL_EXIT="${injected}" PATH="${scratch}/bin:${PATH}" \
    bash "${trigger}" --stage live 2>&1)"
  status=$?
  set -e
  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf '%s\n' "${output}"
    fail "trigger helper with awscurl exit ${injected}: expected exit ${expected_status}, got ${status}"
    return
  fi
  if [[ -n "${expected_text}" ]] && ! grep -Fq "${expected_text}" <<< "${output}"; then
    printf '%s\n' "${output}"
    fail "trigger helper with awscurl exit ${injected}: missing message '${expected_text}'"
  fi
}

# A failing awscurl must fail closed and keep the real status (7) in diagnostics.
run_trigger 7 1 "(exit 7)"
# A succeeding awscurl must pass.
run_trigger 0 0 "trigger-theorycloud-publish: PASS"

if [[ "${failures}" -ne 0 ]]; then
  echo "theorycloud-publish-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "theorycloud-publish-policy: PASS (branch/full-ref guard before checkout/OIDC; pinned tooling before role assumption; awscurl status preserved)"
