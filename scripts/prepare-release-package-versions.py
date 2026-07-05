#!/usr/bin/env python3
"""Stamp release-build package metadata from a GitHub release tag.

This script intentionally writes only workspace-local package metadata used by
release asset builds. Release workflows must run it after release-please emits a
tag and before `npm pack` / `python -m build`; the resulting file changes are
not release-cycle source of truth and must not be committed by automation.
"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


VERSION_RE = re.compile(r"^\d+\.\d+\.\d+(?:-rc\.\d+)?$")


def release_version(raw: str) -> str:
    version = raw.strip()
    if version.startswith("v"):
        version = version[1:]
    if not VERSION_RE.fullmatch(version):
        raise SystemExit(
            "release-package-versions: FAIL "
            f"(version must be stable X.Y.Z or numbered RC X.Y.Z-rc.N, got {raw!r})"
        )
    return version


def write_json(path: Path, data: object) -> None:
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Write TS/Python release-build versions from a tag or version."
    )
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--tag-name", help="GitHub release tag, e.g. v1.10.1-rc.1")
    group.add_argument("--version", help="Release version without leading v")
    parser.add_argument(
        "--repo-root",
        default=".",
        help="Repository root to modify (default: current working directory).",
    )
    args = parser.parse_args()

    version = release_version(args.tag_name or args.version or "")
    root = Path(args.repo_root).resolve()

    ts_package_path = root / "ts/package.json"
    ts_lock_path = root / "ts/package-lock.json"
    py_version_path = root / "py/src/theorydb_py/version.json"

    for path in (ts_package_path, ts_lock_path, py_version_path):
        if not path.is_file():
            raise SystemExit(f"release-package-versions: FAIL (missing {path})")

    ts_package = json.loads(ts_package_path.read_text(encoding="utf-8"))
    ts_package["version"] = version
    write_json(ts_package_path, ts_package)

    ts_lock = json.loads(ts_lock_path.read_text(encoding="utf-8"))
    ts_lock["version"] = version
    packages = ts_lock.setdefault("packages", {})
    if not isinstance(packages, dict):
        raise SystemExit("release-package-versions: FAIL (ts/package-lock.json packages is not an object)")
    root_package = packages.setdefault("", {})
    if not isinstance(root_package, dict):
        raise SystemExit(
            "release-package-versions: FAIL (ts/package-lock.json packages[''] is not an object)"
        )
    root_package["version"] = version
    write_json(ts_lock_path, ts_lock)

    write_json(py_version_path, {"version": version})

    print(f"release-package-versions: PASS (version={version})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
