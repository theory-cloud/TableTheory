#!/usr/bin/env bash
set -euo pipefail

workflow=".github/workflows/release-pr.yml"
generator="scripts/create-stable-release-pr.py"
retired_tool_dir="scripts/release-please-cli"

if [[ -e "${retired_tool_dir}" ]]; then
  echo "release-pr-tool-policy-test: retired release-please CLI wrapper must not exist"
  exit 1
fi

if grep -Fq "npx --yes" "${workflow}"; then
  echo "release-pr-tool-policy-test: release-pr workflow must not run npx --yes in the privileged release token path"
  exit 1
fi

if grep -Fq "${retired_tool_dir}" "${workflow}"; then
  echo "release-pr-tool-policy-test: release-pr workflow must not use the retired release-please CLI wrapper"
  exit 1
fi

grep -Fq "python3 ${generator}" "${workflow}" || {
  echo "release-pr-tool-policy-test: release-pr workflow must invoke the deterministic stable Release PR generator"
  exit 1
}

grep -Fq -- "--head release-please--branches--main" "${workflow}" || {
  echo "release-pr-tool-policy-test: release-pr workflow must use the canonical stable Release PR branch"
  exit 1
}

python3 - "${workflow}" "${generator}" <<'PY'
import importlib.util
import sys
from pathlib import Path

workflow_path = Path(sys.argv[1])
generator_path = Path(sys.argv[2])
workflow = workflow_path.read_text(encoding="utf-8")
lines = workflow.splitlines()

try:
    step_index = next(
        index
        for index, line in enumerate(lines)
        if line.strip() == "- name: Compute release-as (normalize single-manifest RC)"
    )
except StopIteration as exc:
    raise SystemExit(
        "release-pr-tool-policy-test: release-pr workflow must keep the release-as compute step"
    ) from exc

step_header = "\n".join(lines[step_index : step_index + 5])
if "if: steps.cycle.outputs.pending_stable_promotion == 'true'" not in step_header:
    raise SystemExit(
        "release-pr-tool-policy-test: release-as compute step must be gated to pending stable promotion"
    )

if "steps.cycle.outputs.pending_stable_promotion == 'true'" not in workflow:
    raise SystemExit(
        "release-pr-tool-policy-test: stable Release PR generation must be gated to pending stable promotion"
    )

spec = importlib.util.spec_from_file_location("stable_release_pr", generator_path)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(module)

if module.PENDING_RELEASE_LABEL != "autorelease: pending":
    raise SystemExit("release-pr-tool-policy-test: generator must apply the Release Please pending label")

body = module.stable_pull_request_body(
    "theory-cloud/TableTheory",
    "2.0.1-rc.1",
    "2.0.1",
    "2026-07-07",
)
if "\n---\n\n\n## [2.0.1]" not in body:
    raise SystemExit("release-pr-tool-policy-test: generator body must be Release Please-compatible")
if "## [2.0.1-rc" in body:
    raise SystemExit("release-pr-tool-policy-test: generator body must not advertise an RC release")

generator_text = generator_path.read_text(encoding="utf-8")
required = [
    "--force-with-lease=refs/heads/",
    "display_args",
    "ls-remote",
]
for needle in required:
    if needle not in generator_text:
        raise SystemExit(f"release-pr-tool-policy-test: generator missing {needle!r}")
PY

echo "release-pr-tool-policy-test: PASS"
