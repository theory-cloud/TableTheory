#!/usr/bin/env bash
set -euo pipefail

workflow=".github/workflows/release-pr.yml"
tool_dir="scripts/release-please-cli"

if grep -Fq "npx --yes" "${workflow}"; then
  echo "release-pr-tool-policy-test: release-pr workflow must not run npx --yes in the privileged release token path"
  exit 1
fi

grep -Fq "npm ci --prefix ${tool_dir} --ignore-scripts" "${workflow}" || {
  echo "release-pr-tool-policy-test: release-pr workflow must install the locked CLI with npm ci --ignore-scripts"
  exit 1
}

grep -Fq "${tool_dir}/node_modules/.bin/release-please" "${workflow}" || {
  echo "release-pr-tool-policy-test: release-pr workflow must execute the locked local release-please binary"
  exit 1
}

python3 - <<'PY'
from pathlib import Path

workflow = Path(".github/workflows/release-pr.yml").read_text(encoding="utf-8")
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

if "release-pr: strict stable state; no single-manifest RC to normalize" not in workflow:
    raise SystemExit(
        "release-pr-tool-policy-test: strict stable main state must no-op successfully"
    )
PY

node <<'NODE'
const fs = require('node:fs');

const pkg = JSON.parse(fs.readFileSync('scripts/release-please-cli/package.json', 'utf8'));
if (pkg.private !== true) {
  throw new Error('release-please CLI tool package must remain private');
}
if (pkg.dependencies?.['release-please'] !== '17.3.0') {
  throw new Error(`release-please dependency must be exactly 17.3.0, got ${pkg.dependencies?.['release-please'] ?? 'missing'}`);
}

const lock = JSON.parse(fs.readFileSync('scripts/release-please-cli/package-lock.json', 'utf8'));
const rootVersion = lock.packages?.['']?.dependencies?.['release-please'];
if (rootVersion !== '17.3.0') {
  throw new Error(`lockfile root dependency must be release-please 17.3.0, got ${rootVersion ?? 'missing'}`);
}

const locked = lock.packages?.['node_modules/release-please'];
if (locked?.version !== '17.3.0') {
  throw new Error(`lockfile must pin node_modules/release-please to 17.3.0, got ${locked?.version ?? 'missing'}`);
}
if (typeof locked.integrity !== 'string' || !locked.integrity.startsWith('sha512-')) {
  throw new Error('lockfile release-please entry must include sha512 integrity');
}
NODE

echo "release-pr-tool-policy-test: PASS"
