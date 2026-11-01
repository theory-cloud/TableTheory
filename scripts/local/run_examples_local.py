#!/usr/bin/env python3
"""TableTheory local examples harness (``make examples-local``).

Runs every example entrypoint classified by ``scripts/local/examples-local.json``
against the checked-in DynamoDB Local profile, asserting each example's real
outcome. It never contacts AWS: the harness sets the endpoint overrides and
static dummy credentials itself, and the preflight refuses to start when an SDK
would resolve DynamoDB anywhere else.

Rules enforced here:

* enumeration is derived from the repository tree, so an entrypoint that is not
  in the manifest fails the run;
* every manifest row names a file that exists in the tree, pending rows
  included; only ``coveredBy`` may point at a non-entrypoint helper;
* a skip is a failure, including a ``t.Skip`` inside a Go test;
* entries owned by a later milestone are reported as ``PENDING-MILESTONE`` and
  are never counted as pass;
* every assertion is made against real output or real HTTP behavior, never
  against a mock.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import re
import signal
import socket
import subprocess
import sys
import tempfile
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable

ROOT = Path(__file__).resolve().parents[2]
HARNESS_DIR = Path(__file__).resolve().parent
MANIFEST_PATH = HARNESS_DIR / "examples-local.json"

CLASSES = ("L1", "L2", "L3", "I")
MILESTONES = ("T2", "T3", "T4a", "T4b")
# Logical services every local profile must wire, and the AWS-standard endpoint
# variable each one must expose. KMS and STS deliberately share one container.
REQUIRED_ENDPOINTS = ("dynamodb", "kms", "sts", "s3", "sqs")
STANDARD_ENDPOINT_ENV = {
    "dynamodb": "AWS_ENDPOINT_URL_DYNAMODB",
    "kms": "AWS_ENDPOINT_URL_KMS",
    "sts": "AWS_ENDPOINT_URL_STS",
    "s3": "AWS_ENDPOINT_URL_S3",
    "sqs": "AWS_ENDPOINT_URL_SQS",
}
DIGEST_PIN_RE = re.compile(r"@sha256:[0-9a-f]{64}$")
# Dependency trees and build output: never entrypoints.
EXCLUDED_DIR_PARTS = {
    "node_modules",
    "cdk.out",
    "dist",
    "vendor",
    "__pycache__",
    ".venv",
}
# Support modules and test trees inside an example: real code, no entrypoint.
NON_ENTRYPOINT_DIR_PARTS = {"lib", "test", "tests"}
TS_SUFFIXES = frozenset({".ts", ".mts", ".cts", ".js", ".mjs", ".cjs"})
RUNTIME_SUFFIXES = TS_SUFFIXES | {".py"}
TEST_FILE_MARKERS = (".test.", ".spec.", "_test.")
GENERATED_MARKER_RE = re.compile(r"Code generated .*\bDO NOT EDIT\b")
GO_MAIN_RE = re.compile(r"^func main\(\)", re.MULTILINE)
MULTI_TENANT = ROOT / "examples" / "multi-tenant"
JWT_SECRET = "tabletheory-local-harness-secret"
DEFAULT_TIMEOUT = 900
FAILURE_OUTPUT_LINES = 60


class HarnessFailure(Exception):
    """A row failed. ``output`` carries the child process output for reporting."""

    def __init__(self, message: str, output: str = "") -> None:
        super().__init__(message)
        self.message = message
        self.output = output


@dataclass
class Entry:
    key: str
    kind: str
    data: dict

    @property
    def class_name(self) -> str:
        return str(self.data.get("class", ""))

    @property
    def pending(self) -> str:
        return str(self.data.get("pending", ""))

    @property
    def runner(self) -> str:
        return str(self.data.get("run", ""))

    @property
    def covered_by(self) -> str:
        return str(self.data.get("coveredBy", ""))

    @property
    def description(self) -> str:
        return str(self.data.get("description", ""))

    @property
    def label(self) -> str:
        return f"{self.key} [{self.kind}]" if self.kind == "suite" else self.key


@dataclass(frozen=True)
class Container:
    name: str
    image: str
    container_port: int


@dataclass(frozen=True)
class Endpoint:
    service: str
    container: str
    env: tuple[str, ...]


@dataclass
class LocalProfile:
    """One harness run's view of the checked-in profile.

    ``project`` is unique per checkout and run id, and every service publishes
    an ephemeral loopback port, so concurrent harness instances never share a
    container, a network or a port. ``host_ports`` is filled from
    ``docker compose port`` after the stack starts.
    """

    name: str
    compose_file: Path
    containers: dict[str, Container]
    endpoints: dict[str, Endpoint]
    run_id: str
    project: str
    host_ports: dict[str, int] = field(default_factory=dict)

    def url(self, service: str) -> str:
        try:
            port = self.host_ports[self.endpoints[service].container]
        except KeyError as exc:
            raise HarnessFailure(f"endpoint {service!r} has no discovered host port") from exc
        return f"http://127.0.0.1:{port}"

    def endpoint_env(self) -> dict[str, str]:
        env: dict[str, str] = {}
        for service in sorted(self.endpoints):
            for name in self.endpoints[service].env:
                env[name] = self.url(service)
        return env


def project_name(profile_name: str, root: Path, run_id: str) -> str:
    """Return a compose project name unique to this checkout and run.

    Two harness instances in different checkouts, or two runs in the same
    checkout, get different project names and therefore different containers
    and networks. ``--run-id`` makes the value reproducible for debugging.
    """
    material = f"{root.resolve()}|{run_id}".encode("utf-8")
    return f"{profile_name}-{hashlib.sha256(material).hexdigest()[:10]}"


def build_profile(profile: dict, root: Path, run_id: str) -> LocalProfile:
    containers = {
        name: Container(
            name=name,
            image=str(spec["image"]),
            container_port=int(spec["containerPort"]),
        )
        for name, spec in profile["containers"].items()
    }
    endpoints = {
        name: Endpoint(
            service=name,
            container=str(spec["container"]),
            env=tuple(str(item) for item in spec["env"]),
        )
        for name, spec in profile["endpoints"].items()
    }
    return LocalProfile(
        name=str(profile["name"]),
        compose_file=root / str(profile["composeFile"]),
        containers=containers,
        endpoints=endpoints,
        run_id=run_id,
        project=project_name(str(profile["name"]), root, run_id),
    )


@dataclass
class Harness:
    env: dict
    ports: dict
    timeout: int = DEFAULT_TIMEOUT

    def run(
        self,
        argv: list[str],
        *,
        cwd: Path | None = None,
        env: dict | None = None,
        timeout: int | None = None,
        input: str | None = None,
        check: bool = True,
    ) -> subprocess.CompletedProcess:
        child_env = dict(self.env)
        if env:
            child_env.update(env)
        try:
            result = subprocess.run(
                argv,
                cwd=str(cwd or ROOT),
                env=child_env,
                input=input,
                capture_output=True,
                text=True,
                timeout=timeout or self.timeout,
                check=False,
            )
        except FileNotFoundError as exc:
            raise HarnessFailure(f"{argv[0]} is not available: {exc}") from exc
        except subprocess.TimeoutExpired as exc:
            output = _decode_timeout_output(exc)
            raise HarnessFailure(
                f"timed out after {timeout or self.timeout}s: {' '.join(argv)}", output
            ) from exc

        if check and result.returncode != 0:
            raise HarnessFailure(
                f"command exited {result.returncode}: {' '.join(argv)}",
                _combined_output(result),
            )
        return result


def _decode_timeout_output(exc: subprocess.TimeoutExpired) -> str:
    parts = []
    for blob in (exc.stdout, exc.stderr):
        if blob is None:
            continue
        if isinstance(blob, bytes):
            parts.append(blob.decode("utf-8", "replace"))
        else:
            parts.append(blob)
    return "".join(parts)


def _combined_output(result: subprocess.CompletedProcess) -> str:
    stdout = result.stdout or ""
    stderr = result.stderr or ""
    if stdout and stderr:
        return f"{stdout}{stderr}" if stdout.endswith("\n") else f"{stdout}\n{stderr}"
    return stdout or stderr


def _tail(text: str, lines: int = FAILURE_OUTPUT_LINES) -> str:
    rows = text.rstrip().splitlines()
    if len(rows) <= lines:
        return "\n".join(rows)
    return "\n".join([f"... ({len(rows) - lines} earlier lines omitted)"] + rows[-lines:])


def expect_output(output: str, needles: list[str], label: str) -> None:
    missing = [needle for needle in needles if needle not in output]
    if missing:
        rendered = "\n".join(f"  missing: {needle!r}" for needle in missing)
        raise HarnessFailure(f"{label}: expected output not observed\n{rendered}", output)


def reject_output(output: str, needles: list[str], label: str) -> None:
    found = [needle for needle in needles if needle in output]
    if found:
        rendered = "\n".join(f"  unexpected: {needle!r}" for needle in found)
        raise HarnessFailure(f"{label}: unexpected output observed\n{rendered}", output)


def decode_jwt(token: str) -> tuple[dict, dict]:
    parts = token.split(".")
    if len(parts) != 3 or not all(parts):
        raise HarnessFailure(f"token is not a three-segment JWT: {token!r}")
    try:
        header = json.loads(_b64url_decode(parts[0]))
        payload = json.loads(_b64url_decode(parts[1]))
    except (ValueError, json.JSONDecodeError) as exc:
        raise HarnessFailure(f"token payload is not decodable JSON: {exc}") from exc
    return header, payload


def _b64url_decode(segment: str) -> str:
    padded = segment + "=" * (-len(segment) % 4)
    return base64.urlsafe_b64decode(padded).decode("utf-8")


def mint_token(
    harness: Harness, user: str, org: str, *, secret: str = JWT_SECRET
) -> str:
    result = harness.run(
        [
            "go",
            "run",
            "./cmd/token",
            "-secret",
            secret,
            "-user",
            user,
            "-org",
            org,
            "-ttl",
            "3600",
        ],
        cwd=MULTI_TENANT,
    )
    lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    if not lines:
        raise HarnessFailure("cmd/token produced no token", _combined_output(result))
    return lines[-1]


def http_request(
    method: str,
    url: str,
    *,
    token: str | None = None,
    payload: dict | None = None,
    timeout: float = 20.0,
) -> tuple[int, str]:
    data = None
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(url, data=data, method=method)
    if token:
        request.add_header("Authorization", f"Bearer {token}")
    if data is not None:
        request.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return response.status, response.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace")
    except urllib.error.URLError as exc:
        raise HarnessFailure(f"{method} {url} failed: {exc}") from exc


def expect_status(actual: int, expected: int, label: str, body: str = "") -> None:
    if actual != expected:
        raise HarnessFailure(
            f"{label}: expected HTTP {expected}, got {actual}", f"response body:\n{_tail(body)}"
        )


# --- discovery and manifest -------------------------------------------------


def discover(root: Path) -> dict[str, str]:
    """Return every example entrypoint discovered from the tree.

    The rules are structural, so an example cannot hide from the manifest:

    * ``examples/**/*.go`` that declares ``func main`` is a Go entrypoint
      (``func main`` is the entrypoint marker; path conventions do not apply);
    * a ``.sh`` file under ``examples/``, ``ts/examples/`` or ``py/examples/``
      is a shell runner;
    * files under ``examples/``, ``ts/examples/`` and ``py/examples/`` with a
      runtime suffix (``.ts``, ``.mts``, ``.cts``, ``.js``, ``.mjs``, ``.cjs``,
      ``.py``) are runtime entrypoints;
    * any other executable file under those trees is a runner.

    Non-entrypoint files are skipped by a documented convention:

    * dependency trees and build output (``node_modules``, ``cdk.out``,
      ``dist``, ``vendor``, ``__pycache__``, ``.venv``);
    * support modules and test trees inside an example (``lib/``, ``test/``,
      ``tests/``) and test files (``*.test.*``, ``*.spec.*``, ``*_test.*``);
    * generated output, marked ``Code generated ... DO NOT EDIT`` in its first
      lines (``examples/cdk-multilang/local/generated-table.ts``).

    ``bin/`` is deliberately *not* excluded: a CDK app entrypoint such as
    ``examples/cdk-multilang/bin/app.ts`` is a real entrypoint that must be
    classified like any other runtime file.
    """
    found: dict[str, str] = {}

    def relative(path: Path) -> str:
        return path.relative_to(root).as_posix()

    def excluded_dir(path: Path) -> bool:
        return any(part in EXCLUDED_DIR_PARTS for part in path.relative_to(root).parts)

    def excluded_runtime_file(path: Path) -> bool:
        parts = path.relative_to(root).parts
        if any(part in EXCLUDED_DIR_PARTS for part in parts):
            return True
        if any(part in NON_ENTRYPOINT_DIR_PARTS for part in parts[:-1]):
            return True
        if any(marker in path.name for marker in TEST_FILE_MARKERS):
            return True
        return generated_output(path)

    for path in sorted((root / "examples").rglob("*.go")):
        if excluded_dir(path):
            continue
        text = path.read_text(encoding="utf-8", errors="replace")
        if GO_MAIN_RE.search(text):
            found[relative(path)] = "go-main"

    for path in sorted((root / "examples").rglob("*.sh")):
        if excluded_runtime_file(path):
            continue
        found[relative(path)] = "shell-runner"

    for base_parts in (("examples",), ("ts", "examples"), ("py", "examples")):
        base = root.joinpath(*base_parts)
        if not base.is_dir():
            continue
        for path in _walk_files(base):
            if excluded_runtime_file(path):
                continue
            suffix = path.suffix.lower()
            if suffix == ".sh":
                # The examples/**/*.sh pass above already covers examples/;
                # this branch keeps shell runners under ts/examples and
                # py/examples visible instead of silently dropping them.
                found[relative(path)] = "shell-runner"
            elif suffix in RUNTIME_SUFFIXES:
                found[relative(path)] = "ts" if suffix in TS_SUFFIXES else "py"
            elif os.access(path, os.X_OK):
                found[relative(path)] = "executable"

    return found


def _walk_files(base: Path):
    """Yield files below ``base`` without descending into excluded trees."""
    for current, dirnames, filenames in os.walk(base):
        dirnames[:] = sorted(
            name
            for name in dirnames
            if name not in EXCLUDED_DIR_PARTS and name not in NON_ENTRYPOINT_DIR_PARTS
        )
        for name in sorted(filenames):
            yield Path(current) / name


def generated_output(path: Path) -> bool:
    """True when the file carries the ``Code generated ... DO NOT EDIT`` marker."""
    try:
        head = path.read_text(encoding="utf-8", errors="replace").splitlines()[:5]
    except OSError:
        return False
    return any(GENERATED_MARKER_RE.search(line) for line in head)


def load_manifest(path: Path) -> tuple[dict, list[Entry]]:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise HarnessFailure(f"missing classification manifest: {path}") from exc
    except json.JSONDecodeError as exc:
        raise HarnessFailure(f"invalid JSON in {path}: {exc}") from exc

    if not isinstance(raw, dict) or raw.get("version") != 1:
        raise HarnessFailure(f"{path}: expected a version 1 classification manifest")

    profile = raw.get("profile")
    if not isinstance(profile, dict):
        raise HarnessFailure(f"{path}: missing profile object")
    for key in ("composeFile", "containers", "endpoints"):
        if key not in profile:
            raise HarnessFailure(f"{path}: profile is missing {key}")

    entries: list[Entry] = []
    for key, data in raw.get("entrypoints", {}).items():
        if not isinstance(data, dict):
            raise HarnessFailure(f"{path}: entry {key} must be an object")
        entries.append(Entry(key=key, kind="entrypoint", data=data))
    for key, data in raw.get("suites", {}).items():
        if not isinstance(data, dict):
            raise HarnessFailure(f"{path}: suite {key} must be an object")
        entries.append(Entry(key=key, kind="suite", data=data))

    return profile, entries


def validate_manifest(
    root: Path,
    manifest_path: Path,
    profile: dict,
    entries: list[Entry],
    discovered: dict[str, str],
    runners: dict[str, Callable],
) -> list[str]:
    errors: list[str] = []

    entrypoint_keys = {entry.key for entry in entries if entry.kind == "entrypoint"}
    by_key = {entry.key: entry for entry in entries}
    for path in sorted(set(discovered) - entrypoint_keys):
        errors.append(f"unclassified entrypoint (add it to {manifest_path.name}): {path}")

    # Every row names a real file, pending rows included: a pending row is a
    # claim about the tree just like a classified one. Only a coveredBy row may
    # point at a file that discovery does not treat as an entrypoint, because it
    # documents a helper executed inside another row.
    for path in sorted(entrypoint_keys):
        entry = by_key[path]
        if not (root / path).is_file():
            errors.append(f"manifest entry does not name a file in the tree: {path}")
        elif not entry.covered_by and path not in discovered:
            errors.append(
                f"{path}: pending and classified rows must name a discovered entrypoint"
            )

    runner_owner: dict[str, str] = {}

    for entry in entries:
        if entry.kind == "suite":
            suite_dir = root / entry.key
            if not (suite_dir / "go.mod").is_file():
                errors.append(f"suite is not a Go module: {entry.key}")

        if entry.class_name and entry.pending:
            errors.append(f"{entry.key}: declare either class or pending, not both")
        if not entry.class_name and not entry.pending:
            errors.append(f"{entry.key}: declare a class ({'/'.join(CLASSES)}) or pending <milestone>")
        if entry.class_name and entry.class_name not in CLASSES:
            errors.append(f"{entry.key}: unknown class {entry.class_name!r}")
        if entry.pending and entry.pending not in MILESTONES:
            errors.append(f"{entry.key}: unknown milestone {entry.pending!r}")

        if entry.pending:
            if entry.runner or entry.covered_by:
                errors.append(f"{entry.key}: a pending entry must not be executed")
            continue

        if entry.covered_by:
            target = by_key.get(entry.covered_by)
            if target is None:
                errors.append(f"{entry.key}: coveredBy target is not in the manifest: {entry.covered_by}")
            elif not target.runner or target.pending:
                errors.append(f"{entry.key}: coveredBy target has no runner: {entry.covered_by}")
            elif entry.covered_by == entry.key:
                errors.append(f"{entry.key}: coveredBy cannot reference itself")
            continue

        if not entry.runner:
            errors.append(f"{entry.key}: classified examples need a runner")
            continue
        if entry.runner not in runners:
            errors.append(f"{entry.key}: unknown runner {entry.runner!r}")
        if entry.runner in runner_owner:
            errors.append(
                f"{entry.key}: runner {entry.runner!r} is already used by {runner_owner[entry.runner]}"
            )
        else:
            runner_owner[entry.runner] = entry.key

    compose_path = root / str(profile.get("composeFile", ""))
    containers = profile.get("containers", {})
    endpoints = profile.get("endpoints", {})
    if not isinstance(containers, dict) or not containers:
        errors.append("profile.containers must be a non-empty object")
    if not isinstance(endpoints, dict) or not endpoints:
        errors.append("profile.endpoints must be a non-empty object")
    if not isinstance(containers, dict) or not isinstance(endpoints, dict):
        return errors

    seen_env: dict[str, str] = {}
    for service in REQUIRED_ENDPOINTS:
        spec = endpoints.get(service)
        if not isinstance(spec, dict):
            errors.append(f"profile.endpoints.{service} is required")
            continue
        container_name = str(spec.get("container", ""))
        if container_name not in containers:
            errors.append(f"profile.endpoints.{service} names an undeclared container: {container_name!r}")
        env = spec.get("env")
        if not isinstance(env, list) or not env:
            errors.append(f"profile.endpoints.{service}.env must be a non-empty list")
            continue
        standard = STANDARD_ENDPOINT_ENV[service]
        if standard not in env:
            errors.append(
                f"profile.endpoints.{service}.env must include the AWS-standard override {standard}"
            )
        for name in env:
            name = str(name)
            if name in seen_env:
                errors.append(
                    f"endpoint variable {name} is used by both {seen_env[name]} and {service}"
                )
            else:
                seen_env[name] = service

    for name, spec in containers.items():
        if not isinstance(spec, dict):
            errors.append(f"profile.containers.{name} must be an object")
            continue
        image = str(spec.get("image", ""))
        port = spec.get("containerPort")
        if not DIGEST_PIN_RE.search(image):
            errors.append(
                f"profile.containers.{name}.image must be pinned by digest (@sha256:...), got {image!r}"
            )
        if not isinstance(port, int) or port <= 0:
            errors.append(f"profile.containers.{name}.containerPort must be a positive integer")

    if not compose_path.is_file():
        errors.append(f"profile compose file is missing: {profile.get('composeFile')}")
    else:
        compose_text = compose_path.read_text(encoding="utf-8")
        if "container_name:" in compose_text:
            errors.append(f"{compose_path}: container_name is fixed and would collide across runs")
        for line in compose_text.splitlines():
            if line.startswith("name:"):
                errors.append(f"{compose_path}: a top-level name fixes the compose project across runs")
                break
        for name, spec in containers.items():
            if not isinstance(spec, dict):
                continue
            image = str(spec.get("image", ""))
            port = spec.get("containerPort")
            if image and f"image: {image}" not in compose_text:
                errors.append(f"{compose_path}: image pin does not match the manifest for {name} ({image})")
            if isinstance(port, int):
                if f"127.0.0.1::{port}" not in compose_text:
                    errors.append(
                        f"{compose_path}: {name} must publish an ephemeral loopback port "
                        f"(127.0.0.1::{port}) so concurrent runs cannot collide"
                    )

    return errors


# --- preflight --------------------------------------------------------------


def port_open(host: str, port: int, timeout: float = 0.5) -> bool:
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return True
    except OSError:
        return False


def go_toolchain() -> str:
    match = re.search(r"^toolchain (\S+)$", (ROOT / "go.mod").read_text(encoding="utf-8"), re.MULTILINE)
    if not match:
        raise HarnessFailure("go.mod has no toolchain directive")
    return match.group(1)


def build_env(profile: LocalProfile) -> dict:
    # Start from a clean AWS surface: an inherited profile, session token or
    # endpoint override must not be able to redirect any child process.
    env = {key: value for key, value in os.environ.items() if not key.startswith("AWS_")}
    for key in (
        "JWT_SECRET",
        "MULTI_TENANT_LISTEN_ADDR",
        "CDK_LOCAL_PORT",
        "DEMO_TABLE_NAME",
        "FACETHEORY_CACHE_TABLE_NAME",
        "ORG_ID",
        "USER",
        "KMS_KEY_ARN",
        "KMS_ENDPOINT",
        "S3_ENDPOINT",
        "STS_ENDPOINT",
        "SQS_ENDPOINT",
    ):
        env.pop(key, None)
    # Proxy variables are cleared in both cases for the same reason: no child
    # request may be routed anywhere the harness did not choose.
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"):
        env.pop(key, None)
        env.pop(key.lower(), None)
    env.update(
        {
            "AWS_REGION": "us-east-1",
            "AWS_DEFAULT_REGION": "us-east-1",
            "AWS_ACCESS_KEY_ID": "dummy",
            "AWS_SECRET_ACCESS_KEY": "dummy",
            "AWS_EC2_METADATA_DISABLED": "true",
            "AWS_CONFIG_FILE": os.devnull,
            "AWS_SHARED_CREDENTIALS_FILE": os.devnull,
            "GOTOOLCHAIN": go_toolchain(),
        }
    )
    env.update(profile.endpoint_env())
    return env


def free_port(taken: set[int]) -> int:
    """Return a loopback port nothing is listening on right now."""
    while True:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            probe.bind(("127.0.0.1", 0))
            port = int(probe.getsockname()[1])
        if port in taken or port_open("127.0.0.1", port):
            continue
        taken.add(port)
        return port


def compose(
    harness: Harness,
    profile: LocalProfile,
    *args: str,
    check: bool = True,
    timeout: int = 300,
) -> subprocess.CompletedProcess:
    return harness.run(
        [
            "docker",
            "compose",
            "-f",
            str(profile.compose_file),
            "-p",
            profile.project,
            *args,
        ],
        timeout=timeout,
        check=check,
    )


def start_profile(harness: Harness, profile: LocalProfile) -> None:
    """Start this run's private compose stack and discover its ports.

    The project name is unique to the checkout and run id, so ``down`` here can
    only ever remove this run's own containers. Every host port is chosen by
    Docker and read back, so two concurrent runs cannot claim the same port.
    """
    if not _docker_available(harness):
        raise HarnessFailure(
            "Docker is required for the local profile; start Colima/Docker first"
        )
    compose(harness, profile, "down", "-v", "--remove-orphans", check=False, timeout=120)
    compose(harness, profile, "up", "-d", timeout=600)
    for name, container in profile.containers.items():
        profile.host_ports[name] = _compose_host_port(harness, profile, container)
    ports = list(profile.host_ports.values())
    if len(ports) != len(set(ports)):
        raise HarnessFailure(f"compose assigned duplicate host ports: {profile.host_ports}")
    print(
        "  PASS  started profile "
        + ", ".join(f"{name}={port}" for name, port in sorted(profile.host_ports.items()))
    )


def stop_profile(harness: Harness, profile: LocalProfile) -> None:
    compose(harness, profile, "down", "-v", "--remove-orphans", check=False, timeout=180)


def _compose_host_port(harness: Harness, profile: LocalProfile, container: Container) -> int:
    result = compose(
        harness,
        profile,
        "port",
        container.name,
        str(container.container_port),
        timeout=60,
    )
    lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    if not lines:
        raise HarnessFailure(
            f"docker compose did not publish a host port for {container.name}:{container.container_port}"
        )
    host, _, raw_port = lines[0].rpartition(":")
    if host not in ("127.0.0.1", "0.0.0.0", "::1", "[::1]"):
        raise HarnessFailure(
            f"{container.name} is published on {host!r}, not loopback; refusing to run"
        )
    try:
        return int(raw_port)
    except ValueError as exc:
        raise HarnessFailure(f"cannot parse {container.name} host port from {lines[0]!r}") from exc


def container_id(harness: Harness, profile: LocalProfile, service: str) -> str:
    result = compose(harness, profile, "ps", "-q", service, timeout=60, check=False)
    return result.stdout.strip().splitlines()[0] if result.stdout.strip() else ""


def wait_for_dynamodb_local(harness: Harness, endpoint: str, seconds: int = 120) -> None:
    deadline = time.monotonic() + seconds
    last = ""
    while time.monotonic() < deadline:
        result = harness.run(
            [
                "go",
                "run",
                "./internal/dynamodbprobe/cmd/dynamodb-local-check",
                "-endpoint",
                endpoint,
            ],
            timeout=180,
            check=False,
        )
        if result.returncode == 0:
            return
        last = _combined_output(result)
        time.sleep(1)
    raise HarnessFailure(f"DynamoDB Local did not become ready at {endpoint}", last)


def wait_for_http(url: str, seconds: int = 120) -> None:
    deadline = time.monotonic() + seconds
    last = ""
    # An inherited proxy must not be able to intercept this readiness probe.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    while time.monotonic() < deadline:
        try:
            with opener.open(url, timeout=2) as response:
                if response.status == 200:
                    return
                last = f"HTTP {response.status}"
        except (urllib.error.URLError, OSError) as exc:
            last = str(exc)
        time.sleep(0.5)
    raise HarnessFailure(f"local service did not answer at {url}", last)


def wait_for_port(port: int, host: str = "127.0.0.1", seconds: int = 120) -> None:
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if port_open(host, port):
            return
        time.sleep(0.5)
    raise HarnessFailure(f"local service did not start listening at {host}:{port}")


def preflight(harness: Harness, profile: LocalProfile) -> None:
    wait_for_dynamodb_local(harness, profile.url("dynamodb"))
    print(f"  PASS  DynamoDB Local answers at {profile.url('dynamodb')}")
    wait_for_http(f"{profile.url('kms').rstrip('/')}/moto-api/")
    print(f"  PASS  moto answers at {profile.url('kms')} (KMS and STS)")
    wait_for_port(int(profile.host_ports[profile.endpoints["s3"].container]))
    print(f"  PASS  s3proxy listens at {profile.url('s3')}")
    wait_for_port(int(profile.host_ports[profile.endpoints["sqs"].container]))
    print(f"  PASS  ElasticMQ listens at {profile.url('sqs')}")

    for service in REQUIRED_ENDPOINTS:
        endpoint = profile.url(service)
        for name in profile.endpoints[service].env:
            resolved = harness.env.get(name)
            if resolved != endpoint:
                raise HarnessFailure(f"{name} is {resolved!r}; the harness must own it ({endpoint})")
    print(
        "  PASS  harness owns every endpoint override (DynamoDB, KMS, STS, S3, SQS) "
        "and the static dummy credentials"
    )

    for name in ("http", "cdk"):
        port = int(harness.ports[name])
        if port_open("127.0.0.1", port):
            raise HarnessFailure(
                f"port {port} ({name}) is already in use; stop the process or container that holds it"
            )
    print("  PASS  example ports are free (HTTP server and cross-language container)")

    flags: list[str] = []
    for service in REQUIRED_ENDPOINTS:
        flags.extend([f"-{service}", profile.url(service)])
    probe = harness.run(
        ["go", "run", "./scripts/local/cmd/examples-local-preflight", *flags],
        timeout=300,
    )
    for line in probe.stdout.strip().splitlines():
        print(f"  PASS  {line.removeprefix('preflight: ')}")


def _docker_available(harness: Harness) -> bool:
    result = harness.run(["docker", "info"], timeout=60, check=False)
    return result.returncode == 0


def cleanup_projects(prefix: str) -> int:
    """Remove every harness container and network, from any checkout or run.

    ``make examples-local-down`` uses this after a crashed run left its private
    project behind. Normal runs remove their own project in ``finally``.
    """
    harness = Harness(env=dict(os.environ), ports={})
    containers = harness.run(
        ["docker", "ps", "-aq", "--filter", "label=com.docker.compose.project"],
        timeout=60,
        check=False,
    )
    removed = 0
    for container in containers.stdout.split():
        label = harness.run(
            [
                "docker",
                "inspect",
                "--format",
                '{{index .Config.Labels "com.docker.compose.project"}}',
                container,
            ],
            timeout=60,
            check=False,
        )
        project = label.stdout.strip()
        if project == prefix or project.startswith(f"{prefix}-"):
            harness.run(["docker", "rm", "-f", container], timeout=120, check=False)
            removed += 1
    networks = harness.run(
        [
            "docker",
            "network",
            "ls",
            "--filter",
            "label=com.docker.compose.project",
            "--format",
            '{{.ID}} {{.Label "com.docker.compose.project"}}',
        ],
        timeout=60,
        check=False,
    )
    for line in networks.stdout.splitlines():
        parts = line.split()
        if len(parts) != 2:
            continue
        network_id, project = parts
        if project == prefix or project.startswith(f"{prefix}-"):
            harness.run(["docker", "network", "rm", network_id], timeout=60, check=False)
            removed += 1
    print(f"examples-local: removed {removed} container/network ref(s) for {prefix}-*")
    return 0


def prepare(harness: Harness) -> None:
    tsx = ROOT / "ts" / "node_modules" / ".bin" / "tsx"
    if not tsx.exists():
        print("  ... installing the TypeScript example toolchain (npm ci)")
        harness.run(["npm", "--prefix", "ts", "ci", "--ignore-scripts"], timeout=1800)
    if not tsx.exists():
        raise HarnessFailure(f"TypeScript runner is still missing after npm ci: {tsx}")

    # The cdk-multilang Node reader resolves @aws-sdk/client-dynamodb from the
    # example's own dependency tree, which a clean checkout does not have.
    example_sdk = (
        ROOT / "examples" / "cdk-multilang" / "node_modules" / "@aws-sdk" / "client-dynamodb"
    )
    if not example_sdk.is_dir():
        print("  ... installing the cdk-multilang example toolchain (npm ci)")
        harness.run(
            ["npm", "--prefix", "examples/cdk-multilang", "ci", "--ignore-scripts"],
            timeout=1800,
        )
    if not example_sdk.is_dir():
        raise HarnessFailure(f"cdk-multilang dependencies are still missing after npm ci: {example_sdk}")

    # Provision the envelope key and prove the local KMS wraps and unwraps a
    # data key with the framework's EncryptionContext before any example runs.
    key = harness.run(
        ["go", "run", "./scripts/local/cmd/examples-local-kms-key"],
        timeout=300,
    )
    key_arn = key.stdout.strip().splitlines()[-1] if key.stdout.strip() else ""
    if not key_arn.startswith("arn:"):
        raise HarnessFailure("the local KMS did not return a key ARN", _combined_output(key))
    harness.env["KMS_KEY_ARN"] = key_arn
    print(f"  PASS  local KMS envelope key ready ({key_arn})")


# --- runners ----------------------------------------------------------------


def run_go_encryption(harness: Harness) -> str:
    result = harness.run(
        ["go", "run", "./examples/encryption"],
        env={"DYNAMORM_ENCRYPTION_CREATE_TABLE": "1"},
    )
    output = _combined_output(result)
    label = "examples/encryption"
    expect_output(
        output,
        [
            'decrypted secret: "hello from theorydb"',
            'plaintext round-trip: "hello from theorydb" == "hello from theorydb"',
            "stored attribute is ciphertext: envelope v=2",
            "OK: KMS envelope round-trip encrypted on write and decrypted on read",
        ],
        label,
    )
    reject_output(
        output,
        ["decrypt assertion failed", "stored ciphertext contains the plaintext"],
        label,
    )
    return "real KMS GenerateDataKey/Decrypt; plaintext round-trip and stored-ciphertext assertions"


def run_go_local_quickstart(harness: Harness) -> str:
    result = harness.run(["go", "run", "./examples/local-quickstart"])
    expect_output(
        result.stdout,
        [
            "created note NOTE#local",
            'read note: title="Hello TableTheory" value=42',
            'updated note: title="Hello TableTheory (updated)"',
            "deleted note NOTE#local",
            "OK: TableTheory Go quickstart CRUD against DynamoDB Local succeeded",
        ],
        "local-quickstart",
    )
    return "5 CRUD output checks"


def run_shell_cdk_multilang_local(harness: Harness) -> str:
    result = harness.run(
        ["bash", "examples/cdk-multilang/local/run-local.sh"],
        env={"CDK_LOCAL_PORT": str(harness.ports["cdk"])},
    )
    expect_output(
        result.stdout,
        [
            "go: wrote demo#1/v1 (lang=go, secret encrypted)",
            "node: read demo#1/v1 value=shared-value lang=go secret=<decrypted>",
            "python: read demo#1/v1 value='shared-value' lang='go' secret=<decrypted>",
            "LOCAL CROSS-LANGUAGE NO-DRIFT: PASS",
        ],
        "cdk-multilang local",
    )
    return "drift check + Go/TS/Py round-trip with a decrypted KMS secret"


def run_ts_local(harness: Harness) -> str:
    tsx = ROOT / "ts" / "node_modules" / ".bin" / "tsx"
    result = harness.run([str(tsx), "ts/examples/local.ts"])
    expect_output(
        result.stdout,
        [
            "created:",
            "query page1:",
            "nickname: 'Alice'",
            "query page2: []",
            "updated:",
            "nickname: 'Alicia'",
            "version: '1'",
            "deleted",
        ],
        "ts/examples/local.ts",
    )
    return "create/get/query/update/delete checks"


def _run_scripted_cli(
    harness: Harness,
    example: str,
    expected: list[str],
    *,
    env: dict | None = None,
) -> str:
    transcript_path = ROOT / "examples" / "basic" / example / "transcript.txt"
    if not transcript_path.is_file():
        raise HarnessFailure(f"missing scripted transcript: {transcript_path}")
    transcript = transcript_path.read_text(encoding="utf-8")
    result = harness.run(
        ["go", "run", "."],
        cwd=ROOT / "examples" / "basic" / example,
        input=transcript,
        env=env,
    )
    output = _combined_output(result)
    label = f"examples/basic/{example}"
    expect_output(output, expected, label)
    reject_output(
        output,
        ["Error:", "panic:", "exit status", "Failed to initialize app"],
        label,
    )
    return output


def run_basic_todo(harness: Harness) -> str:
    output = _run_scripted_cli(
        harness,
        "todo",
        [
            "🚀 Welcome to TableTheory Todo App!",
            "✅ Created todo: Ship the local harness",
            "✅ Created todo: Verify GSIs in notes and contacts",
            "1. [ ] Ship the local harness",
            "1. [✓] Ship the local harness",
            "2. [ ] Verify GSIs in notes and contacts",
            "✅ Todo deleted successfully",
            "👋 Goodbye!",
        ],
    )
    deleted_tail = _after(output, "✅ Todo deleted successfully", "todo transcript")
    expect_output(
        deleted_tail,
        ["1. [✓] Ship the local harness"],
        "todo transcript after delete",
    )
    reject_output(
        deleted_tail,
        ["Verify GSIs in notes and contacts"],
        "todo transcript after delete",
    )
    return "8 transcript checks (create/list/update/delete)"


def run_basic_notes(harness: Harness) -> str:
    output = _run_scripted_cli(
        harness,
        "notes",
        [
            "🗒️  Welcome to TableTheory Notes App!",
            "Logged in as: harness-user",
            "✅ Created note: TableTheory local notes",
            "✅ Created note: Personal note",
            "Notes in 'work' (1 notes):",
            "TableTheory local notes (work)",
            "Notes tagged 'golang' (1 notes):",
            "Notes from last 1 days (2 notes):",
            "✅ Note updated successfully",
            "Total Notes: 2",
            "Total Words: 9",
            "Average Words/Note: 4",
            "Pinned Notes: 1",
            "✅ Note deleted successfully",
            "Your Notes (1 notes):",
            "👋 Goodbye!",
        ],
        env={"USER": "harness-user"},
    )
    # The gsi-user and gsi-category reads must not bring the personal note into
    # the work category listing.
    work_section = output.split("Notes in 'work'", 1)[1].split("Notes tagged", 1)[0]
    expect_output(work_section, ["TableTheory local notes"], "notes work category")
    reject_output(work_section, ["Personal note"], "notes work category")

    deleted_tail = _after(output, "✅ Note deleted successfully", "notes transcript")
    expect_output(
        deleted_tail,
        ["Your Notes (1 notes):", "TableTheory local notes"],
        "notes transcript after delete",
    )
    reject_output(deleted_tail, ["Personal note"], "notes transcript after delete")
    return "15 transcript checks (CRUD + gsi-user + gsi-category)"


def run_basic_contacts(harness: Harness) -> str:
    output = _run_scripted_cli(
        harness,
        "contacts",
        [
            "📇 Welcome to TableTheory Contacts App!",
            "Organization: org-harness",
            "✅ Created contact: Jane Doe",
            "Contact found (1 contacts):",
            "   📧 jane@example.com | 📱 5550101",
            "Contacts with phone 555-0101 (1 contacts):",
            "Contacts named 'Doe Jane' (1 contacts):",
            "1. ⭐ Jane Doe at Acme Corp",
            "Favorite Contacts (1 contacts):",
            "✅ Contact deleted successfully",
        ],
        env={"ORG_ID": "org-harness"},
    )
    deleted_tail = _after(output, "✅ Contact deleted successfully", "contacts transcript")
    expect_output(deleted_tail, ["No contacts found"], "contacts transcript after delete")
    reject_output(deleted_tail, ["jane@example.com"], "contacts transcript after delete")
    return "11 transcript checks (CRUD + gsi-email + gsi-phone + gsi-name)"


def _after(output: str, marker: str, label: str) -> str:
    if marker not in output:
        raise HarnessFailure(f"{label}: expected marker not observed: {marker!r}", output)
    return output.split(marker, 1)[1]


def run_py_local_crud(harness: Harness) -> str:
    result = harness.run(
        ["uv", "--directory", "py", "run", "python", "examples/local_crud.py"]
    )
    expect_output(
        result.stdout,
        [
            "get: Note(pk='A', sk='010', value=Decimal('10'))",
            "query begins_with('0'):",
            "sk='001', value=Decimal('1')",
            "sk='010', value=Decimal('10')",
        ],
        "py/examples/local_crud.py",
    )
    query_line = next(
        (line for line in result.stdout.splitlines() if line.startswith("query begins_with('0'):")),
        "",
    )
    if "sk='100'" in query_line:
        raise HarnessFailure(
            "py/examples/local_crud.py: begins_with query returned a sort key outside the prefix",
            query_line,
        )
    return "put/get/begins_with checks"


def run_go_initialization_local(harness: Harness) -> str:
    result = harness.run(["go", "run", "./examples/initialization", "-mode", "local"])
    endpoint = harness.env["DYNAMODB_ENDPOINT"]
    expect_output(
        result.stdout,
        [
            "1. Local Development (DynamoDB Local)",
            f"✓ Initialized successfully against {endpoint}",
            "✓ real round-trip: wrote and read INIT#local#",
        ],
        "examples/initialization",
    )
    reject_output(
        result.stdout,
        ["AWS Environment", "AWS init failed", "✗"],
        "examples/initialization",
    )
    return "real local write + read; cloud modes excluded"


def run_go_typed(harness: Harness) -> str:
    result = harness.run(["go", "run", "./examples/typed-go"])
    expect_output(
        result.stdout,
        [
            "OK: typed-go typedFlow wrote and updated TENANT#demo/USER#ada",
            'name="Ada Lovelace"',
            "matches=1",
        ],
        "examples/typed-go",
    )
    return "create/get/update/query through the typed API"


def run_go_loose_examples(harness: Harness) -> str:
    result = harness.run(["go", "run", "./examples/drivers"])
    expect_output(
        result.stdout,
        [
            "List operations demonstrated successfully!",
            "Found 1 customers for tenant",
            "Customer has 1 orders",
            (
                "OK: loose examples drove conditional helpers, transaction builder, "
                + "batch get (4 invoices), list operations (5 tags), embedded structs "
                + "(1 orders) and the FaceTheory TTL cache on DynamoDB Local"
            ),
        ],
        "examples/drivers",
    )
    return "6 loose snippets with seeded rows and read-back assertions"


RELEASE_STATE_LINE = (
    "release-state: actual=RELEASE#service-a/ACTUAL version=8 status=active "
    "event=EVENT#2026-04-24T19:00:00Z#rel_002 outbox=OUTBOX#lambda-alias#rel_002"
)


def _expect_release_state(output: str, label: str) -> str:
    expect_output(output, [RELEASE_STATE_LINE], label)
    reject_output(output, ["want ", "Error", "panic"], label)
    return "real TransactWrite + read-back + stale-version refusal"


def run_go_release_state(harness: Harness) -> str:
    result = harness.run(["go", "run", "./examples/release-state"])
    return _expect_release_state(result.stdout, "examples/release-state")


def run_ts_release_state(harness: Harness) -> str:
    tsx = ROOT / "ts" / "node_modules" / ".bin" / "tsx"
    result = harness.run([str(tsx), "ts/examples/release-state.ts"])
    return _expect_release_state(result.stdout, "ts/examples/release-state.ts")


def run_py_release_state(harness: Harness) -> str:
    result = harness.run(
        ["uv", "--directory", "py", "run", "python", "examples/release_state.py"]
    )
    return _expect_release_state(result.stdout, "py/examples/release_state.py")


def run_py_gsi_query(harness: Harness) -> str:
    result = harness.run(
        ["uv", "--directory", "py", "run", "python", "examples/gsi_query.py"]
    )
    expect_output(
        result.stdout,
        ["gsi_items=1 email=ada@example.com"],
        "py/examples/gsi_query.py",
    )
    return "real GSI query returning only the matching partition"


def run_go_library_demos(harness: Harness) -> str:
    planner = harness.run(["go", "run", "./examples/optimization"])
    expect_output(
        planner.stdout,
        [
            "=== TableTheory Query Optimizer Examples ===",
            "Example 1: Efficient Query with Partition Key",
            "Operation: Query",
            "Optimization Hints:",
            "Example 8: Adaptive Optimization Demo",
            "Query Statistics:",
            "Executions: 20",
            "After clearing cache:",
        ],
        "examples/optimization",
    )

    expected = {
        "TestGetUser",
        "TestGetUser_NotFound",
        "TestGetActiveUsers",
        "TestCreateUser",
        "TestDeactivateUser",
        "TestWithMatchedBy",
    }
    _, passed, _ = _go_test_json(
        harness, ["./examples/testing"], cwd=ROOT, label="examples/testing"
    )
    missing = sorted(expected - passed)
    if missing:
        raise HarnessFailure(
            f"examples/testing: tests did not run: {', '.join(missing)}"
        )
    return f"query-planner demo + {len(expected)} mock-based tests passed"


def run_multi_tenant_token(harness: Harness) -> str:
    token = mint_token(harness, "user#token-row", "org#token-row")
    header, payload = decode_jwt(token)
    if header.get("alg") != "HS256":
        raise HarnessFailure(f"cmd/token: expected HS256, got {header.get('alg')!r}", token)
    for claim, expected in (
        ("sub", "user#token-row"),
        ("org", "org#token-row"),
    ):
        if payload.get(claim) != expected:
            raise HarnessFailure(f"cmd/token: claim {claim} is {payload.get(claim)!r}, want {expected!r}", token)
    if not isinstance(payload.get("iat"), int) or not isinstance(payload.get("exp"), int):
        raise HarnessFailure("cmd/token: iat/exp claims are not integers", token)
    if payload["exp"] <= payload["iat"]:
        raise HarnessFailure("cmd/token: exp must be after iat", token)

    refusal = harness.run(["go", "run", "./cmd/token"], cwd=MULTI_TENANT, check=False)
    if refusal.returncode == 0:
        raise HarnessFailure(
            "cmd/token: incomplete arguments exited 0; the CLI must refuse them",
            _combined_output(refusal),
        )
    expect_output(
        _combined_output(refusal),
        ["usage: token -secret <secret> -user <user> -org <org>"],
        "cmd/token refusal",
    )
    return "JWT claims + argument refusal"


def _go_test_json(
    harness: Harness, args: list[str], *, cwd: Path, label: str
) -> tuple[subprocess.CompletedProcess, set[str], set[str]]:
    result = harness.run(
        ["go", "test", "-count=1", "-json", *args],
        cwd=cwd,
        timeout=harness.timeout,
        check=False,
    )
    passed: set[str] = set()
    skipped: set[str] = set()
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        action = event.get("Action")
        test = event.get("Test")
        if action == "pass" and test:
            passed.add(test)
        elif action == "skip" and test:
            skipped.add(test)

    if skipped:
        raise HarnessFailure(
            f"{label}: {len(skipped)} test(s) skipped; a skip is a failure: {', '.join(sorted(skipped))}",
            _combined_output(result),
        )
    if result.returncode != 0:
        raise HarnessFailure(f"{label}: go test exited {result.returncode}", _combined_output(result))
    return result, passed, skipped


def run_multi_tenant_lambda(harness: Harness) -> str:
    expected = {
        "TestAPIGatewayRouteMatrix",
        "TestLocalHTTPRouteMatrix",
        "TestWarmWrongSecretsReuseTheCachedVerifier",
    }
    pattern = "^(" + "|".join(sorted(expected)) + ")$"
    _, passed, _ = _go_test_json(
        harness,
        ["-run", pattern, "./cmd/lambda"],
        cwd=MULTI_TENANT,
        label="multi-tenant route matrix",
    )
    missing = sorted(expected - passed)
    if missing:
        raise HarnessFailure(f"multi-tenant route matrix: tests did not run: {', '.join(missing)}")
    return f"{len(expected)} route-matrix tests passed"


def run_multi_tenant_module_tests(harness: Harness) -> str:
    for required in ("auth", "handlers", "cmd/lambda", "cmd/local", "tests"):
        if not (MULTI_TENANT / required).is_dir():
            raise HarnessFailure(f"multi-tenant module is missing package {required}")
    _, passed, _ = _go_test_json(
        harness, ["./..."], cwd=MULTI_TENANT, label="multi-tenant module tests"
    )
    required_tests = {
        "TestAPIGatewayRouteMatrix",
        "TestLocalHTTPRouteMatrix",
        "TestWarmWrongSecretsReuseTheCachedVerifier",
        "TestCreateOrganization",
    }
    missing = sorted(required_tests - passed)
    if missing:
        raise HarnessFailure(
            f"multi-tenant module tests: tests did not run: {', '.join(missing)}"
        )
    return f"{len(passed)} tests passed across the module"


def run_multi_tenant_http(harness: Harness) -> str:
    port = int(harness.ports["http"])
    base = f"http://127.0.0.1:{port}"
    env = dict(harness.env)
    env.update(
        {
            "JWT_SECRET": JWT_SECRET,
            "MULTI_TENANT_LISTEN_ADDR": f"127.0.0.1:{port}",
        }
    )
    log = tempfile.NamedTemporaryFile(prefix="tabletheory-multi-tenant-", suffix=".log", delete=False)
    try:
        process = subprocess.Popen(
            ["go", "run", "./cmd/local"],
            cwd=str(MULTI_TENANT),
            env=env,
            stdout=log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        try:
            _wait_for_health(harness, process, base)
            status, body = http_request("GET", f"{base}/health")
            expect_status(status, 200, "GET /health", body)
            if body.strip() != "OK":
                raise HarnessFailure(f"GET /health returned {body!r}, expected 'OK'")

            missing_org = urllib.parse.quote("org#missing", safe="")
            status, body = http_request("GET", f"{base}/organizations/{missing_org}/users")
            expect_status(status, 401, "unauthenticated request", body)

            forged = mint_token(harness, "user#forged", "org#forged", secret="not-the-server-secret")
            status, body = http_request(
                "GET", f"{base}/organizations/{missing_org}/users", token=forged
            )
            expect_status(status, 401, "request with a token signed by another secret", body)

            signup = mint_token(harness, "user#signup", "org#signup")
            slug = f"harness-{uuid.uuid4().hex[:12]}"
            status, body = http_request(
                "POST",
                f"{base}/organizations",
                token=signup,
                payload={
                    "name": "Harness Corp",
                    "slug": slug,
                    "owner_email": "owner@harness.example.test",
                },
            )
            expect_status(status, 200, "POST /organizations", body)
            org = _json_body(body, "POST /organizations")
            org_id = str(org.get("id", ""))
            owner_id = str(org.get("owner_id", ""))
            if not org_id.startswith("org#") or not owner_id.startswith("user#"):
                raise HarnessFailure("POST /organizations: response is missing id/owner_id", body)
            if org.get("slug") != slug or org.get("status") != "active":
                raise HarnessFailure("POST /organizations: response does not carry the created state", body)

            owner_token = mint_token(harness, owner_id, org_id)
            quoted_org = urllib.parse.quote(org_id, safe="")
            status, body = http_request(
                "GET", f"{base}/organizations/{quoted_org}", token=owner_token
            )
            expect_status(status, 200, "GET /organizations/{id} as its owner", body)
            fetched = _json_body(body, "GET /organizations/{id}")
            if fetched.get("id") != org_id or fetched.get("slug") != slug:
                raise HarnessFailure("GET /organizations/{id}: stored organization does not match", body)

            status, body = http_request(
                "GET", f"{base}/organizations/{quoted_org}/users", token=owner_token
            )
            expect_status(status, 200, "GET /organizations/{id}/users as its owner", body)
            users = _json_body(body, "GET /organizations/{id}/users")
            if not isinstance(users, list) or not any(
                user.get("user_id") == owner_id and user.get("role") == "owner" for user in users
            ):
                raise HarnessFailure("GET /organizations/{id}/users: owner is not listed", body)

            other_org = urllib.parse.quote("org#other", safe="")
            status, body = http_request(
                "GET", f"{base}/organizations/{other_org}/users", token=owner_token
            )
            expect_status(status, 404, "cross-tenant read", body)

            status, body = http_request("GET", f"{base}/organizations", token=owner_token)
            expect_status(status, 200, "GET /organizations", body)
            listed = _json_body(body, "GET /organizations")
            if not isinstance(listed, list) or not any(item.get("id") == org_id for item in listed):
                raise HarnessFailure("GET /organizations: created organization is not listed", body)
        finally:
            _stop_process(process)
            log.close()
        with open(log.name, encoding="utf-8", errors="replace") as handle:
            server_log = handle.read()
        expect_output(
            server_log,
            ["Tables created successfully", "Multi-tenant SaaS API starting on 127.0.0.1:"],
            "multi-tenant server startup",
        )
        return "10 HTTP assertions"
    finally:
        try:
            os.unlink(log.name)
        except FileNotFoundError:
            pass


def _json_body(body: str, label: str):
    try:
        return json.loads(body)
    except json.JSONDecodeError as exc:
        raise HarnessFailure(f"{label}: response is not JSON: {exc}", body) from exc


def _wait_for_health(harness: Harness, process: subprocess.Popen, base: str) -> None:
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise HarnessFailure(f"multi-tenant server exited early (code {process.returncode})")
        try:
            status, _ = http_request("GET", f"{base}/health", timeout=2)
        except HarnessFailure:
            status = 0
        if status == 200:
            return
        time.sleep(0.25)
    raise HarnessFailure(f"multi-tenant server did not become healthy at {base}")


def _stop_process(process: subprocess.Popen) -> None:
    if process.poll() is not None:
        return
    try:
        os.killpg(os.getpgid(process.pid), signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(os.getpgid(process.pid), signal.SIGKILL)
        except ProcessLookupError:
            # The process group can exit between the wait timeout and this kill;
            # SIGKILL then has no target, which is the outcome it was sent for.
            pass
        process.wait(timeout=15)


RUNNERS: dict[str, Callable[[Harness], str]] = {
    "go-local-quickstart": run_go_local_quickstart,
    "go-encryption": run_go_encryption,
    "shell-cdk-multilang-local": run_shell_cdk_multilang_local,
    "ts-local": run_ts_local,
    "py-local-crud": run_py_local_crud,
    "go-basic-todo": run_basic_todo,
    "go-basic-notes": run_basic_notes,
    "go-basic-contacts": run_basic_contacts,
    "go-initialization-local": run_go_initialization_local,
    "go-typed-go": run_go_typed,
    "go-loose-examples": run_go_loose_examples,
    "go-release-state": run_go_release_state,
    "ts-release-state": run_ts_release_state,
    "py-release-state": run_py_release_state,
    "py-gsi-query": run_py_gsi_query,
    "go-library-demos": run_go_library_demos,
    "multi-tenant-token": run_multi_tenant_token,
    "multi-tenant-lambda": run_multi_tenant_lambda,
    "multi-tenant-module-tests": run_multi_tenant_module_tests,
    "multi-tenant-http": run_multi_tenant_http,
}


# --- reporting --------------------------------------------------------------


def print_failure(label: str, failure: HarnessFailure) -> None:
    print(f"        {failure.message}")
    if failure.output.strip():
        for line in _tail(failure.output).splitlines():
            print(f"        | {line}")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run every classified TableTheory example against the local profile."
    )
    parser.add_argument(
        "--run-id",
        default="",
        help="reproducible run id mixed into the compose project name (default: random per run)",
    )
    parser.add_argument(
        "--cleanup",
        action="store_true",
        help="remove the containers and networks of every harness run, from any checkout",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    try:
        raw_profile, entries = load_manifest(MANIFEST_PATH)
    except HarnessFailure as failure:
        print(f"examples-local: {failure.message}", file=sys.stderr)
        return 2

    if args.cleanup:
        return cleanup_projects(str(raw_profile.get("name", "tabletheory-examples-local")))

    discovered = discover(ROOT)
    errors = validate_manifest(ROOT, MANIFEST_PATH, raw_profile, entries, discovered, RUNNERS)
    if errors:
        print("examples-local: manifest validation failed", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 2

    run_id = args.run_id or f"{os.getpid()}-{uuid.uuid4().hex[:8]}"
    try:
        profile = build_profile(raw_profile, ROOT, run_id)
    except (KeyError, TypeError, ValueError) as exc:
        print(f"examples-local: invalid profile: {exc}", file=sys.stderr)
        return 2

    taken: set[int] = set()
    harness = Harness(env=dict(os.environ), ports={"http": free_port(taken), "cdk": free_port(taken)})

    print("TableTheory local examples harness")
    print(f"  repository: {ROOT}")
    print(f"  profile:    {raw_profile['composeFile']}")
    print(f"  project:    {profile.project} (run id {run_id})")
    print(f"  discovered: {len(discovered)} entrypoints; manifest entries: {len(entries)}")
    print()
    print("preflight")

    try:
        start_profile(harness, profile)
        harness.env = build_env(profile)
        for service in REQUIRED_ENDPOINTS:
            print(f"          {service:<9} {profile.url(service)}")
        try:
            preflight(harness, profile)
            prepare(harness)
        except HarnessFailure as failure:
            print_failure("preflight", failure)
            print()
            print("RESULT: FAIL (preflight)")
            return 1

        executed = [entry for entry in entries if entry.runner]
        results: dict[str, tuple[str, str]] = {}
        failures: list[tuple[Entry, HarnessFailure]] = []

        print()
        total = len(executed)
        for index, entry in enumerate(executed, start=1):
            print(
                f"  [{index:>2}/{total}] {entry.label:<58} {entry.class_name:<3} RUN",
                flush=True,
            )
            try:
                detail = RUNNERS[entry.runner](harness)
                results[entry.key] = ("PASS", detail)
                print(f"  [{index:>2}/{total}] {entry.label:<58} {entry.class_name:<3} PASS  {detail}")
            except HarnessFailure as failure:
                results[entry.key] = ("FAIL", failure.message)
                failures.append((entry, failure))
                print(f"  [{index:>2}/{total}] {entry.label:<58} {entry.class_name:<3} FAIL")
                print_failure(entry.label, failure)
            except Exception as exc:  # noqa: BLE001 - a harness bug must fail the row, not abort the run
                failure = HarnessFailure(f"{type(exc).__name__}: {exc}", traceback.format_exc())
                results[entry.key] = ("FAIL", failure.message)
                failures.append((entry, failure))
                print(f"  [{index:>2}/{total}] {entry.label:<58} {entry.class_name:<3} FAIL")
                print_failure(entry.label, failure)

        for entry in entries:
            if not entry.covered_by:
                continue
            target = results.get(entry.covered_by)
            if target is None:
                results[entry.key] = ("FAIL", f"covered row {entry.covered_by} did not run")
                failures.append((entry, HarnessFailure(results[entry.key][1])))
            else:
                results[entry.key] = target

        pending = [entry for entry in entries if entry.pending]
        print()
        print("pending milestones (reported, never counted as pass)")
        if pending:
            for entry in pending:
                print(f"  {entry.key:<58} PENDING-MILESTONE {entry.pending}")
        else:
            print("  (none)")

        print()
        print("summary")
        print(f"  {'result':<8} {'class':<22} example")
        passed = 0
        failed = 0
        for entry in entries:
            if entry.pending:
                print(f"  {'PENDING':<8} {'PENDING-MILESTONE ' + entry.pending:<22} {entry.key}")
                continue
            result, detail = results.get(entry.key, ("FAIL", "row did not run"))
            suffix = f"  ({detail})" if detail else ""
            if entry.covered_by:
                suffix = f"  (via {entry.covered_by})"
            print(f"  {result:<8} {entry.class_name:<22} {entry.key}{suffix}")
            if result == "PASS":
                passed += 1
            else:
                failed += 1

        print()
        print(f"counts: {passed} passed, {failed} failed, {len(pending)} pending-milestone")
        if failures:
            print("RESULT: FAIL")
            return 1
        print("RESULT: PASS")
        return 0
    finally:
        try:
            stop_profile(harness, profile)
        except HarnessFailure as exc:
            print(f"  WARN  could not remove profile {profile.project}: {exc.message}")
        else:
            print(f"  PASS  removed profile {profile.project} (containers and network)")


if __name__ == "__main__":
    sys.exit(main())
