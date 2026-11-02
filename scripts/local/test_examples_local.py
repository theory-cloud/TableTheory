#!/usr/bin/env python3
"""Mutation self-tests for the local examples harness (``make examples-local-selftest``).

The harness only holds if its classification and its local profile fail closed,
so these tests build throwaway repository trees, mutate them, and assert that
validation notices:

* a rogue ``.ts`` and a rogue ``.py`` under ``examples/`` that nothing classifies;
* a rogue ``.sh`` under ``ts/examples/`` that nothing classifies;
* a pending row that names a file which does not exist;
* a pending row that names a file discovery does not treat as an entrypoint;
* a profile image that is not digest-pinned, a fixed host port, a fixed
  container name, or an endpoint that omits the AWS-standard override.

They also pin the documented exclusions, so ``bin/app.ts`` stays visible while
``lib/``, ``test/`` and generated output stay out of the entrypoint set.

``ConcurrencyIsolationTests`` starts two real profiles at once and proves the
second run cannot recreate or remove the first run's containers: the container
ids stay stable, the endpoints differ, and the first database still answers
after the second stack is torn down.
"""

from __future__ import annotations

import os
import re
import sys
import tempfile
import unittest
import unittest.mock
from pathlib import Path

HARNESS_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(HARNESS_DIR))

import run_examples_local as harness  # noqa: E402
import lambda_runtime  # noqa: E402

DIGEST = "a" * 64
DIGEST_B = "b" * 64
DIGEST_C = "c" * 64
DIGEST_D = "d" * 64
DIGEST_E = "e" * 64
DIGEST_F = "f" * 64
DIGEST_0 = "0" * 64

COMPOSE = f"""\
services:
  dynamodb-local:
    image: amazon/dynamodb-local@sha256:{DIGEST}
    ports:
      - 127.0.0.1::8000
  moto:
    image: motoserver/moto@sha256:{DIGEST_B}
    ports:
      - 127.0.0.1::5000
  s3proxy:
    image: andrewgaul/s3proxy@sha256:{DIGEST_C}
    ports:
      - 127.0.0.1::80
  elasticmq:
    image: softwaremill/elasticmq-native@sha256:{DIGEST_D}
    ports:
      - 127.0.0.1::9324
"""

PROFILE = {
    "name": "tabletheory-examples-local",
    "composeFile": "scripts/local/docker-compose.yml",
    "containers": {
        "dynamodb-local": {"image": f"amazon/dynamodb-local@sha256:{DIGEST}", "containerPort": 8000},
        "moto": {"image": f"motoserver/moto@sha256:{DIGEST_B}", "containerPort": 5000},
        "s3proxy": {"image": f"andrewgaul/s3proxy@sha256:{DIGEST_C}", "containerPort": 80},
        "elasticmq": {"image": f"softwaremill/elasticmq-native@sha256:{DIGEST_D}", "containerPort": 9324},
    },
    "endpoints": {
        "dynamodb": {
            "container": "dynamodb-local",
            "env": ["AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_DYNAMODB", "DYNAMODB_ENDPOINT"],
        },
        "kms": {"container": "moto", "env": ["AWS_ENDPOINT_URL_KMS", "KMS_ENDPOINT"]},
        "sts": {"container": "moto", "env": ["AWS_ENDPOINT_URL_STS", "STS_ENDPOINT"]},
        "s3": {"container": "s3proxy", "env": ["AWS_ENDPOINT_URL_S3", "S3_ENDPOINT"]},
        "sqs": {"container": "elasticmq", "env": ["AWS_ENDPOINT_URL_SQS", "SQS_ENDPOINT"]},
    },
    "lambda": {
        "images": {
            "go": {
                "tag": "public.ecr.aws/lambda/provided:al2023",
                "digest": f"sha256:{DIGEST_E}",
                "pinned": "2026-11-01",
            },
            "node": {
                "tag": "public.ecr.aws/lambda/nodejs:24",
                "digest": f"sha256:{DIGEST_F}",
                "pinned": "2026-11-01",
            },
            "python": {
                "tag": "public.ecr.aws/lambda/python:3.14",
                "digest": f"sha256:{DIGEST_0}",
                "pinned": "2026-11-01",
            },
        }
    },
}

GO_MAIN = "package main\n\nfunc main() {}\n"
NOOP_RUNNERS = {"noop": lambda harness: "ok"}


def write(root: Path, relative: str, text: str = "", *, executable: bool = False) -> Path:
    path = root / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")
    if executable:
        path.chmod(0o755)
    return path


class HarnessTree:
    """A minimal repository tree with one classified Go entrypoint."""

    def __init__(self, root: Path) -> None:
        self.root = root
        write(root, "scripts/local/docker-compose.yml", COMPOSE)
        write(root, "examples/quickstart/main.go", GO_MAIN)
        self.entries = {"examples/quickstart/main.go": {"run": "noop", "class": "L1"}}

    def validate(self, entries: dict | None = None, profile: dict | None = None) -> list[str]:
        rows = [
            harness.Entry(key, "entrypoint", dict(data))
            for key, data in (entries if entries is not None else self.entries).items()
        ]
        return harness.validate_manifest(
            self.root,
            self.root / "scripts/local/examples-local.json",
            profile if profile is not None else PROFILE,
            rows,
            harness.discover(self.root),
            NOOP_RUNNERS,
        )


class DiscoveryTests(unittest.TestCase):
    def test_baseline_tree_validates(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            self.assertEqual(HarnessTree(Path(tmp)).validate(), [])

    def test_runtime_files_are_discovered_under_examples(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            HarnessTree(root)
            write(root, "examples/edge/bin/app.ts", "const app = 1;\n")
            write(root, "examples/edge/lambdas/node/handler.ts", "export const handler = 1;\n")
            write(root, "examples/edge/local/read.mts", "export {};\n")
            write(root, "examples/edge/local/read.py", "print('read')\n")
            write(root, "examples/edge/local/tool.mjs", "export {};\n")
            write(root, "examples/edge/local/run.sh", "#!/usr/bin/env bash\n", executable=True)
            write(root, "examples/edge/local/run.custom", "#!/usr/bin/env bash\n", executable=True)

            discovered = harness.discover(root)

            for path in (
                "examples/edge/bin/app.ts",
                "examples/edge/lambdas/node/handler.ts",
                "examples/edge/local/read.mts",
                "examples/edge/local/read.py",
                "examples/edge/local/tool.mjs",
                "examples/edge/local/run.sh",
                "examples/edge/local/run.custom",
            ):
                self.assertIn(path, discovered)
            self.assertEqual(discovered["examples/edge/local/run.sh"], "shell-runner")
            self.assertEqual(discovered["examples/edge/local/run.custom"], "executable")

    def test_shell_runners_under_runtime_example_trees_are_discovered(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            HarnessTree(root)
            write(root, "ts/examples/edge/run.sh", "#!/usr/bin/env bash\n", executable=True)
            write(root, "py/examples/edge/run.sh", "#!/usr/bin/env bash\n", executable=True)

            discovered = harness.discover(root)

            for path in ("ts/examples/edge/run.sh", "py/examples/edge/run.sh"):
                self.assertIn(path, discovered)
                self.assertEqual(discovered[path], "shell-runner")

    def test_documented_non_entrypoints_are_not_discovered(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            HarnessTree(root)
            write(root, "examples/edge/lib/stack.ts", "export const stack = 1;\n")
            write(root, "examples/edge/test/stack.test.ts", "export const test = 1;\n")
            write(root, "examples/edge/local/module_test.py", "print('test')\n")
            write(
                root,
                "examples/edge/local/generated-table.ts",
                "// Code generated by tabletheory gen --cdk; DO NOT EDIT.\nexport {};\n",
            )
            write(root, "examples/edge/node_modules/pkg/index.js", "module.exports = {};\n")
            write(root, "examples/edge/cdk.out/asset/app.js", "module.exports = {};\n")

            discovered = harness.discover(root)

            for path in (
                "examples/edge/lib/stack.ts",
                "examples/edge/test/stack.test.ts",
                "examples/edge/local/module_test.py",
                "examples/edge/local/generated-table.ts",
                "examples/edge/node_modules/pkg/index.js",
                "examples/edge/cdk.out/asset/app.js",
            ):
                self.assertNotIn(path, discovered)

    def test_repository_runtime_files_are_discovered_and_classified(self) -> None:
        discovered = harness.discover(harness.ROOT)
        expected = {
            "examples/blog/authorizer/main.go",
            "examples/blog/cmd/comments/main.go",
            "examples/blog/cmd/local/main.go",
            "examples/blog/cmd/posts/main.go",
            "examples/cdk-multilang/bin/app.ts",
            "examples/cdk-multilang/lambdas/archive/handler.ts",
            "examples/cdk-multilang/lambdas/go/main.go",
            "examples/cdk-multilang/lambdas/node/handler.ts",
            "examples/cdk-multilang/lambdas/python/handler.py",
            "examples/cdk-multilang/local/read.mts",
            "examples/cdk-multilang/local/read.py",
            "examples/cdk-multilang/scripts/smoke-local.sh",
            "examples/cdk-multilang/scripts/smoke.sh",
        }
        self.assertEqual(sorted(expected - set(discovered)), [])
        for path in (
            "examples/cdk-multilang/lib/multilang-demo-stack.ts",
            "examples/cdk-multilang/test/tabletheory-ttl-archive.test.ts",
            "examples/cdk-multilang/local/generated-table.ts",
        ):
            self.assertNotIn(path, discovered)

        profile, entries = harness.load_manifest(harness.MANIFEST_PATH)
        self.assertEqual(
            harness.validate_manifest(
                harness.ROOT, harness.MANIFEST_PATH, profile, entries, discovered, harness.RUNNERS
            ),
            [],
        )


class UnclassifiedMutationTests(unittest.TestCase):
    def test_rogue_typescript_under_examples_fails_validation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            self.assertEqual(tree.validate(), [])

            rogue = write(tree.root, "examples/rogue/main.ts", "console.log('rogue');\n")
            errors = tree.validate()

            self.assertIn("examples/rogue/main.ts", harness.discover(tree.root))
            self.assertTrue(
                any(
                    "unclassified entrypoint" in error and "examples/rogue/main.ts" in error
                    for error in errors
                ),
                errors,
            )

            rogue.unlink()
            self.assertEqual(tree.validate(), [])

    def test_rogue_python_under_examples_fails_validation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            self.assertEqual(tree.validate(), [])

            rogue = write(tree.root, "examples/rogue/tool.py", "print('rogue')\n")
            errors = tree.validate()

            self.assertIn("examples/rogue/tool.py", harness.discover(tree.root))
            self.assertTrue(
                any(
                    "unclassified entrypoint" in error and "examples/rogue/tool.py" in error
                    for error in errors
                ),
                errors,
            )

            rogue.unlink()
            self.assertEqual(tree.validate(), [])

    def test_rogue_shell_runner_under_ts_examples_fails_validation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            self.assertEqual(tree.validate(), [])

            rogue = write(
                tree.root,
                "ts/examples/rogue/run.sh",
                "#!/usr/bin/env bash\n",
                executable=True,
            )
            errors = tree.validate()

            self.assertIn("ts/examples/rogue/run.sh", harness.discover(tree.root))
            self.assertTrue(
                any(
                    "unclassified entrypoint" in error and "ts/examples/rogue/run.sh" in error
                    for error in errors
                ),
                errors,
            )

            rogue.unlink()
            self.assertEqual(tree.validate(), [])


class ManifestRowTests(unittest.TestCase):
    def test_pending_row_naming_a_missing_file_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            entries = dict(tree.entries)
            entries["examples/ghost/main.go"] = {"pending": "T2"}

            errors = tree.validate(entries)

            self.assertTrue(
                any(
                    "does not name a file in the tree" in error and "examples/ghost/main.go" in error
                    for error in errors
                ),
                errors,
            )

    def test_pending_row_naming_an_existing_entrypoint_passes(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            write(tree.root, "examples/encryption/main.go", GO_MAIN)
            entries = dict(tree.entries)
            entries["examples/encryption/main.go"] = {"pending": "T3"}

            self.assertEqual(tree.validate(entries), [])

    def test_pending_row_naming_a_non_entrypoint_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            write(tree.root, "examples/edge/lib/helper.ts", "export const helper = 1;\n")
            entries = dict(tree.entries)
            entries["examples/edge/lib/helper.ts"] = {"pending": "T2"}

            errors = tree.validate(entries)

            self.assertTrue(
                any(
                    "must name a discovered entrypoint" in error
                    and "examples/edge/lib/helper.ts" in error
                    for error in errors
                ),
                errors,
            )

    def test_covered_by_may_reference_a_non_entrypoint_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            write(tree.root, "examples/edge/lib/helper.ts", "export const helper = 1;\n")
            entries = dict(tree.entries)
            entries["examples/edge/lib/helper.ts"] = {
                "coveredBy": "examples/quickstart/main.go",
                "class": "L1",
            }

            self.assertEqual(tree.validate(entries), [])


class ManifestGapTests(unittest.TestCase):
    def test_l3_and_i_rows_require_a_named_gap(self) -> None:
        for class_name in ("L3", "I"):
            with self.subTest(class_name=class_name), tempfile.TemporaryDirectory() as tmp:
                tree = HarnessTree(Path(tmp))
                entries = dict(tree.entries)
                entries["examples/quickstart/main.go"] = {"run": "noop", "class": class_name}

                errors = tree.validate(entries)

                self.assertTrue(
                    any("requires a named gap" in error for error in errors), errors
                )

    def test_l2_row_may_carry_a_fidelity_note(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            entries = dict(tree.entries)
            entries["examples/quickstart/main.go"] = {
                "run": "noop",
                "class": "L2",
                "gap": "KMS is emulated.",
            }

            self.assertEqual(tree.validate(entries), [])

    def test_gap_must_be_a_non_empty_string(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            entries = dict(tree.entries)
            entries["examples/quickstart/main.go"] = {
                "run": "noop",
                "class": "L2",
                "gap": "   ",
            }

            errors = tree.validate(entries)

            self.assertTrue(
                any("gap must be a non-empty string" in error for error in errors), errors
            )

    def test_repository_l3_and_i_rows_all_name_a_gap(self) -> None:
        _, entries = harness.load_manifest(harness.MANIFEST_PATH)

        missing = [
            entry.key
            for entry in entries
            if entry.class_name in ("L3", "I") and not entry.gap
        ]

        self.assertEqual(missing, [])


class SummaryTests(unittest.TestCase):
    def test_summary_prints_named_gaps_for_l2_l3_and_i(self) -> None:
        entries = [
            harness.Entry("examples/local/main.go", "entrypoint", {"class": "L1"}),
            harness.Entry(
                "examples/l2/main.go",
                "entrypoint",
                {"class": "L2", "gap": "KMS is emulated."},
            ),
            harness.Entry(
                "examples/l3/main.go",
                "entrypoint",
                {"class": "L3", "gap": "Managed stream delivery."},
            ),
            harness.Entry(
                "examples/infrastructure/bin/app.ts",
                "entrypoint",
                {"class": "I", "gap": "Infrastructure only."},
            ),
        ]
        results = {entry.key: ("PASS", "real checks") for entry in entries}

        lines = harness.summary_lines(entries, results)

        self.assertIn(" " * harness.SUMMARY_GAP_INDENT + "gap: KMS is emulated.", lines)
        self.assertIn(
            " " * harness.SUMMARY_GAP_INDENT + "gap: Managed stream delivery.", lines
        )
        self.assertIn(
            " " * harness.SUMMARY_GAP_INDENT + "gap: Infrastructure only.", lines
        )
        self.assertNotIn(
            "gap: ",
            "\n".join(line for line in lines if "examples/local/main.go" in line),
        )

    def test_gap_does_not_change_the_result_or_covered_note(self) -> None:
        target = harness.Entry(
            "examples/l3/main.go",
            "entrypoint",
            {"class": "L3", "gap": "Managed stream delivery."},
        )
        covered = harness.Entry(
            "examples/covered.sh",
            "entrypoint",
            {
                "coveredBy": target.key,
                "class": "L3",
                "gap": "Deployed AWS stack smoke.",
            },
        )
        results = {target.key: ("FAIL", "boom"), covered.key: ("FAIL", "boom")}

        lines = harness.summary_lines([target, covered], results)
        text = "\n".join(lines)

        self.assertIn("  FAIL     L3                     examples/l3/main.go  (boom)", text)
        self.assertIn("examples/covered.sh  (via examples/l3/main.go)", text)
        self.assertIn(
            " " * harness.SUMMARY_GAP_INDENT + "gap: Deployed AWS stack smoke.", lines
        )

    def test_every_manifest_gap_is_printed(self) -> None:
        _, entries = harness.load_manifest(harness.MANIFEST_PATH)
        results = {entry.key: ("PASS", "verified") for entry in entries if not entry.pending}

        text = "\n".join(harness.summary_lines(entries, results))

        for entry in entries:
            if entry.gap:
                self.assertIn("gap: " + entry.gap, text)


class ProfileValidationTests(unittest.TestCase):
    def mutate(self, mutate) -> list[str]:
        with tempfile.TemporaryDirectory() as tmp:
            tree = HarnessTree(Path(tmp))
            profile = {
                "name": PROFILE["name"],
                "composeFile": PROFILE["composeFile"],
                "containers": {
                    name: dict(spec) for name, spec in PROFILE["containers"].items()
                },
                "endpoints": {
                    name: {**spec, "env": list(spec["env"])}
                    for name, spec in PROFILE["endpoints"].items()
                },
                "lambda": {
                    "images": {
                        name: dict(spec) for name, spec in PROFILE["lambda"]["images"].items()
                    }
                },
            }
            mutate(profile, tree.root)
            return tree.validate(profile=profile)

    def test_tag_only_image_fails_digest_pinning(self) -> None:
        def mutate(profile, root):
            profile["containers"]["moto"]["image"] = "motoserver/moto:5.2.5"

        errors = self.mutate(mutate)
        self.assertTrue(
            any("pinned by digest" in error and "moto" in error for error in errors), errors
        )

    def test_tag_only_lambda_image_fails_digest_pinning(self) -> None:
        def mutate(profile, root):
            profile["lambda"]["images"]["node"]["digest"] = ""

        errors = self.mutate(mutate)
        self.assertTrue(
            any("pinned by digest" in error and "lambda.images.node" in error for error in errors),
            errors,
        )

    def test_lambda_runtime_pin_cannot_be_dropped(self) -> None:
        def mutate(profile, root):
            del profile["lambda"]["images"]["python"]

        errors = self.mutate(mutate)
        self.assertTrue(
            any("lambda.images.python is required" in error for error in errors), errors
        )

    def test_lambda_pin_records_the_date(self) -> None:
        def mutate(profile, root):
            profile["lambda"]["images"]["go"]["pinned"] = "whenever"

        errors = self.mutate(mutate)
        self.assertTrue(any("ISO date" in error and "go" in error for error in errors), errors)

    def test_lambda_tag_must_name_a_tag(self) -> None:
        def mutate(profile, root):
            profile["lambda"]["images"]["go"]["tag"] = "public.ecr.aws/lambda/provided"

        errors = self.mutate(mutate)
        self.assertTrue(
            any("tagged image reference" in error and "go" in error for error in errors), errors
        )

    def test_fixed_host_port_fails_ephemeral_isolation(self) -> None:
        def mutate(profile, root):
            write(
                root,
                "scripts/local/docker-compose.yml",
                COMPOSE.replace("127.0.0.1::8000", "127.0.0.1:8123:8000"),
            )

        errors = self.mutate(mutate)
        self.assertTrue(
            any("ephemeral loopback port" in error and "dynamodb-local" in error for error in errors),
            errors,
        )

    def test_fixed_container_name_fails(self) -> None:
        def mutate(profile, root):
            write(
                root,
                "scripts/local/docker-compose.yml",
                COMPOSE.replace("  moto:\n", "  moto:\n    container_name: tabletheory-moto\n"),
            )

        errors = self.mutate(mutate)
        self.assertTrue(any("container_name is fixed" in error for error in errors), errors)

    def test_fixed_compose_project_name_fails(self) -> None:
        def mutate(profile, root):
            write(root, "scripts/local/docker-compose.yml", "name: fixed\n" + COMPOSE)

        errors = self.mutate(mutate)
        self.assertTrue(any("top-level name fixes" in error for error in errors), errors)

    def test_missing_standard_endpoint_override_fails(self) -> None:
        def mutate(profile, root):
            profile["endpoints"]["kms"]["env"] = ["KMS_ENDPOINT"]

        errors = self.mutate(mutate)
        self.assertTrue(
            any("AWS_ENDPOINT_URL_KMS" in error for error in errors), errors
        )

    def test_missing_endpoint_service_fails(self) -> None:
        def mutate(profile, root):
            del profile["endpoints"]["sqs"]

        errors = self.mutate(mutate)
        self.assertTrue(any("endpoints.sqs is required" in error for error in errors), errors)

    def test_endpoint_variable_cannot_be_shared(self) -> None:
        def mutate(profile, root):
            profile["endpoints"]["sts"]["env"] = ["AWS_ENDPOINT_URL_KMS"]

        errors = self.mutate(mutate)
        self.assertTrue(any("used by both" in error for error in errors), errors)


class LambdaImagePinTests(unittest.TestCase):
    """Lambda runtime images must be digest-pinned in one place and nowhere else.

    A moving tag in the runtime module is exactly the supply-chain hole these
    tests close: the image reference may only come from the manifest, and the
    runtime must refuse a pin that is not ``tag@sha256:...``.
    """

    def profile(self) -> object:
        return harness.build_profile(PROFILE, Path("/tmp/checkout"), "run-1")

    def runtime(self, profile) -> lambda_runtime.LambdaRuntime:
        return lambda_runtime.LambdaRuntime(
            harness.Harness(env={}, ports={}), profile, Path("/tmp/scratch")
        )

    def test_runtime_module_names_no_image_literal(self) -> None:
        source = (HARNESS_DIR / "lambda_runtime.py").read_text(encoding="utf-8")
        literals = re.findall(
            r"public\.ecr\.aws/lambda/[a-z0-9-]+:[a-z0-9.]+(?:@sha256:[0-9a-f]{64})?", source
        )
        self.assertEqual(literals, [])
        for marker in ("al2023", "nodejs:24", "python:3.14"):
            self.assertNotIn(
                marker, source, f"lambda_runtime.py must not name an image literal: {marker!r}"
            )

    def test_runtime_rejects_a_tag_only_pin(self) -> None:
        profile = self.profile()
        profile.lambda_images["node"] = "public.ecr.aws/lambda/nodejs:24"
        with self.assertRaises(lambda_runtime.LambdaFailure):
            self.runtime(profile).image_for("node")

    def test_runtime_rejects_a_missing_pin(self) -> None:
        profile = self.profile()
        del profile.lambda_images["python"]
        with self.assertRaises(lambda_runtime.LambdaFailure):
            self.runtime(profile).image_for("python")

    def test_every_runtime_starts_the_pinned_image(self) -> None:
        profile = self.profile()
        runtime = self.runtime(profile)
        runtime.build_go = lambda name, package, cwd=None: Path("/tmp/task")
        runtime.build_node = lambda name, entry: Path("/tmp/task")
        runtime.build_python = lambda name, handler, requirements=None: Path("/tmp/task")
        images: dict[str, str] = {}

        def fake_start(name, image, task_dir, handler_arg, extra_env):
            images[name] = image

        runtime._start = fake_start
        runtime.start_go("go-row", "./x")
        runtime.start_node("node-row", Path("/tmp/handler.ts"))
        runtime.start_python("python-row", Path("/tmp/handler.py"))

        self.assertEqual(
            images["go-row"], f"public.ecr.aws/lambda/provided:al2023@sha256:{DIGEST_E}"
        )
        self.assertEqual(images["node-row"], f"public.ecr.aws/lambda/nodejs:24@sha256:{DIGEST_F}")
        self.assertEqual(
            images["python-row"], f"public.ecr.aws/lambda/python:3.14@sha256:{DIGEST_0}"
        )

    def test_documented_pins_match_the_manifest(self) -> None:
        profile, _ = harness.load_manifest(harness.MANIFEST_PATH)
        document = (harness.ROOT / "docs/examples/local-harness.md").read_text(encoding="utf-8")
        for runtime, spec in profile["lambda"]["images"].items():
            reference = f"{spec['tag']}@{spec['digest']}"
            rows = [line for line in document.splitlines() if reference in line]
            self.assertTrue(
                rows, f"docs/examples/local-harness.md does not document {runtime}: {reference}"
            )
            self.assertTrue(
                any(spec["pinned"] in line for line in rows),
                f"docs/examples/local-harness.md does not record the pin date {spec['pinned']} for {runtime}",
            )


class EnvironmentTests(unittest.TestCase):
    def test_build_env_clears_proxy_and_aws_variables(self) -> None:
        injected = {
            "HTTP_PROXY": "http://proxy.invalid:3128",
            "http_proxy": "http://proxy.invalid:3128",
            "HTTPS_PROXY": "http://proxy.invalid:3128",
            "https_proxy": "http://proxy.invalid:3128",
            "ALL_PROXY": "socks5://proxy.invalid:1080",
            "all_proxy": "socks5://proxy.invalid:1080",
            "NO_PROXY": "example.test",
            "no_proxy": "example.test",
            "AWS_PROFILE": "production",
            "AWS_ACCESS_KEY_ID": "AKIAREALCREDENTIAL",
            "AWS_ENDPOINT_URL": "https://dynamodb.us-east-1.amazonaws.com",
            "AWS_ENDPOINT_URL_KMS": "https://kms.us-east-1.amazonaws.com",
            "AWS_ENDPOINT_URL_SQS": "https://sqs.us-east-1.amazonaws.com",
            "JWT_SECRET": "leaked",
            "KMS_KEY_ARN": "arn:aws:kms:us-east-1:1:key/leaked",
        }

        profile = harness.build_profile(PROFILE, Path("/tmp/checkout"), "run-1")
        profile.host_ports = {
            "dynamodb-local": 9001,
            "moto": 9002,
            "s3proxy": 9003,
            "elasticmq": 9004,
        }
        with unittest.mock.patch.dict(os.environ, injected):
            env = harness.build_env(profile)

        cleared = set(injected) - {
            "AWS_ACCESS_KEY_ID",
            "AWS_ENDPOINT_URL",
            "AWS_ENDPOINT_URL_KMS",
            "AWS_ENDPOINT_URL_SQS",
        }
        for name in sorted(cleared):
            self.assertNotIn(name, env)
        self.assertEqual(env["AWS_ACCESS_KEY_ID"], "dummy")
        self.assertEqual(env["AWS_SECRET_ACCESS_KEY"], "dummy")
        self.assertEqual(env["AWS_ENDPOINT_URL"], "http://127.0.0.1:9001")
        self.assertEqual(env["AWS_ENDPOINT_URL_DYNAMODB"], "http://127.0.0.1:9001")
        self.assertEqual(env["DYNAMODB_ENDPOINT"], "http://127.0.0.1:9001")
        self.assertEqual(env["AWS_ENDPOINT_URL_KMS"], "http://127.0.0.1:9002")
        self.assertEqual(env["KMS_ENDPOINT"], "http://127.0.0.1:9002")
        self.assertEqual(env["AWS_ENDPOINT_URL_STS"], "http://127.0.0.1:9002")
        self.assertEqual(env["AWS_ENDPOINT_URL_S3"], "http://127.0.0.1:9003")
        self.assertEqual(env["AWS_ENDPOINT_URL_SQS"], "http://127.0.0.1:9004")

    def test_kms_and_sts_share_the_moto_endpoint(self) -> None:
        profile = harness.build_profile(PROFILE, Path("/tmp/checkout"), "run-1")
        profile.host_ports = {"moto": 9002, "dynamodb-local": 9001, "s3proxy": 9003, "elasticmq": 9004}
        self.assertEqual(profile.url("kms"), profile.url("sts"))


class ProjectIsolationTests(unittest.TestCase):
    def test_project_name_is_unique_per_checkout_and_run(self) -> None:
        a = harness.project_name("tabletheory-examples-local", Path("/tmp/checkout-a"), "run-1")
        b = harness.project_name("tabletheory-examples-local", Path("/tmp/checkout-b"), "run-1")
        c = harness.project_name("tabletheory-examples-local", Path("/tmp/checkout-a"), "run-2")
        d = harness.project_name("tabletheory-examples-local", Path("/tmp/checkout-a"), "run-1")

        self.assertNotEqual(a, b)
        self.assertNotEqual(a, c)
        self.assertEqual(a, d)
        self.assertTrue(a.startswith("tabletheory-examples-local-"))


class ConcurrencyIsolationTests(unittest.TestCase):
    """Two harness instances must not recreate, remove or share each other.

    This is the regression test for the fixed-project/fixed-port failure: run B
    starts while run A is live, and A's container id, endpoint and database must
    survive B's startup and B's teardown.
    """

    def setUp(self) -> None:
        probe = harness.Harness(env=dict(os.environ), ports={})
        if not harness._docker_available(probe):
            self.fail(
                "Docker is required to prove harness concurrency isolation; start Colima/Docker first"
            )
        raw, _ = harness.load_manifest(harness.MANIFEST_PATH)
        self.profile_a = harness.build_profile(raw, harness.ROOT, "selftest-a")
        self.profile_b = harness.build_profile(raw, harness.ROOT, "selftest-b")
        self.harness_a = harness.Harness(env=dict(os.environ), ports={})
        self.harness_b = harness.Harness(env=dict(os.environ), ports={})

    def tearDown(self) -> None:
        for hs, profile in ((self.harness_a, self.profile_a), (self.harness_b, self.profile_b)):
            harness.stop_profile(hs, profile)

    def test_two_concurrent_profiles_do_not_collide(self) -> None:
        self.assertNotEqual(self.profile_a.project, self.profile_b.project)

        harness.start_profile(self.harness_a, self.profile_a)
        container_a = harness.container_id(self.harness_a, self.profile_a, "dynamodb-local")
        self.assertNotEqual(container_a, "")
        harness.wait_for_dynamodb_local(self.harness_a, self.profile_a.url("dynamodb"))

        harness.start_profile(self.harness_b, self.profile_b)
        self.assertNotEqual(self.profile_a.url("dynamodb"), self.profile_b.url("dynamodb"))
        self.assertNotEqual(self.profile_a.url("kms"), self.profile_b.url("kms"))

        # B's startup must not have recreated A's container.
        self.assertEqual(
            harness.container_id(self.harness_a, self.profile_a, "dynamodb-local"), container_a
        )
        harness.wait_for_dynamodb_local(self.harness_a, self.profile_a.url("dynamodb"))

        # B's teardown must not remove A's container or network.
        harness.stop_profile(self.harness_b, self.profile_b)
        self.assertEqual(
            harness.compose(self.harness_b, self.profile_b, "ps", "-q", check=False).stdout.strip(),
            "",
        )
        self.assertEqual(
            harness.container_id(self.harness_a, self.profile_a, "dynamodb-local"), container_a
        )
        harness.wait_for_dynamodb_local(self.harness_a, self.profile_a.url("dynamodb"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
