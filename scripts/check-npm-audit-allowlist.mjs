#!/usr/bin/env node
import fs from 'node:fs';

const [reportPath, allowlistPath, projectPathArg] = process.argv.slice(2);

if (!reportPath || !allowlistPath || !projectPathArg) {
  console.error(
    'usage: node scripts/check-npm-audit-allowlist.mjs <audit-json> <allowlist> <project-path>',
  );
  process.exit(2);
}

const projectPath = projectPathArg.replace(/^\.\//, '').replace(/\/$/, '');
const report = JSON.parse(fs.readFileSync(reportPath, 'utf8'));
const allowlist = new Set(
  fs
    .readFileSync(allowlistPath, 'utf8')
    .split(/\r?\n/)
    .map((line) => line.replace(/#.*/, '').trim())
    .filter(Boolean),
);

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

const vulnerabilities = report.vulnerabilities ?? {};
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
      findings.push(`NPM-AUDIT:${projectPath}:${packageName}:${advisory}:${node}`);
    }
  }
}

if (findings.length === 0) {
  console.log(`npm-audit: no vulnerability findings in ${projectPath}`);
  process.exit(0);
}

const missing = findings.filter((finding) => !allowlist.has(finding));
if (missing.length > 0) {
  console.error(`npm-audit: ${missing.length} unallowlisted finding(s) in ${projectPath}`);
  for (const finding of missing) {
    console.error(`  ${finding}`);
  }
  process.exit(1);
}

console.log(`npm-audit: ${findings.length} allowlisted finding(s) in ${projectPath}`);
for (const finding of findings) {
  console.log(`  ${finding}`);
}
