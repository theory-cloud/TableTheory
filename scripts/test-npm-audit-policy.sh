#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

finding="NPM-AUDIT:examples/cdk-multilang:brace-expansion:GHSA-jxxr-4gwj-5jf2:node_modules/aws-cdk-lib/node_modules/brace-expansion"
checker="scripts/check-npm-audit-allowlist.mjs"

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
      "status": "upstream-bundled-pending-fix"
    }
  ]
}
JSON

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

expect_success_contains \
  "visible unallowlisted finding(s)" \
  node "${checker}" "${tmpdir}/audit.json" "${tmpdir}/empty-allowlist.txt" examples/cdk-multilang "${tmpdir}/visible.json"

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

if grep -Fxq "${finding}" hgm-infra/planning/theorydb-supply-chain-allowlist.txt; then
  echo "npm-audit-policy-test: THE-1757 brace-expansion finding must not be in the suppression allowlist"
  exit 1
fi

node <<'NODE'
const fs = require('node:fs');

const lock = JSON.parse(fs.readFileSync('examples/cdk-multilang/package-lock.json', 'utf8'));
const cdkLib = lock.packages?.['node_modules/aws-cdk-lib'];
if (cdkLib?.version !== '2.257.0') {
  throw new Error(`expected aws-cdk-lib lock entry to remain 2.257.0 while the visible policy is active, got ${cdkLib?.version ?? 'missing'}`);
}

const bundledBrace = lock.packages?.['node_modules/aws-cdk-lib/node_modules/brace-expansion'];
if (bundledBrace?.version !== '5.0.5') {
  throw new Error(`expected aws-cdk-lib bundled brace-expansion lock entry to remain 5.0.5, got ${bundledBrace?.version ?? 'missing'}`);
}

const policy = JSON.parse(fs.readFileSync('hgm-infra/planning/theorydb-visible-npm-audit-findings.json', 'utf8'));
const hasVisibleBracePolicy = policy.findings?.some(
  (finding) =>
    finding.project === 'examples/cdk-multilang' &&
    finding.package === 'brace-expansion' &&
    finding.advisory === 'GHSA-jxxr-4gwj-5jf2' &&
    finding.node === 'node_modules/aws-cdk-lib/node_modules/brace-expansion' &&
    finding.visibility === 'visible' &&
    finding.current_upstream_version === '2.257.0' &&
    finding.current_bundled_version === '5.0.5',
);
if (!hasVisibleBracePolicy) {
  throw new Error('expected visible THE-1757 brace-expansion policy for bundled version 5.0.5');
}
NODE

echo "npm-audit-policy-test: PASS"
