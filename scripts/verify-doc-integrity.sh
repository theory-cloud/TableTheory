#!/usr/bin/env bash
set -euo pipefail

# Checks repo-local markdown links resolve (files + section anchors) and that key version claims match go.mod.
#
# Scope:
# - README.md
# - docs/**/*.md

python3 - <<'PY'
from __future__ import annotations

import os
import re
import sys
from pathlib import Path
from urllib.parse import unquote


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8", errors="replace")


def iter_md_files(repo_root: Path) -> list[Path]:
    files: list[Path] = []
    readme = repo_root / "README.md"
    if readme.exists():
        files.append(readme)
    # Monorepo support: validate docs in the repo-level `docs/` directory and per-SDK docs
    # directories like `ts/docs/` and `py/docs/`.
    docs_dirs: list[Path] = []
    root_docs = repo_root / "docs"
    if root_docs.exists():
        docs_dirs.append(root_docs)

    for candidate in repo_root.glob("*/docs"):
        if candidate.is_dir():
            docs_dirs.append(candidate)

    for docs_dir in docs_dirs:
        files.extend(sorted(docs_dir.rglob("*.md")))
    return files


LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


def is_external(link: str) -> bool:
    link = link.strip()
    if not link:
        return True
    lowered = link.lower()
    return (
        lowered.startswith("http://")
        or lowered.startswith("https://")
        or lowered.startswith("mailto:")
        or lowered.startswith("data:")
    )


def parse_link_target(raw: str) -> tuple[str, str]:
    # Strip optional title: (path "title") or (path 'title')
    raw = raw.strip()
    if raw.startswith("<") and raw.endswith(">"):
        raw = raw[1:-1].strip()
    # Split off title if present.
    # This is a conservative split: first whitespace ends the URL.
    raw = raw.split()[0]

    raw, _, frag = raw.partition("#")
    raw, _, _ = raw.partition("?")

    raw = raw.strip()
    frag = unquote(frag.strip())
    if frag.startswith("#"):
        frag = frag.lstrip("#")

    return raw, frag


HEADING_RE = re.compile(r"^(#{1,6})\s+(.*?)(?:\s+#+\s*)?$")


def github_slug(text: str) -> str:
    text = text.strip().lower()
    # Remove simple HTML tags that occasionally appear in headings.
    text = re.sub(r"<[^>]+>", "", text)
    text = re.sub(r"\s+", "-", text)
    # Keep alphanumerics, underscore, and hyphen to match typical GitHub anchor behavior.
    text = re.sub(r"[^a-z0-9_-]", "", text)
    text = re.sub(r"-{2,}", "-", text).strip("-")
    return text


def extract_anchors(md_content: str) -> set[str]:
    anchors: set[str] = set()
    seen: dict[str, int] = {}

    for line in md_content.splitlines():
        m = HEADING_RE.match(line)
        if not m:
            continue
        slug = github_slug(m.group(2))
        if not slug:
            continue

        count = seen.get(slug, 0)
        seen[slug] = count + 1
        if count == 0:
            anchors.add(slug)
        else:
            anchors.add(f"{slug}-{count}")

    # Explicit HTML anchors are also valid targets.
    for m in re.finditer(r"<a\s+[^>]*(?:id|name)=[\"']([^\"']+)[\"']", md_content, flags=re.IGNORECASE):
        anchors.add(m.group(1))

    return anchors


def check_links(repo_root: Path) -> list[str]:
    problems: list[str] = []
    anchor_cache: dict[Path, set[str]] = {}

    for md in iter_md_files(repo_root):
        content = read_text(md)
        for m in LINK_RE.finditer(content):
            raw = m.group(1)
            if is_external(raw):
                continue

            target, fragment = parse_link_target(raw)
            if not target and not fragment:
                continue
            if target == ".":
                continue

            if not target and fragment:
                resolved = md.resolve()
            else:
                # Treat absolute repo paths as relative to repo root.
                if target.startswith("/"):
                    rel = target.lstrip("/")
                    resolved = (repo_root / rel).resolve()
                else:
                    resolved = (md.parent / target).resolve()

            # Only enforce links that resolve inside the repo.
            try:
                resolved.relative_to(repo_root.resolve())
            except ValueError:
                continue

            if not resolved.exists():
                problems.append(f"{md}: broken link target '{raw}' -> '{resolved.relative_to(repo_root)}'")
                continue

            if fragment and resolved.suffix.lower() == ".md":
                anchors = anchor_cache.get(resolved)
                if anchors is None:
                    anchors = extract_anchors(read_text(resolved))
                    anchor_cache[resolved] = anchors
                if fragment not in anchors:
                    problems.append(
                        f"{md}: broken link fragment '{raw}' -> '{resolved.relative_to(repo_root)}#{fragment}'"
                    )

    return problems


def parse_go_version(repo_root: Path) -> str | None:
    go_mod = repo_root / "go.mod"
    if not go_mod.exists():
        return None
    for line in read_text(go_mod).splitlines():
        if line.startswith("go "):
            return line.split()[1].strip()
    return None


def check_go_version_badge(repo_root: Path) -> list[str]:
    problems: list[str] = []
    readme = repo_root / "README.md"
    if not readme.exists():
        return problems

    go_ver = parse_go_version(repo_root)
    if not go_ver:
        return problems

    content = read_text(readme)
    # Example: https://img.shields.io/badge/go-1.21+-blue.svg
    m = re.search(r"img\.shields\.io/badge/go-([0-9]+\.[0-9]+)\+", content)
    if not m:
        return problems

    badge_ver = m.group(1)
    if badge_ver != go_ver:
        problems.append(
            f"README.md: Go version badge claims {badge_ver}+ but go.mod declares go {go_ver}"
        )

    return problems


def main() -> int:
    repo_root = Path(os.getcwd())
    problems: list[str] = []
    problems.extend(check_links(repo_root))
    problems.extend(check_go_version_badge(repo_root))

    if problems:
        print("doc-integrity: FAIL")
        for p in problems:
            print(f"- {p}")
        return 1

    print("doc-integrity: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
PY
