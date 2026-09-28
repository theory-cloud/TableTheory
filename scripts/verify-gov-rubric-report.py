#!/usr/bin/env python3
"""Validate a gov-infra rubric report against the gov_rubric_report.v1 contract.

`gov-infra/verifiers/gov-verify-rubric.sh` writes
`gov-infra/evidence/gov-rubric-report.json` and exits; nothing then proves the
document it produced is a well-formed `gov_rubric_report.v1` report, which R-G1
requires. This validator closes that gap deterministically and offline.

The served schema document is a namespace resource that is not reachable from
the agent route, so this encodes the contract the pack-pinned emitter produces:
`schemaVersion` 1, the `$schema` URI, the required objects, the
PASS/FAIL/BLOCKED status enum, per-result id uniqueness, repo-relative evidence
paths that exist on disk, and summary counts that agree with the results list.

Usage (from the repository root):
  python3 scripts/verify-gov-rubric-report.py [REPORT]
  python3 scripts/verify-gov-rubric-report.py --self-test
"""

from __future__ import annotations

import copy
import json
import pathlib
import re
import sys

SCHEMA_URI = "https://gov.pai.dev/schemas/gov-rubric-report.schema.json"
SCHEMA_VERSION = 1
STATUSES = ("PASS", "FAIL", "BLOCKED")
REQUIRED_TOP_LEVEL = (
    "$schema",
    "schemaVersion",
    "timestamp",
    "pack",
    "project",
    "summary",
    "results",
)
RESULT_FIELDS = ("id", "category", "status", "message", "evidencePath")
TIMESTAMP_RE = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z")
GIT_HEAD_RE = re.compile(r"[0-9a-f]{40}")


def problems_for(report: object, repo_root: pathlib.Path) -> list[str]:
    """Return every contract violation found in `report` (empty when valid)."""
    problems: list[str] = []

    if not isinstance(report, dict):
        return ["report must be a JSON object"]

    for key in REQUIRED_TOP_LEVEL:
        if key not in report:
            problems.append(f"missing required key: {key}")
    if problems:
        return problems

    if report["$schema"] != SCHEMA_URI:
        problems.append(f"$schema must be {SCHEMA_URI!r}, got {report['$schema']!r}")
    if report["schemaVersion"] != SCHEMA_VERSION:
        problems.append(f"schemaVersion must be {SCHEMA_VERSION} (gov_rubric_report.v1)")

    timestamp = report["timestamp"]
    if not isinstance(timestamp, str) or not TIMESTAMP_RE.fullmatch(timestamp):
        problems.append(f"timestamp must be an RFC3339 UTC instant, got {timestamp!r}")

    git_head = report.get("git_head")
    if git_head is not None and not GIT_HEAD_RE.fullmatch(str(git_head)):
        problems.append(f"git_head must be a 40-hex commit id, got {git_head!r}")

    for key, fields in (("pack", ("version", "digest")), ("project", ("name", "slug"))):
        value = report[key]
        if not isinstance(value, dict):
            problems.append(f"{key} must be an object")
            continue
        for field in fields:
            if not value.get(field):
                problems.append(f"{key}.{field} must be a non-empty value")

    summary = report["summary"]
    if not isinstance(summary, dict):
        problems.append("summary must be an object")
        summary = {}

    status = summary.get("status")
    if status not in STATUSES:
        problems.append(f"summary.status must be one of {STATUSES}, got {status!r}")

    counts: dict[str, int] = {}
    for key in ("pass", "fail", "blocked"):
        value = summary.get(key)
        if not isinstance(value, int) or isinstance(value, bool) or value < 0:
            problems.append(f"summary.{key} must be a non-negative integer")
        else:
            counts[key] = value

    results = report["results"]
    if not isinstance(results, list) or not results:
        problems.append("results must be a non-empty array")
        results = []

    seen_ids: set[str] = set()
    derived = dict.fromkeys(counts, 0)
    for index, result in enumerate(results):
        where = f"results[{index}]"
        if not isinstance(result, dict):
            problems.append(f"{where} must be an object")
            continue

        for field in RESULT_FIELDS:
            if not isinstance(result.get(field), str) or result.get(field) == "":
                problems.append(f"{where}.{field} must be a non-empty string")

        result_id = result.get("id")
        if isinstance(result_id, str) and result_id:
            if result_id in seen_ids:
                problems.append(f"{where}.id is duplicated: {result_id}")
            seen_ids.add(result_id)

        result_status = result.get("status")
        if result_status not in STATUSES:
            problems.append(f"{where}.status must be one of {STATUSES}, got {result_status!r}")
        elif result_status.lower() in derived:
            derived[result_status.lower()] += 1

        evidence = result.get("evidencePath")
        if isinstance(evidence, str) and evidence:
            candidate = pathlib.PurePosixPath(evidence)
            if candidate.is_absolute() or ".." in candidate.parts:
                problems.append(
                    f"{where}.evidencePath must be a repo-relative path, got {evidence!r}"
                )
            elif not (repo_root / evidence).exists():
                problems.append(f"{where}.evidencePath does not exist: {evidence}")

    if counts and derived and counts != derived:
        problems.append(f"summary counts {counts} disagree with results {derived}")

    if counts and status in STATUSES and len(counts) == 3:
        if counts["fail"]:
            expected = "FAIL"
        elif counts["blocked"]:
            expected = "BLOCKED"
        else:
            expected = "PASS"
        if status != expected:
            problems.append(
                f"summary.status {status!r} disagrees with counts {counts}; expected {expected!r}"
            )

    return problems


def _valid_report(repo_root: pathlib.Path) -> dict:
    evidence = "gov-infra/planning/theorydb-10of10-rubric.md"
    assert (repo_root / evidence).exists()
    return {
        "$schema": SCHEMA_URI,
        "schemaVersion": SCHEMA_VERSION,
        "timestamp": "2026-09-28T12:00:00Z",
        "git_head": "0" * 40,
        "pack": {"version": "816465a1618d", "digest": "1" * 64},
        "project": {"name": "theorydb", "slug": "theorydb"},
        "summary": {"status": "PASS", "pass": 2, "fail": 0, "blocked": 0},
        "results": [
            {
                "id": "QUA-1",
                "category": "Quality",
                "status": "PASS",
                "message": "Command succeeded",
                "evidencePath": evidence,
            },
            {
                "id": "COM-1",
                "category": "Completeness",
                "status": "PASS",
                "message": "Command succeeded",
                "evidencePath": evidence,
            },
        ],
    }


def self_test(repo_root: pathlib.Path) -> int:
    """Prove the validator accepts a conforming report and rejects mutations."""
    base = _valid_report(repo_root)
    if problems_for(base, repo_root):
        print(f"gov-rubric-report: SELF-TEST FAIL (conforming report rejected: "
              f"{problems_for(base, repo_root)})")
        return 1

    def mutate(apply) -> bool:
        candidate = copy.deepcopy(base)
        apply(candidate)
        return bool(problems_for(candidate, repo_root))

    cases = {
        "schemaVersion drift": lambda r: r.__setitem__("schemaVersion", 2),
        "missing pack": lambda r: r.pop("pack"),
        "invalid summary status": lambda r: r["summary"].__setitem__("status", "GREEN"),
        "status contradicting counts": lambda r: r["summary"].__setitem__("fail", 1),
        "duplicate result id": lambda r: r["results"][1].__setitem__("id", "QUA-1"),
        "unknown result status": lambda r: r["results"][0].__setitem__("status", "WARN"),
        "absolute evidence path": lambda r: r["results"][0].__setitem__(
            "evidencePath", "/home/aron/evidence.log"
        ),
        "missing evidence path": lambda r: r["results"][0].__setitem__(
            "evidencePath", "gov-infra/evidence/does-not-exist.log"
        ),
    }

    failures = [name for name, apply in cases.items() if not mutate(apply)]
    if failures:
        print("gov-rubric-report: SELF-TEST FAIL (mutations accepted: "
              + ", ".join(sorted(failures))
              + ")")
        return 1

    print(f"gov-rubric-report: SELF-TEST PASS (conforming report accepted; "
          f"{len(cases)} mutations rejected)")
    return 0


def main(argv: list[str]) -> int:
    repo_root = pathlib.Path.cwd()
    args = [arg for arg in argv[1:] if not arg.startswith("-")]

    if "--self-test" in argv[1:]:
        return self_test(repo_root)

    unknown = [arg for arg in argv[1:] if arg.startswith("-")]
    if unknown:
        print(f"gov-rubric-report: FAIL (unknown option {unknown[0]})")
        return 1

    report_path = pathlib.Path(args[0]) if args else pathlib.Path(
        "gov-infra/evidence/gov-rubric-report.json"
    )

    try:
        report = json.loads(report_path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        print(f"gov-rubric-report: FAIL (missing {report_path}); run bash gov-infra/verifiers/gov-verify-rubric.sh first")
        return 1
    except json.JSONDecodeError as exc:
        print(f"gov-rubric-report: FAIL ({report_path} is not valid JSON: {exc})")
        return 1

    problems = problems_for(report, repo_root)
    if problems:
        print(f"gov-rubric-report: FAIL ({len(problems)} issue(s) in {report_path})")
        for problem in problems:
            print(f"- {problem}")
        return 1

    print(f"gov-rubric-report: VALID ({report_path} conforms to gov_rubric_report.v1)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
