#!/usr/bin/env bash
# Shared release-cycle helpers for local gate and watch verifiers.
# Source this file from verifier scripts; do not execute it directly.

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


def git_text(args: list[str]) -> str | None:
    try:
        return subprocess.check_output(
            ["git", *args],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
    except Exception:
        return None


def git_ok(args: list[str]) -> bool:
    return subprocess.run(
        ["git", *args],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    ).returncode == 0


def ref_exists(ref: str) -> bool:
    return git_ok(["rev-parse", "--verify", f"{ref}^{{commit}}"])


def manifest_at_ref(ref: str) -> str:
    raw = git_text(["show", f"{ref}:.release-please-manifest.json"])
    if not raw:
        return ""
    try:
        value = json.loads(raw).get(".", "")
    except json.JSONDecodeError:
        return ""
    return value if isinstance(value, str) else ""


def stable_baseline_reached(
    manifest_base: tuple[int, int, int],
) -> tuple[bool, str]:
    main_ref = "origin/main"
    if not ref_exists(main_ref):
        return False, ""
    main_manifest = manifest_at_ref(main_ref)
    if not main_manifest:
        return False, ""
    main_base, main_is_prerelease = version_info(
        "origin/main .release-please-manifest.json",
        main_manifest,
    )
    if not main_is_prerelease and main_base >= manifest_base:
        return True, (
            "origin/main already carries stable baseline "
            f"{main_manifest}; backmerge main to staging instead"
        )
    return False, ""


def verified_premain_rc_repair_to_staging(
    manifest: str,
    manifest_base: tuple[int, int, int],
) -> tuple[bool, str]:
    premain_ref = "origin/premain"
    if not ref_exists(premain_ref):
        return False, "origin/premain is not available for in-flight premain RC repair verification"
    if not git_ok(["merge-base", "--is-ancestor", premain_ref, "HEAD"]):
        return False, "origin/premain is not an ancestor of this staging repair candidate"

    staging_ref = "origin/staging"
    if ref_exists(staging_ref) and not git_ok(
        ["merge-base", "--is-ancestor", staging_ref, premain_ref],
    ):
        return (
            False,
            "origin/premain does not contain origin/staging; "
            "use the normal staging -> premain promotion path first",
        )

    premain_manifest = manifest_at_ref(premain_ref)
    if premain_manifest != manifest:
        return (
            False,
            "staging repair candidate RC manifest does not match origin/premain "
            f"({manifest} != {premain_manifest or 'missing'})",
        )

    stable_reached, stable_reason = stable_baseline_reached(manifest_base)
    if stable_reached:
        return False, stable_reason

    return True, ""


def verified_staging_rc_followup_repair(
    manifest: str,
    manifest_base: tuple[int, int, int],
) -> tuple[bool, str]:
    staging_ref = "origin/staging"
    if not ref_exists(staging_ref):
        return False, "origin/staging is not available for staging RC follow-up verification"
    if not git_ok(["merge-base", "--is-ancestor", staging_ref, "HEAD"]):
        return False, "origin/staging is not an ancestor of this staging follow-up candidate"

    staging_manifest = manifest_at_ref(staging_ref)
    if staging_manifest != manifest:
        return (
            False,
            "staging follow-up candidate RC manifest does not match origin/staging "
            f"({manifest} != {staging_manifest or 'missing'})",
        )

    stable_reached, stable_reason = stable_baseline_reached(manifest_base)
    if stable_reached:
        return False, stable_reason

    return True, ""


if Path(".release-please-manifest.premain.json").exists():
    fail(".release-please-manifest.premain.json must be retired; use .release-please-manifest.json")

manifest = read(".release-please-manifest.json").get(".", "")
if not isinstance(manifest, str) or not manifest.strip():
    fail(".release-please-manifest.json is missing a version")

manifest_base, manifest_is_prerelease = version_info(".release-please-manifest.json", manifest)
stable_base = ".".join(str(part) for part in manifest_base)

pull_request_events = {"pull_request", "pull_request_review", "pull_request_target"}


def is_pull_request_merge_ref(value: str) -> bool:
    return bool(re.fullmatch(r"[0-9]+/merge", value)) or value.startswith("refs/pull/")


def event_payload() -> dict:
    path = os.environ.get("GITHUB_EVENT_PATH", "")
    if not path:
        return {}
    try:
        payload = json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    return payload if isinstance(payload, dict) else {}


def payload_pull_request(payload: dict) -> dict:
    pull_request = payload.get("pull_request")
    return pull_request if isinstance(pull_request, dict) else {}


def payload_ref(payload: dict, side: str) -> str:
    ref = payload_pull_request(payload).get(side)
    if not isinstance(ref, dict):
        return ""
    value = ref.get("ref")
    return value if isinstance(value, str) else ""


def payload_describes_this_run(payload: dict) -> bool:
    expected_repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
    repository = payload.get("repository")
    if expected_repo and isinstance(repository, dict):
        full_name = repository.get("full_name")
        if isinstance(full_name, str) and full_name and full_name != expected_repo:
            return False
    merge_ref = re.fullmatch(
        r"refs/pull/([0-9]+)/merge",
        os.environ.get("GITHUB_REF", "").strip(),
    )
    if merge_ref:
        number = payload_pull_request(payload).get("number")
        if isinstance(number, int) and str(number) != merge_ref.group(1):
            return False
    return True


def resolve_branch_context() -> tuple[str, str]:
    event = os.environ.get("GITHUB_EVENT_NAME", "").strip()
    explicit_base = os.environ.get("GITHUB_BASE_REF", "").strip()
    explicit_head = os.environ.get("GITHUB_HEAD_REF", "").strip()

    if event not in pull_request_events:
        branch = explicit_base or os.environ.get("GITHUB_REF_NAME", "").strip()
        head = explicit_head
    else:
        # A pull_request_review run carries neither GITHUB_BASE_REF nor
        # GITHUB_HEAD_REF, and GITHUB_REF_NAME is the synthetic <number>/merge
        # ref. The verified pull request context is the only branch source.
        payload = event_payload()
        if payload and not payload_describes_this_run(payload):
            fail(
                f"{event} event payload does not describe this run "
                f"({os.environ.get('GITHUB_REPOSITORY', '<unknown>')})"
            )
        payload_base = payload_ref(payload, "base")
        payload_head = payload_ref(payload, "head")
        for label, explicit, payload_value in (
            ("base", explicit_base, payload_base),
            ("head", explicit_head, payload_head),
        ):
            if explicit and payload_value and explicit != payload_value:
                fail(
                    f"GITHUB_{label.upper()}_REF {explicit} disagrees with the "
                    f"{event} pull request {label} {payload_value}"
                )
        branch = explicit_base or payload_base
        head = explicit_head or payload_head
        if not branch:
            fail(
                f"cannot determine the pull request base branch for a {event} run "
                "(GITHUB_BASE_REF is unset and the event payload has no "
                "pull_request.base.ref)"
            )

    if branch and is_pull_request_merge_ref(branch):
        fail(
            f"pull request merge ref {branch} is not a branch; release-cycle "
            "classification must use the pull request base"
        )
    if not branch:
        try:
            branch = subprocess.check_output(
                ["git", "rev-parse", "--abbrev-ref", "HEAD"], text=True
            ).strip()
        except Exception:
            branch = ""
    if head and is_pull_request_merge_ref(head):
        fail(
            f"pull request merge ref {head} is not a branch; release-cycle "
            "classification must use the pull request head"
        )
    return branch, head


current_branch, current_head_branch = resolve_branch_context()

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
    if current_head_branch not in {"", "premain"}:
        fail(
            "pending stable promotion mode is only allowed for premain -> main "
            f"(head branch: {current_head_branch})"
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
    followup_ok, followup_reason = verified_staging_rc_followup_repair(
        manifest,
        manifest_base,
    )
    if followup_ok:
        print(
            "release-cycle-state: PASS "
            f"(branch={current_branch}, mode=staging-rc-followup-repair, "
            f"rc={manifest}, stable={stable_base})"
        )
        raise SystemExit(0)
    if followup_reason:
        print(f"release-cycle-state: INFO ({followup_reason})")

    repair_ok, repair_reason = verified_premain_rc_repair_to_staging(
        manifest,
        manifest_base,
    )
    if repair_ok:
        print(
            "release-cycle-state: PASS "
            f"(branch={current_branch}, mode=premain-rc-repair, "
            f"rc={manifest}, stable={stable_base})"
        )
        raise SystemExit(0)
    if repair_reason:
        print(f"release-cycle-state: INFO ({repair_reason})")
    fail(f"staging must not carry release-cycle RC state ({manifest})")

print(
    "release-cycle-state: PASS "
    f"(branch={current_branch}, manifest={manifest})"
)
PY
  )
}
