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

node <<'NODE'
const fs = require('node:fs');

const lock = JSON.parse(fs.readFileSync('examples/cdk-multilang/package-lock.json', 'utf8'));

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

const cdkLib = lock.packages?.['node_modules/aws-cdk-lib'];
if (!versionAtLeast(cdkLib?.version, '2.260.0')) {
  throw new Error(`expected aws-cdk-lib lock entry to be at least 2.260.0, got ${cdkLib?.version ?? 'missing'}`);
}

const bundledBrace = lock.packages?.['node_modules/aws-cdk-lib/node_modules/brace-expansion'];
if (!versionAtLeast(bundledBrace?.version, '5.0.6')) {
  throw new Error(`expected aws-cdk-lib bundled brace-expansion lock entry to be at least 5.0.6, got ${bundledBrace?.version ?? 'missing'}`);
}

const policy = JSON.parse(fs.readFileSync('gov-infra/planning/theorydb-visible-npm-audit-findings.json', 'utf8'));
const visibleBracePolicy = policy.findings?.find(
  (finding) =>
    finding.project === 'examples/cdk-multilang' &&
    finding.package === 'brace-expansion' &&
    finding.advisory === 'GHSA-mh99-v99m-4gvg' &&
    finding.node === 'node_modules/aws-cdk-lib/node_modules/brace-expansion'
);
if (!versionAtLeast(bundledBrace?.version, '5.0.7')) {
  throw new Error(`expected aws-cdk-lib to bundle brace-expansion 5.0.7 or newer, got ${bundledBrace?.version ?? 'missing'}`);
}
if (!visibleBracePolicy) {
  throw new Error('expected the current aws-cdk-lib bundled brace-expansion advisory to remain visibly documented');
}
if (versionAtLeast(bundledBrace?.version, '5.0.8')) {
  throw new Error('aws-cdk-lib now bundles fixed brace-expansion; remove the visible exception before merging');
}
NODE

echo "npm-audit-policy-test: PASS"
