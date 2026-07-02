#!/usr/bin/env python3
"""Generate a pip --find-links index for TableTheory Python wheels.

The GitHub Pages workflow uses this script to build a static HTML page from
published GitHub Release assets. Consumers can then run:

    pip install --find-links https://tabletheory.theorycloud.ai/python/find-links/ \
        tabletheory-py==X.Y.Z

The script also accepts a local releases JSON fixture so the index shape and pip
resolution can be proven without mutating GitHub Pages or publishing releases.
"""

from __future__ import annotations

import argparse
import html
import json
import os
import re
import sys
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any

WHEEL_NAME_RE = re.compile(r"^tabletheory_py-(?P<version>.+)-py3-none-any\.whl$")
DEFAULT_API_BASE = "https://api.github.com"
DEFAULT_REPO = "theory-cloud/TableTheory"
DEFAULT_OUTPUT = "docs/python/find-links/index.html"
DEFAULT_INDEX_URL = "https://tabletheory.theorycloud.ai/python/find-links/"


@dataclass(frozen=True)
class WheelLink:
    filename: str
    url: str
    version: str
    release_tag: str
    prerelease: bool
    published_at: str


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate a static pip --find-links index from TableTheory GitHub Release wheel assets.",
    )
    parser.add_argument("--repo", default=DEFAULT_REPO, help="GitHub repository in owner/name form.")
    parser.add_argument("--api-base", default=DEFAULT_API_BASE, help="GitHub API base URL.")
    parser.add_argument(
        "--output",
        default=DEFAULT_OUTPUT,
        help="Output HTML path. Parent directories are created when needed.",
    )
    parser.add_argument(
        "--releases-json",
        help="Optional fixture path containing a GitHub releases JSON array or {'releases': [...]} object.",
    )
    parser.add_argument(
        "--index-url",
        default=DEFAULT_INDEX_URL,
        help="Published index URL to show in generated usage examples.",
    )
    parser.add_argument(
        "--allow-empty",
        action="store_true",
        help="Write an empty index instead of failing when no wheel assets are found.",
    )
    return parser.parse_args(argv)


def load_releases(args: argparse.Namespace) -> list[dict[str, Any]]:
    if args.releases_json:
        return normalize_releases_json(json.loads(Path(args.releases_json).read_text(encoding="utf-8")))
    return fetch_releases(args.repo, args.api_base)


def normalize_releases_json(raw: Any) -> list[dict[str, Any]]:
    if isinstance(raw, list):
        return [release for release in raw if isinstance(release, dict)]
    if isinstance(raw, dict):
        releases = raw.get("releases")
        if isinstance(releases, list):
            return [release for release in releases if isinstance(release, dict)]
        if "assets" in raw:
            return [raw]
    raise ValueError("releases JSON must be an array, a release object, or an object with a 'releases' array")


def fetch_releases(repo: str, api_base: str) -> list[dict[str, Any]]:
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    releases: list[dict[str, Any]] = []
    url = f"{api_base.rstrip('/')}/repos/{repo}/releases?per_page=100"

    while url:
        request = urllib.request.Request(url)
        request.add_header("Accept", "application/vnd.github+json")
        request.add_header("X-GitHub-Api-Version", "2022-11-28")
        if token:
            request.add_header("Authorization", f"Bearer {token}")
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                payload = json.loads(response.read().decode("utf-8"))
                if not isinstance(payload, list):
                    raise ValueError(f"GitHub releases response was not a list: {type(payload).__name__}")
                releases.extend(release for release in payload if isinstance(release, dict))
                url = next_link(response.headers.get("Link"))
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"GitHub releases request failed with HTTP {exc.code}: {body}") from exc
    return releases


def next_link(link_header: str | None) -> str | None:
    if not link_header:
        return None
    for part in link_header.split(","):
        url_part, _, rel_part = part.partition(";")
        if 'rel="next"' not in rel_part:
            continue
        url_part = url_part.strip()
        if url_part.startswith("<") and url_part.endswith(">"):
            return url_part[1:-1]
    return None


def collect_wheels(releases: list[dict[str, Any]]) -> list[WheelLink]:
    wheels: list[WheelLink] = []
    seen: set[tuple[str, str]] = set()

    for release in releases:
        if release.get("draft"):
            continue
        release_tag = str(release.get("tag_name") or "")
        published_at = str(release.get("published_at") or "")
        prerelease = bool(release.get("prerelease"))
        assets = release.get("assets") or []
        if not isinstance(assets, list):
            continue

        for asset in assets:
            if not isinstance(asset, dict):
                continue
            if asset.get("state") not in (None, "uploaded"):
                continue
            filename = str(asset.get("name") or "")
            match = WHEEL_NAME_RE.match(filename)
            if not match:
                continue
            url = str(asset.get("browser_download_url") or "")
            if not url:
                continue
            dedupe_key = (filename, url)
            if dedupe_key in seen:
                continue
            seen.add(dedupe_key)
            wheels.append(
                WheelLink(
                    filename=filename,
                    url=url,
                    version=match.group("version"),
                    release_tag=release_tag,
                    prerelease=prerelease,
                    published_at=published_at,
                )
            )

    return sorted(wheels, key=lambda wheel: (wheel.published_at, wheel.release_tag, wheel.filename), reverse=True)


def render_index(wheels: list[WheelLink], index_url: str) -> str:
    usage = f"pip install --find-links {index_url} tabletheory-py==X.Y.Z"
    latest = f"pip install --find-links {index_url} tabletheory-py"
    lines = [
        "<!doctype html>",
        '<html lang="en">',
        "<head>",
        '  <meta charset="utf-8">',
        "  <title>TableTheory Python wheels</title>",
        "</head>",
        "<body>",
        "  <h1>TableTheory Python wheels</h1>",
        "  <p>This pip find-links index is generated from immutable TableTheory GitHub Release assets.</p>",
        f"  <p>Install an exact version: <code>{html.escape(usage)}</code></p>",
        f"  <p>Install the latest stable candidate visible to pip: <code>{html.escape(latest)}</code></p>",
        "  <ul>",
    ]
    for wheel in wheels:
        label_parts = [wheel.release_tag or wheel.version]
        if wheel.prerelease:
            label_parts.append("prerelease")
        if wheel.published_at:
            label_parts.append(wheel.published_at)
        label = " · ".join(label_parts)
        lines.append(
            "    <li>"
            f'<a href="{html.escape(wheel.url, quote=True)}">{html.escape(wheel.filename)}</a>'
            f" <small>{html.escape(label)}</small>"
            "</li>"
        )
    lines.extend(
        [
            "  </ul>",
            "</body>",
            "</html>",
            "",
        ]
    )
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    releases = load_releases(args)
    wheels = collect_wheels(releases)
    if not wheels and not args.allow_empty:
        raise RuntimeError("no tabletheory_py wheel assets found in release metadata")

    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(render_index(wheels, args.index_url), encoding="utf-8")
    print(f"wrote {output} with {len(wheels)} wheel link(s)")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except Exception as exc:  # noqa: BLE001 - CLI boundary should print a clear failure.
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
