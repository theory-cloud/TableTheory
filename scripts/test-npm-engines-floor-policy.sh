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
#
# The root spec defaults to ">=22", the repository floor. The sentinel "<omit>"
# writes a root entry with no engines key at all, which is the absent-root case
# the gate must fail closed on.
write_fixture() {
  local target="$1"
  local declared="$2"
  local root_declared="${3:->=22}"

  node - "${target}" "${declared}" "${root_declared}" <<'NODE'
const fs = require('node:fs');
const [target, declared, rootDeclared] = process.argv.slice(2);
const dependency = { version: '1.0.0' };
if (declared !== '') dependency.engines = { node: declared };
const root = {
  name: 'synthetic-engines-floor-fixture',
  version: '0.0.0',
};
if (rootDeclared !== '<omit>') root.engines = { node: rootDeclared };
const lockfile = {
  name: 'synthetic-engines-floor-fixture',
  version: '0.0.0',
  lockfileVersion: 3,
  requires: true,
  packages: {
    '': root,
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

# --- lockfile roots --------------------------------------------------------
# One rule, converged across the framework repos: every audited lockfile root
# must DECLARE engines.node, and the declaration must not admit a release below
# the repository floor. Absence fails closed, and a declaration above the floor
# is legitimate rather than drift.
write_fixture "${tmpdir}/root-at-floor.json" ">=22" ">=22"
expect_success_contains \
  "project roots judged 1" \
  node "${checker}" "${tmpdir}/root-at-floor.json"

# Declaring a HIGHER floor than the repository floor passes: the rule is about
# the lower edge, not about matching the repository floor exactly. This is the
# case examples/cdk-multilang exercises in the real tree with its ">=24".
write_fixture "${tmpdir}/root-above-floor.json" ">=22" ">=24"
expect_success_contains \
  "project roots judged 1" \
  node "${checker}" "${tmpdir}/root-above-floor.json"
expect_success_contains \
  "excluded 0" \
  node "${checker}" "${tmpdir}/root-above-floor.json"

# A prerelease anchored on the floor's own line admits no release below the
# floor, so it is not the drift class (the FaceTheory "root false-fail").
write_fixture "${tmpdir}/root-prerelease-floor.json" ">=22" ">=22.0.0-0"
expect_success_contains \
  "project roots judged 1" \
  node "${checker}" "${tmpdir}/root-prerelease-floor.json"

# A root that still reaches below the floor fails, and is named with the
# lockfile and the declaration it made.
write_fixture "${tmpdir}/root-below-floor.json" ">=22" ">=20"
expect_failure_contains \
  "root-below-floor.json <root>" \
  node "${checker}" "${tmpdir}/root-below-floor.json"
expect_failure_contains \
  "admits a release below the Node 22 floor" \
  node "${checker}" "${tmpdir}/root-below-floor.json"

# A disjunction whose floor-admitting branch still leaves an older release
# reachable fails: roots are judged branch by branch, like dependencies.
write_fixture "${tmpdir}/root-disjunction-below.json" ">=22" "^20.19.0 || >=22"
expect_failure_contains \
  "admits a release below the Node 22 floor" \
  node "${checker}" "${tmpdir}/root-disjunction-below.json"

# A wildcard root admits every release, the sub-floor ones included.
write_fixture "${tmpdir}/root-wildcard.json" ">=22" "*"
expect_failure_contains \
  "admits a release below the Node 22 floor" \
  node "${checker}" "${tmpdir}/root-wildcard.json"

# A root with no engines declaration at all fails closed. This is the hole the
# rule exists to close, and it must never read as a pass.
write_fixture "${tmpdir}/root-absent.json" ">=22" "<omit>"
expect_failure_contains \
  "declares no engines.node" \
  node "${checker}" "${tmpdir}/root-absent.json"

# A root whose declaration is not a string is refused rather than coerced.
write_fixture "${tmpdir}/root-non-string.json" ">=22" ">=22"
node -e '
const fs = require("node:fs");
const target = process.argv[1];
const lockfile = JSON.parse(fs.readFileSync(target, "utf8"));
lockfile.packages[""].engines.node = 22;
fs.writeFileSync(target, `${JSON.stringify(lockfile, null, 2)}\n`);
' "${tmpdir}/root-non-string.json"
expect_failure_contains \
  "engines.node is not a string" \
  node "${checker}" "${tmpdir}/root-non-string.json"

# Unmodelled root grammar fails closed rather than being skipped.
write_fixture "${tmpdir}/root-unmodelled.json" ">=22" "~>22"
expect_failure_contains \
  "does not model" \
  node "${checker}" "${tmpdir}/root-unmodelled.json"

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

# --- the real tree, through the shipped wrapper -----------------------------
# A scanner-level probe, not a predicate probe: the numbers below are the
# shipped gate's own summary line over the real lockfile set.
expect_success_contains \
  "lockfiles 3" \
  bash "${wrapper}"
expect_success_contains \
  "project roots judged 3" \
  bash "${wrapper}"
expect_success_contains \
  "excluded 0" \
  bash "${wrapper}"

# There is no waiver machinery: the shipped gate refuses arguments.
expect_failure_contains \
  "this gate takes no arguments" \
  bash "${wrapper}" --allowlist "${tmpdir}/never-written.txt"

echo "npm-engines-floor-policy-test: PASS"
