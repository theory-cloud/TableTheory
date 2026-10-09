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

  if ! python3 - "${workflow}" <<'PY'
import sys
from pathlib import Path

lines = Path(sys.argv[1]).read_text(encoding="utf-8").splitlines()


def step_index(name: str) -> int:
    for i, line in enumerate(lines):
        if line.strip() == f"- name: {name}":
            return i
    raise SystemExit(f"missing step: {name}")


install = step_index("Install pinned publish tooling (before role assumption)")
assume = step_index("Assume stage-scoped theorycloud publish role")
if install > assume:
    raise SystemExit("publish tooling install step must precede role assumption")
PY
  then
    fail "publish tooling must install before the stage-scoped role is assumed"
  fi
fi

# --- 00ba12fe: awscurl failure-status injection --------------------------------
scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT
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

echo "theorycloud-publish-policy: PASS (pinned tooling before role assumption; awscurl status preserved)"
