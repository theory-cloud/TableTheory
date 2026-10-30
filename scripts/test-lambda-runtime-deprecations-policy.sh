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

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

# shellcheck source=scripts/lib/ensure-node-scanner-deps.sh
source scripts/lib/ensure-node-scanner-deps.sh
if ! ensure_node_scanner_deps "${repo_root}"; then
  echo "lambda-runtime-deprecations-policy-test: FAIL (pinned scanner dependencies are unavailable)" >&2
  exit 1
fi

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

# expect_failure_reports_a_fail_line <expected substring> <command...>
# Same as expect_failure_contains, plus the assertion that the failure really is
# the gate's own FAIL line rather than an uncaught error's stack trace. The
# message substring alone cannot prove that: node prints an uncaught error's
# message inside its stack trace, so a substring probe passes on the very output
# this rejects.
expect_failure_reports_a_fail_line() {
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
  if ! grep -Fq "lambda-runtime-deprecations: FAIL (" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "lambda-runtime-deprecations-policy-test: expected the gate's own 'lambda-runtime-deprecations: FAIL (' line"
    exit 1
  fi
  local frames
  frames="$(grep -E '^[[:space:]]+at .*\(|node:internal/|^Node\.js v' <<<"${output}" || true)"
  if [[ -n "${frames}" ]]; then
    printf '%s\n' "${output}"
    echo "lambda-runtime-deprecations-policy-test: failure was reported as a node stack trace, not the gate's FAIL line:"
    printf '%s\n' "${frames}"
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

# write_surface_verbatim <repository root> <relative path> < content-on-stdin>
# For content that contains backticks or dollar signs, where shell quoting would
# otherwise mangle a fenced snippet.
write_surface_verbatim() {
  local root="$1"
  local relative="$2"

  mkdir -p "${root}/$(dirname "${relative}")"
  cat >"${root}/${relative}"
}

# The declared surfaces of the two classes added with the SAM/snippet scope,
# restated here so a probe that targets one of them fails loudly if the scope
# moves.
template_a="examples/blog/deployment/sam-template.yaml"
template_b="examples/multi-tenant/deployment/template.yaml"
snippet_a="examples/lambda/README.md"

# write_healthy_tree <repository root>
# Every declared surface of every class, present and current: 5 surfaces and 5
# declarations (one per CDK stack, one per template, one snippet flag).
write_healthy_tree() {
  local root="$1"

  mkdir -p "${root}/scripts" "${root}/docs"
  write_surface "${root}" "${surface_a}" "const fn = { runtime: lambda.Runtime.NODEJS_24_X };
"
  write_surface "${root}" "${surface_b}" "const fn = { runtime: lambda.Runtime.PROVIDED_AL2023 };
"
  write_surface "${root}" "${template_a}" "Globals:
  Function:
    Runtime: provided.al2023
"
  write_surface "${root}" "${template_b}" "Globals:
  Function:
    Runtime: provided.al2023
"
  write_surface_verbatim "${root}" "${snippet_a}" <<'SNIPPET'
### Deploy with AWS CLI

```bash
aws lambda create-function \
  --function-name theorydb-example \
  --runtime provided.al2023 \
  --handler bootstrap
```
SNIPPET
}

# --- the real tree, through the shipped wrapper -----------------------------
# A scanner-level probe, not a predicate probe: these are the shipped gate's own
# summary numbers over the real scanned surfaces.
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" bash "${wrapper}"
expect_success_contains "declarations 7" bash "${wrapper}"
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
# The checker resolves the pinned TypeScript compiler and YAML parser from
# ts/package.json. Link the repository's installed, lock-pinned packages into
# the synthetic root so the copy runs the shipped extraction path.
ln -s "${repo_root}/ts" "${synthetic}/ts"
synthetic_checker="${synthetic}/scripts/check-lambda-runtime-deprecations.mjs"

# A scope with no scan root fails closed instead of scanning nothing and
# reporting a pass, through the shipped path and through the self-test alike.
# Asserted as a real FAIL line rather than a message substring: an uncaught
# GateFailure prints its message inside a node stack trace, so a substring probe
# would pass on exactly the defect this asserts against. The bare path is the one
# that exercises it, because the self-test reads a root it walks itself.
expect_failure_reports_a_fail_line \
  "scan root examples/cdk-multilang/lib is missing" \
  node "${synthetic_checker}"
expect_failure_reports_a_fail_line \
  "scan root examples/cdk-multilang/lib is missing" \
  node "${synthetic_checker}" --self-test

# A healthy synthetic tree: every declared surface of every class present and
# current.
write_healthy_tree "${synthetic}"
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"
expect_success_contains "declarations 5" node "${synthetic_checker}"
expect_success_contains \
  "self-test 88 synthetic surfaces + 6 scope cases + 1 entry-point probe" \
  node "${synthetic_checker}" --self-test

# --- templates and snippets are judged, not merely walked -------------------
# The class this gate could not see before: a SAM Globals runtime is a YAML
# scalar, so `Runtime: provided.al2` shipped while every check stayed green.
write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: provided.al2
"
expect_failure_contains \
  "${template_a}:3 Runtime: provided.al2 declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

# The same value in a normally formatted CloudFormation JSON template resolves
# through the same rule.
write_surface "${synthetic}" "${template_a}" "{
  \"Resources\": {
    \"Function\": {
      \"Properties\": {
        \"Runtime\": \"provided.al2\"
      }
    }
  }
}
"
expect_failure_contains \
  "declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: ruby3.2
"
expect_failure_contains \
  "declares 'ruby3.2', which is neither a deprecated runtime nor a supported pinned runtime" \
  node "${synthetic_checker}"

# A copy-paste deploy snippet is judged at the flag it writes, inside its fence.
write_surface_verbatim "${synthetic}" "${snippet_a}" <<'SNIPPET'
### Deploy with AWS CLI

```bash
aws lambda create-function \
  --function-name theorydb-example \
  --runtime provided.al2 \
  --handler bootstrap
```
SNIPPET
expect_failure_contains \
  "${snippet_a}:6 --runtime provided.al2 declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

# Prose is not a deployment: the same sentence outside a fence declares nothing,
# so a doc whose only runtime is prose fails closed instead of passing on it.
write_surface "${synthetic}" "${snippet_a}" "Deploy with \`--runtime provided.al2\` when you need it.
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# A template whose runtime is a deploy-time parameter declares nothing readable
# and fails closed rather than being judged by a guess.
write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: !Ref RuntimeParameter
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# --- unmodelled surfaces of the new classes fail closed --------------------
# A new template nobody declared is caught by the walk over the template roots.
write_healthy_tree "${synthetic}"
write_surface "${synthetic}" "examples/blog/deployment/extra-template.yaml" "Globals:
  Function:
    Runtime: provided.al2023
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/blog/deployment/extra-template.yaml; add each to SCANNED_TEMPLATE_SURFACES" \
  node "${synthetic_checker}"
rm -f "${synthetic}/examples/blog/deployment/extra-template.yaml"

# A new deploy doc nobody declared is caught by the walk over the snippet roots,
# including one that only reaches `docs`.
write_surface "${synthetic}" "docs/examples/new-deploy.md" "\`\`\`bash
aws lambda create-function --runtime provided.al2023 --handler bootstrap
\`\`\`
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: docs/examples/new-deploy.md; add each to SCANNED_SNIPPET_SURFACES" \
  node "${synthetic_checker}"
rm -rf "${synthetic}/docs/examples"

# A declared surface that declares nothing is refused, so the scope cannot be
# narrowed by emptying a template.
write_healthy_tree "${synthetic}"
write_surface "${synthetic}" "${template_b}" "# Runtime: provided.al2023
Description: no runtime here
"
expect_failure_contains \
  "declared surface ${template_b} declares no modelled Lambda runtime" \
  node "${synthetic_checker}"
write_healthy_tree "${synthetic}"

# --- deprecated declarations ------------------------------------------------
# A deprecated declaration fails and is reported at file:line with the idiom the
# surface actually used.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.NODEJS_18_X };
"
expect_failure_contains \
  "${surface_a}:1 lambda.Runtime.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# The python and provided.al families are modelled too, so those deprecations
# are caught rather than falling through as unmodelled.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.PYTHON_3_8 };
"
expect_failure_contains \
  "declares 'python3.8', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.PROVIDED_AL2 };
"
expect_failure_contains \
  "declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

# A literal form is judged by the same rule.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.fromString('python3.9') };
"
expect_failure_contains \
  "declares 'python3.9', which AWS has deprecated" \
  node "${synthetic_checker}"

# --- receiver idioms cannot bypass the classifier ---------------------------
# A named import and a local alias both declare a runtime. Before the receiver
# list was widened these declared nothing, so a surface written that way escaped
# the classifier and the coverage walk alike.
write_surface "${synthetic}" "${surface_a}" "import { Runtime } from 'aws-cdk-lib/aws-lambda';
const fn = { runtime: Runtime.NODEJS_18_X };
"
expect_failure_contains \
  "Runtime.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "import * as lambda from 'aws-cdk-lib/aws-lambda';
const R = lambda.Runtime;
const fn = { runtime: R.PYTHON_3_8 };
"
expect_failure_contains \
  "R.PYTHON_3_8 declares 'python3.8', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "import { Runtime as R } from 'aws-cdk-lib/aws-lambda';
const fn = { runtime: R.NODEJS_18_X };
"
expect_failure_contains \
  "R.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# --- unmodelled surfaces fail closed ---------------------------------------
# The same idiom written into a file nobody declared must be caught by the
# coverage walk, which is the half that makes an undeclared surface impossible.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.NODEJS_24_X };
"
write_surface "${synthetic}" "examples/cdk-multilang/lib/scratch-named-import.ts" "import { Runtime } from 'aws-cdk-lib/aws-lambda';
const fn = { runtime: Runtime.NODEJS_24_X };
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/cdk-multilang/lib/scratch-named-import.ts" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "import * as lambda from 'aws-cdk-lib/aws-lambda';
const R = lambda.Runtime;
const fn = { runtime: R.PROVIDED_AL2023 };
"
write_surface "${synthetic}" "examples/cdk-multilang/lib/scratch-alias.ts" "const fn = { runtime: lambda.Runtime.NODEJS_24_X };
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
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.fromString(runtimeName) };
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# Binding the namespace and then hiding the member is the same outcome.
write_surface "${synthetic}" "${surface_a}" "import { Runtime } from 'aws-cdk-lib/aws-lambda';
const fn = { runtime: Runtime[legacyRuntimeName] };
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# A runtime enum the model does not know is a declaration, not a skip.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.RUBY_3_2 };
"
expect_failure_contains \
  "names the runtime enum RUBY_3_2" \
  node "${synthetic_checker}"

# So is a family with no data: an AWS-preview runtime is not silently admitted.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.fromString('nodejs26.x') };
"
expect_failure_contains \
  "declares 'nodejs26.x', which is neither a deprecated runtime nor a supported pinned runtime" \
  node "${synthetic_checker}"

# The moving alias fails on its own reason rather than being read as deprecated.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.NODEJS_LATEST };
"
expect_failure_contains \
  "names the moving alias lambda.Runtime.NODEJS_LATEST" \
  node "${synthetic_checker}"

# --- a property read is not a second declaration ----------------------------
# `lambda.Runtime.PROVIDED_AL2023.bundlingImage` reads the bundling image of the
# runtime the `runtime:` line already declared. Declarations are counted per
# surface by the runtime they resolve to, so adding the property read must not
# move the count: 5 declarations across the 5 surfaces, not 6.
write_healthy_tree "${synthetic}"
expect_success_contains "declarations 5" node "${synthetic_checker}"

# The same tree with the bundling-image read added: still five, not six.
write_surface "${synthetic}" "${surface_a}" "const fn = {
  runtime: lambda.Runtime.PROVIDED_AL2023,
  code: lambda.Code.fromAsset(dir, {
    bundling: { image: lambda.Runtime.PROVIDED_AL2023.bundlingImage },
  }),
};
"
expect_success_contains "declarations 5" node "${synthetic_checker}"

# A deprecated runtime read the same way is still one violation, not two, and
# the property read does not hide it.
write_surface "${synthetic}" "${surface_a}" "const fn = {
  runtime: lambda.Runtime.NODEJS_18_X,
  bundling: { image: lambda.Runtime.NODEJS_18_X.bundlingImage },
};
"
expect_failure_contains \
  "1 Lambda runtime declaration(s) are deprecated or unmodelled (5 declarations across 5 surfaces)" \
  node "${synthetic_checker}"
write_healthy_tree "${synthetic}"

# --- comments cannot hide a declaration, nor invent one ---------------------
# The original hostile wrote a block comment between `fromString(` and its
# argument inside a new, undeclared stack, so the coverage walk saw no
# declaration to judge and never opened the file. The commented literal is a real
# declaration, so the undeclared surface is now caught.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.NODEJS_24_X };
"
write_surface "${synthetic}" "${surface_b}" "const fn = { runtime: lambda.Runtime.PROVIDED_AL2023 };
"
write_surface "${synthetic}" "examples/cdk-multilang/lib/scratch-commented.ts" "const fn = { runtime: lambda.Runtime.fromString(/* pinned */ 'nodejs18.x') };
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/cdk-multilang/lib/scratch-commented.ts" \
  node "${synthetic_checker}"
rm -f "${synthetic}/examples/cdk-multilang/lib/scratch-commented.ts"

# The same commented literal on a declared surface is judged, not missed.
write_surface "${synthetic}" "${surface_a}" "const fn = { runtime: lambda.Runtime.fromString(/* pinned */ 'nodejs18.x') };
"
expect_failure_contains \
  "declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# A declaration that is entirely commented out does not count, so a surface left
# with no readable declaration fails closed instead of passing on a comment. Both
# comment forms are pinned.
write_surface "${synthetic}" "${surface_a}" "// const fn = { runtime: lambda.Runtime.NODEJS_24_X };
"
write_surface "${synthetic}" "${surface_b}" "const fn = { runtime: lambda.Runtime.PROVIDED_AL2023 };
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${surface_a}" "/* const fn = { runtime: lambda.Runtime.PYTHON_3_8 }; */
"
expect_failure_contains \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# Ordinary supported forms behind comments, and a URL inside a string, are
# unaffected.
write_surface "${synthetic}" "${surface_a}" "const endpoint = \"https://runtime.example.com/path\";
const fn = { runtime: lambda.Runtime.fromString(/* keep */ 'nodejs24.x') }; // current
"
write_surface "${synthetic}" "${surface_b}" "const fn = { runtime: lambda.Runtime.PROVIDED_AL2023 };
"
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"

# --- hostile source shapes (R37) --------------------------------------------
# A regex character class can contain the character pairs a comment stripper
# treats as comment openers. The declaration beside it must still be judged.
write_healthy_tree "${synthetic}"
write_surface "${synthetic}" "${surface_a}" "const marker = /[//]/; const re = /[/*]/; const fn = { runtime: lambda.Runtime.NODEJS_18_X };
"
expect_failure_reports_a_fail_line \
  "${surface_a}:1 lambda.Runtime.NODEJS_18_X declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# A template expression is executable code, not opaque text.
write_surface_verbatim "${synthetic}" "${surface_a}" <<'TS'
const note = `${lambda.Runtime.NODEJS_18_X}`;
const fn = { runtime: lambda.Runtime.NODEJS_24_X };
TS
expect_failure_reports_a_fail_line \
  "declares 'nodejs18.x', which AWS has deprecated" \
  node "${synthetic_checker}"

# Ordinary division is not a comment opener and does not change the verdict.
write_healthy_tree "${synthetic}"
write_surface "${synthetic}" "${surface_a}" "const ratio = total / count; const fn = { runtime: lambda.Runtime.NODEJS_24_X };
"
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"

# A source the parser rejects fails closed even when the recovered text still
# contains a supported runtime.
write_surface "${synthetic}" "${surface_a}" "function ( {
const fn = { runtime: lambda.Runtime.NODEJS_24_X };
"
expect_failure_reports_a_fail_line \
  "could not be parsed as TypeScript" \
  node "${synthetic_checker}"
write_healthy_tree "${synthetic}"

# --- hostile template shapes (R50) ------------------------------------------
# Flow YAML and compact JSON never put Runtime at the start of a line.
write_surface "${synthetic}" "${template_a}" "{Globals: {Function: {Runtime: provided.al2}}}
"
expect_failure_reports_a_fail_line \
  "declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${template_a}" "{Globals: {Function: {Runtime: provided.al2023}}}
"
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"

write_surface "${synthetic}" "${template_a}" '{"Resources":{"Function":{"Properties":{"Runtime":"provided.al2"}}}}
'
expect_failure_reports_a_fail_line \
  "declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

write_surface "${synthetic}" "${template_a}" '{"Resources":{"Function":{"Properties":{"Runtime":"provided.al2023"}}}}
'
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"

# The resource runtime is effective over the global. A supported global does not
# hide a deprecated resource override.
write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: provided.al2023
Resources:
  Fn:
    Type: AWS::Serverless::Function
    Properties:
      Runtime: provided.al2
"
expect_failure_reports_a_fail_line \
  "declares 'provided.al2', which AWS has deprecated" \
  node "${synthetic_checker}"

# A deprecated global that every runtime-bearing resource overrides is not the
# effective runtime, so it does not fail.
write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: provided.al2
Resources:
  Fn:
    Type: AWS::Serverless::Function
    Properties:
      Runtime: provided.al2023
"
expect_success_contains "declarations 5" node "${synthetic_checker}"

# A dynamic resource override is what deploys, so a supported global cannot
# rescue it.
write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: provided.al2023
Resources:
  Fn:
    Type: AWS::Serverless::Function
    Properties:
      Runtime: !Ref RuntimeParameter
"
expect_failure_reports_a_fail_line \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"

# The same ambiguity in JSON, in an undeclared template, is discovered by the
# coverage walk and fails closed on the dynamic effective runtime.
write_surface "${synthetic}" "examples/blog/deployment/extra.json" '{"Resources":{"Fn":{"Type":"AWS::Serverless::Function","Properties":{"Runtime":{"Ref":"RuntimeParameter"}}}}}'
expect_failure_reports_a_fail_line \
  "declares no modelled Lambda runtime" \
  node "${synthetic_checker}"
rm -f "${synthetic}/examples/blog/deployment/extra.json"

# A malformed declared template is parse ambiguity, not an empty template.
write_surface "${synthetic}" "${template_a}" "Globals:
  Function:
    Runtime: provided.al2023
Resources: [
"
expect_failure_reports_a_fail_line \
  "could not be parsed as YAML" \
  node "${synthetic_checker}"

# An undeclared flow template is still discovered, and one that declares a
# deprecated runtime is judged rather than skipped.
write_healthy_tree "${synthetic}"
write_surface "${synthetic}" "examples/blog/deployment/extra.yaml" "{Globals: {Function: {Runtime: provided.al2}}}
"
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/blog/deployment/extra.yaml" \
  node "${synthetic_checker}"
rm -f "${synthetic}/examples/blog/deployment/extra.yaml"

# The same is true of a compact JSON template nobody declared.
write_surface "${synthetic}" "examples/blog/deployment/extra.json" '{"Resources":{"Function":{"Properties":{"Runtime":"provided.al2"}}}}'
expect_failure_contains \
  "unmodelled surface(s) declare a Lambda runtime but are not judged: examples/blog/deployment/extra.json" \
  node "${synthetic_checker}"
rm -f "${synthetic}/examples/blog/deployment/extra.json"

# An unrelated JSON/YAML document is not a template: an invalid-JSON file with
# no CloudFormation marker and no Runtime key is ignored, not parsed as a
# template and not reported.
write_surface "${synthetic}" "docs/search.json" "---
layout: null
---
No runtime here
"
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"
rm -f "${synthetic}/docs/search.json"

# Ordinary valid sources and joined work keep passing after the hostile cases.
write_healthy_tree "${synthetic}"
expect_success_contains "surfaces 5 (cdk 2, template 2, snippet 1)" node "${synthetic_checker}"

echo "lambda-runtime-deprecations-policy-test: PASS"
