#!/usr/bin/env bash
set -euo pipefail

python3 - "$@" <<'PY'
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from urllib.parse import unquote

MODULE_NAME = "theorycloud"
SUBTREE_NAME = "tabletheory"
DEFAULT_SOURCE_REPO = "theory-cloud/TableTheory"
DEFAULT_OUTPUT_ROOT = "/tmp/theorycloud-tabletheory-source"
REQUIRED_FIELDS = [
    "module",
    "subtree",
    "source_repo",
    "source_revision",
    "generated_at",
    "included_file_count",
    "excluded_file_count",
    "exclusion_rules",
]
EXPECTED_EXCLUSION_RULES = [
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
]
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
    print(f"verify-theorycloud-tabletheory-subtree: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


def run(repo_root: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        list(args),
        cwd=repo_root,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )


def run_git(repo_root: Path, *args: str) -> str:
    return run(repo_root, "git", *args).stdout.strip()


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


def resolve_within_repo(repo_root: Path, path: Path, label: str) -> Path:
    try:
        resolved = path.resolve(strict=True)
    except FileNotFoundError:
        fail(f"{label} {path} does not exist")
    except OSError as exc:
        fail(f"unable to resolve {label} {path}: {exc}")

    try:
        resolved.relative_to(repo_root)
    except ValueError:
        fail(f"{label} {path} resolves outside repo root: {resolved}")

    return resolved


def collect_expected(repo_root: Path) -> tuple[list[str], list[str]]:
    included: list[str] = []
    excluded: list[str] = []

    for surface in SURFACES:
        surface_root = repo_root / surface["root"]
        if not surface_root.is_dir():
            fail(f"missing docs root {surface_root}")
        if surface_root.is_symlink():
            fail(f"docs root {surface_root} must not be a symlink")

        surface_root = resolve_within_repo(repo_root, surface_root, "docs root")
        for current_root, dirnames, filenames in os.walk(surface_root, topdown=True, followlinks=False):
            current_path = Path(current_root)

            safe_dirnames: list[str] = []
            for dirname in sorted(dirnames):
                dir_path = current_path / dirname
                if dir_path.is_symlink():
                    fail(f"symlinked docs input {dir_path.relative_to(repo_root)} is not allowed")
                resolve_within_repo(repo_root, dir_path, "docs directory")
                safe_dirnames.append(dirname)
            dirnames[:] = safe_dirnames

            for filename in sorted(filenames):
                path = current_path / filename
                if path.is_symlink():
                    fail(f"symlinked docs input {path.relative_to(repo_root)} is not allowed")

                resolve_within_repo(repo_root, path, "docs input")
                rel_path = path.relative_to(surface_root).as_posix()
                staged_rel = stage_rel_path(surface["stage_prefix"], rel_path)
                if is_excluded(rel_path, surface["exclusions"]):
                    excluded.append(staged_rel)
                else:
                    included.append(staged_rel)

    return sorted(dict.fromkeys(included)), sorted(dict.fromkeys(excluded))


LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.*?)(?:\s+#+\s*)?$")


def parse_link_target(raw: str) -> tuple[str, str]:
    raw = raw.strip()
    if raw.startswith("<") and raw.endswith(">"):
        raw = raw[1:-1].strip()
    raw = raw.split()[0]
    raw, _, frag = raw.partition("#")
    raw, _, _ = raw.partition("?")
    raw = raw.strip()
    frag = unquote(frag.strip())
    if frag.startswith("#"):
        frag = frag.lstrip("#")
    return raw, frag


def is_external(link: str) -> bool:
    lowered = link.strip().lower()
    return (
        not lowered
        or lowered.startswith("http://")
        or lowered.startswith("https://")
        or lowered.startswith("mailto:")
        or lowered.startswith("data:")
    )


def github_slug(text: str) -> str:
    text = text.strip().lower()
    text = re.sub(r"<[^>]+>", "", text)
    text = re.sub(r"\s+", "-", text)
    text = re.sub(r"[^a-z0-9_-]", "", text)
    text = re.sub(r"-{2,}", "-", text).strip("-")
    return text


def extract_anchors(md_content: str) -> set[str]:
    anchors: set[str] = set()
    seen: dict[str, int] = {}

    for line in md_content.splitlines():
        match = HEADING_RE.match(line)
        if not match:
            continue
        slug = github_slug(match.group(2))
        if not slug:
            continue

        count = seen.get(slug, 0)
        seen[slug] = count + 1
        anchors.add(slug if count == 0 else f"{slug}-{count}")

    for match in re.finditer(r"<a\s+[^>]*(?:id|name)=[\"']([^\"']+)[\"']", md_content, flags=re.IGNORECASE):
        anchors.add(match.group(1))

    return anchors


def verify_doc_links(subtree_root: Path) -> None:
    anchor_cache: dict[Path, set[str]] = {}

    for md in sorted(subtree_root.rglob("*.md")):
        content = md.read_text(encoding="utf-8", errors="replace")
        for match in LINK_RE.finditer(content):
            raw = match.group(1)
            if is_external(raw):
                continue

            target, fragment = parse_link_target(raw)
            if not target and not fragment:
                continue
            if target == ".":
                continue

            if not target and fragment:
                resolved = md.resolve()
            elif target.startswith("/"):
                resolved = (subtree_root / target.lstrip("/")).resolve()
            else:
                resolved = (md.parent / target).resolve()

            try:
                resolved.relative_to(subtree_root.resolve())
            except ValueError:
                fail(f"{md.relative_to(subtree_root)} links outside subtree: {raw}")

            if not resolved.exists():
                fail(f"{md.relative_to(subtree_root)} has broken link target {raw}")

            if fragment and resolved.suffix.lower() == ".md":
                anchors = anchor_cache.get(resolved)
                if anchors is None:
                    anchors = extract_anchors(resolved.read_text(encoding="utf-8", errors="replace"))
                    anchor_cache[resolved] = anchors
                if fragment not in anchors:
                    fail(f"{md.relative_to(subtree_root)} has broken link fragment {raw}")


def collect_staged_files(subtree_root: Path) -> list[str]:
    actual_rel_paths: list[str] = []

    for current_root, dirnames, filenames in os.walk(subtree_root, topdown=True, followlinks=False):
        current_path = Path(current_root)

        safe_dirnames: list[str] = []
        for dirname in sorted(dirnames):
            dir_path = current_path / dirname
            if dir_path.is_symlink():
                fail(f"staged subtree contains symlinked directory {dir_path.relative_to(subtree_root)}")
            safe_dirnames.append(dirname)
        dirnames[:] = safe_dirnames

        for filename in sorted(filenames):
            path = current_path / filename
            if path.is_symlink():
                fail(f"staged subtree contains symlinked file {path.relative_to(subtree_root)}")
            if path.name == "source-manifest.json":
                continue
            actual_rel_paths.append(path.relative_to(subtree_root).as_posix())

    return sorted(actual_rel_paths)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "output",
        nargs="?",
        default=os.environ.get("THEORYCLOUD_TABLETHEORY_SUBTREE_OUTPUT_DIR", DEFAULT_OUTPUT_ROOT),
        help="Directory containing the staged tabletheory subtree (default: %(default)s)",
    )
    parser.add_argument("--repo-root", default=".", help="TableTheory repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    output_root = Path(args.output).resolve()
    subtree_root = output_root / SUBTREE_NAME
    manifest_path = subtree_root / "source-manifest.json"

    if not (repo_root / ".git").exists():
        fail(f"repo root {repo_root} is not a git repository")

    run(repo_root, "bash", "./scripts/stage_theorycloud_tabletheory_subtree.sh", "--output", str(output_root))

    if not subtree_root.is_dir():
        fail(f"missing staged subtree {subtree_root}")
    if not manifest_path.is_file():
        fail(f"missing provenance manifest {manifest_path}")
    if (output_root / "docs").exists():
        fail(f"unexpected docs wrapper at {output_root / 'docs'}")
    if (subtree_root / "docs").exists():
        fail(f"unexpected docs wrapper at {subtree_root / 'docs'}")

    expected_included, expected_excluded = collect_expected(repo_root)
    actual_rel_paths = collect_staged_files(subtree_root)

    missing = sorted(set(expected_included) - set(actual_rel_paths))
    extra = sorted(set(actual_rel_paths) - set(expected_included))
    if missing:
        fail(f"missing staged files: {missing}")
    if extra:
        fail(f"unexpected staged files: {extra}")

    if not any(path == "README.md" for path in actual_rel_paths):
        fail("missing Go docs surface at subtree root")
    if not any(path.startswith("ts/") for path in actual_rel_paths):
        fail("missing TypeScript staged surface under ts/")
    if not any(path.startswith("py/") for path in actual_rel_paths):
        fail("missing Python staged surface under py/")

    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    missing_fields = [field for field in REQUIRED_FIELDS if field not in manifest]
    if missing_fields:
        fail(f"manifest missing required fields: {missing_fields}")

    if manifest["module"] != MODULE_NAME:
        fail(f"manifest module={manifest['module']!r} want {MODULE_NAME!r}")
    if manifest["subtree"] != SUBTREE_NAME:
        fail(f"manifest subtree={manifest['subtree']!r} want {SUBTREE_NAME!r}")

    try:
        remote_url = run_git(repo_root, "config", "--get", "remote.origin.url")
    except subprocess.CalledProcessError:
        remote_url = DEFAULT_SOURCE_REPO
    expected_source_repo = normalize_remote_url(os.environ.get("SOURCE_REPO") or remote_url)
    if manifest["source_repo"] != expected_source_repo:
        fail(f"manifest source_repo={manifest['source_repo']!r} want {expected_source_repo!r}")

    try:
        expected_source_revision = os.environ.get("SOURCE_REVISION") or run_git(repo_root, "rev-parse", "HEAD")
    except subprocess.CalledProcessError as exc:
        fail(f"unable to determine source revision: {exc.stderr.strip()}")
    if manifest["source_revision"] != expected_source_revision:
        fail(f"manifest source_revision={manifest['source_revision']!r} want {expected_source_revision!r}")

    try:
        datetime.fromisoformat(manifest["generated_at"].replace("Z", "+00:00"))
    except ValueError as exc:
        fail(f"manifest generated_at is not RFC3339: {exc}")

    if manifest["included_file_count"] != len(actual_rel_paths):
        fail(f"manifest included_file_count={manifest['included_file_count']} want {len(actual_rel_paths)}")
    if manifest["excluded_file_count"] != len(expected_excluded):
        fail(f"manifest excluded_file_count={manifest['excluded_file_count']} want {len(expected_excluded)}")
    if manifest["excluded_file_count"] < 0:
        fail("manifest excluded_file_count must be >= 0")
    if sorted(manifest["exclusion_rules"]) != sorted(EXPECTED_EXCLUSION_RULES):
        fail(
            "manifest exclusion_rules do not match expected value: "
            f"{sorted(manifest['exclusion_rules'])!r} want {sorted(EXPECTED_EXCLUSION_RULES)!r}"
        )

    verify_doc_links(subtree_root)

    print(
        f"verify-theorycloud-tabletheory-subtree: PASS (output={subtree_root}; included={len(actual_rel_paths)}; excluded={len(expected_excluded)})"
    )


if __name__ == "__main__":
    main()
PY
