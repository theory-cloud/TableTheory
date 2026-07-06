#!/usr/bin/env python3
"""Verify release asset package metadata matches the GitHub release tag."""

from __future__ import annotations

import argparse
import json
import re
import tarfile
import zipfile
from email.parser import Parser
from pathlib import Path


VERSION_RE = re.compile(r"^\d+\.\d+\.\d+(?:-rc(?:\.\d+)?)?$")


def release_version(raw: str) -> str:
    version = raw.strip()
    if version.startswith("v"):
        version = version[1:]
    if not VERSION_RE.fullmatch(version):
        raise SystemExit(
            "release-package-version-assets: FAIL "
            f"(version must be stable X.Y.Z or RC X.Y.Z-rc or X.Y.Z-rc.N, got {raw!r})"
        )
    return version


def pep440(version: str) -> str:
    match = re.fullmatch(r"(\d+\.\d+\.\d+)-rc(?:\.(\d+))?", version)
    if match:
        return f"{match.group(1)}rc{match.group(2) or '0'}"
    return version


def fail(message: str) -> None:
    raise SystemExit(f"release-package-version-assets: FAIL ({message})")


def one(paths: list[Path], label: str) -> Path:
    if not paths:
        fail(f"missing {label}")
    if len(paths) > 1:
        fail(f"multiple {label} files: {', '.join(path.name for path in paths)}")
    return paths[0]


def read_npm_package(tgz: Path) -> dict[str, object]:
    with tarfile.open(tgz, "r:gz") as archive:
        member = next(
            (item for item in archive.getmembers() if item.name == "package/package.json"),
            None,
        )
        if member is None:
            fail(f"{tgz.name} does not contain package/package.json")
        extracted = archive.extractfile(member)
        if extracted is None:
            fail(f"{tgz.name} package/package.json is unreadable")
        return json.loads(extracted.read().decode("utf-8"))


def metadata_version(text: str) -> str:
    parsed = Parser().parsestr(text)
    version = parsed.get("Version", "")
    if not version:
        fail("Python package metadata has no Version field")
    return version


def read_wheel_metadata(wheel: Path) -> tuple[str, str]:
    with zipfile.ZipFile(wheel) as archive:
        metadata_name = next(
            (name for name in archive.namelist() if name.endswith(".dist-info/METADATA")),
            "",
        )
        if not metadata_name:
            fail(f"{wheel.name} does not contain METADATA")
        version_json_name = next(
            (name for name in archive.namelist() if name.endswith("tabletheory_py/version.json")),
            "",
        )
        if not version_json_name:
            fail(f"{wheel.name} does not contain tabletheory_py/version.json")
        metadata = archive.read(metadata_name).decode("utf-8")
        version_json = json.loads(archive.read(version_json_name).decode("utf-8"))
    return metadata_version(metadata), str(version_json.get("version", ""))


def read_sdist_metadata(sdist: Path) -> tuple[str, str]:
    with tarfile.open(sdist, "r:gz") as archive:
        pkg_info = next((item for item in archive.getmembers() if item.name.endswith("/PKG-INFO")), None)
        if pkg_info is None:
            fail(f"{sdist.name} does not contain PKG-INFO")
        version_json = next(
            (item for item in archive.getmembers() if item.name.endswith("/src/tabletheory_py/version.json")),
            None,
        )
        if version_json is None:
            fail(f"{sdist.name} does not contain src/tabletheory_py/version.json")
        pkg_info_file = archive.extractfile(pkg_info)
        version_json_file = archive.extractfile(version_json)
        if pkg_info_file is None or version_json_file is None:
            fail(f"{sdist.name} metadata is unreadable")
        pkg_info_text = pkg_info_file.read().decode("utf-8")
        version_json_data = json.loads(version_json_file.read().decode("utf-8"))
    return metadata_version(pkg_info_text), str(version_json_data.get("version", ""))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--tag-name", help="GitHub release tag, e.g. v1.10.1-rc or v1.10.1-rc.1")
    group.add_argument("--version", help="Release version without leading v")
    parser.add_argument(
        "--assets-dir",
        default="release-assets",
        help="Directory containing release assets (default: release-assets).",
    )
    args = parser.parse_args()

    expected = release_version(args.tag_name or args.version or "")
    expected_pep440 = pep440(expected)
    assets = Path(args.assets_dir)
    if not assets.is_dir():
        fail(f"missing assets directory {assets}")

    npm_tgz = one(sorted(assets.glob("theory-cloud-tabletheory-ts-*.tgz")), "TypeScript npm pack")
    wheel = one(sorted(assets.glob("tabletheory_py-*.whl")), "Python wheel")
    sdist = one(sorted(assets.glob("tabletheory_py-*.tar.gz")), "Python sdist")

    npm_package = read_npm_package(npm_tgz)
    if npm_package.get("version") != expected:
        fail(f"{npm_tgz.name} version {npm_package.get('version')!r} != {expected!r}")
    if npm_package.get("name") != "@theory-cloud/tabletheory-ts":
        fail(f"{npm_tgz.name} package name {npm_package.get('name')!r} is unexpected")

    wheel_version, wheel_repo_version = read_wheel_metadata(wheel)
    if wheel_version != expected_pep440:
        fail(f"{wheel.name} metadata version {wheel_version!r} != {expected_pep440!r}")
    if wheel_repo_version != expected:
        fail(f"{wheel.name} version.json {wheel_repo_version!r} != {expected!r}")

    sdist_version, sdist_repo_version = read_sdist_metadata(sdist)
    if sdist_version != expected_pep440:
        fail(f"{sdist.name} metadata version {sdist_version!r} != {expected_pep440!r}")
    if sdist_repo_version != expected:
        fail(f"{sdist.name} version.json {sdist_repo_version!r} != {expected!r}")

    print(
        "release-package-version-assets: PASS "
        f"(version={expected}, python_metadata={expected_pep440})"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
