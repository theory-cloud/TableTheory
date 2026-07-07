#!/usr/bin/env bash
set -euo pipefail

# Verifies the one allowed premain no-op state while main is pending stable
# promotion: premain and origin/main carry the exact same RC manifest, and the
# checked-out premain commit contains the deterministic stable Release PR
# machinery needed to repair main on the next premain -> main promotion.

repo_root=""

usage() {
  cat <<'USAGE'
Usage: bash scripts/verify-premain-pending-stable-repair.sh [--repo-root PATH]

Validates a premain checkout that should skip prerelease generation because
origin/main is already in pending stable promotion for the same RC. This command
is read-only except for fetching origin/main.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      repo_root="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "premain-pending-stable-repair: FAIL (unknown argument: $1)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${repo_root}" ]]; then
  repo_root="$(cd "${script_dir}/.." && pwd)"
fi
cd "${repo_root}"

fail() {
  echo "premain-pending-stable-repair: FAIL ($1)" >&2
  exit 1
}

branch="${GITHUB_REF_NAME:-}"
if [[ -z "${branch}" ]]; then
  branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
fi
[[ "${branch}" == "premain" ]] || fail "only premain can skip prerelease generation in pending stable repair mode"

[[ ! -f ".release-please-manifest.premain.json" ]] || \
  fail ".release-please-manifest.premain.json must be retired"

[[ -f "scripts/create-stable-release-pr.py" ]] || \
  fail "deterministic stable Release PR generator is missing"
grep -Fq "scripts/create-stable-release-pr.py" ".github/workflows/release-pr.yml" || \
  fail "release-pr.yml does not use the deterministic stable Release PR generator"
grep -Fq "autorelease: pending" "scripts/create-stable-release-pr.py" || \
  fail "deterministic stable Release PR generator does not apply the Release Please pending label"

git fetch --quiet --depth=1 origin main

candidate="$(python3 - <<'PY'
import json
from pathlib import Path

data = json.loads(Path(".release-please-manifest.json").read_text(encoding="utf-8"))
print(data.get(".", "") if isinstance(data, dict) else "")
PY
)"

main_manifest="$(git show origin/main:.release-please-manifest.json | python3 -c '
import json
import sys

data = json.load(sys.stdin)
print(data.get(".", "") if isinstance(data, dict) else "")
')"

PREMAIN_RC="${candidate}" MAIN_RC="${main_manifest}" python3 - <<'PY'
import os
import re
import sys

premain = os.environ["PREMAIN_RC"]
main = os.environ["MAIN_RC"]
rc_re = re.compile(r"^\d+\.\d+\.\d+-rc(?:\.\d+)?$")

if not rc_re.fullmatch(main):
    print(
        "premain-pending-stable-repair: FAIL "
        f"(origin/main is not pending stable promotion; manifest={main!r})",
        file=sys.stderr,
    )
    sys.exit(1)

if not rc_re.fullmatch(premain):
    print(
        "premain-pending-stable-repair: FAIL "
        f"(checked-out premain is not RC-shaped; manifest={premain!r})",
        file=sys.stderr,
    )
    sys.exit(1)

if premain != main:
    print(
        "premain-pending-stable-repair: FAIL "
        f"(checked-out premain RC {premain} does not match origin/main pending RC {main})",
        file=sys.stderr,
    )
    sys.exit(1)

stable = premain.split("-", 1)[0]
print(
    "premain-pending-stable-repair: PASS "
    f"(premain={premain}, origin/main={main}, stable={stable})"
)
PY
