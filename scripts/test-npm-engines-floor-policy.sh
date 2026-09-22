#!/usr/bin/env bash
# Purpose: negative-proof the npm engines-floor gate.
#
# The positive path is the real lockfile set. The negative proof drives the
# checker with synthetic lockfiles so the gate cannot pass review on a green
# path alone, and so the "no allowlist, no waiver" property is enforced rather
# than asserted.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

checker="scripts/check-npm-engines-floor.mjs"
wrapper="scripts/verify-npm-engines-floor.sh"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

# write_fixture <path> <dependency engines.node spec, or "" for none> [root engines.node spec]
write_fixture() {
  local target="$1"
  local declared="$2"
  local root_declared="${3:->=22}"

  node - "${target}" "${declared}" "${root_declared}" <<'NODE'
const fs = require('node:fs');
const [target, declared, rootDeclared] = process.argv.slice(2);
const dependency = { version: '1.0.0' };
if (declared !== '') dependency.engines = { node: declared };
const lockfile = {
  name: 'synthetic-engines-floor-fixture',
  version: '0.0.0',
  lockfileVersion: 3,
  requires: true,
  packages: {
    '': {
      name: 'synthetic-engines-floor-fixture',
      version: '0.0.0',
      engines: { node: rootDeclared },
    },
    'node_modules/synthetic-dep': dependency,
  },
};
fs.writeFileSync(target, `${JSON.stringify(lockfile, null, 2)}\n`);
NODE
}

expect_success_contains() {
  local expected="$1"
  shift

  local output
  if ! output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "npm-engines-floor-policy-test: expected command to succeed"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "npm-engines-floor-policy-test: expected output to contain: ${expected}"
    exit 1
  fi
}

expect_failure_contains() {
  local expected="$1"
  shift

  local output
  if output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "npm-engines-floor-policy-test: expected command to fail"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "npm-engines-floor-policy-test: expected failure output to contain: ${expected}"
    exit 1
  fi
}

# A dependency that cannot run on the floor must fail, and must be named.
write_fixture "${tmpdir}/excludes.json" ">=24"
expect_failure_contains \
  "node_modules/synthetic-dep" \
  node "${checker}" "${tmpdir}/excludes.json"
expect_failure_contains \
  "excludes Node 22.x" \
  node "${checker}" "${tmpdir}/excludes.json"

# A disjunction whose only floor-admitting branch is 24+ must still fail.
write_fixture "${tmpdir}/excludes-via-disjunction.json" "^20.19.0 || >=24"
expect_failure_contains \
  "exclude the Node 22 floor" \
  node "${checker}" "${tmpdir}/excludes-via-disjunction.json"

# --- prerelease-anchored ranges -------------------------------------------
# Prereleases of the line above the floor sort above every floor release, so a
# range anchored only on one admits no floor release even though the anchor
# itself is numerically above the floor's lower edge.
write_fixture "${tmpdir}/prerelease-next-line.json" ">=23.0.0-0"
expect_failure_contains \
  "exclude the Node 22 floor" \
  node "${checker}" "${tmpdir}/prerelease-next-line.json"

# A range that matches a single prerelease on the floor's own line matches no
# release at all.
write_fixture "${tmpdir}/prerelease-only-pin.json" "=22.13.0-rc.1"
expect_failure_contains \
  "exclude the Node 22 floor" \
  node "${checker}" "${tmpdir}/prerelease-only-pin.json"

# Adjacent exclusive bounds admit no release between them.
write_fixture "${tmpdir}/adjacent-exclusive-bounds.json" ">22.0.0 <22.0.1"
expect_failure_contains \
  "exclude the Node 22 floor" \
  node "${checker}" "${tmpdir}/adjacent-exclusive-bounds.json"

# A prerelease-anchored hyphen range above the floor line admits no floor
# release either.
write_fixture "${tmpdir}/prerelease-hyphen-excludes.json" "23.0.0-0 - 24.0.0"
expect_failure_contains \
  "exclude the Node 22 floor" \
  node "${checker}" "${tmpdir}/prerelease-hyphen-excludes.json"

# A hyphen range anchored on a prerelease of the floor line still admits floor
# releases, so it must pass.
write_fixture "${tmpdir}/prerelease-hyphen-admits.json" "22.0.0-0 - 22.9.9"
expect_success_contains \
  "dependency engine ranges 1" \
  node "${checker}" "${tmpdir}/prerelease-hyphen-admits.json"

# A disjunction mixing release branches with a prerelease-anchored branch is
# judged branch by branch, so the prerelease branch cannot smuggle the floor in
# when no release branch reaches it.
write_fixture "${tmpdir}/prerelease-disjunction.json" "^18.18.0 || ^20.9.0 || >=23.0.0-0"
expect_failure_contains \
  "exclude the Node 22 floor" \
  node "${checker}" "${tmpdir}/prerelease-disjunction.json"

# The same shape with a floor-admitting release branch passes.
write_fixture "${tmpdir}/prerelease-disjunction-admits.json" "^18.18.0 || ^20.9.0 || ^22.13.0"
expect_success_contains \
  "dependency engine ranges 1" \
  node "${checker}" "${tmpdir}/prerelease-disjunction-admits.json"

# A project root declaring a prerelease-anchored floor is a project floor, not
# an upstream dependency: it is counted and left unjudged, so it must not be
# reported as a violation. Judging roots would also fail the real tree, whose
# examples/cdk-multilang declares a floor above the repository floor.
write_fixture "${tmpdir}/root-prerelease-floor.json" ">=22" ">=22.0.0-0"
expect_success_contains \
  "own-project engine declarations not judged 1" \
  node "${checker}" "${tmpdir}/root-prerelease-floor.json"

# --- unmodelled grammar ----------------------------------------------------
# Ranges the matcher does not model fail closed rather than being skipped. That
# includes the legacy tilde alias `~>`, which npm's semver reads as `~` but the
# matcher deliberately refuses to guess at.
write_fixture "${tmpdir}/legacy-tilde-alias.json" "~>22"
expect_failure_contains \
  "does not model" \
  node "${checker}" "${tmpdir}/legacy-tilde-alias.json"

# A wildcard component followed by a concrete one is semver-invalid, so it must
# be refused rather than silently coerced to `22.x`.
write_fixture "${tmpdir}/invalid-wildcard-tail.json" "22.x.1"
expect_failure_contains \
  "does not model" \
  node "${checker}" "${tmpdir}/invalid-wildcard-tail.json"

# A range that admits the floor passes.
write_fixture "${tmpdir}/admits-floor.json" ">=22"
expect_success_contains \
  "dependency engine ranges 1" \
  node "${checker}" "${tmpdir}/admits-floor.json"
expect_success_contains \
  "excluded 0" \
  node "${checker}" "${tmpdir}/admits-floor.json"

# A dependency that admits the floor through a lower floor is not this gate's
# business: the gate catches exclusion, not declared generosity.
write_fixture "${tmpdir}/admits-lower.json" ">=20"
expect_success_contains \
  "dependency engine ranges 1" \
  node "${checker}" "${tmpdir}/admits-lower.json"

# A range the matcher does not model fails outright rather than being skipped.
write_fixture "${tmpdir}/unmodelled.json" "lts/*"
expect_failure_contains \
  "does not model" \
  node "${checker}" "${tmpdir}/unmodelled.json"

# A dependency with no engines.node declaration is not judged.
write_fixture "${tmpdir}/no-engines.json" ""
expect_success_contains \
  "dependency engine ranges 0" \
  node "${checker}" "${tmpdir}/no-engines.json"

# The real tree must pass through the shipped wrapper.
expect_success_contains \
  "lockfiles 3" \
  bash "${wrapper}"
expect_success_contains \
  "excluded 0" \
  bash "${wrapper}"

# There is no waiver machinery: the shipped gate refuses arguments.
expect_failure_contains \
  "this gate takes no arguments" \
  bash "${wrapper}" --allowlist "${tmpdir}/never-written.txt"

echo "npm-engines-floor-policy-test: PASS"
