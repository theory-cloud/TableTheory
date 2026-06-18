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
