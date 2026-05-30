#!/usr/bin/env node
import fs from 'node:fs';

const [reportPath, allowlistPath, projectPathArg, visiblePolicyPath] = process.argv.slice(2);

if (!reportPath || !allowlistPath || !projectPathArg) {
  console.error(
    'usage: node scripts/check-npm-audit-allowlist.mjs <audit-json> <allowlist> <project-path> [visible-policy-json]',
  );
  process.exit(2);
}

const projectPath = projectPathArg.replace(/^\.\//, '').replace(/\/$/, '');
let report;
try {
  report = JSON.parse(fs.readFileSync(reportPath, 'utf8'));
} catch (err) {
  console.error(
    `npm-audit: invalid JSON report for ${projectPath}: ${err instanceof Error ? err.message : String(err)}`,
  );
  process.exit(2);
}
const allowlist = new Set(
  fs
    .readFileSync(allowlistPath, 'utf8')
    .split(/\r?\n/)
    .map((line) => line.replace(/#.*/, '').trim())
    .filter(Boolean),
);

function findingId(project, packageName, advisory, node) {
  return `NPM-AUDIT:${project}:${packageName}:${advisory}:${node}`;
}

function policyError(message) {
  console.error(`npm-audit: invalid visible policy: ${message}`);
  process.exit(2);
}

function requirePolicyString(entry, index, field) {
  const value = entry?.[field];
  if (typeof value !== 'string' || value.trim() === '') {
    policyError(`findings[${index}].${field} must be a non-empty string`);
  }
  return value.trim();
}

function loadVisiblePolicy(policyPath) {
  if (!policyPath || !fs.existsSync(policyPath)) {
    return new Map();
  }

  let policy;
  try {
    policy = JSON.parse(fs.readFileSync(policyPath, 'utf8'));
  } catch (err) {
    policyError(`${policyPath}: ${err instanceof Error ? err.message : String(err)}`);
  }

  if (
    !policy ||
    typeof policy !== 'object' ||
    Array.isArray(policy) ||
    !Array.isArray(policy.findings)
  ) {
    policyError(`${policyPath}: expected an object with a findings array`);
  }

  const visible = new Map();
  for (const [index, entry] of policy.findings.entries()) {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
      policyError(`findings[${index}] must be an object`);
    }

    const visibility = requirePolicyString(entry, index, 'visibility');
    if (visibility !== 'visible') {
      policyError(`findings[${index}].visibility must be "visible"`);
    }

    const id = findingId(
      requirePolicyString(entry, index, 'project'),
      requirePolicyString(entry, index, 'package'),
      requirePolicyString(entry, index, 'advisory'),
      requirePolicyString(entry, index, 'node'),
    );

    visible.set(id, entry);
  }

  return visible;
}

const visiblePolicy = loadVisiblePolicy(visiblePolicyPath);

function advisoryId(via) {
  if (typeof via === 'string') {
    return via;
  }
  if (!via || typeof via !== 'object') {
    return 'unknown';
  }
  const url = typeof via.url === 'string' ? via.url : '';
  const ghsa = url.match(/GHSA-[a-z0-9-]+/i)?.[0];
  if (ghsa) {
    return ghsa;
  }
  if (via.source !== undefined && via.source !== null) {
    return String(via.source);
  }
  return typeof via.title === 'string' ? via.title : 'unknown';
}

function describeAuditFailure(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return 'report root is not an object';
  }

  const fields = [];
  for (const key of ['error', 'statusCode', 'message']) {
    const field = value[key];
    if (field !== undefined && field !== null && String(field).trim() !== '') {
      fields.push(`${key}=${String(field)}`);
    }
  }
  return fields.length > 0 ? fields.join(' ') : 'missing vulnerabilities object';
}

if (
  !report ||
  typeof report !== 'object' ||
  Array.isArray(report) ||
  !Object.hasOwn(report, 'vulnerabilities') ||
  !report.vulnerabilities ||
  typeof report.vulnerabilities !== 'object' ||
  Array.isArray(report.vulnerabilities)
) {
  console.error(
    `npm-audit: invalid audit report for ${projectPath}; ${describeAuditFailure(report)}`,
  );
  process.exit(2);
}

const vulnerabilities = report.vulnerabilities;
const findings = [];

for (const vulnerability of Object.values(vulnerabilities)) {
  if (!vulnerability || typeof vulnerability !== 'object') {
    continue;
  }

  const packageName = vulnerability.name ?? 'unknown';
  const nodes =
    Array.isArray(vulnerability.nodes) && vulnerability.nodes.length > 0
      ? vulnerability.nodes
      : ['unknown-node'];
  const advisories = (Array.isArray(vulnerability.via) ? vulnerability.via : [])
    .filter((via) => typeof via !== 'string')
    .map(advisoryId);

  // If npm only reports dependency names in `via`, still produce an exact finding
  // so a new report shape cannot pass without an intentional allowlist entry.
  if (advisories.length === 0) {
    advisories.push(
      ...(Array.isArray(vulnerability.via) ? vulnerability.via.map(advisoryId) : ['unknown-advisory']),
    );
  }

  for (const node of nodes) {
    for (const advisory of advisories) {
      findings.push(findingId(projectPath, packageName, advisory, node));
    }
  }
}

if (findings.length === 0) {
  console.error(
    `npm-audit: audit command failed for ${projectPath} but reported no vulnerability findings`,
  );
  process.exit(2);
}

const policyConflicts = findings.filter((finding) => allowlist.has(finding) && visiblePolicy.has(finding));
if (policyConflicts.length > 0) {
  console.error(
    `npm-audit: ${policyConflicts.length} visible finding(s) cannot also be in the suppression allowlist`,
  );
  for (const finding of policyConflicts) {
    console.error(`  ${finding}`);
  }
  process.exit(2);
}

const missing = findings.filter((finding) => !allowlist.has(finding));
const visibleMissing = missing.filter((finding) => visiblePolicy.has(finding));
const unhandledMissing = missing.filter((finding) => !visiblePolicy.has(finding));
if (unhandledMissing.length > 0) {
  console.error(`npm-audit: ${unhandledMissing.length} unallowlisted finding(s) in ${projectPath}`);
  for (const finding of unhandledMissing) {
    console.error(`  ${finding}`);
  }
  process.exit(1);
}

const allowlistedFindings = findings.filter((finding) => allowlist.has(finding));
if (allowlistedFindings.length > 0) {
  console.log(`npm-audit: ${allowlistedFindings.length} allowlisted finding(s) in ${projectPath}`);
  for (const finding of allowlistedFindings) {
    console.log(`  ${finding}`);
  }
}

if (visibleMissing.length > 0) {
  console.log(`npm-audit: ${visibleMissing.length} visible unallowlisted finding(s) in ${projectPath}`);
  console.log('npm-audit: visible findings remain in SEC-2 evidence and are not supply-chain allowlist suppressions');
  for (const finding of visibleMissing) {
    console.log(`  ${finding}`);
  }
}
