#!/usr/bin/env bash
set -euo pipefail

# CI-safe local release-cycle checks. By default this verifier validates the
# checkout that contains the trusted script. Release Hygiene may pass an explicit
# target checkout root after same-repository provenance verification so trusted
# base scripts inspect PR-head files without executing PR-head scripts. Remote
# branch drift is reported by scripts/watch-release-cycle.sh.

failures=0

fail() {
  echo "release-cycle-state: FAIL ($1)"
  failures=$((failures + 1))
}

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-release-cycle-state.sh [--repo-root ABSOLUTE_PATH]

Options:
  --repo-root ABSOLUTE_PATH  Validate this checkout root instead of the script checkout.
  -h, --help                Show this help.
USAGE
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
script_repo_root="$(cd "${script_dir}/.." && pwd -P)"

# Bootstrap note: keep this helper self-contained on main until the v2
# release-cycle core helper is available on the target branch. This body is
# copied from origin/premain:scripts/lib/release-cycle-core.sh so trusted main
# release hygiene can validate PR-head v2 single-manifest state without adding
# scripts/lib/release-cycle-core.sh in the scoped bootstrap PR.
release_cycle_json_value_at_ref() {
  local ref="$1"
  local path="$2"
  local expr="$3"

  if ! git cat-file -e "${ref}:${path}" 2>/dev/null; then
    return 0
  fi

  git show "${ref}:${path}" 2>/dev/null | python3 -c '
import json
import sys

expr = sys.argv[1]
ref = sys.argv[2]
path = sys.argv[3]
try:
    data = json.load(sys.stdin)
except json.JSONDecodeError as exc:
    print(
        f"{ref}:{path} contains malformed JSON: {exc.msg} "
        f"at line {exc.lineno} column {exc.colno}",
        file=sys.stderr,
    )
    raise SystemExit(65)
if expr == ".":
    print(data.get(".", "") if isinstance(data, dict) else "")
    raise SystemExit(0)
value = data
for part in expr.split("."):
    if isinstance(value, dict):
        value = value.get(part, "")
    else:
        value = ""
if isinstance(value, bool):
    print("true" if value else "false")
elif value is None:
    print("")
elif isinstance(value, (str, int, float)):
    print(value)
else:
    print(json.dumps(value, separators=(",", ":")))
' "${expr}" "${ref}" "${path}"
}

release_cycle_json_string_value() {
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

release_cycle_toolchain_at_ref() {
  local ref="$1"
  local path="$2"
  git show "${ref}:${path}" 2>/dev/null | awk '$1 == "toolchain" { print $2; exit }'
}

release_cycle_semver_base() {
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

release_cycle_semver_lt() {
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

release_cycle_verify_local_state() {
  local repo_root="$1"

  (
    cd "${repo_root}"
    python3 - <<'PY'
import json
import os
import re
import subprocess
from pathlib import Path

semver_re = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$")


def read(path: str) -> object:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def fail(message: str) -> None:
    print(f"release-cycle-state: FAIL ({message})")
    raise SystemExit(1)


def version_info(label: str, version: str) -> tuple[tuple[int, int, int], bool]:
    match = semver_re.match(version.strip())
    if not match:
        fail(f"{label} has invalid semver {version!r}")
    base = tuple(int(part) for part in match.group(1, 2, 3))
    return base, bool(match.group(4))


if Path(".release-please-manifest.premain.json").exists():
    fail(".release-please-manifest.premain.json must be retired; use .release-please-manifest.json")

manifest = read(".release-please-manifest.json").get(".", "")
if not isinstance(manifest, str) or not manifest.strip():
    fail(".release-please-manifest.json is missing a version")

manifest_base, manifest_is_prerelease = version_info(".release-please-manifest.json", manifest)
stable_base = ".".join(str(part) for part in manifest_base)

current_branch = (
    os.environ.get("GITHUB_BASE_REF")
    or os.environ.get("GITHUB_REF_NAME")
    or ""
)
if not current_branch:
    try:
        current_branch = subprocess.check_output(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"], text=True
        ).strip()
    except Exception:
        current_branch = ""

pending_mode_raw = os.environ.get("RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION", "")
pending_mode = pending_mode_raw == "true"
if pending_mode_raw and pending_mode_raw not in {"true", "false"}:
    fail("RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION must be exactly true or false")

if pending_mode:
    if current_branch != "main":
        fail(
            "pending stable promotion mode is only allowed on main "
            f"(current branch: {current_branch})"
        )
    if os.environ.get("GITHUB_HEAD_REF", "") not in {"", "premain"}:
        fail(
            "pending stable promotion mode is only allowed for premain -> main "
            f"(head branch: {os.environ.get('GITHUB_HEAD_REF')})"
        )
    if not manifest_is_prerelease:
        fail(
            "pending stable promotion mode requires a single-manifest RC "
            f"(got {manifest})"
        )

    print(
        "release-cycle-state: PASS "
        f"(branch={current_branch}, mode=pending-stable-promotion, "
        f"rc={manifest}, stable={stable_base})"
    )
    raise SystemExit(0)

if current_branch == "main" and manifest_is_prerelease:
    fail(
        ".release-please-manifest.json is prerelease "
        f"{manifest} on main; release-pr.yml must normalize it to {stable_base}"
    )

if current_branch == "staging" and manifest_is_prerelease:
    fail(f"staging must not carry release-cycle RC state ({manifest})")

print(
    "release-cycle-state: PASS "
    f"(branch={current_branch}, manifest={manifest})"
)
PY
  )
}


repo_root="${script_repo_root}"
explicit_repo_root="${RELEASE_CYCLE_REPO_ROOT:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      if [[ $# -lt 2 ]]; then
        echo "release-cycle-state: FAIL (--repo-root requires a value)" >&2
        exit 2
      fi
      explicit_repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "release-cycle-state: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -n "${explicit_repo_root}" ]]; then
  if [[ "${explicit_repo_root}" != /* ]]; then
    echo "release-cycle-state: FAIL (--repo-root/RELEASE_CYCLE_REPO_ROOT must be an absolute path)" >&2
    exit 2
  fi
  if [[ ! -d "${explicit_repo_root}" ]]; then
    echo "release-cycle-state: FAIL (--repo-root/RELEASE_CYCLE_REPO_ROOT does not exist: ${explicit_repo_root})" >&2
    exit 2
  fi
  repo_root="$(cd "${explicit_repo_root}" && pwd -P)"
fi

cd "${repo_root}"

require_file() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    fail "missing ${path}"
  fi
}

required_files=(
  "scripts/prepare-release-package-versions.py"
  "scripts/verify-release-package-version-assets.py"
  "scripts/watch-release-cycle.sh"
  ".release-please-manifest.json"
  "ts/package.json"
  "ts/package-lock.json"
  "py/src/tabletheory_py/version.json"
)

for path in "${required_files[@]}"; do
  require_file "${path}"
done

if [[ "${failures}" -eq 0 ]]; then
  if ! release_cycle_verify_local_state "${repo_root}"; then
    failures=$((failures + 1))
  fi
fi

if grep -RInE 'git push +origin +(main|premain|staging)|git push +[^[:space:]]+ +(main|premain|staging)' .github/workflows scripts | grep -v 'verify-release-cycle-state.sh'; then
  fail "release automation contains direct branch mutation"
fi

if grep -RInE 'gh api .*git/refs/(heads/)?(main|premain|staging)|gh api .*contents/.*(main|premain|staging)' .github/workflows scripts | grep -v 'verify-release-cycle-state.sh'; then
  fail "release automation appears to mutate protected branch refs through GitHub API"
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "release-cycle-state: FAIL (${failures} issue(s))"
  exit 1
fi

echo "release-cycle-state: PASS"
