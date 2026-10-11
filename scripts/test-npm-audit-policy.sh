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
# examples/cdk-multilang now pins aws-cdk-lib 2.273.0, which bundles brace-expansion
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
const CDK_LIB_FLOOR = '2.273.0';
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

echo "npm-audit-policy-test: PASS"
