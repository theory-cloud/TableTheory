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

# write_fixture <path> <dependency engines.node spec, or "" for none>
write_fixture() {
  local target="$1"
  local declared="$2"

  node - "${target}" "${declared}" <<'NODE'
const fs = require('node:fs');
const [target, declared] = process.argv.slice(2);
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
      engines: { node: '>=22' },
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

# Ranges that admit the floor pass.
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
