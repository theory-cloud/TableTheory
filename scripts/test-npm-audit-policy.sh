#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

finding="NPM-AUDIT:examples/cdk-multilang:brace-expansion:GHSA-jxxr-4gwj-5jf2:node_modules/aws-cdk-lib/node_modules/brace-expansion"
checker="scripts/check-npm-audit-allowlist.mjs"

# Keep negative policy assertions deterministic when the SEC-2 CI job enables
# reviewed visible findings for the real scanner path.
unset NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS

cat >"${tmpdir}/audit.json" <<'JSON'
{
  "auditReportVersion": 2,
  "vulnerabilities": {
    "brace-expansion": {
      "name": "brace-expansion",
      "severity": "moderate",
      "via": [
        {
          "source": 1234567,
          "name": "brace-expansion",
          "dependency": "brace-expansion",
          "title": "brace-expansion: Large numeric range defeats documented max DoS protection",
          "url": "https://github.com/advisories/GHSA-jxxr-4gwj-5jf2"
        }
      ],
      "effects": [],
      "range": "5.0.2 - 5.0.5",
      "nodes": [
        "node_modules/aws-cdk-lib/node_modules/brace-expansion"
      ],
      "fixAvailable": true
    }
  }
}
JSON

cat >"${tmpdir}/visible.json" <<'JSON'
{
  "findings": [
    {
      "project": "examples/cdk-multilang",
      "package": "brace-expansion",
      "advisory": "GHSA-jxxr-4gwj-5jf2",
      "node": "node_modules/aws-cdk-lib/node_modules/brace-expansion",
      "visibility": "visible",
      "status": "upstream-bundled-pending-fix",
      "reviewed_on": "2026-06-18",
      "expires_on": "2099-12-31",
      "remove_when": "Remove when aws-cdk-lib bundles a fixed brace-expansion.",
      "justification": "Synthetic test fixture for visible npm audit policy handling."
    }
  ]
}
JSON

cp "${tmpdir}/visible.json" "${tmpdir}/expired-visible.json"
node - "${tmpdir}/expired-visible.json" <<'NODE'
const fs = require('node:fs');
const path = process.argv[2];
const policy = JSON.parse(fs.readFileSync(path, 'utf8'));
policy.findings[0].expires_on = '2000-01-01';
fs.writeFileSync(path, `${JSON.stringify(policy, null, 2)}\n`);
NODE

: >"${tmpdir}/empty-allowlist.txt"

expect_success_contains() {
  local expected="$1"
  shift

  local output
  if ! output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "npm-audit-policy-test: expected command to succeed"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "npm-audit-policy-test: expected output to contain: ${expected}"
    exit 1
  fi
}

expect_failure_contains() {
  local expected="$1"
  shift

  local output
  if output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "npm-audit-policy-test: expected command to fail"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "npm-audit-policy-test: expected failure output to contain: ${expected}"
    exit 1
  fi
}

expect_failure_contains \
  "visible unallowlisted finding(s)" \
  node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang "${tmpdir}/visible.json"

expect_success_contains \
  "visible unallowlisted finding(s)" \
  env NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS=true node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang "${tmpdir}/visible.json"

expect_failure_contains \
  "expires_on 2000-01-01 is expired" \
  env NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS=true node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang "${tmpdir}/expired-visible.json"

expect_failure_contains \
  "unallowlisted finding(s)" \
  node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang

printf '%s\n' "${finding}" >"${tmpdir}/allowlist.txt"
expect_success_contains \
  "allowlisted finding(s)" \
  node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/allowlist.txt" examples/cdk-multilang

expect_failure_contains \
  "visible finding(s) cannot also be in the suppression allowlist" \
  node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/allowlist.txt" examples/cdk-multilang "${tmpdir}/visible.json"

if grep -Fxq "${finding}" gov-infra/planning/theorydb-supply-chain-allowlist.txt; then
  echo "npm-audit-policy-test: THE-1757 brace-expansion finding must not be in the suppression allowlist"
  exit 1
fi

# Checker transport-failure negatives: a truncated report, a registry failure
# object, and an empty-but-parseable report must each fail closed instead of
# being read as "no findings".
cat >"${tmpdir}/malformed-audit.json" <<'JSON'
{ "auditReportVersion": 2, "vulnerabilities":
JSON
expect_failure_contains \
  "invalid JSON report" \
  node "${checker}" "${tmpdir}/malformed-audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang

cat >"${tmpdir}/transport-failure.json" <<'JSON'
{
  "error": {
    "code": "EAI_AGAIN",
    "summary": "request to https://registry.npmjs.org/-/npm/v1/security/audits/quick failed, reason: getaddrinfo EAI_AGAIN registry.npmjs.org"
  }
}
JSON
expect_failure_contains \
  "invalid audit report" \
  node "${checker}" "${tmpdir}/transport-failure.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang

cat >"${tmpdir}/empty-report.json" <<'JSON'
{ "auditReportVersion": 2, "vulnerabilities": {} }
JSON
expect_failure_contains \
  "reported no vulnerability findings" \
  node "${checker}" "${tmpdir}/empty-report.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang

# Positive fixed-graph guard for the retired AWS-bundled brace-expansion exception.
# examples/cdk-multilang now pins aws-cdk-lib 2.275.0, which bundles brace-expansion
# 5.0.12 (the fixed version for GHSA-q2hr-2g5m-vwhr / qhr7-859c-m2p7 / 6j4f-fj2g-mc7p).
# This guard keeps that state honest: it asserts the actual bundled graph, proves the
# bundled minimatch still accepts the bundled brace-expansion, refuses any vulnerable
# brace-expansion anywhere in the lockfile, and refuses any residual visible policy
# entry referencing the retired exception. It fails closed on every malformed input.
# It replaces the earlier pinned-vulnerable (5.0.9) coherence anchor: that anchor only
# existed to keep the retired exception honest while aws-cdk-lib was still vulnerable.
cat >"${tmpdir}/verify-bundled-brace-graph.mjs" <<'NODE'
import fs from 'node:fs';

const [lockPath, policyPath, label] = process.argv.slice(2);
if (!lockPath || !policyPath) {
  console.error('verify-bundled-brace-graph: usage: <lockfile> <policy> [label]');
  process.exit(2);
}
const tag = label ? `${label}: ` : '';

function fail(message) {
  console.error(`verify-bundled-brace-graph: ${tag}${message}`);
  process.exit(1);
}

function versionAtLeast(actual, minimum) {
  if (typeof actual !== 'string') return false;
  const actualParts = actual.split('.').map((part) => Number(part));
  const minimumParts = minimum.split('.').map((part) => Number(part));
  if (actualParts.length !== 3 || minimumParts.length !== 3 || actualParts.some(Number.isNaN)) {
    return false;
  }
  for (let i = 0; i < 3; i += 1) {
    if (actualParts[i] > minimumParts[i]) return true;
    if (actualParts[i] < minimumParts[i]) return false;
  }
  return true;
}

// Minimal declared-range check (no external semver dependency). Supports exactly
// five shapes npm writes into a bundled graph: caret ^X.Y.Z, tilde ~X.Y.Z,
// >=X.Y.Z, strict >X.Y.Z, and a bare exact X.Y.Z (string equality). Every other
// shape (including `<=`/`<`, `=`, compound, hyphen, `||`, or `*` ranges) fails
// closed instead of silently passing.
function satisfiesRange(actual, range) {
  if (typeof actual !== 'string' || typeof range !== 'string') return false;
  const trimmed = range.trim();

  const caret = trimmed.match(/^\^(\d+)\.(\d+)\.(\d+)$/);
  if (caret) {
    const [major, minor, patch] = caret.slice(1).map(Number);
    if (!versionAtLeast(actual, `${major}.${minor}.${patch}`)) return false;
    if (major > 0) return !versionAtLeast(actual, `${major + 1}.0.0`);
    if (minor > 0) return !versionAtLeast(actual, `0.${minor + 1}.0`);
    return !versionAtLeast(actual, `0.0.${patch + 1}`);
  }

  const tilde = trimmed.match(/^~(\d+)\.(\d+)\.(\d+)$/);
  if (tilde) {
    const [major, minor, patch] = tilde.slice(1).map(Number);
    if (!versionAtLeast(actual, `${major}.${minor}.${patch}`)) return false;
    return !versionAtLeast(actual, `${major}.${minor + 1}.0`);
  }

  if (/^\d+\.\d+\.\d+$/.test(trimmed)) return actual === trimmed;

  const comparator = trimmed.match(/^(>=|>)\s*(\d+\.\d+\.\d+)$/);
  if (comparator) {
    const [, operator, floor] = comparator;
    if (!versionAtLeast(actual, floor)) return false;
    if (operator === '>') return !versionAtLeast(floor, actual);
    return true;
  }

  fail(`unsupported declared range "${range}"`);
}

function readJson(path, description) {
  let raw;
  try {
    raw = fs.readFileSync(path, 'utf8');
  } catch (err) {
    fail(`${description} not readable: ${err instanceof Error ? err.message : String(err)}`);
  }
  try {
    return JSON.parse(raw);
  } catch (err) {
    fail(`${description} is not valid JSON: ${err instanceof Error ? err.message : String(err)}`);
  }
}

const lock = readJson(lockPath, 'lockfile');
const policy = readJson(policyPath, 'visible policy');

const packages = lock && typeof lock === 'object' && !Array.isArray(lock) ? lock.packages : null;
if (!packages || typeof packages !== 'object' || Array.isArray(packages)) {
  fail('lockfile has no packages map');
}

const CDK_LIB_PATH = 'node_modules/aws-cdk-lib';
const BRACE_PATH = 'node_modules/aws-cdk-lib/node_modules/brace-expansion';
const MINIMATCH_PATH = 'node_modules/aws-cdk-lib/node_modules/minimatch';
const CDK_LIB_FLOOR = '2.275.0';
const BRACE_FLOOR = '5.0.12';
const RETIRED_ADVISORIES = ['GHSA-q2hr-2g5m-vwhr', 'GHSA-qhr7-859c-m2p7', 'GHSA-6j4f-fj2g-mc7p'];

if (!BRACE_PATH.startsWith(`${CDK_LIB_PATH}/node_modules/`)) fail('internal: unexpected bundled brace path');
if (!MINIMATCH_PATH.startsWith(`${CDK_LIB_PATH}/node_modules/`)) fail('internal: unexpected bundled minimatch path');

function requireEntry(path) {
  const entry = packages[path];
  if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
    fail(`required lock entry ${path} is missing`);
  }
  return entry;
}

const cdkLib = requireEntry(CDK_LIB_PATH);
if (!versionAtLeast(cdkLib.version, CDK_LIB_FLOOR)) {
  fail(`aws-cdk-lib lock entry must be >= ${CDK_LIB_FLOOR}, got ${cdkLib.version ?? 'missing'}`);
}
if (typeof cdkLib.resolved !== 'string' || cdkLib.resolved.trim() === '') {
  fail('aws-cdk-lib lock entry is missing resolved');
}
if (typeof cdkLib.integrity !== 'string' || !/^sha512-/.test(cdkLib.integrity)) {
  fail('aws-cdk-lib lock entry is missing sha512 integrity');
}

const brace = requireEntry(BRACE_PATH);
if (brace.inBundle !== true) {
  fail(`${BRACE_PATH} must be inBundle:true`);
}
if (!versionAtLeast(brace.version, BRACE_FLOOR)) {
  fail(`bundled brace-expansion must be >= ${BRACE_FLOOR}, got ${brace.version ?? 'missing'}`);
}

const minimatch = requireEntry(MINIMATCH_PATH);
if (minimatch.inBundle !== true) {
  fail(`${MINIMATCH_PATH} must be inBundle:true`);
}
const declaredRange = minimatch.dependencies?.['brace-expansion'];
if (typeof declaredRange !== 'string' || declaredRange.trim() === '') {
  fail('bundled minimatch must declare a brace-expansion range');
}
if (!satisfiesRange(brace.version, declaredRange)) {
  fail(`bundled brace-expansion ${brace.version} does not satisfy bundled minimatch range ${declaredRange}`);
}

// No vulnerable brace-expansion may exist anywhere in the lockfile.
for (const [path, entry] of Object.entries(packages)) {
  if (!/(^|\/)brace-expansion$/.test(path)) continue;
  if (!versionAtLeast(entry?.version, BRACE_FLOOR)) {
    fail(`vulnerable brace-expansion ${entry?.version ?? 'missing'} at ${path}`);
  }
}

// Zero residual visible policy entries may reference the retired exception.
const findings = Array.isArray(policy?.findings) ? policy.findings : null;
if (!findings) {
  fail('visible policy is missing a findings array');
}
const residual = findings.filter((finding) => {
  if (!finding || typeof finding !== 'object' || Array.isArray(finding)) return false;
  const node = typeof finding.node === 'string' ? finding.node : '';
  const referencesBundledNode = node.includes('aws-cdk-lib/node_modules/brace-expansion');
  const isRetiredAdvisory = RETIRED_ADVISORIES.includes(finding.advisory);
  const isExampleBraceExpansion =
    finding.project === 'examples/cdk-multilang' && finding.package === 'brace-expansion';
  return referencesBundledNode || isRetiredAdvisory || isExampleBraceExpansion;
});
if (residual.length > 0) {
  fail(`${residual.length} residual visible policy entry(ies) still reference the retired examples/cdk-multilang brace-expansion exception`);
}

console.log(
  `verify-bundled-brace-graph: ${tag}PASS (aws-cdk-lib ${cdkLib.version}, bundled brace-expansion ${brace.version} via bundled minimatch ${minimatch.version} ${declaredRange})`,
);
NODE

cat >"${tmpdir}/make-bundled-brace-fixtures.mjs" <<'NODE'
import fs from 'node:fs';

const [, , lockPath, policyPath, outDir] = process.argv;
const lock = JSON.parse(fs.readFileSync(lockPath, 'utf8'));
const policy = JSON.parse(fs.readFileSync(policyPath, 'utf8'));
const clone = (value) => JSON.parse(JSON.stringify(value));

const CDK_LIB_PATH = 'node_modules/aws-cdk-lib';
const BRACE_PATH = 'node_modules/aws-cdk-lib/node_modules/brace-expansion';
const MINIMATCH_PATH = 'node_modules/aws-cdk-lib/node_modules/minimatch';

function writeJson(name, value) {
  fs.writeFileSync(`${outDir}/${name}`, `${JSON.stringify(value, null, 2)}\n`);
}

function visibleEntry(overrides) {
  return {
    project: 'examples/cdk-multilang',
    package: 'brace-expansion',
    advisory: 'GHSA-q2hr-2g5m-vwhr',
    node: BRACE_PATH,
    visibility: 'visible',
    status: 'upstream-bundled-pending-fix',
    reviewed_on: '2026-10-03',
    expires_on: '2099-12-31',
    remove_when: 'Remove when aws-cdk-lib bundles brace-expansion >= 5.0.12.',
    justification: 'Synthetic negative-control fixture for the bundled brace-expansion guard.',
    ...overrides,
  };
}

{
  const mutated = clone(lock);
  mutated.packages[BRACE_PATH].version = '5.0.9';
  writeJson('lock-old-5.0.9.json', mutated);
}
{
  const mutated = clone(lock);
  delete mutated.packages[BRACE_PATH];
  writeJson('lock-missing-brace.json', mutated);
}
{
  const mutated = clone(lock);
  delete mutated.packages[CDK_LIB_PATH];
  writeJson('lock-missing-cdk.json', mutated);
}
{
  const mutated = clone(lock);
  delete mutated.packages[CDK_LIB_PATH].integrity;
  writeJson('lock-no-integrity.json', mutated);
}
{
  const mutated = clone(lock);
  delete mutated.packages[CDK_LIB_PATH].resolved;
  writeJson('lock-no-resolved.json', mutated);
}
{
  const mutated = clone(lock);
  delete mutated.packages[MINIMATCH_PATH].inBundle;
  writeJson('lock-minimatch-not-bundled.json', mutated);
}
{
  const mutated = clone(lock);
  delete mutated.packages[BRACE_PATH].inBundle;
  writeJson('lock-brace-not-bundled.json', mutated);
}
{
  const mutated = clone(lock);
  mutated.packages[MINIMATCH_PATH].dependencies['brace-expansion'] = '^6.0.0';
  writeJson('lock-range-mismatch.json', mutated);
}
{
  const mutated = clone(lock);
  mutated.packages[MINIMATCH_PATH].dependencies['brace-expansion'] = '5.0.11';
  writeJson('lock-exact-pin-higher-actual.json', mutated);
}
{
  const mutated = clone(lock);
  mutated.packages[MINIMATCH_PATH].dependencies['brace-expansion'] = '>5.0.12';
  writeJson('lock-strict-gt-equal-actual.json', mutated);
}
{
  const mutated = clone(lock);
  mutated.packages['node_modules/brace-expansion'] = { version: '5.0.9', license: 'MIT' };
  writeJson('lock-root-vuln.json', mutated);
}
fs.writeFileSync(`${outDir}/lock-malformed.json`, '{\n  "packages": {\n');

writeJson('policy-bundled-entry.json', { ...policy, findings: [visibleEntry({})] });
writeJson('policy-wrong-package.json', {
  ...policy,
  findings: [
    visibleEntry({ package: 'minimatch', advisory: 'GHSA-qhr7-859c-m2p7', node: MINIMATCH_PATH }),
  ],
});
writeJson('policy-wrong-node.json', {
  ...policy,
  findings: [visibleEntry({ advisory: 'GHSA-aaaa-bbbb-cccc', node: 'node_modules/brace-expansion' })],
});
fs.writeFileSync(`${outDir}/policy-malformed.json`, '{ "findings": [ }\n');
NODE

graph_lock="examples/cdk-multilang/package-lock.json"
graph_policy="gov-infra/planning/theorydb-visible-npm-audit-findings.json"
graph_checker="${tmpdir}/verify-bundled-brace-graph.mjs"

# Positive: the committed fixed graph must satisfy the guard.
expect_success_contains \
  "verify-bundled-brace-graph: committed: PASS" \
  node "${graph_checker}" "${graph_lock}" "${graph_policy}" committed

node "${tmpdir}/make-bundled-brace-fixtures.mjs" "${graph_lock}" "${graph_policy}" "${tmpdir}"

# Negative: the guard must fail closed on every drift and malformed input.
expect_failure_contains \
  "bundled brace-expansion must be >= 5.0.12" \
  node "${graph_checker}" "${tmpdir}/lock-old-5.0.9.json" "${graph_policy}" negative
expect_failure_contains \
  "required lock entry node_modules/aws-cdk-lib/node_modules/brace-expansion is missing" \
  node "${graph_checker}" "${tmpdir}/lock-missing-brace.json" "${graph_policy}" negative
expect_failure_contains \
  "required lock entry node_modules/aws-cdk-lib is missing" \
  node "${graph_checker}" "${tmpdir}/lock-missing-cdk.json" "${graph_policy}" negative
expect_failure_contains \
  "aws-cdk-lib lock entry is missing sha512 integrity" \
  node "${graph_checker}" "${tmpdir}/lock-no-integrity.json" "${graph_policy}" negative
expect_failure_contains \
  "aws-cdk-lib lock entry is missing resolved" \
  node "${graph_checker}" "${tmpdir}/lock-no-resolved.json" "${graph_policy}" negative
expect_failure_contains \
  "node_modules/aws-cdk-lib/node_modules/minimatch must be inBundle:true" \
  node "${graph_checker}" "${tmpdir}/lock-minimatch-not-bundled.json" "${graph_policy}" negative
expect_failure_contains \
  "node_modules/aws-cdk-lib/node_modules/brace-expansion must be inBundle:true" \
  node "${graph_checker}" "${tmpdir}/lock-brace-not-bundled.json" "${graph_policy}" negative
expect_failure_contains \
  "does not satisfy bundled minimatch range ^6.0.0" \
  node "${graph_checker}" "${tmpdir}/lock-range-mismatch.json" "${graph_policy}" negative
# Exact-pin regression (ADV678-F1): a declared exact version must be honored as
# equality, so a higher actual version is rejected instead of silently accepted.
expect_failure_contains \
  "does not satisfy bundled minimatch range 5.0.11" \
  node "${graph_checker}" "${tmpdir}/lock-exact-pin-higher-actual.json" "${graph_policy}" negative
# Strict-greater regression (ADV678-F1): ">" must reject an actual version that
# only equals the declared floor rather than collapsing into ">=".
expect_failure_contains \
  "does not satisfy bundled minimatch range >5.0.12" \
  node "${graph_checker}" "${tmpdir}/lock-strict-gt-equal-actual.json" "${graph_policy}" negative
expect_failure_contains \
  "vulnerable brace-expansion 5.0.9 at node_modules/brace-expansion" \
  node "${graph_checker}" "${tmpdir}/lock-root-vuln.json" "${graph_policy}" negative
expect_failure_contains \
  "lockfile is not valid JSON" \
  node "${graph_checker}" "${tmpdir}/lock-malformed.json" "${graph_policy}" negative
expect_failure_contains \
  "residual visible policy entry" \
  node "${graph_checker}" "${graph_lock}" "${tmpdir}/policy-bundled-entry.json" negative
expect_failure_contains \
  "residual visible policy entry" \
  node "${graph_checker}" "${graph_lock}" "${tmpdir}/policy-wrong-package.json" negative
expect_failure_contains \
  "residual visible policy entry" \
  node "${graph_checker}" "${graph_lock}" "${tmpdir}/policy-wrong-node.json" negative
expect_failure_contains \
  "visible policy is not valid JSON" \
  node "${graph_checker}" "${graph_lock}" "${tmpdir}/policy-malformed.json" negative

# Active guard for the AWS-bundled minimatch exception (GHSA-g3hv-75wg-m249).
# examples/cdk-multilang pins aws-cdk-lib 2.275.0, which bundles minimatch
# 10.2.6 (vulnerable; fixed in 11.0.1). aws-cdk-lib 2.275.0, still the latest
# release at review time (2026-11-01), declares and bundles minimatch ^10.2.6, so no aws-cdk-lib
# release fixes it and npm overrides cannot replace a bundled dependency.
# The exception is exactly two visible, expiring policy entries (the bundled
# leaf and npm's derived aws-cdk-lib parent entry) under the operator ruling
# "if a vulnerable dependency is bundled in AWS we make an exception until
# its updated there" (2026-10-03; 2026-10-04 build-toolchain extension).
# Dev-tooling copies (ts eslint chain) are fixed by override, never excepted.
# This guard pins the whole shape and fails closed on every drift: a bundled
# version change, a fixed bundle that must retire the exception, a vulnerable
# non-bundled minimatch, a ts override drift, a missing/extra/wrong policy
# entry, and malformed inputs.
cat >"${tmpdir}/verify-minimatch-exception.mjs" <<'NODE'
import fs from 'node:fs';

const [cdkLockPath, policyPath, tsLockPath, tsManifestPath, runnersLockPath, label] =
  process.argv.slice(2);
if (!cdkLockPath || !policyPath || !tsLockPath || !tsManifestPath || !runnersLockPath) {
  console.error(
    'verify-minimatch-exception: usage: <cdk-lock> <policy> <ts-lock> <ts-manifest> <runners-lock> [label]',
  );
  process.exit(2);
}
const tag = label ? `${label}: ` : '';

const CDK_LIB_PATH = 'node_modules/aws-cdk-lib';
const BUNDLED_PATH = 'node_modules/aws-cdk-lib/node_modules/minimatch';
const CDK_LIB_FLOOR = '2.275.0';
const FIXED_MINIMATCH = '11.0.1';
const BUNDLED_PINNED = '10.2.6';
const PROJECT = 'examples/cdk-multilang';
const ADVISORY = 'GHSA-g3hv-75wg-m249';

function fail(message) {
  console.error(`verify-minimatch-exception: ${tag}${message}`);
  process.exit(1);
}

function versionAtLeast(actual, minimum) {
  if (typeof actual !== 'string') return false;
  const actualParts = actual.split('.').map((part) => Number(part));
  const minimumParts = minimum.split('.').map((part) => Number(part));
  if (actualParts.length !== 3 || minimumParts.length !== 3 || actualParts.some(Number.isNaN)) {
    return false;
  }
  for (let i = 0; i < 3; i += 1) {
    if (actualParts[i] > minimumParts[i]) return true;
    if (actualParts[i] < minimumParts[i]) return false;
  }
  return true;
}

function readJson(path, description) {
  let raw;
  try {
    raw = fs.readFileSync(path, 'utf8');
  } catch (err) {
    fail(`${description} not readable: ${err instanceof Error ? err.message : String(err)}`);
  }
  try {
    return JSON.parse(raw);
  } catch (err) {
    fail(`${description} is not valid JSON: ${err instanceof Error ? err.message : String(err)}`);
  }
}

function requirePackages(lock, description) {
  const packages = lock && typeof lock === 'object' && !Array.isArray(lock) ? lock.packages : null;
  if (!packages || typeof packages !== 'object' || Array.isArray(packages)) {
    fail(`${description} has no packages map`);
  }
  return packages;
}

function minimatchEntries(packages) {
  return Object.entries(packages).filter(([path]) => /(^|\/)minimatch$/.test(path));
}

// 1. Dev tooling surfaces are fixed by upgrade, never by exception.
const tsManifest = readJson(tsManifestPath, 'ts manifest');
const tsOverride = tsManifest?.overrides?.minimatch;
if (tsOverride !== FIXED_MINIMATCH) {
  fail(`ts/package.json must override minimatch to exactly ${FIXED_MINIMATCH}, got ${String(tsOverride)}`);
}

const tsPackages = requirePackages(readJson(tsLockPath, 'ts lockfile'), 'ts lockfile');
const tsMinimatch = minimatchEntries(tsPackages);
if (tsMinimatch.length === 0) {
  fail('ts lockfile must resolve minimatch through the eslint toolchain');
}
for (const [path, entry] of tsMinimatch) {
  if (entry?.version !== FIXED_MINIMATCH) {
    fail(
      `ts lockfile minimatch at ${path} must be exactly ${FIXED_MINIMATCH}, got ${entry?.version ?? 'missing'}`,
    );
  }
}

const runnersPackages = requirePackages(
  readJson(runnersLockPath, 'contract-tests runners lockfile'),
  'contract-tests runners lockfile',
);
for (const [path, entry] of minimatchEntries(runnersPackages)) {
  if (!versionAtLeast(entry?.version, FIXED_MINIMATCH)) {
    fail(`vulnerable contract-tests runners minimatch ${entry?.version ?? 'missing'} at ${path}`);
  }
}

// 2. The aws-cdk-lib bundled copy is the only permitted vulnerable minimatch,
// and only while it is exactly the reviewed bundle.
const cdkPackages = requirePackages(readJson(cdkLockPath, 'cdk lockfile'), 'cdk lockfile');
const cdkLib = cdkPackages[CDK_LIB_PATH];
if (!cdkLib || typeof cdkLib !== 'object' || Array.isArray(cdkLib)) {
  fail(`required lock entry ${CDK_LIB_PATH} is missing`);
}
if (!versionAtLeast(cdkLib.version, CDK_LIB_FLOOR)) {
  fail(`aws-cdk-lib lock entry must be >= ${CDK_LIB_FLOOR}, got ${cdkLib.version ?? 'missing'}`);
}
if (typeof cdkLib.resolved !== 'string' || cdkLib.resolved.trim() === '') {
  fail('aws-cdk-lib lock entry is missing resolved');
}
if (typeof cdkLib.integrity !== 'string' || !/^sha512-/.test(cdkLib.integrity)) {
  fail('aws-cdk-lib lock entry is missing sha512 integrity');
}

const bundled = cdkPackages[BUNDLED_PATH];
if (!bundled || typeof bundled !== 'object' || Array.isArray(bundled)) {
  fail(`required lock entry ${BUNDLED_PATH} is missing`);
}
if (bundled.inBundle !== true) {
  fail(`${BUNDLED_PATH} must be inBundle:true`);
}
if (bundled.version !== BUNDLED_PINNED) {
  if (versionAtLeast(bundled.version, FIXED_MINIMATCH)) {
    fail(
      `aws-cdk-lib bundles minimatch ${bundled.version} >= ${FIXED_MINIMATCH}; remove the visible exception`,
    );
  }
  fail(
    `bundled minimatch must be exactly ${BUNDLED_PINNED} while the exception is active, got ${bundled.version ?? 'missing'}`,
  );
}
for (const [path, entry] of minimatchEntries(cdkPackages)) {
  if (path === BUNDLED_PATH) continue;
  if (!versionAtLeast(entry?.version, FIXED_MINIMATCH)) {
    fail(`vulnerable non-bundled minimatch ${entry?.version ?? 'missing'} at ${path}`);
  }
}

// 3. The visible policy carries exactly the reviewed pair and nothing else
// that could widen the exception.
const policy = readJson(policyPath, 'visible policy');
const findings = Array.isArray(policy?.findings) ? policy.findings : null;
if (!findings) {
  fail('visible policy is missing a findings array');
}

const exceptionFindings = findings.filter((finding) => {
  if (!finding || typeof finding !== 'object' || Array.isArray(finding)) return false;
  const node = typeof finding.node === 'string' ? finding.node : '';
  return (
    finding.package === 'minimatch' ||
    finding.package === 'aws-cdk-lib' ||
    node.includes('minimatch') ||
    node === CDK_LIB_PATH
  );
});
if (exceptionFindings.length !== 2) {
  fail(
    `visible policy must contain exactly 2 minimatch exception entries, found ${exceptionFindings.length}`,
  );
}

const expected = [
  { project: PROJECT, package: 'minimatch', advisory: ADVISORY, node: BUNDLED_PATH },
  { project: PROJECT, package: 'aws-cdk-lib', advisory: 'minimatch', node: CDK_LIB_PATH },
];
for (const want of expected) {
  const found = exceptionFindings.filter(
    (finding) =>
      finding.project === want.project &&
      finding.package === want.package &&
      finding.advisory === want.advisory &&
      finding.node === want.node,
  );
  if (found.length !== 1) {
    fail(
      `visible policy must contain exactly one entry ${want.project}/${want.package}/${want.advisory}/${want.node}`,
    );
  }
  const entry = found[0];
  if (entry.visibility !== 'visible') {
    fail(`${want.package} entry visibility must be "visible"`);
  }
  if (entry.status !== 'upstream-bundled-pending-fix') {
    fail(`${want.package} entry status must be "upstream-bundled-pending-fix"`);
  }
  if (typeof entry.expires_on !== 'string' || entry.expires_on.trim() === '') {
    fail(`${want.package} entry must declare expires_on`);
  }
  if (typeof entry.remove_when !== 'string' || !entry.remove_when.includes(FIXED_MINIMATCH)) {
    fail(`${want.package} entry remove_when must name ${FIXED_MINIMATCH}`);
  }
}

console.log(
  `verify-minimatch-exception: ${tag}PASS (aws-cdk-lib ${cdkLib.version} bundles minimatch ${bundled.version}; ts pins ${FIXED_MINIMATCH}; runners clean; 2 scoped visible entries)`,
);
NODE

cat >"${tmpdir}/make-minimatch-fixtures.mjs" <<'NODE'
import fs from 'node:fs';

const [cdkLockPath, tsLockPath, tsManifestPath, runnersLockPath, policyPath, outDir] =
  process.argv.slice(2);
const clone = (value) => JSON.parse(JSON.stringify(value));
const readJson = (path) => JSON.parse(fs.readFileSync(path, 'utf8'));
const writeJson = (name, value) => {
  fs.writeFileSync(`${outDir}/${name}`, `${JSON.stringify(value, null, 2)}\n`);
};

const CDK_LIB_PATH = 'node_modules/aws-cdk-lib';
const BUNDLED_PATH = 'node_modules/aws-cdk-lib/node_modules/minimatch';
const POLICY_LEAF = {
  project: 'examples/cdk-multilang',
  package: 'minimatch',
  advisory: 'GHSA-g3hv-75wg-m249',
  node: BUNDLED_PATH,
};
const cdk = readJson(cdkLockPath);
const ts = readJson(tsLockPath);
const manifest = readJson(tsManifestPath);
const runners = readJson(runnersLockPath);
const policy = readJson(policyPath);

{
  const mutated = clone(cdk);
  mutated.packages[BUNDLED_PATH].version = '11.0.1';
  writeJson('mm-lock-bundled-fixed.json', mutated);
}
{
  const mutated = clone(cdk);
  mutated.packages[BUNDLED_PATH].version = '10.2.7';
  writeJson('mm-lock-bundled-drift.json', mutated);
}
{
  const mutated = clone(cdk);
  delete mutated.packages[BUNDLED_PATH].inBundle;
  writeJson('mm-lock-bundled-not-bundled.json', mutated);
}
{
  const mutated = clone(cdk);
  delete mutated.packages[BUNDLED_PATH];
  writeJson('mm-lock-missing-bundled.json', mutated);
}
{
  const mutated = clone(cdk);
  delete mutated.packages[CDK_LIB_PATH];
  writeJson('mm-lock-missing-cdk.json', mutated);
}
{
  const mutated = clone(cdk);
  delete mutated.packages[CDK_LIB_PATH].integrity;
  writeJson('mm-lock-no-integrity.json', mutated);
}
{
  const mutated = clone(cdk);
  mutated.packages['node_modules/minimatch'] = { version: '10.2.6', license: 'BlueOak-1.0.0' };
  writeJson('mm-lock-root-vuln.json', mutated);
}
fs.writeFileSync(`${outDir}/mm-lock-malformed.json`, '{\n  "packages": {\n');

{
  const mutated = clone(ts);
  mutated.packages['node_modules/minimatch'].version = '10.2.6';
  writeJson('mm-ts-lock-old.json', mutated);
}
{
  const mutated = clone(manifest);
  delete mutated.overrides.minimatch;
  writeJson('mm-ts-manifest-no-override.json', mutated);
}
{
  const mutated = clone(manifest);
  mutated.overrides.minimatch = '^11.0.1';
  writeJson('mm-ts-manifest-wrong-override.json', mutated);
}
{
  const mutated = clone(runners);
  mutated.packages['node_modules/minimatch'] = { version: '10.2.6', license: 'BlueOak-1.0.0' };
  writeJson('mm-runners-lock-vuln.json', mutated);
}

function policyWithout(predicate) {
  return { ...policy, findings: policy.findings.filter((finding) => !predicate(finding)) };
}

writeJson(
  'mm-policy-missing-leaf.json',
  policyWithout((finding) => finding.package === 'minimatch'),
);
writeJson(
  'mm-policy-missing-parent.json',
  policyWithout((finding) => finding.package === 'aws-cdk-lib'),
);
writeJson('mm-policy-wrong-node.json', {
  ...policy,
  findings: policy.findings.map((finding) =>
    finding.package === 'minimatch' ? { ...finding, node: 'node_modules/minimatch' } : finding,
  ),
});
writeJson('mm-policy-wrong-advisory.json', {
  ...policy,
  findings: policy.findings.map((finding) =>
    finding.package === 'minimatch' ? { ...finding, advisory: 'GHSA-aaaa-bbbb-cccc' } : finding,
  ),
});
writeJson('mm-policy-extra-entry.json', {
  ...policy,
  findings: [
    ...policy.findings,
    {
      ...POLICY_LEAF,
      project: 'ts',
      visibility: 'visible',
      status: 'upstream-bundled-pending-fix',
      reviewed_on: '2026-11-01',
      expires_on: '2099-12-31',
      remove_when: 'Synthetic negative-control fixture for the minimatch exception guard.',
      justification: 'Synthetic negative-control fixture for the minimatch exception guard.',
    },
  ],
});
fs.writeFileSync(`${outDir}/mm-policy-malformed.json`, '{ "findings": [ }\n');

// Real checker reports used to prove the committed visible policy clears
// SEC-2 for the advisory and fails for anything else.
writeJson('mm-audit.json', {
  auditReportVersion: 2,
  vulnerabilities: {
    'aws-cdk-lib': { name: 'aws-cdk-lib', severity: 'moderate', via: ['minimatch'], nodes: [CDK_LIB_PATH] },
    minimatch: {
      name: 'minimatch',
      severity: 'moderate',
      via: [
        {
          source: 1244012,
          name: 'minimatch',
          dependency: 'minimatch',
          title: 'minimatch has potential denial of service via globparts amplification',
          url: 'https://github.com/advisories/GHSA-g3hv-75wg-m249',
        },
      ],
      nodes: [BUNDLED_PATH],
    },
  },
});
writeJson('mm-audit-other-node.json', {
  auditReportVersion: 2,
  vulnerabilities: {
    minimatch: {
      name: 'minimatch',
      severity: 'moderate',
      via: [
        {
          source: 1244012,
          name: 'minimatch',
          dependency: 'minimatch',
          title: 'minimatch has potential denial of service via globparts amplification',
          url: 'https://github.com/advisories/GHSA-g3hv-75wg-m249',
        },
      ],
      nodes: ['node_modules/minimatch'],
    },
  },
});
NODE

minimatch_checker="${tmpdir}/verify-minimatch-exception.mjs"
mm_cdk_lock="examples/cdk-multilang/package-lock.json"
mm_policy="gov-infra/planning/theorydb-visible-npm-audit-findings.json"
mm_ts_lock="ts/package-lock.json"
mm_ts_manifest="ts/package.json"
mm_runners_lock="contract-tests/runners/ts/package-lock.json"

mm_check() {
  local cdk_lock="$1"
  local policy="$2"
  local ts_lock="$3"
  local ts_manifest="$4"
  local runners_lock="$5"
  local label="${6:-negative}"
  node "${minimatch_checker}" "${cdk_lock}" "${policy}" "${ts_lock}" "${ts_manifest}" "${runners_lock}" "${label}"
}

# Positive: the committed graph, override, and policy pair must satisfy the guard.
expect_success_contains \
  "verify-minimatch-exception: committed: PASS" \
  mm_check "${mm_cdk_lock}" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}" committed

node "${tmpdir}/make-minimatch-fixtures.mjs" \
  "${mm_cdk_lock}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}" "${mm_policy}" "${tmpdir}"

# Negative: every drift and malformed input must fail closed.
expect_failure_contains \
  "aws-cdk-lib bundles minimatch 11.0.1 >= 11.0.1; remove the visible exception" \
  mm_check "${tmpdir}/mm-lock-bundled-fixed.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "bundled minimatch must be exactly 10.2.6 while the exception is active, got 10.2.7" \
  mm_check "${tmpdir}/mm-lock-bundled-drift.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "node_modules/aws-cdk-lib/node_modules/minimatch must be inBundle:true" \
  mm_check "${tmpdir}/mm-lock-bundled-not-bundled.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "required lock entry node_modules/aws-cdk-lib/node_modules/minimatch is missing" \
  mm_check "${tmpdir}/mm-lock-missing-bundled.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "required lock entry node_modules/aws-cdk-lib is missing" \
  mm_check "${tmpdir}/mm-lock-missing-cdk.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "aws-cdk-lib lock entry is missing sha512 integrity" \
  mm_check "${tmpdir}/mm-lock-no-integrity.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "vulnerable non-bundled minimatch 10.2.6 at node_modules/minimatch" \
  mm_check "${tmpdir}/mm-lock-root-vuln.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "cdk lockfile is not valid JSON" \
  mm_check "${tmpdir}/mm-lock-malformed.json" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "ts lockfile minimatch at node_modules/minimatch must be exactly 11.0.1, got 10.2.6" \
  mm_check "${mm_cdk_lock}" "${mm_policy}" "${tmpdir}/mm-ts-lock-old.json" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "ts/package.json must override minimatch to exactly 11.0.1, got undefined" \
  mm_check "${mm_cdk_lock}" "${mm_policy}" "${mm_ts_lock}" "${tmpdir}/mm-ts-manifest-no-override.json" "${mm_runners_lock}"
expect_failure_contains \
  "ts/package.json must override minimatch to exactly 11.0.1, got ^11.0.1" \
  mm_check "${mm_cdk_lock}" "${mm_policy}" "${mm_ts_lock}" "${tmpdir}/mm-ts-manifest-wrong-override.json" "${mm_runners_lock}"
expect_failure_contains \
  "vulnerable contract-tests runners minimatch 10.2.6 at node_modules/minimatch" \
  mm_check "${mm_cdk_lock}" "${mm_policy}" "${mm_ts_lock}" "${mm_ts_manifest}" "${tmpdir}/mm-runners-lock-vuln.json"
expect_failure_contains \
  "visible policy must contain exactly 2 minimatch exception entries, found 1" \
  mm_check "${mm_cdk_lock}" "${tmpdir}/mm-policy-missing-leaf.json" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "visible policy must contain exactly 2 minimatch exception entries, found 1" \
  mm_check "${mm_cdk_lock}" "${tmpdir}/mm-policy-missing-parent.json" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "visible policy must contain exactly one entry examples/cdk-multilang/minimatch/GHSA-g3hv-75wg-m249/node_modules/aws-cdk-lib/node_modules/minimatch" \
  mm_check "${mm_cdk_lock}" "${tmpdir}/mm-policy-wrong-node.json" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "visible policy must contain exactly one entry examples/cdk-multilang/minimatch/GHSA-g3hv-75wg-m249/node_modules/aws-cdk-lib/node_modules/minimatch" \
  mm_check "${mm_cdk_lock}" "${tmpdir}/mm-policy-wrong-advisory.json" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "visible policy must contain exactly 2 minimatch exception entries, found 3" \
  mm_check "${mm_cdk_lock}" "${tmpdir}/mm-policy-extra-entry.json" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"
expect_failure_contains \
  "visible policy is not valid JSON" \
  mm_check "${mm_cdk_lock}" "${tmpdir}/mm-policy-malformed.json" "${mm_ts_lock}" "${mm_ts_manifest}" "${mm_runners_lock}"

# The committed policy must clear the real SEC-2 checker for exactly the live
# report, and must not clear a different node.
expect_success_contains \
  "2 visible unallowlisted finding(s) in examples/cdk-multilang (accepted)" \
  env NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS=true node "${checker}" "${tmpdir}/mm-audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang "${mm_policy}"
expect_failure_contains \
  "unallowlisted finding(s)" \
  env NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS=true node "${checker}" "${tmpdir}/mm-audit-other-node.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang "${mm_policy}"

for finding_id in \
  "NPM-AUDIT:examples/cdk-multilang:minimatch:GHSA-g3hv-75wg-m249:node_modules/aws-cdk-lib/node_modules/minimatch" \
  "NPM-AUDIT:examples/cdk-multilang:aws-cdk-lib:minimatch:node_modules/aws-cdk-lib"; do
  if grep -Fxq "${finding_id}" gov-infra/planning/theorydb-supply-chain-allowlist.txt; then
    echo "npm-audit-policy-test: ${finding_id} must not be in the suppression allowlist"
    exit 1
  fi
done

echo "npm-audit-policy-test: PASS"
