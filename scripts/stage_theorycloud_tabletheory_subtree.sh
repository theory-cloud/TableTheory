#!/usr/bin/env bash
set -euo pipefail

python3 - "$@" <<'PY'
from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from collections import OrderedDict
from datetime import datetime, timezone
from pathlib import Path

MODULE_NAME = "theorycloud"
SUBTREE_NAME = "tabletheory"
DEFAULT_SOURCE_REPO = "theory-cloud/TableTheory"

SURFACES = (
    {
        "root": "docs",
        "stage_prefix": "",
        "exclusions": (
            "development/**",
            "planning/**",
            "internal/**",
            "archive/**",
            "_contract.yaml",
            "development-guidelines.md",
            "PAY_THEORY_DOCUMENTATION_GUIDE.md",
        ),
    },
    {
        "root": "ts/docs",
        "stage_prefix": "ts",
        "exclusions": (
            "development/**",
            "planning/**",
            "internal/**",
            "archive/**",
            "_contract.yaml",
            "development-guidelines.md",
        ),
    },
    {
        "root": "py/docs",
        "stage_prefix": "py",
        "exclusions": (
            "development/**",
            "planning/**",
            "internal/**",
            "archive/**",
            "_contract.yaml",
            "development-guidelines.md",
        ),
    },
)


def fail(message: str) -> None:
    print(f"stage-theorycloud-tabletheory-subtree: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


def run_git(repo_root: Path, *args: str) -> str:
    result = subprocess.run(
        ["git", *args],
        cwd=repo_root,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return result.stdout.strip()


def normalize_remote_url(value: str | None) -> str:
    if not value:
        return DEFAULT_SOURCE_REPO

    url = value.strip()
    prefixes = (
        "https://github.com/",
        "http://github.com/",
        "ssh://git@github.com/",
        "git@github.com:",
    )
    for prefix in prefixes:
        if url.startswith(prefix):
            url = url[len(prefix) :]
            break
    url = url.removesuffix(".git").strip("/")
    return url or DEFAULT_SOURCE_REPO


def is_excluded(rel_path: str, exclusions: tuple[str, ...]) -> bool:
    path = Path(rel_path)
    parts = path.parts

    if "_contract.yaml" in exclusions and path.name == "_contract.yaml":
        return True
    if "development-guidelines.md" in exclusions and path.name == "development-guidelines.md":
        return True
    if "PAY_THEORY_DOCUMENTATION_GUIDE.md" in exclusions and path.name == "PAY_THEORY_DOCUMENTATION_GUIDE.md":
        return True
    if "development/**" in exclusions and parts and parts[0] == "development":
        return True
    if "planning/**" in exclusions and parts and parts[0] == "planning":
        return True
    if "internal/**" in exclusions and parts and parts[0] == "internal":
        return True
    if "archive/**" in exclusions and parts and parts[0] == "archive":
        return True

    return False


def stage_rel_path(stage_prefix: str, rel_path: str) -> str:
    if not stage_prefix:
        return rel_path
    return f"{stage_prefix}/{rel_path}"


def collect_surface(repo_root: Path, root: str, stage_prefix: str, exclusions: tuple[str, ...]) -> tuple[list[tuple[Path, str]], list[str]]:
    surface_root = (repo_root / root).resolve()
    if not surface_root.is_dir():
        fail(f"docs root {surface_root} does not exist")

    included: list[tuple[Path, str]] = []
    excluded: list[str] = []

    for path in sorted(surface_root.rglob("*")):
        if not path.is_file():
            continue
        rel_path = path.relative_to(surface_root).as_posix()
        staged_path = stage_rel_path(stage_prefix, rel_path)
        if is_excluded(rel_path, exclusions):
            excluded.append(staged_path)
            continue
        included.append((path, staged_path))

    return included, excluded


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, help="Directory where the tabletheory subtree should be staged")
    parser.add_argument("--repo-root", default=".", help="TableTheory repository root")
    parser.add_argument("--source-repo", default=None, help="Override source_repo in the subtree manifest")
    parser.add_argument(
        "--source-revision",
        default=None,
        help="Override source_revision in the subtree manifest (defaults to current git HEAD)",
    )
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    output_root = Path(args.output).resolve()

    if not (repo_root / ".git").exists():
        fail(f"repo root {repo_root} is not a git repository")

    included: list[tuple[Path, str]] = []
    excluded: list[str] = []

    for surface in SURFACES:
        surface_included, surface_excluded = collect_surface(
            repo_root,
            surface["root"],
            surface["stage_prefix"],
            surface["exclusions"],
        )
        included.extend(surface_included)
        excluded.extend(surface_excluded)

    staged_rel_paths = [staged_rel for _, staged_rel in included]
    duplicates = sorted({path for path in staged_rel_paths if staged_rel_paths.count(path) > 1})
    if duplicates:
        fail(f"staged path collision(s): {duplicates}")

    try:
        remote_url = run_git(repo_root, "config", "--get", "remote.origin.url")
    except subprocess.CalledProcessError:
        remote_url = DEFAULT_SOURCE_REPO
    source_repo = normalize_remote_url(args.source_repo or os.environ.get("SOURCE_REPO") or remote_url)

    try:
        source_revision = args.source_revision or os.environ.get("SOURCE_REVISION") or run_git(repo_root, "rev-parse", "HEAD")
    except subprocess.CalledProcessError as exc:
        fail(f"unable to determine source revision: {exc.stderr.strip()}")

    generated_at = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")

    subtree_root = output_root / SUBTREE_NAME
    shutil.rmtree(subtree_root, ignore_errors=True)
    subtree_root.mkdir(parents=True, exist_ok=True)

    for source_path, staged_rel in included:
        destination_path = subtree_root / staged_rel
        destination_path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source_path, destination_path)

    manifest = OrderedDict(
        [
            ("module", MODULE_NAME),
            ("subtree", SUBTREE_NAME),
            ("source_repo", source_repo),
            ("source_revision", source_revision),
            ("generated_at", generated_at),
            ("included_file_count", len(included)),
            ("excluded_file_count", len(excluded)),
            (
                "exclusion_rules",
                [
                    "development/**",
                    "planning/**",
                    "internal/**",
                    "archive/**",
                    "_contract.yaml",
                    "development-guidelines.md",
                    "PAY_THEORY_DOCUMENTATION_GUIDE.md",
                    "ts/_contract.yaml",
                    "ts/development-guidelines.md",
                    "py/_contract.yaml",
                    "py/development-guidelines.md",
                ],
            ),
        ]
    )
    (subtree_root / "source-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

    print(
        f"stage-theorycloud-tabletheory-subtree: PASS (output={subtree_root}; included={len(included)}; excluded={len(excluded)})"
    )


if __name__ == "__main__":
    main()
PY
