"""Lambda runtime helpers for ``make examples-local``.

Every handler is built with its real runtime toolchain and started in the
official AWS Lambda runtime image, which embeds the Runtime Interface Emulator.
The harness invokes it over the RIE HTTP API with real event payloads; no
handler is called in process.

The function container joins the run's private compose network, resolves the
profile services through ``/etc/hosts``, and gets a closed DNS server so a real
AWS hostname cannot resolve. That, the static dummy credentials and the endpoint
overrides together keep every request local; the in-container preflight proves
it before any example runs.
"""

from __future__ import annotations

import json
import os
import shutil
import socket
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
IMAGES = {
    "go": "public.ecr.aws/lambda/provided:al2023",
    "node": "public.ecr.aws/lambda/nodejs:24",
    "python": "public.ecr.aws/lambda/python:3.14",
}
PREFLIGHT_PACKAGE = "./scripts/local/cmd/examples-local-lambda-preflight"
RIE_PORT = 8080
DEFAULT_TIMEOUT = 120
CLOSED_DNS = "127.0.0.2"
HANDLER_NAME = "handler.handler"


class LambdaFailure(Exception):
    """A Lambda runtime step failed. ``output`` carries the container logs."""

    def __init__(self, message: str, output: str = "") -> None:
        super().__init__(message)
        self.message = message
        self.output = output


@dataclass
class LambdaFunction:
    runtime: "LambdaRuntime"
    name: str
    container: str
    image: str
    port: int
    task_dir: Path
    _logs_seen: int = field(default=0)

    def invoke(self, payload: dict, *, timeout: int = DEFAULT_TIMEOUT) -> object:
        """POST one event to the RIE endpoint and return the decoded response."""
        url = f"http://127.0.0.1:{self.port}/2015-03-31/functions/function/invocations"
        request = urllib.request.Request(
            url,
            data=json.dumps(payload).encode("utf-8"),
            method="POST",
            headers={"Content-Type": "application/json"},
        )
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        try:
            with opener.open(request, timeout=timeout) as response:
                body = response.read().decode("utf-8", "replace")
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", "replace")
            raise LambdaFailure(
                f"{self.name}: RIE returned HTTP {exc.code}",
                body + "\n" + self.logs(),
            ) from exc
        except (urllib.error.URLError, OSError) as exc:
            raise LambdaFailure(f"{self.name}: RIE invocation failed: {exc}", self.logs()) from exc

        try:
            decoded = json.loads(body)
        except json.JSONDecodeError as exc:
            raise LambdaFailure(f"{self.name}: RIE response is not JSON", body + "\n" + self.logs()) from exc
        if isinstance(decoded, dict) and ("errorType" in decoded or "errorMessage" in decoded):
            raise LambdaFailure(f"{self.name}: handler error", body + "\n" + self.logs())
        return decoded

    def logs(self, since: float | None = None) -> str:
        """Return container logs, optionally only lines since a Unix timestamp."""
        argv = ["docker", "logs"]
        if since is not None:
            argv.extend(["--since", str(int(since))])
        argv.append(self.container)
        result = self.runtime.harness.run(argv, timeout=60, check=False)
        output = (result.stdout or "") + (result.stderr or "")
        return output

    def new_logs(self) -> str:
        """Return only lines added since the last call (or ``mark_logs``)."""
        lines = self.logs().splitlines()
        fresh = lines[self._logs_seen :]
        self._logs_seen = len(lines)
        return "\n".join(fresh)

    def mark_logs(self) -> int:
        self._logs_seen = len(self.logs().splitlines())
        return self._logs_seen

    def stop(self) -> None:
        self.runtime.harness.run(["docker", "rm", "-f", self.container], timeout=120, check=False)


class LambdaRuntime:
    """Starts and invokes Lambda function containers for one harness run."""

    def __init__(self, harness, profile, scratch: Path) -> None:
        self.harness = harness
        self.profile = profile
        self.scratch = scratch
        self.network = f"{profile.project}_default"
        self.functions: dict[str, LambdaFunction] = {}
        self._service_ips: dict[str, str] = {}
        self._go_arch: str | None = None

    # --- environment --------------------------------------------------------

    def _compose(self, *args: str):
        return self.harness.run(
            [
                "docker",
                "compose",
                "-f",
                str(self.profile.compose_file),
                "-p",
                self.profile.project,
                *args,
            ],
            timeout=120,
        )

    def service_ips(self) -> dict[str, str]:
        if self._service_ips:
            return self._service_ips
        for name in self.profile.containers:
            result = self._compose("ps", "-q", name)
            container = result.stdout.strip().splitlines()
            if not container:
                raise LambdaFailure(f"profile service {name} has no container")
            inspect = self.harness.run(
                [
                    "docker",
                    "inspect",
                    "--format",
                    "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
                    container[0],
                ],
                timeout=60,
            )
            address = inspect.stdout.strip()
            if not address:
                raise LambdaFailure(f"profile service {name} has no network address")
            self._service_ips[name] = address
        return self._service_ips

    def container_endpoints(self) -> dict[str, str]:
        endpoints: dict[str, str] = {}
        for service, spec in self.profile.endpoints.items():
            container = self.profile.containers[spec.container]
            endpoints[service] = f"http://{spec.container}:{container.container_port}"
        return endpoints

    def _container_env(self, extra: dict[str, str] | None) -> dict[str, str]:
        env = {
            "AWS_REGION": "us-east-1",
            "AWS_DEFAULT_REGION": "us-east-1",
            "AWS_ACCESS_KEY_ID": "dummy",
            "AWS_SECRET_ACCESS_KEY": "dummy",
            "AWS_EC2_METADATA_DISABLED": "true",
            "AWS_CONFIG_FILE": os.devnull,
            "AWS_SHARED_CREDENTIALS_FILE": os.devnull,
            "AWS_LAMBDA_FUNCTION_TIMEOUT": "60",
            "PYTHONUNBUFFERED": "1",
        }
        for service, endpoint in self.profile.endpoints.items():
            url = self.container_endpoints()[service]
            for name in endpoint.env:
                env[name] = url
        if self.harness.env.get("KMS_KEY_ARN"):
            env["KMS_KEY_ARN"] = self.harness.env["KMS_KEY_ARN"]
        if extra:
            env.update({name: str(value) for name, value in extra.items() if value is not None})
        return env

    def _go_architecture(self) -> str:
        if self._go_arch is None:
            result = self.harness.run(["docker", "version", "--format", "{{.Server.Arch}}"], timeout=60)
            arch = result.stdout.strip().lower()
            mapping = {"arm64": "arm64", "aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64"}
            if arch not in mapping:
                raise LambdaFailure(f"unsupported Docker server architecture {arch!r}")
            self._go_arch = mapping[arch]
        return self._go_arch

    # --- builds -------------------------------------------------------------

    def _task_dir(self, name: str) -> Path:
        path = self.scratch / name
        if path.exists():
            shutil.rmtree(path)
        path.mkdir(parents=True)
        return path

    def build_go(self, name: str, package: str) -> Path:
        task = self._task_dir(name)
        self.harness.run(
            ["go", "build", "-o", str(task / "bootstrap"), package],
            env={
                "GOOS": "linux",
                "GOARCH": self._go_architecture(),
                "CGO_ENABLED": "0",
            },
            timeout=600,
        )
        if not (task / "bootstrap").is_file():
            raise LambdaFailure(f"{name}: go build produced no bootstrap")
        return task

    def build_node(self, name: str, entry: Path) -> Path:
        esbuild = ROOT / "ts" / "node_modules" / ".bin" / "esbuild"
        if not esbuild.exists():
            raise LambdaFailure(f"esbuild is missing at {esbuild}; run npm ci in ts/")
        task = self._task_dir(name)
        self.harness.run(
            [
                str(esbuild),
                str(entry),
                "--bundle",
                "--platform=node",
                "--format=cjs",
                f"--outfile={task / 'handler.js'}",
                "--log-level=warning",
            ],
            timeout=300,
        )
        if not (task / "handler.js").is_file():
            raise LambdaFailure(f"{name}: esbuild produced no handler.js")
        return task

    def build_python(self, name: str, handler: Path) -> Path:
        task = self._task_dir(name)
        package = ROOT / "py" / "src" / "tabletheory_py"
        shutil.copytree(
            package,
            task / "tabletheory_py",
            ignore=shutil.ignore_patterns("__pycache__", "*.pyc"),
        )
        shutil.copy2(handler, task / "handler.py")
        return task

    # --- lifecycle ----------------------------------------------------------

    def _start(
        self,
        name: str,
        image: str,
        task_dir: Path,
        handler_arg: str | None,
        extra_env: dict[str, str] | None,
    ) -> LambdaFunction:
        if name in self.functions:
            raise LambdaFailure(f"Lambda function {name} is already running")
        container = f"{self.profile.project}-lambda-{name}"
        self.harness.run(["docker", "rm", "-f", container], timeout=120, check=False)

        argv = [
            "docker",
            "run",
            "-d",
            "--name",
            container,
            "--network",
            self.network,
            "--dns",
            CLOSED_DNS,
            "-p",
            f"127.0.0.1::{RIE_PORT}",
            "-v",
            f"{task_dir}:/var/task:ro",
        ]
        for service, address in self.service_ips().items():
            argv.extend(["--add-host", f"{service}:{address}"])
        for key, value in sorted(self._container_env(extra_env).items()):
            argv.extend(["-e", f"{key}={value}"])
        argv.append(image)
        if handler_arg:
            argv.append(handler_arg)

        self.harness.run(argv, timeout=300)
        try:
            port = self._published_port(container)
        except LambdaFailure:
            self.harness.run(["docker", "rm", "-f", container], timeout=120, check=False)
            raise
        function = LambdaFunction(
            runtime=self,
            name=name,
            container=container,
            image=image,
            port=port,
            task_dir=task_dir,
        )
        self.functions[name] = function
        return function

    def _published_port(self, container: str, seconds: int = 60) -> int:
        deadline = time.monotonic() + seconds
        last = ""
        while time.monotonic() < deadline:
            result = self.harness.run(
                ["docker", "port", container, str(RIE_PORT)], timeout=60, check=False
            )
            lines = [line.strip() for line in (result.stdout or "").splitlines() if line.strip()]
            if lines:
                host, _, raw_port = lines[0].rpartition(":")
                if host in ("127.0.0.1", "0.0.0.0", "::1", "[::1]"):
                    try:
                        port = int(raw_port)
                    except ValueError:
                        last = lines[0]
                    else:
                        if self._port_open(port):
                            return port
                        last = f"port {port} is not accepting connections yet"
                else:
                    last = f"published on {host!r}, not loopback"
            time.sleep(0.25)
        raise LambdaFailure(f"{container}: RIE did not publish a loopback port ({last})")

    @staticmethod
    def _port_open(port: int, timeout: float = 0.5) -> bool:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=timeout):
                return True
        except OSError:
            return False

    def start_go(self, name: str, package: str, extra_env: dict[str, str] | None = None) -> LambdaFunction:
        return self._start(
            name, IMAGES["go"], self.build_go(name, package), "bootstrap", extra_env
        )

    def start_node(self, name: str, entry: Path, extra_env: dict[str, str] | None = None) -> LambdaFunction:
        return self._start(
            name, IMAGES["node"], self.build_node(name, entry), HANDLER_NAME, extra_env
        )

    def start_python(self, name: str, handler: Path, extra_env: dict[str, str] | None = None) -> LambdaFunction:
        return self._start(
            name, IMAGES["python"], self.build_python(name, handler), HANDLER_NAME, extra_env
        )

    def preflight(self) -> dict:
        """Prove the function environment cannot reach real AWS."""
        function = self.start_go("preflight", PREFLIGHT_PACKAGE)
        try:
            result = function.invoke({"endpoints": self.container_endpoints()}, timeout=120)
        finally:
            function.stop()
            self.functions.pop("preflight", None)
        if not isinstance(result, dict) or result.get("ok") is not True:
            raise LambdaFailure("in-container Lambda preflight did not report ok", json.dumps(result))
        return result

    def close(self) -> None:
        for function in list(self.functions.values()):
            function.stop()
        self.functions.clear()
        if self.scratch.exists():
            shutil.rmtree(self.scratch, ignore_errors=True)
