#!/usr/bin/env bash
# Shared release-cycle helpers for local gate and watch verifiers.
# Source this file from verifier scripts; do not execute it directly.

release_cycle_json_value_at_ref() {
  local ref="$1"
  local path="$2"
  local expr="$3"

  git show "${ref}:${path}" 2>/dev/null | python3 -c '
import json
import sys

expr = sys.argv[1]
data = json.load(sys.stdin)
if expr == ".":
    print(data.get(".", "") if isinstance(data, dict) else "")
    raise SystemExit(0)
value = data
for part in expr.split("."):
    if not part:
        continue
    if isinstance(value, dict):
        value = value.get(part, "")
    else:
        value = ""
print(value if isinstance(value, str) else "")
' "${expr}"
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


stable = read(".release-please-manifest.json").get(".", "")
premain = read(".release-please-manifest.premain.json").get(".", "")
ts_package = read("ts/package.json").get("version", "")
ts_lock = read("ts/package-lock.json")
ts_lock_root = ts_lock.get("version", "")
ts_lock_pkg = ts_lock.get("packages", {}).get("", {}).get("version", "")
py_version = read("py/src/theorydb_py/version.json").get("version", "")

promotion_files = {
    ".release-please-manifest.premain.json": premain,
    "ts/package.json": ts_package,
    "ts/package-lock.json": ts_lock_root,
    "ts/package-lock.json packages['']": ts_lock_pkg,
    "py/src/theorydb_py/version.json": py_version,
}

for label, version in {".release-please-manifest.json": stable, **promotion_files}.items():
    if not isinstance(version, str) or not version.strip():
        fail(f"{label} is missing a version")

stable_base, stable_is_prerelease = version_info(".release-please-manifest.json", stable)
if stable_is_prerelease:
    fail(f".release-please-manifest.json is prerelease {stable}")

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


def consistent_version(
    files: dict[str, str],
    mode_label: str,
) -> tuple[tuple[int, int, int], str, bool]:
    expected_base = None
    expected_version = None
    expected_is_prerelease = None
    for label, version in files.items():
        base, is_prerelease = version_info(label, version)
        if expected_version is None:
            expected_base = base
            expected_version = version
            expected_is_prerelease = is_prerelease
            continue
        if version != expected_version or base != expected_base:
            fail(
                f"{mode_label} files are inconsistent "
                f"({label} {version} != {expected_version})"
            )
    if (
        expected_base is None
        or expected_version is None
        or expected_is_prerelease is None
    ):
        fail(f"{mode_label} has no version files")
    return expected_base, expected_version, expected_is_prerelease


def stable_file_mismatch(branch: str) -> str | None:
    for label, version in promotion_files.items():
        base, is_prerelease = version_info(label, version)
        if is_prerelease:
            return f"{label} is prerelease {version} on {branch}"
        if base != stable_base or version != stable:
            return f"{label} {version} does not match stable manifest {stable}"
    return None


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

    pending_base, pending_version, pending_is_prerelease = consistent_version(
        promotion_files,
        "pending stable promotion",
    )
    if not pending_is_prerelease:
        fail(
            "pending stable promotion files must carry the promoted RC version "
            f"(got {pending_version})"
        )
    if pending_base <= stable_base:
        fail(
            "pending stable promotion version must be ahead of the stable manifest "
            f"({pending_version} <= {stable})"
        )

    print(
        "release-cycle-state: PASS "
        f"(branch={current_branch}, mode=pending-stable-promotion, "
        f"stable={stable}, pending={pending_version})"
    )
    raise SystemExit(0)

if current_branch == "main":
    mismatch = stable_file_mismatch(current_branch)
    if mismatch:
        fail(mismatch)

if current_branch == "staging":
    mismatch = stable_file_mismatch(current_branch)
    if mismatch:
        (
            reconciliation_base,
            reconciliation_version,
            reconciliation_is_prerelease,
        ) = consistent_version(
            promotion_files,
            "staging RC reconciliation",
        )
        if not reconciliation_is_prerelease:
            fail(mismatch)
        if reconciliation_base <= stable_base:
            fail(
                "staging RC reconciliation version must be ahead of the stable manifest "
                f"({reconciliation_version} <= {stable})"
            )
        print(
            "release-cycle-state: PASS "
            f"(branch={current_branch}, mode=staging-rc-reconciliation, "
            f"stable={stable}, rc={reconciliation_version})"
        )
        raise SystemExit(0)

print(
    "release-cycle-state: PASS "
    f"(branch={current_branch}, stable={stable}, premain={premain})"
)
PY
  )
}
