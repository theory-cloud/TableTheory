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
* a skip is a failure, including a ``t.Skip`` inside a Go test;
* entries owned by a later milestone are reported as ``PENDING-MILESTONE`` and
  are never counted as pass;
* every assertion is made against real output or real HTTP behavior, never
  against a mock.
"""

from __future__ import annotations

import base64
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
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

ROOT = Path(__file__).resolve().parents[2]
HARNESS_DIR = Path(__file__).resolve().parent
MANIFEST_PATH = HARNESS_DIR / "examples-local.json"

CLASSES = ("L1", "L2", "L3", "I")
MILESTONES = ("T2", "T3", "T4a", "T4b")
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
COMPOSE_PROJECT = "tabletheory-examples-local"
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
    * ``examples/**/*.sh`` is a shell runner;
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
                continue  # already classified by the shell pass above
            if suffix in RUNTIME_SUFFIXES:
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
    for key in ("composeFile", "dynamodbLocal"):
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
    manifest_path: Path,
    profile: dict,
    entries: list[Entry],
    discovered: dict[str, str],
    runners: dict[str, Callable],
) -> list[str]:
    errors: list[str] = []

    entrypoint_keys = {entry.key for entry in entries if entry.kind == "entrypoint"}
    for path in sorted(set(discovered) - entrypoint_keys):
        errors.append(f"unclassified entrypoint (add it to {manifest_path.name}): {path}")
    for path in sorted(entrypoint_keys - set(discovered)):
        entry = next(item for item in entries if item.key == path)
        if not entry.covered_by and not entry.pending:
            errors.append(f"manifest entry is not an entrypoint in the tree: {path}")

    runner_owner: dict[str, str] = {}
    by_key = {entry.key: entry for entry in entries}

    for entry in entries:
        if entry.kind == "suite":
            suite_dir = ROOT / entry.key
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

    compose_path = ROOT / str(profile.get("composeFile", ""))
    if not compose_path.is_file():
        errors.append(f"profile compose file is missing: {profile.get('composeFile')}")
    else:
        compose_text = compose_path.read_text(encoding="utf-8")
        local = profile.get("dynamodbLocal", {})
        image = str(local.get("image", ""))
        port = int(local.get("port", 0))
        if image and f"image: {image}" not in compose_text:
            errors.append(f"{compose_path}: image pin does not match the manifest ({image})")
        if port and f"127.0.0.1:{port}:8000" not in compose_text:
            errors.append(f"{compose_path}: port does not match the manifest ({port})")
        if port == 8000:
            errors.append(f"{compose_path}: the profile must not reuse the shared :8000")

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


def build_env(port: int) -> dict:
    endpoint = f"http://127.0.0.1:{port}"
    # Start from a clean AWS surface: an inherited profile, session token or
    # endpoint override must not be able to redirect any child process.
    env = {key: value for key, value in os.environ.items() if not key.startswith("AWS_")}
    for key in (
        "JWT_SECRET",
        "MULTI_TENANT_LISTEN_ADDR",
        "CDK_LOCAL_PORT",
        "DEMO_TABLE_NAME",
    ):
        env.pop(key, None)
    env.update(
        {
            "AWS_REGION": "us-east-1",
            "AWS_DEFAULT_REGION": "us-east-1",
            "AWS_ACCESS_KEY_ID": "dummy",
            "AWS_SECRET_ACCESS_KEY": "dummy",
            "AWS_EC2_METADATA_DISABLED": "true",
            "AWS_CONFIG_FILE": os.devnull,
            "AWS_SHARED_CREDENTIALS_FILE": os.devnull,
            "AWS_ENDPOINT_URL": endpoint,
            "AWS_ENDPOINT_URL_DYNAMODB": endpoint,
            "DYNAMODB_ENDPOINT": endpoint,
            "GOTOOLCHAIN": go_toolchain(),
        }
    )
    return env


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


def preflight(harness: Harness, profile: dict) -> None:
    local = profile["dynamodbLocal"]
    port = int(local["port"])
    endpoint = f"http://127.0.0.1:{port}"
    compose_file = ROOT / str(profile["composeFile"])

    docker_here = _docker_available(harness)
    if docker_here and _profile_container_id(harness, compose_file):
        # The harness owns this container, so every run starts from an empty
        # in-memory database. Examples must not depend on state a previous run
        # (or another session) left behind.
        harness.run(
            ["docker", "compose", "-f", str(compose_file), "-p", COMPOSE_PROJECT, "down", "-v"],
            timeout=120,
            check=False,
        )
        print("  PASS  recreated the profile container (clean database)")

    if not port_open("127.0.0.1", port):
        if not docker_here:
            raise HarnessFailure(
                "DynamoDB Local is not listening and Docker is unavailable; start Colima/Docker first"
            )
        harness.run(
            [
                "docker",
                "compose",
                "-f",
                str(compose_file),
                "-p",
                COMPOSE_PROJECT,
                "up",
                "-d",
            ],
            timeout=300,
        )
    wait_for_dynamodb_local(harness, endpoint)
    print(f"  PASS  DynamoDB Local answers at {endpoint}")

    for key in ("AWS_ENDPOINT_URL_DYNAMODB", "DYNAMODB_ENDPOINT", "AWS_ENDPOINT_URL"):
        resolved = harness.env.get(key)
        if resolved != endpoint:
            raise HarnessFailure(f"{key} is {resolved!r}; the harness must own it ({endpoint})")
    print("  PASS  harness owns AWS_ENDPOINT_URL_DYNAMODB, DYNAMODB_ENDPOINT and AWS_ENDPOINT_URL")

    for name in ("http", "cdk"):
        port = int(harness.ports[name])
        if port_open("127.0.0.1", port):
            raise HarnessFailure(
                f"port {port} ({name}) is already in use; stop the process or container that holds it"
            )
    print("  PASS  example ports are free (HTTP server and cross-language container)")

    probe = harness.run(
        [
            "go",
            "run",
            "./scripts/local/cmd/examples-local-preflight",
            "-endpoint",
            endpoint,
        ],
        timeout=300,
    )
    for line in probe.stdout.strip().splitlines():
        print(f"  PASS  {line.removeprefix('preflight: ')}")


def _docker_available(harness: Harness) -> bool:
    result = harness.run(["docker", "info"], timeout=60, check=False)
    return result.returncode == 0


def _profile_container_id(harness: Harness, compose_file: Path) -> str:
    result = harness.run(
        [
            "docker",
            "compose",
            "-f",
            str(compose_file),
            "-p",
            COMPOSE_PROJECT,
            "ps",
            "-q",
            "dynamodb-local",
        ],
        timeout=60,
        check=False,
    )
    return result.stdout.strip()


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


# --- runners ----------------------------------------------------------------


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
            "go: wrote demo#1/v1 (lang=go)",
            "node: read demo#1/v1 value=shared-value lang=go",
            "python: read demo#1/v1 value='shared-value' lang='go'",
            "LOCAL CROSS-LANGUAGE NO-DRIFT: PASS",
        ],
        "cdk-multilang local",
    )
    return "drift check + Go/TS/Py round-trip"


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
            pass
        process.wait(timeout=15)


RUNNERS: dict[str, Callable[[Harness], str]] = {
    "go-local-quickstart": run_go_local_quickstart,
    "shell-cdk-multilang-local": run_shell_cdk_multilang_local,
    "ts-local": run_ts_local,
    "py-local-crud": run_py_local_crud,
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


def main() -> int:
    try:
        profile, entries = load_manifest(MANIFEST_PATH)
    except HarnessFailure as failure:
        print(f"examples-local: {failure.message}", file=sys.stderr)
        return 2

    discovered = discover(ROOT)
    errors = validate_manifest(MANIFEST_PATH, profile, entries, discovered, RUNNERS)
    if errors:
        print("examples-local: manifest validation failed", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 2

    port = int(profile["dynamodbLocal"]["port"])
    ports = {"ddb": port, "http": port + 1, "cdk": port + 2}
    harness = Harness(env=build_env(port), ports=ports)

    print("TableTheory local examples harness")
    print(f"  repository: {ROOT}")
    print(f"  profile:    {profile['composeFile']} ({profile['dynamodbLocal']['image']})")
    print(f"  endpoint:   {harness.env['DYNAMODB_ENDPOINT']}")
    print(f"  discovered: {len(discovered)} entrypoints; manifest entries: {len(entries)}")
    print()
    print("preflight")
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


if __name__ == "__main__":
    sys.exit(main())
