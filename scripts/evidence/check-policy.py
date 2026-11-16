#!/usr/bin/env python3
"""Fail closed when the validation policy drifts from the CI that verifies it.

The policy registry (``scripts/evidence/validation-gates.json``) is the
machine-readable half of the execution-policy change; ``docs/development/
validation-evidence.md`` is the human half. This checker keeps the two in step
with the release-please configuration, the release-pull-request trigger filters
and the rubric's composition markers, so an extra release-only path, a gate that
silently stops composing, or a record path that stops being trackable all fail
here instead of in CI.

It also fails closed when a gate declares a proof profile that nothing verifies:
every record a steward can produce must be read by the verifying workflow or
re-verified through composition, or it is evidence no one checks.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
REGISTRY = ROOT / "scripts/evidence/validation-gates.json"
DOCS = ROOT / "docs/development/validation-evidence.md"
WORKFLOW = ROOT / ".github/workflows/quality-gates.yml"

APPROVED_GATES = {
    "rubric",
    "unit",
    "integration",
    "coverage",
    "format",
    "lint",
    "builds",
    "ts",
    "py",
    "go-integration",
    "examples-local",
}

# Gates the rubric composes instead of re-running. The rubric is the catch-all
# staging context, so each of these must keep both halves of the composition:
# the rubric command's reuse list and the gate script's consume call.
COMPOSED_GATES = {
    "unit",
    "integration",
    "coverage",
    "format",
    "lint",
    "builds",
}

REQUIRED_LEGS = {
    "pr-staging",
    "push-staging",
    "pr-staging-premain",
    "push-premain",
    "release-pr-premain",
    "release-merge-premain",
    "pr-premain-main",
    "push-main",
    "release-pr-main",
    "release-merge-main",
    "backmerge-main-staging",
}

REQUIRED_RELEASE_ONLY = (
    ".release-please-manifest.json",
    "CHANGELOG.md",
)


def fail(message: str) -> None:
    print(f"validation-policy: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


def is_release_only(relative: str, policy: dict) -> bool:
    if relative in set(policy.get("exact", [])):
        return True
    if any(relative.startswith(prefix) for prefix in policy.get("prefixes", [])):
        return True
    return any(fnmatch.fnmatchcase(relative, pattern) for pattern in policy.get("patterns", []))


def paths_ignore(workflow: str) -> list[str]:
    """The trigger-level paths-ignore entries of one workflow, or nothing.

    A release workflow that ignores nothing has no block to classify. The check
    that matters is that every path a workflow *does* ignore is a release-only
    path, so a release commit cannot silently stale a product record.
    """
    text = (ROOT / ".github/workflows" / workflow).read_text(encoding="utf-8")
    try:
        block = text.split("    paths-ignore:\n", 1)[1]
    except IndexError:
        return []
    entries: list[str] = []
    for line in block.splitlines():
        stripped = line.strip()
        if not stripped.startswith("- "):
            break
        entries.append(stripped[2:].strip().strip('"'))
    return entries


def gate_command_scripts(gate: dict) -> list[str]:
    scripts: list[str] = []
    for command in gate.get("commands", []):
        argv = command.get("argv") or []
        if len(argv) >= 2 and argv[0] == "bash":
            scripts.append(argv[1])
    return scripts


# One verification step per gate/profile pair. A step that names no profile
# verifies the gate's first declared profile, which is what verify-gate.sh
# resolves for itself.
VERIFY_STEP = re.compile(
    r"bash\s+scripts/evidence/verify-gate\.sh\s+--gate\s+"
    r"(?P<gate>[a-z0-9][a-z0-9-]*)"
    r"(?:\s+--profile\s+(?P<profile>[a-z0-9][a-z0-9.-]*))?"
)


def gate_profiles(gate: dict) -> list[str]:
    profiles = gate.get("profiles") or ["default"]
    return [str(item) for item in profiles]


def workflow_target_errors(registry: dict, workflow_text: str) -> list[str]:
    """Verification steps that name a gate or profile the registry does not.

    verify-gate.sh fails closed at run time, but a typo there would land as a
    red required check after the push instead of a policy failure before it.
    """
    gates = registry.get("gates") or {}
    errors: list[str] = []
    for match in VERIFY_STEP.finditer(workflow_text):
        gate_id = match.group("gate")
        profile = match.group("profile")
        gate = gates.get(gate_id)
        if not isinstance(gate, dict):
            errors.append(f"the verifying workflow names unknown gate {gate_id!r}")
            continue
        if profile is not None and profile not in gate_profiles(gate):
            errors.append(
                f"the verifying workflow names profile {profile!r} that gate {gate_id!r} does not declare"
            )
    return errors


def verified_targets(registry: dict, workflow_text: str) -> set[tuple[str, str]]:
    """The gate/profile pairs the verifying workflow actually proves.

    A workflow step proves the pair it names. A proven gate also re-verifies each
    gate it declares as a dependency at the same profile: the verifier walks the
    record's consumed coverage and refuses a dependency whose record is missing,
    stale, failed or produced for another profile. That closure is what lets the
    catch-all rubric cover the composed gates instead of executing them again.
    """
    gates = registry.get("gates") or {}
    verified: set[tuple[str, str]] = set()
    pending: list[tuple[str, str]] = []
    for match in VERIFY_STEP.finditer(workflow_text):
        gate_id = match.group("gate")
        gate = gates.get(gate_id)
        if not isinstance(gate, dict):
            continue
        profile = match.group("profile") or gate_profiles(gate)[0]
        if (gate_id, profile) not in verified:
            verified.add((gate_id, profile))
            pending.append((gate_id, profile))
    while pending:
        gate_id, profile = pending.pop()
        gate = gates.get(gate_id) or {}
        for dependency in gate.get("dependsOn") or []:
            dependency_gate = gates.get(str(dependency))
            if not isinstance(dependency_gate, dict):
                continue
            if profile not in gate_profiles(dependency_gate):
                continue
            pair = (str(dependency), profile)
            if pair not in verified:
                verified.add(pair)
                pending.append(pair)
    return verified


def record_coverage_gaps(registry: dict, workflow_text: str) -> list[str]:
    """Declared gate/profile pairs that nothing verifies.

    A gate declares every profile it can be collected under. Each of those pairs
    is a proof a steward can produce, so each must have a verification path: a
    step in the verifying workflow, or composition by a gate that step verifies.
    A record nothing verifies is evidence no one checks, which is exactly the
    hole this check closes.
    """
    gates = registry.get("gates") or {}
    verified = verified_targets(registry, workflow_text)
    gaps: list[str] = []
    for gate_id in sorted(gates):
        gate = gates[gate_id]
        if not isinstance(gate, dict):
            continue
        for profile in gate_profiles(gate):
            if (gate_id, profile) in verified:
                continue
            gaps.append(
                f"gate {gate_id!r} profile {profile!r} is producible but unverified: "
                "no verification step names it and no verified gate composes it "
                f"(add 'bash scripts/evidence/verify-gate.sh --gate {gate_id} --profile {profile}' "
                "to the verifying workflow, or drop the profile)"
            )
    return gaps


def report_problems(problems: list[str]) -> None:
    for problem in problems:
        print(f"validation-policy: FAIL ({problem})", file=sys.stderr)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registry", type=Path, default=REGISTRY, help="tracked gate registry")
    parser.add_argument("--workflow", type=Path, default=WORKFLOW, help="workflow that verifies the records")
    parser.add_argument(
        "--coverage-gaps",
        action="store_true",
        help="report and exit on gate/profile pairs nothing verifies; runs no other check",
    )
    args = parser.parse_args()

    if args.coverage_gaps:
        registry = json.loads(args.registry.read_text(encoding="utf-8"))
        workflow_text = args.workflow.read_text(encoding="utf-8")
        problems = workflow_target_errors(registry, workflow_text) + record_coverage_gaps(
            registry, workflow_text
        )
        if problems:
            report_problems(problems)
            return 1
        verified = verified_targets(registry, workflow_text)
        print(
            "validation-policy: covered "
            f"({len(verified)} recorded gate/profile pairs are verified by {args.workflow.name})"
        )
        return 0

    registry = json.loads(args.registry.read_text(encoding="utf-8"))
    if registry.get("schemaVersion") != 1:
        fail("registry schemaVersion must be 1")
    gates = registry.get("gates", {})
    if set(gates) != APPROVED_GATES:
        fail(f"approved gate set drifted: {sorted(set(gates) ^ APPROVED_GATES)}")

    runner = registry.get("runner") or {}
    if int(runner.get("maxWorkers", 0)) < 1:
        fail("runner.maxWorkers must be a positive conservative bound")
    resources = runner.get("resources") or {}
    if not resources:
        fail("runner.resources must declare the tracked resource groups")
    for name, capacity in resources.items():
        if int(capacity) < 1:
            fail(f"resource {name!r} must have a positive capacity")
    if not registry.get("nonProductPaths"):
        fail("nonProductPaths must declare the paths whose change cannot affect product proof")
    if not registry.get("securityInputs"):
        fail("securityInputs must declare the dependency surface of the repository")

    profiles = registry.get("proofProfiles")
    if not isinstance(profiles, dict) or not profiles:
        fail("registry must declare proofProfiles")
    for profile_id, profile in profiles.items():
        if not isinstance(profile, dict):
            fail(f"proof profile {profile_id} must be an object")
        environment = profile.get("environment")
        if not isinstance(environment, dict):
            fail(f"proof profile {profile_id} must declare an environment")
        for dimension in ("go", "node", "python"):
            if not isinstance(environment.get(dimension), str) or not environment[dimension]:
                fail(f"proof profile {profile_id} must declare {dimension}")
        platform_value = environment.get("platform")
        if not isinstance(platform_value, dict) or not platform_value.get("os") or not platform_value.get("arch"):
            fail(f"proof profile {profile_id} must declare platform os/arch")

    for gate_id, gate in gates.items():
        paths = gate.get("paths")
        if not isinstance(paths, list) or not paths:
            fail(f"gate {gate_id} must declare its input scope")
        gate_profiles = gate.get("profiles") or ["default"]
        if not isinstance(gate_profiles, list) or not gate_profiles:
            fail(f"gate {gate_id} must declare at least one supported proof profile")
        for profile_id in gate_profiles:
            if profile_id not in profiles:
                fail(f"gate {gate_id} supports undeclared proof profile {profile_id!r}")
        fallbacks = gate.get("fallbackProfiles", [])
        if not isinstance(fallbacks, list):
            fail(f"gate {gate_id} fallbackProfiles must be a list")
        for fallback_profile in fallbacks:
            if fallback_profile not in gate_profiles:
                fail(f"gate {gate_id} declares fallback profile {fallback_profile!r} it does not support")
        declared_resources = gate.get("resources", [gate.get("resource")])
        if isinstance(declared_resources, str):
            declared_resources = [declared_resources]
        if not declared_resources:
            fail(f"gate {gate_id} must declare its resource groups")
        for declared_resource in declared_resources:
            if declared_resource not in resources:
                fail(f"gate {gate_id} declares unknown resource {declared_resource!r}")
        for dependency in gate.get("dependsOn", []):
            if dependency not in gates:
                fail(f"gate {gate_id} depends on unknown gate {dependency!r}")
        composed_from = gate.get("composedFrom", [])
        if not isinstance(composed_from, list):
            fail(f"gate {gate_id} composedFrom must be a list")
        for dependency in composed_from:
            if dependency not in gates:
                fail(f"gate {gate_id} composes unknown gate {dependency!r}")
            if dependency not in gate.get("dependsOn", []):
                fail(f"gate {gate_id} composes {dependency!r} without a declared dependency")

    # The catch-all gate must stay the one gate whose scope is every path, so an
    # edit to verifier tooling under a nonProduct path still stales the record
    # the staging pull request verifies.
    catch_all = [gate_id for gate_id, gate in gates.items() if "**" in (gate.get("paths") or [])]
    if catch_all != ["rubric"]:
        fail(f"exactly the rubric gate must carry the catch-all ** scope, found {sorted(catch_all)}")

    rubric_reuse: list[str] = []
    for command in gates["rubric"].get("commands", []):
        reuse = (command.get("env") or {}).get("TABLE_THEORY_EVIDENCE_REUSE", "")
        rubric_reuse.extend([item for item in reuse.split(",") if item])
    if set(rubric_reuse) != COMPOSED_GATES:
        fail(
            "the rubric gate must compose exactly the recorded product gates; "
            f"reuses {sorted(set(rubric_reuse))}, expected {sorted(COMPOSED_GATES)}"
        )
    for gate_id in sorted(COMPOSED_GATES):
        scripts = gate_command_scripts(gates[gate_id])
        if not scripts:
            fail(f"composed gate {gate_id} must run a tracked shell script")
        consumed = False
        for script_path in scripts:
            path = ROOT / script_path
            if not path.is_file():
                fail(f"composed gate {gate_id} script {script_path} is missing")
            if f"reuse-if-recorded.sh {gate_id}" in path.read_text(encoding="utf-8"):
                consumed = True
        if not consumed:
            fail(f"composed gate {gate_id} never consumes its own record in {scripts[0]}")

    # Every declared gate/profile pair must be verified by the workflow or
    # composed by a gate that is. A record that is produced but never verified is
    # a proof with no reader, so it fails here instead of shipping.
    workflow_text = args.workflow.read_text(encoding="utf-8")
    coverage_problems = workflow_target_errors(registry, workflow_text) + record_coverage_gaps(
        registry, workflow_text
    )
    if coverage_problems:
        report_problems(coverage_problems)
        raise SystemExit(1)

    policy = registry["releaseOnly"]
    for prefix in policy.get("prefixes", []):
        if not prefix.endswith("/") or len(prefix.strip("/")) < 3:
            fail(f"release-only prefix {prefix!r} is not a tightly named directory")

    for config_name in ("release-please-config.json", "release-please-config.premain.json"):
        config_path = ROOT / config_name
        if not config_path.is_file():
            continue
        config = json.loads(config_path.read_text(encoding="utf-8"))
        for package in config.get("packages", {}).values():
            for extra in package.get("extra-files", []):
                path = extra.get("path", "")
                if path and not is_release_only(path, policy):
                    fail(f"{config_name} extra-file {path} is not classified as release-only")

    for workflow in ("prerelease-pr.yml", "release-pr.yml"):
        for path in paths_ignore(workflow):
            if not is_release_only(path, policy):
                fail(f"{workflow} paths-ignore {path} is not classified as release-only")

    for required in REQUIRED_RELEASE_ONLY:
        if not is_release_only(required, policy):
            fail(f"release-only set must cover {required}")

    if int(registry.get("securityMaxAgeSeconds", -1)) != 86400:
        fail("registry securityMaxAgeSeconds must stay the tracked 24-hour default")

    matrix = {entry.get("id"): entry for entry in registry.get("executionMatrix", [])}
    if set(matrix) != REQUIRED_LEGS:
        fail(f"execution matrix drifted: {sorted(set(matrix) ^ REQUIRED_LEGS)}")
    for leg_id, entry in matrix.items():
        if not entry.get("old") or not entry.get("new"):
            fail(f"execution matrix leg {leg_id} must record both old and new execution")

    docs = DOCS.read_text(encoding="utf-8")
    for leg_id in sorted(REQUIRED_LEGS):
        if f"| `{leg_id}` |" not in docs:
            fail(f"docs table is missing execution leg {leg_id}")
    for token in (
        "--force",
        "--dry-run",
        "zero commands",
        "resource group",
        "declared input scope",
        "--profile",
        "proof profile",
        "fallback profile",
        "production host",
        "produce evidence locally",
        "no gate executes",
        "produced but unverified",
    ):
        if token not in docs:
            fail(f"docs must document the operator surface: missing {token!r}")

    result = subprocess.run(
        ["git", "check-ignore", "-q", "gov-infra/evidence/validation/rubric/default.json"],
        cwd=ROOT,
        check=False,
    )
    if result.returncode == 0:
        fail("evidence records must be trackable, but the records path is git-ignored")
    result = subprocess.run(
        ["git", "check-ignore", "-q", "gov-infra/evidence/validation/local/rubric.log"],
        cwd=ROOT,
        check=False,
    )
    if result.returncode != 0:
        fail("local evidence logs must stay ignored")

    print(
        "validation-policy: PASS "
        f"({len(gates)} gates, {len(verified_targets(registry, workflow_text))} verified gate/profile pairs, "
        f"{len(COMPOSED_GATES)} composed, {len(matrix)} execution legs, "
        "release-only paths cover both release triggers)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
