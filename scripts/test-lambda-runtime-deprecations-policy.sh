#!/usr/bin/env bash
# Purpose: negative-proof the Lambda runtime deprecation gate.
#
# The positive path is the real scanned surfaces. The negative proof drives a
# copy of the checker against a synthetic repository root, because the checker
# accepts only `--self-test` - there is no positional scope argument, and the
# shipped wrapper can never be pointed anywhere but the real scope. A synthetic
# root is therefore the only way to hand the classifier foreign surfaces, and it
# keeps the proof off the real tree.
#
# Each probe below has to fail with the message that names what is wrong, so the
# gate cannot pass review on a green path alone.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

checker="scripts/check-lambda-runtime-deprecations.mjs"
wrapper="scripts/verify-lambda-runtime-deprecations.sh"

# The declared surfaces, restated here so a probe that targets one of them fails
# loudly if the scope moves.
surface_a="examples/cdk-multilang/lib/multilang-demo-stack.ts"
surface_b="examples/cdk-multilang/lib/tabletheory-ttl-archive.ts"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

expect_success_contains() {
  local expected="$1"
  shift

  local output
  if ! output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "lambda-runtime-deprecations-policy-test: expected command to succeed"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "lambda-runtime-deprecations-policy-test: expected output to contain: ${expected}"
    exit 1
  fi
}

expect_failure_contains() {
  local expected="$1"
  shift

  local output
  if output="$("$@" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "lambda-runtime-deprecations-policy-test: expected command to fail"
    exit 1
  fi
  if ! grep -Fq "${expected}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "lambda-runtime-deprecations-policy-test: expected failure output to contain: ${expected}"
    exit 1
  fi
}

# write_surface <repository root> <relative path> <content>
write_surface() {
  local root="$1"
  local relative="$2"
  local content="$3"

  mkdir -p "${root}/$(dirname "${relative}")"
  printf '%s' "${content}" >"${root}/${relative}"
}

# --- the real tree, through the shipped wrapper -----------------------------
# A scanner-level probe, not a predicate probe: these are the shipped gate's own
# summary numbers over the real scanned surfaces.
expect_success_contains "surfaces 2" bash "${wrapper}"
expect_success_contains "declarations 4" bash "${wrapper}"
expect_success_contains "deprecated set 18" bash "${wrapper}"
expect_success_contains "supported set 8" bash "${wrapper}"

# There is no waiver machinery: the shipped gate refuses arguments...
expect_failure_contains \
  "this gate takes no arguments" \
  bash "${wrapper}" --allowlist "${tmpdir}/never-written.txt"

# ...and so does the checker, so there is no positional scope argument that
# could narrow the scan behind the wrapper's back.
expect_failure_contains \
  "this gate takes no arguments" \
  node "${checker}" "${tmpdir}/somewhere.ts"

# --- synthetic repository root ---------------------------------------------
synthetic="${tmpdir}/repo"
mkdir -p "${synthetic}/scripts"
cp "${checker}" "${synthetic}/scripts/check-lambda-runtime-deprecations.mjs"
synthetic_checker="${synthetic}/scripts/check-lambda-runtime-deprecations.mjs"

# A scope with no scan root fails closed instead of scanning nothing and
# reporting a pass, through the shipped path and through the self-test alike.
expect_failure_contains \
  "scan root examples/cdk-multilang/lib is missing" \
  node "${synthetic_checker}"
expect_failure_contains \
  "scan root examples/cdk-multilang/lib is missing" \
  node "${synthetic_checker}" --self-test

# A healthy synthetic tree: both declared surfaces present, both current.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.NODEJS_24_X,
"
write_surface "${synthetic}" "${surface_b}" "runtime: lambda.Runtime.PROVIDED_AL2023,
"
expect_success_contains "surfaces 2" node "${synthetic_checker}"
expect_success_contains "declarations 2" node "${synthetic_checker}"
expect_success_contains \
  "self-test 35 synthetic surfaces + 2 scope cases" \
  node "${synthetic_checker}" --self-test

# --- deprecated declarations ------------------------------------------------
# A deprecated declaration fails and is reported at file:line with the idiom the
# surface actually used.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.NODEJS_18_X,
"
expect_failure_contains \
  "${surface_a}:1 lambda.Runtime.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# The python and provided.al families are modelled too, so those deprecations
# are caught rather than falling through as unmodelled.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.PYTHON_3_8,
"
expect_failure_contains \
  "declares 'python3.8', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.PROVIDED_AL2,
"
expect_failure_contains \
  "declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

# A literal form is judged by the same rule.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.fromString('python3.9'),
"
expect_failure_contains \
  "declares 'python3.9', which AWS has deprecated" \
  node "${synthetic_checker}"

# --- receiver idioms cannot bypass the classifier ---------------------------
# A named import and a local alias both declare a runtime. Before the receiver
# list was widened these declared nothing, so a surface written that way escaped
# the classifier and the coverage walk alike.
write_surface "${synthetic}" "${surface_a}" "import { Runtime } from 'aws-cdk-lib/aws-lambda';
runtime: Runtime.NODEJS_18_X,
"
expect_failure_contains \
  "Runtime.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "import * as lambda from 'aws-cdk-lib/aws-lambda';
const R = lambda.Runtime;
runtime: R.PYTHON_3_8,
"
expect_failure_contains \
  "R.PYTHON_3_8 declares 'python3.8', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "import { Runtime as R } from 'aws-cdk-lib/aws-lambda';
runtime: R.NODEJS_18_X,
"
expect_failure_contains \
  "R.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# --- unmodelled surfaces fail closed ---------------------------------------
# The same idiom written into a file nobody declared must be caught by the
# coverage walk, which is the half that makes an undeclared surface impossible.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.NODEJS_24_X,
"
write_surface "${synthetic}" "examples/cdk-multilang/lib/scratch-named-import.ts" "import { Runtime } from 'aws-cdk-lib/aws-lambda';
runtime: Runtime.NODEJS_24_X,
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/cdk-multilang/lib/scratch-named-import.ts" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "import * as lambda from 'aws-cdk-lib/aws-lambda';
const R = lambda.Runtime;
runtime: R.PROVIDED_AL2023,
"
write_surface "${synthetic}" "examples/cdk-multilang/lib/scratch-alias.ts" "runtime: lambda.Runtime.NODEJS_24_X,
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/cdk-multilang/lib/scratch-alias.ts" \
  node "${synthetic_checker}"
rm -f \
  "${synthetic}/examples/cdk-multilang/lib/scratch-named-import.ts" \
  "${synthetic}/examples/cdk-multilang/lib/scratch-alias.ts"

# --- declarations the model cannot read fail closed -------------------------
# A dynamic runtime leaves the surface with no readable declaration, so the gate
# refuses rather than judging nothing and passing.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.fromString(runtimeName),
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# Binding the namespace and then hiding the member is the same outcome.
write_surface "${synthetic}" "${surface_a}" "import { Runtime } from 'aws-cdk-lib/aws-lambda';
runtime: Runtime[legacyRuntimeName],
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# A runtime enum the model does not know is a declaration, not a skip.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.RUBY_3_2,
"
expect_failure_contains \
  "names the runtime enum RUBY_3_2" \
  node "${synthetic_checker}"

# So is a family with no data: an AWS-preview runtime is not silently admitted.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.fromString('nodejs26.x'),
"
expect_failure_contains \
  "declares 'nodejs26.x', which is neither a deprecated runtime nor a supported pinned runtime" \
  node "${synthetic_checker}"

# The moving alias fails on its own reason rather than being read as deprecated.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.NODEJS_LATEST,
"
expect_failure_contains \
  "names the moving alias lambda.Runtime.NODEJS_LATEST" \
  node "${synthetic_checker}"

# --- a property read is not a second declaration ----------------------------
# `lambda.Runtime.PROVIDED_AL2023.bundlingImage` reads the bundling image of the
# runtime the `runtime:` line already declared. Declarations are counted per
# surface by the runtime they resolve to, so adding the property read must not
# move the count.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.PROVIDED_AL2023,
"
write_surface "${synthetic}" "${surface_b}" "runtime: lambda.Runtime.PROVIDED_AL2023,
"
expect_success_contains "declarations 2" node "${synthetic_checker}"

# The same tree with the bundling-image read added: still two, not three.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.PROVIDED_AL2023,
code: lambda.Code.fromAsset(dir, {
  bundling: { image: lambda.Runtime.PROVIDED_AL2023.bundlingImage },
}),
"
expect_success_contains "declarations 2" node "${synthetic_checker}"

# A deprecated runtime read the same way is still one violation, not two, and
# the property read does not hide it.
write_surface "${synthetic}" "${surface_a}" "runtime: lambda.Runtime.NODEJS_18_X,
bundling: { image: lambda.Runtime.NODEJS_18_X.bundlingImage },
"
write_surface "${synthetic}" "${surface_b}" "runtime: lambda.Runtime.PROVIDED_AL2023,
"
expect_failure_contains \
  "1 Lambda runtime declaration(s) are deprecated or unmodelled (2 declarations across 2 surfaces)" \
  node "${synthetic_checker}"

echo "lambda-runtime-deprecations-policy-test: PASS"
