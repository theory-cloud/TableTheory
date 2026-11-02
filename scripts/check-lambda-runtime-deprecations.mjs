// Purpose: fail when a TableTheory surface that declares an AWS Lambda runtime
// declares one AWS has deprecated.
//
// ===========================================================================
// Why this gate exists
// ===========================================================================
// AWS deprecates Lambda runtimes on a published schedule, and CDK reports the
// move as a synth-time WARNING. A warning is not a gate: it is easy to read past
// for years while every check stays green. The real deadline is not "the
// function stops working" but "you can no longer create a function on that
// runtime", which is a hard stop for a new stack, a new region, or a
// disaster-recovery rebuild. This gate turns the warning into a failure at PR
// time, before the declaration lands.
//
// ===========================================================================
// The deprecated set, and where its data comes from
// ===========================================================================
// DEPRECATED_LAMBDA_RUNTIMES below is the one place the deprecated set lives,
// and SUPPORTED_LAMBDA_RUNTIMES is the matching positive set. A declared
// runtime in neither set fails the gate, so admitting a runtime is a deliberate
// edit to this file rather than something an unmodelled value slips past.
//
// Every identifier below is cited against AWS's published runtime tables:
//
//   https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html
//
// "Deprecated runtimes" (reached end of support) supplies the deprecated set:
//
//   nodejs            Node.js 0.10   deprecated 2016-08-30
//   nodejs4.3         Node.js 4.3    deprecated 2020-03-05
//   nodejs4.3-edge    Node.js 4.3e   deprecated 2020-03-05
//   nodejs6.10        Node.js 6.10   deprecated 2019-08-12
//   nodejs8.10        Node.js 8.10   deprecated 2020-03-06
//   nodejs10.x        Node.js 10     deprecated 2021-07-30
//   nodejs12.x        Node.js 12     deprecated 2023-03-31
//   nodejs14.x        Node.js 14     deprecated 2023-12-04
//   nodejs16.x        Node.js 16     deprecated 2024-06-12
//   nodejs18.x        Node.js 18     deprecated 2025-09-01
//   nodejs20.x        Node.js 20     deprecated 2026-04-30
//   python2.7         Python 2.7     deprecated 2021-07-15
//   python3.6         Python 3.6     deprecated 2022-07-18
//   python3.7         Python 3.7     deprecated 2023-12-04
//   python3.8         Python 3.8     deprecated 2024-10-14
//   python3.9         Python 3.9     deprecated 2025-12-15
//   provided          OS-only (AL1)  deprecated 2024-01-08
//   provided.al2      OS-only (AL2)  deprecated 2026-07-31
//
// "Supported runtimes" supplies the supported set for the same three families,
// each with its AWS-projected deprecation date so a later editor can see how
// much runway each entry has left:
//
//   nodejs22.x        projected 2027-04-30
//   nodejs24.x        projected 2028-04-30
//   python3.10        projected 2026-10-31
//   python3.11        projected 2027-06-30
//   python3.12        projected 2028-10-31
//   python3.13        projected 2029-06-30
//   python3.14        projected 2029-06-30
//   provided.al2023   projected 2029-06-30
//
// Two families AWS lists as preview rather than generally available
// (nodejs26.x and python3.15, both "Not scheduled") are deliberately NOT in the
// supported set. A preview runtime is not covered by the Lambda SLA, so a
// declaration of one fails closed as unmodelled and has to be admitted here
// deliberately once AWS makes it generally available.
//
// TableTheory ships non-nodejs runtimes - python3.14 and provided.al2023 are
// declared in examples/cdk-multilang today - so the model is extended past
// FaceTheory's nodejs-only set. The other AWS families (ruby, java, dotnet,
// go) are visible to the classifier but carry no data here, so a declaration
// in one of them fails closed rather than being skipped.
//
// A quoted literal is a declaration only as the argument of `fromString` on one
// of the receivers the Scope section lists. Scoping it that way is what keeps
// the rule honest in both directions: an unmodelled argument
// (`fromString('rust1.0')`, or a family with no version such as
// `fromString('provided')`) is recorded and fails closed instead of vanishing,
// while a runtime-shaped string that is not a runtime argument - a log message,
// a description, `'nodejs22.x'` beside a genuinely dynamic
// `fromString(config.runtime)` - is no longer read as a declaration that rescues
// the surface. Two consequences are deliberate rather than oversights:
// `fromString` with a non-literal argument declares nothing readable, and a call
// whose receiver is not traceable to a Runtime binding (a bare
// `fromString('nodejs18.x')` in a helper) is not read either. Both leave the
// surface without the declaration that would otherwise be judged, so they end in
// the same fail-closed rule as an obfuscated surface.
//
// ===========================================================================
// Scope
// ===========================================================================
// SCANNED_SURFACES are the surfaces that decide the runtime of the Lambda
// functions TableTheory ships:
//
//   examples/cdk-multilang/lib/multilang-demo-stack.ts
//   examples/cdk-multilang/lib/tabletheory-ttl-archive.ts
//
// SCANNED_TEMPLATE_SURFACES and SCANNED_SNIPPET_SURFACES are the same thing for
// SAM/CloudFormation templates and for fenced deploy snippets in the docs; both
// classes are described under "Template and deploy-snippet surfaces" below.
//
// Coverage is enforced rather than assumed. Each class owns scan roots, which
// are walked, and every file under them that declares a Lambda runtime must be a
// declared surface of that class; a file
// that declares one without being listed fails the gate, so a new CDK stack
// cannot arrive declaring a runtime nobody judges. Symmetrically, a declared
// surface must exist and must still declare at least one modelled runtime, so
// the gate cannot be neutered by deleting a line or by switching a surface to a
// runtime value the model cannot read.
//
// "Declares a runtime" means the same thing to the coverage walk and to the
// per-surface scan, and it covers the receiver forms a real surface uses:
// `lambda.Runtime.X`, a named import's `Runtime.X`, `lambda['Runtime'].X`, a
// renamed destructure's `const { Runtime: RT } = lambda` with `RT.X`, and
// either of the first two aliased to a local name through any number of hops
// (`const R = lambda.Runtime`, then `const R2 = R`) - plus a runtime literal
// passed to `fromString` on one of those receivers, which is the only reading
// the literal rule has. A surface that binds the namespace and then hides the
// member behind a computed index, a lookup table, or a helper declares nothing
// the model can read, which fails closed rather than passing unjudged.
//
// A form the receiver model cannot read declares nothing, and what that costs
// depends on whether it was the surface's LAST modelled declaration. Hiding the
// only one - behind an index, a lookup table, or a helper - leaves the surface
// with no modelled runtime, which fails closed. Hiding one of several does not:
// the remaining declarations still satisfy the coverage walk, so this checker
// passes, and the obfuscated runtime is caught instead by the policy test's
// `declarations 7` count pin over the real surfaces. The multi-declaration case
// is therefore a caught regression rather than a silent one, but it is caught by
// that pin, not by the coverage walk.
//
// Five forms stay outside the model, measured as declaring nothing. They are
// known limits of this classifier rather than waivers:
//
//   R[process.env.NAME]           a computed key: not a literal member access
//   Runtime lookups / helpers     a runtime reached through a table or a call
//   const RT = lambda['Runtime']  a `['Runtime']` read bound to a local name and
//                                 used later: `RT.X` is not a receiver form,
//                                 because the binder chases names bound to the
//                                 enum or the namespace, not a member read
//   R['PYTHON_3_8']               an enum member read through an index: only the
//                                 `.` member form is modelled
//   fromString('nodejs20.x')      a literal on no receiver at all: the literal
//                                 rule reads only a call on a Runtime receiver,
//                                 so this is not a declaration either
//
// Plain `const { Runtime } = lambda` IS caught, because the bare `Runtime`
// receiver is fixed rather than alias-derived.
//
// A member access that is followed by a property read is the same declaration,
// not a second one: `lambda.Runtime.PROVIDED_AL2023.bundlingImage` reads the
// bundling image of the runtime the surrounding `runtime:` line already
// declared. Declarations are therefore de-duplicated per surface by the
// runtime they resolve to (falling back to the member access as written when
// the runtime cannot be resolved), so counting a surface's declarations counts
// runtimes, not occurrences. This is a deliberate divergence from FaceTheory's
// checker, which counts the property read separately.
//
// ===========================================================================
// Template and deploy-snippet surfaces
// ===========================================================================
// Reading only the CDK stacks is why `Runtime: provided.al2` survived in the
// blog SAM template: the template was never a scanned surface, so no gate ever
// looked at the line that decides the deployed runtime. A SAM template, a
// CloudFormation template, and the copy-paste `aws lambda create-function`
// snippet in an example README decide a Lambda runtime exactly as directly as a
// CDK `runtime:` line does, so they are modelled here as two further surface
// classes.
//
// A template declaration is the EFFECTIVE `Runtime` of a Lambda-bearing
// resource, read structurally from YAML or CloudFormation JSON. A resource-level
// Runtime overrides SAM's Globals.Function.Runtime; the global is judged only
// when it reaches a resource. Flow YAML and compact JSON are the same documents
// as their block forms, so formatting cannot hide a declaration. The parser
// understands CloudFormation's intrinsic-function tags (`!Ref`, `!Sub`, ...)
// explicitly, and every template under a scan root is opened independently of
// whether another declaration parsed.
//
// A snippet declaration is the value of a `--runtime` flag, or a `Runtime:`
// scalar, inside a fenced Markdown code block. Prose outside a fence is never
// read: a runtime named in a sentence is documentation, not a deployable
// declaration. `--runtime-version` and `--runtime-import` are different flags
// that share a prefix, so the flag rule requires whitespace or `=` after
// `--runtime`.
//
// Both classes resolve to the same identifiers as a CDK literal, so they share
// DEPRECATED_LAMBDA_RUNTIMES and SUPPORTED_LAMBDA_RUNTIMES and the same three
// verdicts: deprecated, unmodelled (in neither set), or supported. Declarations
// are de-duplicated per surface by the runtime they resolve to, exactly as in a
// CDK surface, so a template counts runtimes rather than occurrences.
//
// An effective runtime that resolves only at deploy time - a CloudFormation tag,
// an interpolation, a collection such as {"Ref": "..."}, or a non-string scalar
// - fails closed on the "declares no modelled Lambda runtime" rule rather than
// being judged by a guess. A template that cannot be parsed fails closed when it
// is a declared surface or carries a CloudFormation marker; an unrelated
// YAML/JSON document that mentions no runtime is not a template. Commented-out
// declarations are trivia, not declarations. A file is dispatched by extension -
// scripts and stacks by .ts/.tsx/.js/.jsx/.mjs/.cjs/.mts/.cts, templates by
// .yaml/.yml/.json, snippets by .md/.markdown - so an extension this gate does
// not model is a known limit rather than a scanned surface.
//
// Fail-closed cases: a surface that is missing, unreadable, or declares no
// modelled runtime; a runtime literal in a modelled shape whose family or value
// is not modelled, including a `fromString` argument that resolves to no runtime
// at all and a template or snippet value that names no modelled runtime; an
// effective runtime that is ambiguous or a template that cannot be parsed; a
// declaration that names an unpinned moving alias rather than a pinned runtime;
// and any file inside a scan root that declares a Lambda runtime without being a
// declared surface.
//
// A coverage walk that cannot run is a gate failure in its own right, and it
// reports as this gate's FAIL line rather than as an uncaught stack trace: a
// missing scan root means the scope was never checked, which is the one outcome
// this gate must never present as a passing scan.
//
// The scope is owned by this checker and there is no allowlist, no waiver flag,
// and no exception list. EXPECTED_SCANNED_SURFACES and EXPECTED_SCAN_ROOTS are
// independent copies of the scope, so the self-test fails when either is
// narrowed. `--self-test` is the only argument, and it runs the classifier
// self-test plus synthetic surfaces driven through the real read path before
// the real scan. The checker reads no environment variable.
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

// The scanner's dependencies are the repository's own pinned packages:
// ts/package.json and ts/package-lock.json carry the TypeScript compiler and
// the YAML parser. Resolving from ts/package.json keeps the checker on those
// exact versions whether it runs from the repository root or from a policy
// test's synthetic copy that links ts/.
const requireFromTypeScriptPackage = createRequire(path.join(repositoryRoot, "ts", "package.json"));

let cachedTypeScript = null;
let cachedYaml = null;

function loadPinnedDependency(specifier, description) {
  try {
    return requireFromTypeScriptPackage(specifier);
  } catch (err) {
    throw new GateFailure(
      `pinned ${description} is unavailable from ts/node_modules (${specifier}): ${err.message}; ` +
        "run `npm --prefix ts ci --ignore-scripts` before scanning",
    );
  }
}

function typescript() {
  if (cachedTypeScript === null) {
    cachedTypeScript = loadPinnedDependency("typescript", "TypeScript compiler");
  }
  return cachedTypeScript;
}

function yamlParser() {
  if (cachedYaml === null) {
    cachedYaml = loadPinnedDependency("yaml", "YAML parser");
  }
  return cachedYaml;
}

// === the deprecated set, explicit in one place =============================
// Sourced from the "Deprecated runtimes" table at
// https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html
const DEPRECATED_LAMBDA_RUNTIMES = [
  // nodejs family
  "nodejs",
  "nodejs4.3",
  "nodejs4.3-edge",
  "nodejs6.10",
  "nodejs8.10",
  "nodejs10.x",
  "nodejs12.x",
  "nodejs14.x",
  "nodejs16.x",
  "nodejs18.x",
  "nodejs20.x",
  // python family
  "python2.7",
  "python3.6",
  "python3.7",
  "python3.8",
  "python3.9",
  // provided (OS-only) family
  "provided",
  "provided.al2",
];

// Sourced from the "Supported runtimes" table at the same page.
const SUPPORTED_LAMBDA_RUNTIMES = [
  // nodejs family
  "nodejs22.x",
  "nodejs24.x",
  // python family
  "python3.10",
  "python3.11",
  "python3.12",
  "python3.13",
  "python3.14",
  // provided (OS-only) family
  "provided.al2023",
];

const SCANNED_SURFACES = [
  "examples/cdk-multilang/lib/multilang-demo-stack.ts",
  "examples/cdk-multilang/lib/tabletheory-ttl-archive.ts",
];

// The scanned set, restated independently of SCANNED_SURFACES so the self-test
// catches a scan scope that was narrowed instead of widened.
const EXPECTED_SCANNED_SURFACES = [
  "examples/cdk-multilang/lib/multilang-demo-stack.ts",
  "examples/cdk-multilang/lib/tabletheory-ttl-archive.ts",
];

const SCAN_ROOTS = ["examples/cdk-multilang/lib"];

const EXPECTED_SCAN_ROOTS = ["examples/cdk-multilang/lib"];

// SAM and CloudFormation templates whose runtime declarations are judged.
const SCANNED_TEMPLATE_SURFACES = [
  "examples/blog/deployment/sam-template.yaml",
  "examples/multi-tenant/deployment/template.yaml",
];

// The template set, restated independently for the same reason.
const EXPECTED_SCANNED_TEMPLATE_SURFACES = [
  "examples/blog/deployment/sam-template.yaml",
  "examples/multi-tenant/deployment/template.yaml",
];

// Deploy docs whose fenced snippets are judged.
const SCANNED_SNIPPET_SURFACES = ["examples/lambda/README.md"];

const EXPECTED_SCANNED_SNIPPET_SURFACES = ["examples/lambda/README.md"];

// Every template and every deploy snippet under these roots has to be a declared
// surface of its class. `examples` covers the shipped deployment lanes and
// `docs` covers the guides that tell a reader how to deploy.
const TEMPLATE_SCAN_ROOTS = ["examples", "docs"];

const EXPECTED_TEMPLATE_SCAN_ROOTS = ["examples", "docs"];

const SNIPPET_SCAN_ROOTS = ["examples", "docs"];

const EXPECTED_SNIPPET_SCAN_ROOTS = ["examples", "docs"];

// Directory names the coverage walk does not descend into: dependency, build,
// coverage, vendored, and generated trees that hold no declaration of ours.
const SKIPPED_DIRECTORY_NAMES = new Set([
  "node_modules",
  "dist",
  "coverage",
  "cdk.out",
  ".cdk.staging",
  "vendor",
  ".venv",
  "__pycache__",
  ".mypy_cache",
  ".pytest_cache",
  ".ruff_cache",
]);

// CDK's enum names for the runtimes this gate models, read from the pinned
// aws-cdk-lib 2.270.0 Runtime class in examples/cdk-multilang. An enum name that
// is absent here fails the gate rather than being skipped, so a runtime has to
// be added deliberately.
//
// The map is deliberately narrower than the CDK Runtime class: it covers the
// nodejs, python, and provided families TableTheory deploys. RUBY_3_2,
// JAVA_17, GO_1_X and the rest fail closed as unmodelled enums, which is the
// "declarations are never skipped" contract rather than an oversight.
//
// CDK 2.270.0 has no NODEJS_4_3_EDGE member, so the AWS identifier
// `nodejs4.3-edge` is reachable only as a literal. That is why the self-test
// asserts the map is a subset of the two sets rather than that they are equal.
const LAMBDA_RUNTIME_ENUMS = new Map([
  // nodejs family
  ["NODEJS", "nodejs"],
  ["NODEJS_4_3", "nodejs4.3"],
  ["NODEJS_6_10", "nodejs6.10"],
  ["NODEJS_8_10", "nodejs8.10"],
  ["NODEJS_10_X", "nodejs10.x"],
  ["NODEJS_12_X", "nodejs12.x"],
  ["NODEJS_14_X", "nodejs14.x"],
  ["NODEJS_16_X", "nodejs16.x"],
  ["NODEJS_18_X", "nodejs18.x"],
  ["NODEJS_20_X", "nodejs20.x"],
  ["NODEJS_22_X", "nodejs22.x"],
  ["NODEJS_24_X", "nodejs24.x"],
  // python family
  ["PYTHON_2_7", "python2.7"],
  ["PYTHON_3_6", "python3.6"],
  ["PYTHON_3_7", "python3.7"],
  ["PYTHON_3_8", "python3.8"],
  ["PYTHON_3_9", "python3.9"],
  ["PYTHON_3_10", "python3.10"],
  ["PYTHON_3_11", "python3.11"],
  ["PYTHON_3_12", "python3.12"],
  ["PYTHON_3_13", "python3.13"],
  ["PYTHON_3_14", "python3.14"],
  // provided (OS-only) family
  ["PROVIDED", "provided"],
  ["PROVIDED_AL2", "provided.al2"],
  ["PROVIDED_AL2023", "provided.al2023"],
]);

// `NODEJS_LATEST` is not a pinned runtime: it is an alias that follows the CDK
// version, so a dependency bump would change what these stacks deploy without a
// line of stack code changing. That is the opposite of the pinned-runtime
// contract, so it fails on its own reason rather than being reported as
// deprecated.
const UNPINNED_RUNTIME_ALIAS_ENUMS = new Set(["NODEJS_LATEST"]);

// AWS's runtime families. The classifier is not an alternation of these names:
// the list is what makes a runtime-shaped `fromString` argument a declaration at
// all, so an argument in a family with no data below still reaches the
// classifier and fails closed instead of being invisible - `fromString('ruby3.2')`
// is a violation, not a skip. Keeping the list closed is deliberate, because
// loosening it further would turn unrelated identifiers such as the
// `target: 'node24'` bundling option into declarations. The membership test is
// part of the shape predicate rather than a filter over its output, so an
// argument outside the list is recorded with no resolved runtime and fails
// closed as an unmodelled literal:
// `fromString('rust1.0')` and `fromString('provided')` are violations, not
// skips. The header states both of those readings.
const LAMBDA_RUNTIME_FAMILIES = [
  "nodejs",
  "python",
  "ruby",
  "java",
  "dotnet",
  "dotnetcore",
  "go",
  "provided",
];

const UNMODELLED_DECLARATION_HINT =
  "add the runtime deliberately to DEPRECATED_LAMBDA_RUNTIMES or SUPPORTED_LAMBDA_RUNTIMES " +
  "in scripts/check-lambda-runtime-deprecations.mjs - declarations are never skipped";

class GateFailure extends Error {}

function fail(message) {
  console.error(`lambda-runtime-deprecations: FAIL (${message})`);
  process.exit(1);
}

// === declaration forms =====================================================

// Constant-style members only, so `lambda.Runtime.NODEJS_20_X` is a declaration
// while `lambda.Runtime.fromString(...)` is not: a method's argument is judged
// by the literal rule below, and an unmodelled dynamic runtime leaves the
// surface without a declaration, which fails closed on its own.
//
// The receiver is read in five forms, because requiring the literal text
// `lambda.Runtime.` left the others declaring nothing at all:
//
//   lambda.Runtime.NODEJS_20_X   a `lambda` namespace import's member
//   Runtime.NODEJS_20_X          a named import: `import { Runtime } from ...`
//   lambda['Runtime'].NODEJS_20_X
//                                 that member read through a quoted index
//   R.NODEJS_20_X                 any of the above aliased to a local name:
//                                 `const R = lambda.Runtime`,
//                                 `import { Runtime as R } from ...`,
//                                 `const { Runtime: R } = lambda`, or a second
//                                 hop, `const R2 = R` where `R` is an alias
//   R.fromString('nodejs20.x')    a runtime literal passed to the enum factory
//
// A new undeclared surface written in any of those idioms was invisible to the
// classifier and to the coverage walk alike, so it passed the gate without ever
// being judged. The receiver model is therefore built per surface from the AST:
// the fixed receivers plus whatever names that surface binds.
//
// What stays outside the model, deliberately: a member read through a computed
// key (`R[process.env.NAME]`), a runtime hidden behind a lookup table or a
// helper, a `['Runtime']` read bound to a local name and used later, an enum
// member read as `R['NODEJS_20_X']`, and a bare `fromString('nodejs20.x')` on no
// receiver. None of them yields a declaration, so a surface written that way
// fails closed on the "declares no modelled Lambda runtime" rule instead of
// passing unjudged.
//
// Every runtime enum this gate models is SCREAMING_CASE, and that is what keeps
// the receiver model from swallowing unrelated `Runtime.` namespaces and from
// reading a method or property name as a runtime: `Runtime.fromString` and
// `Runtime.PROVIDED_AL2023.bundlingImage` name methods and properties in
// lowerCamelCase, so neither matches.

// The identifier a `fromString` argument names, or null when the argument is not
// runtime-shaped. A runtime identifier is a known family followed by a version
// component, so `python3.14` and `provided.al2023` resolve while `node_modules`,
// `assets`, `go.mod`, and the `node24` esbuild bundling target do not - and
// neither does a family this gate does not model (`rust1.0`), nor a family with
// no version at all (`provided`, which is in the deprecated set but carries no
// version component). Returning null for those is what makes them fail closed as
// an unmodelled literal rather than disappear.
function runtimeIdentifierFromLiteral(value) {
  const match = /^([a-z][a-z0-9-]*?)(\.al[0-9]+|[0-9][0-9A-Za-z._-]*)$/.exec(value);
  if (match === null) return null;
  return LAMBDA_RUNTIME_FAMILIES.includes(match[1]) ? value : null;
}

function lineOf(text, index) {
  let line = 1;
  for (let cursor = 0; cursor < index; cursor += 1) {
    if (text[cursor] === "\n") line += 1;
  }
  return line;
}

function scriptKindFor(label, ts) {
  switch (path.extname(label).toLowerCase()) {
    case ".tsx":
      return ts.ScriptKind.TSX;
    case ".jsx":
      return ts.ScriptKind.JSX;
    case ".js":
    case ".mjs":
    case ".cjs":
      return ts.ScriptKind.JS;
    default:
      return ts.ScriptKind.TS;
  }
}

function stringLiteralValue(node, ts) {
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  return null;
}

// The receiver names a surface binds. Imports, destructures, and simple alias
// hops are collected first; `Runtime` and `lambda` are fixed forms because the
// classifier has always accepted them without an import. A name is only ever
// added to a set that already held its source, so cyclic aliases terminate.
function collectRuntimeReceiverModel(sourceFile, ts) {
  const enumAliases = new Set();
  const namespaces = new Set(["lambda"]);
  const enumPairs = [];
  const namespacePairs = [];

  const isNamespaceName = (name) => name === "lambda" || namespaces.has(name);
  const isEnumName = (name) => name === "Runtime" || enumAliases.has(name);
  const isNamespaceExpr = (expr) => ts.isIdentifier(expr) && isNamespaceName(expr.text);
  const isBindableEnumExpr = (expr) => {
    if (ts.isIdentifier(expr)) return isEnumName(expr.text);
    if (ts.isPropertyAccessExpression(expr) && expr.name.text === "Runtime") {
      return ts.isIdentifier(expr.expression) && isNamespaceExpr(expr.expression);
    }
    return false;
  };

  const visit = (node) => {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      node.moduleSpecifier.text.includes("aws-lambda")
    ) {
      const bindings = node.importClause?.namedBindings;
      if (bindings !== undefined) {
        if (ts.isNamespaceImport(bindings)) namespaces.add(bindings.name.text);
        if (ts.isNamedImports(bindings)) {
          for (const element of bindings.elements) {
            const imported = (element.propertyName ?? element.name).text;
            if (imported === "Runtime") enumAliases.add(element.name.text);
          }
        }
      }
    }

    if (ts.isVariableDeclaration(node) && node.initializer !== undefined) {
      if (ts.isIdentifier(node.name)) {
        if (isBindableEnumExpr(node.initializer)) {
          enumAliases.add(node.name.text);
        } else if (isNamespaceExpr(node.initializer)) {
          namespaces.add(node.name.text);
        } else if (ts.isIdentifier(node.initializer)) {
          enumPairs.push([node.name.text, node.initializer.text]);
          namespacePairs.push([node.name.text, node.initializer.text]);
        }
      } else if (ts.isObjectBindingPattern(node.name)) {
        // `const { Runtime: RT } = lambda` binds the enum under a new name. The
        // original text model accepts this regardless of the right-hand side,
        // so the AST model does too.
        for (const element of node.name.elements) {
          const key = element.propertyName ?? element.name;
          if (ts.isIdentifier(key) && key.text === "Runtime" && ts.isIdentifier(element.name)) {
            enumAliases.add(element.name.text);
          }
        }
      }
    }

    ts.forEachChild(node, visit);
  };
  visit(sourceFile);

  for (;;) {
    let changed = false;
    for (const [target, source] of namespacePairs) {
      if (!namespaces.has(target) && isNamespaceName(source)) {
        namespaces.add(target);
        changed = true;
      }
    }
    for (const [target, source] of enumPairs) {
      if (!enumAliases.has(target) && isEnumName(source)) {
        enumAliases.add(target);
        changed = true;
      }
    }
    if (!changed) break;
  }

  return { enumAliases, namespaces };
}

function isRuntimeMemberReceiver(expr, ts, model) {
  if (ts.isIdentifier(expr)) return expr.text === "Runtime" || model.enumAliases.has(expr.text);
  if (ts.isPropertyAccessExpression(expr) && expr.name.text === "Runtime") {
    // The original text model reads any `X.Runtime` receiver; keeping that
    // shape means an unrelated identifier cannot hide a declaration by
    // receiving it through a different namespace.
    return true;
  }
  if (ts.isElementAccessExpression(expr) && stringLiteralValue(expr.argumentExpression, ts) === "Runtime") {
    return ts.isIdentifier(expr.expression) && model.namespaces.has(expr.expression.text);
  }
  return false;
}

// Collects every runtime declaration in a surface's text, in source order, as
// { line, form, identifier, enumName, literal }. `identifier` is null when the
// form could not be resolved to a modelled runtime, and that is a declaration
// all the same: it is what makes an unmodelled name fail closed instead of
// dropping out of the count. `enumName` is null for literal forms and `literal`
// is null for enum forms. `form` is the receiver and member exactly as written,
// so a violation reports the idiom the surface actually used rather than the one
// the gate prefers.
//
// The source is parsed with the repository's pinned TypeScript compiler API, not
// with a comment stripper. Comments are trivia, so a commented-out declaration
// is invisible while a comment between `fromString(` and its argument is not;
// regex literals and template expressions are scanned as syntax, so a `//` or
// `/*` inside a character class cannot blank the rest of the file. A source the
// parser rejects fails closed instead of being scanned as a recovered tree.
//
// Declarations are de-duplicated by the runtime they resolve to, falling back
// to the member access as written when there is no runtime to resolve. That is
// what keeps `lambda.Runtime.PROVIDED_AL2023.bundlingImage` from being counted
// as a second `lambda.Runtime.PROVIDED_AL2023` declaration: both resolve to
// provided.al2023. Two genuinely different declarations are never merged,
// because two different runtimes, two different unreadable members, or two
// different unmodelled literals resolve to two different keys.
function collectDeclarations(label, text) {
  const ts = typescript();
  const sourceFile = ts.createSourceFile(
    label,
    text,
    ts.ScriptTarget.Latest,
    true,
    scriptKindFor(label, ts),
  );
  const diagnostics = sourceFile.parseDiagnostics ?? [];
  if (diagnostics.length > 0) {
    const first = diagnostics[0];
    const position = sourceFile.getLineAndCharacterOfPosition(first.start ?? 0);
    const message = ts.flattenDiagnosticMessageText(first.messageText, " ");
    throw new GateFailure(
      `${label}:${position.line + 1} could not be parsed as TypeScript (${message}); ` +
        "a recovered tree is not a scanned surface",
    );
  }

  const model = collectRuntimeReceiverModel(sourceFile, ts);
  const byKey = new Map();

  const record = (node, form, enumName, literal, identifier) => {
    const key = identifier === null ? `form:${form}` : `runtime:${identifier}`;
    if (byKey.has(key)) return;
    const index = node.getStart(sourceFile);
    byKey.set(key, {
      index,
      line: sourceFile.getLineAndCharacterOfPosition(index).line + 1,
      form,
      enumName,
      literal,
      identifier,
    });
  };

  const visit = (node) => {
    if (
      ts.isPropertyAccessExpression(node) &&
      /^[A-Z][A-Z0-9_]*$/.test(node.name.text) &&
      isRuntimeMemberReceiver(node.expression, ts, model)
    ) {
      record(
        node,
        `${node.expression.getText(sourceFile)}.${node.name.text}`,
        node.name.text,
        null,
        LAMBDA_RUNTIME_ENUMS.get(node.name.text) ?? null,
      );
    }

    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === "fromString" &&
      isRuntimeMemberReceiver(node.expression.expression, ts, model)
    ) {
      const literal = node.arguments.length > 0 ? stringLiteralValue(node.arguments[0], ts) : null;
      if (literal !== null) {
        record(
          node,
          `${node.expression.expression.getText(sourceFile)}.fromString("${literal}")`,
          null,
          literal,
          runtimeIdentifierFromLiteral(literal),
        );
      }
    }

    ts.forEachChild(node, visit);
  };
  visit(sourceFile);

  return [...byKey.values()].sort((left, right) => left.index - right.index);
}

// === template and deploy-snippet declarations ==============================

// A `.ts` file is a stack, a `.yaml` file is a template, a `.md` file is a
// snippet. Anything else is not a surface class this gate models.
const CDK_SURFACE_EXTENSIONS = new Set([".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts"]);
const TEMPLATE_SURFACE_EXTENSIONS = new Set([".yaml", ".yml", ".json"]);
const SNIPPET_SURFACE_EXTENSIONS = new Set([".md", ".markdown"]);

// A `Runtime:` key at the start of a snippet line. The key may be quoted because
// a fenced YAML block inside a doc writes it that way, and requiring the key to
// start the line is what keeps a commented-out `# Runtime: ...` from being a
// declaration. Templates are read structurally, not with this rule.
const TEMPLATE_RUNTIME_KEY_RE = /^[ \t]*["']?(?:Runtime|runtime)["']?[ \t]*:[ \t]*(.*)$/;

// The `--runtime` flag and the rest of its line. The delimiter is whitespace or
// `=`, so the `--runtime-version` and `--runtime-import` flags this repository
// also uses can never be read as a runtime declaration.
const CLI_RUNTIME_FLAG_RE = /--runtime(?:[ \t]+|=)(.*)$/;

// A fenced code block delimiter. Snippets are read only inside a fence, so a
// runtime named in prose is documentation rather than a declaration.
const FENCE_RE = /^[ \t]*(?:`{3,}|~{3,})/;

// The literal a fenced snippet value names, or null when the value is not a
// readable literal. Quotes are honoured so a quoted scalar keeps its content; an
// unquoted scalar loses a trailing inline comment, separator, or line
// continuation. A tag (`!Ref`, `!Sub`), an interpolation, or a collection
// resolves at deploy time rather than here, so it declares nothing. Templates
// use the structural parser above instead.
function templateScalar(value) {
  const text = value.trim();
  if (text === "") return null;
  const quoted = /^(["'])(.*?)\1/.exec(text);
  if (quoted !== null) return quoted[2];
  const unquoted = text
    .replace(/[ \t]+#.*$/, "")
    .replace(/[\\,]+$/, "")
    .trim();
  if (unquoted === "") return null;
  if (unquoted.startsWith("!") || unquoted.startsWith("{") || unquoted.startsWith("[")) return null;
  if (unquoted.includes("${")) return null;
  return unquoted;
}

// The value of a `--runtime` flag: a quoted string, or the single bare token up
// to the next whitespace, so the flags that follow on the same line are never
// read as part of the value. A trailing comma or line continuation is dropped.
function cliFlagValue(rest) {
  const text = rest.trim();
  if (text === "") return null;
  const quoted = /^(["'])(.*?)\1/.exec(text);
  if (quoted !== null) return quoted[2];
  const token = /^[^\s]+/.exec(text);
  if (token === null) return null;
  const value = token[0].replace(/[\\,]+$/, "");
  return value === "" ? null : value;
}

// Records one template or snippet declaration, de-duplicated by the runtime it
// resolves to so a template counts runtimes rather than occurrences - the same
// rule the CDK collector applies.
function templateDeclarationCollector(text) {
  const byKey = new Map();
  return {
    record(index, literal, form = `Runtime: ${literal}`) {
      const identifier = runtimeIdentifierFromLiteral(literal) ?? literal;
      const key = `runtime:${identifier}`;
      if (byKey.has(key)) return;
      byKey.set(key, {
        index,
        line: lineOf(text, index),
        form,
        enumName: null,
        literal,
        identifier,
      });
    },
    declarations() {
      return [...byKey.values()].sort((left, right) => left.index - right.index);
    },
  };
}

// CloudFormation's YAML shorthand tags. They are syntax, not scalar values: a
// runtime of `!Ref` or `!Sub` resolves at deploy time. The list is explicit, so
// an unknown tag is a parse error rather than an invisible value.
const CLOUDFORMATION_TAG_NAMES = [
  "And",
  "Base64",
  "Cidr",
  "Condition",
  "Equals",
  "FindInMap",
  "ForEach",
  "GetAtt",
  "GetAZs",
  "If",
  "ImportValue",
  "Join",
  "Not",
  "Or",
  "Ref",
  "Select",
  "Split",
  "Sub",
  "ToJsonString",
  "Transform",
];

let cachedCloudFormationTags = null;

function cloudFormationTags() {
  if (cachedCloudFormationTags === null) {
    cachedCloudFormationTags = CLOUDFORMATION_TAG_NAMES.map((name) => ({
      tag: `!${name}`,
      resolve: (value) => ({ cloudFormationTag: name, value }),
    }));
  }
  return cachedCloudFormationTags;
}

// A file that fails to parse is only ignored when it is not a template at all.
// A declared surface, a CloudFormation marker, or a Runtime key means the parse
// failure is ambiguity this gate must not read as "nothing declared".
function looksLikeCloudFormationTemplate(text) {
  return (
    /(^|\n)\s*(?:AWSTemplateFormatVersion|Transform)\s*:/.test(text) ||
    /(^|\n)\s*["']?Resources["']?\s*:/.test(text) ||
    /(^|\n)\s*["']?Runtime["']?\s*:/.test(text)
  );
}

function parseTemplateDocument(label, text) {
  const isJson = path.extname(label).toLowerCase() === ".json";
  const declared = SCANNED_TEMPLATE_SURFACES.includes(label);

  if (isJson) {
    // Strict JSON first: the YAML parser is a superset and would otherwise
    // accept JSON that is not JSON, hiding a malformed template.
    try {
      JSON.parse(text);
    } catch (err) {
      if (declared || looksLikeCloudFormationTemplate(text)) {
        throw new GateFailure(`${label} could not be parsed as JSON: ${err.message}`);
      }
      return null;
    }
  }

  const YAML = yamlParser();
  let document;
  try {
    document = YAML.parseDocument(text, {
      customTags: cloudFormationTags(),
      prettyErrors: false,
      strict: true,
    });
  } catch (err) {
    if (declared || looksLikeCloudFormationTemplate(text)) {
      throw new GateFailure(
        `${label} could not be parsed as ${isJson ? "JSON" : "YAML"}: ${err.message}`,
      );
    }
    return null;
  }

  if (document.errors.length > 0) {
    if (declared || looksLikeCloudFormationTemplate(text)) {
      throw new GateFailure(
        `${label} could not be parsed as ${isJson ? "JSON" : "YAML"}: ${document.errors[0].message}`,
      );
    }
    return null;
  }

  return document;
}

function yamlMapGet(node, key) {
  const YAML = yamlParser();
  if (!YAML.isMap(node)) return undefined;
  for (const pair of node.items) {
    if (YAML.isScalar(pair.key) && pair.key.value === key) return pair.value;
  }
  return undefined;
}

function yamlScalarValue(node) {
  const YAML = yamlParser();
  if (node === undefined || !YAML.isScalar(node)) return null;
  if (node.tag !== undefined && !node.tag.startsWith("tag:yaml.org,2002:")) return null;
  return typeof node.value === "string" ? node.value : null;
}

function yamlNodeOffset(node) {
  return node !== undefined && Array.isArray(node.range) ? node.range[0] : 0;
}

// The value a Runtime node names, or the reason it cannot be read here. A
// CloudFormation tag, an interpolation, a collection, and a non-string scalar
// all resolve only at deploy time, so an effective runtime in one of those forms
// fails closed rather than being skipped.
function templateRuntimeValue(node) {
  const YAML = yamlParser();
  if (node === undefined) return { kind: "missing" };
  if (!YAML.isScalar(node)) return { kind: "dynamic", detail: "a collection" };
  if (node.tag !== undefined && !node.tag.startsWith("tag:yaml.org,2002:")) {
    return { kind: "dynamic", detail: `the CloudFormation tag ${node.tag}` };
  }
  if (typeof node.value !== "string") {
    return {
      kind: "dynamic",
      detail: node.value === null ? "a null value" : `a ${typeof node.value} value`,
    };
  }
  const text = node.value.trim();
  if (text === "") return { kind: "dynamic", detail: "an empty scalar" };
  if (text.includes("${")) return { kind: "dynamic", detail: "an interpolation" };
  return { kind: "literal", literal: text };
}

// Parses a template structurally and records the EFFECTIVE runtime of each
// Lambda-bearing resource: a resource-level Runtime overrides the SAM
// Globals.Function.Runtime that would otherwise apply. A global runtime is only
// judged when it reaches at least one resource (or when the template has no
// runtime-bearing resources, which keeps a Globals-only fixture meaningful).
// Every resource is inspected independently of whether another declaration
// parsed, so one bad resource cannot hide behind a good one.
function collectTemplateDeclarations(text, label) {
  const document = parseTemplateDocument(label, text);
  if (document === null) return [];

  const YAML = yamlParser();
  const root = document.contents;
  if (!YAML.isMap(root)) return [];

  const collector = templateDeclarationCollector(text);
  const recordEffective = (node, pathLabel) => {
    const value = templateRuntimeValue(node);
    if (value.kind === "literal") {
      collector.record(yamlNodeOffset(node), value.literal);
      return;
    }
    if (value.kind === "dynamic") {
      const line = lineOf(text, yamlNodeOffset(node));
      throw new GateFailure(
        `${label}:${line} ${pathLabel} declares no modelled Lambda runtime: it is ${value.detail}, ` +
          "which resolves only at deploy time",
      );
    }
  };

  const globals = yamlMapGet(root, "Globals");
  const globalRuntime = yamlMapGet(yamlMapGet(globals, "Function"), "Runtime");
  const resources = yamlMapGet(root, "Resources");

  let runtimeBearingResources = 0;
  let reliesOnGlobal = false;
  if (YAML.isMap(resources)) {
    for (const pair of resources.items) {
      const resource = pair.value;
      if (!YAML.isMap(resource)) continue;
      const resourceType = yamlScalarValue(yamlMapGet(resource, "Type"));
      const properties = yamlMapGet(resource, "Properties");
      const explicitRuntime = yamlMapGet(properties, "Runtime");
      const packageType = yamlScalarValue(yamlMapGet(properties, "PackageType"));
      const isFunction =
        resourceType === "AWS::Serverless::Function" || resourceType === "AWS::Lambda::Function";
      if (explicitRuntime === undefined && !isFunction) continue;

      runtimeBearingResources += 1;
      if (explicitRuntime !== undefined) {
        const resourceName = YAML.isScalar(pair.key) ? pair.key.value : "?";
        recordEffective(explicitRuntime, `Resources.${resourceName}.Properties.Runtime`);
      } else if (packageType === "Image") {
        // An image-packaged function carries no Lambda runtime.
      } else {
        reliesOnGlobal = true;
      }
    }
  }

  if (globalRuntime !== undefined && (runtimeBearingResources === 0 || reliesOnGlobal)) {
    recordEffective(globalRuntime, "Globals.Function.Runtime");
  }

  return collector.declarations();
}

// Walks a Markdown file and reads only fenced code. A shell comment inside a
// fence is skipped, so `# --runtime provided.al2` documents nothing.
function collectSnippetDeclarations(text) {
  const collector = templateDeclarationCollector(text);
  let inFence = false;
  let offset = 0;
  for (const lineText of text.split("\n")) {
    if (FENCE_RE.test(lineText)) {
      inFence = !inFence;
      offset += lineText.length + 1;
      continue;
    }
    if (inFence && !lineText.trim().startsWith("#")) {
      const keyMatch = TEMPLATE_RUNTIME_KEY_RE.exec(lineText);
      const flagMatch = CLI_RUNTIME_FLAG_RE.exec(lineText);
      if (keyMatch !== null) {
        const literal = templateScalar(keyMatch[1]);
        if (literal !== null) collector.record(offset + lineText.indexOf(":"), literal);
      } else if (flagMatch !== null) {
        const literal = cliFlagValue(flagMatch[1]);
        if (literal !== null) {
          collector.record(
            offset + lineText.indexOf("--runtime"),
            literal,
            `--runtime ${literal}`,
          );
        }
      }
    }
    offset += lineText.length + 1;
  }
  return collector.declarations();
}

// === surface dispatch ======================================================

// The surface class a path belongs to, decided by extension. A file in a scan
// root whose extension is not modelled below is not a surface this gate reads.
function surfaceKind(label) {
  const extension = path.extname(label).toLowerCase();
  if (CDK_SURFACE_EXTENSIONS.has(extension)) return "cdk";
  if (TEMPLATE_SURFACE_EXTENSIONS.has(extension)) return "template";
  if (SNIPPET_SURFACE_EXTENSIONS.has(extension)) return "snippet";
  return null;
}

function collectDeclarationsFor(label, text) {
  switch (surfaceKind(label)) {
    case "template":
      return collectTemplateDeclarations(text, label);
    case "snippet":
      return collectSnippetDeclarations(text);
    default:
      return collectDeclarations(label, text);
  }
}

// === classification ========================================================

// Classifies one declaration. Returns a violation object, or null when the
// declaration is a supported pinned runtime.
function classifyDeclaration(declaration) {
  if (declaration.enumName !== null && UNPINNED_RUNTIME_ALIAS_ENUMS.has(declaration.enumName)) {
    return {
      kind: "unpinned-alias",
      line: declaration.line,
      form: declaration.form,
      detail:
        `names the moving alias lambda.Runtime.${declaration.enumName}, which follows the CDK ` +
        `version instead of pinning a runtime`,
    };
  }
  if (declaration.identifier === null) {
    // Two ways to be unreadable, and both fail closed on their own reason: an
    // enum name this gate has no mapping for, and a `fromString` argument that
    // resolves to no modelled runtime at all. The second is the one an
    // unmodelled family used to escape through, so it is named as a literal
    // rather than reported as an enum with a null name.
    if (declaration.literal !== null) {
      return {
        kind: "unmodelled-literal",
        line: declaration.line,
        form: declaration.form,
        detail:
          `passes '${declaration.literal}' to fromString, which resolves to no runtime this gate ` +
          `models: its family is not in LAMBDA_RUNTIME_FAMILIES, or it carries no version; ` +
          `${UNMODELLED_DECLARATION_HINT}`,
      };
    }
    return {
      kind: "unmodelled-enum",
      line: declaration.line,
      form: declaration.form,
      detail:
        `names the runtime enum ${declaration.enumName}, which ` +
        `scripts/check-lambda-runtime-deprecations.mjs does not model; ${UNMODELLED_DECLARATION_HINT}`,
    };
  }
  if (DEPRECATED_LAMBDA_RUNTIMES.includes(declaration.identifier)) {
    return {
      kind: "deprecated",
      line: declaration.line,
      form: declaration.form,
      detail: `declares '${declaration.identifier}', which AWS has deprecated`,
    };
  }
  if (SUPPORTED_LAMBDA_RUNTIMES.includes(declaration.identifier)) return null;
  return {
    kind: "unmodelled-runtime",
    line: declaration.line,
    form: declaration.form,
    detail:
      `declares '${declaration.identifier}', which is neither a deprecated runtime nor a ` +
      `supported pinned runtime; ${UNMODELLED_DECLARATION_HINT}`,
  };
}

// Classifies a template or snippet declaration. The value is already the literal
// the surface wrote, so there is no receiver or enum to unwrap: a value in the
// deprecated set fails, a value in the supported set passes, and a value in
// neither fails closed as unmodelled rather than being skipped.
function classifyRuntimeValue(declaration) {
  if (DEPRECATED_LAMBDA_RUNTIMES.includes(declaration.identifier)) {
    return {
      kind: "deprecated",
      line: declaration.line,
      form: declaration.form,
      detail: `declares '${declaration.identifier}', which AWS has deprecated`,
    };
  }
  if (SUPPORTED_LAMBDA_RUNTIMES.includes(declaration.identifier)) return null;
  return {
    kind: "unmodelled-runtime",
    line: declaration.line,
    form: declaration.form,
    detail:
      `declares '${declaration.identifier}', which is neither a deprecated runtime nor a ` +
      `supported pinned runtime; ${UNMODELLED_DECLARATION_HINT}`,
  };
}

function classifyDeclarationFor(label, declaration) {
  return surfaceKind(label) === "cdk" ? classifyDeclaration(declaration) : classifyRuntimeValue(declaration);
}

// === scan ==================================================================

function resolveSurface(label) {
  return path.isAbsolute(label) ? label : path.join(repositoryRoot, label);
}

function scanSurface(label) {
  const absolute = resolveSurface(label);
  let text;
  try {
    text = fs.readFileSync(absolute, "utf8");
  } catch (err) {
    throw new GateFailure(`could not read scanned surface ${label}: ${err.message}`);
  }
  const declarations = collectDeclarationsFor(label, text);
  if (declarations.length === 0) {
    throw new GateFailure(
      `${label} declares no modelled Lambda runtime; a scanned surface must still declare one, ` +
        `so the gate cannot be neutered by deleting or obfuscating its runtime`,
    );
  }
  return {
    label,
    declarationCount: declarations.length,
    declarations,
    violations: declarations
      .map((declaration) => classifyDeclarationFor(label, declaration))
      .filter((violation) => violation !== null)
      .map((violation) => ({ ...violation, label })),
  };
}

// Every file inside a scan root that declares a Lambda runtime, per surface
// class, whether or not it is a declared surface. Used to fail closed on an
// unmodelled surface.
//
// The three classes share one walk because they share roots: `examples` is a
// template root and a snippet root, so walking each root set separately would
// read the same tree twice. A file is bucketed by its extension, not by which
// root reached it, and a file whose extension this gate does not model is not a
// surface at all.
function collectDeclarationSurfaces() {
  const found = { cdk: new Set(), template: new Set(), snippet: new Set() };

  const visit = (absolute) => {
    const entries = fs.readdirSync(absolute, { withFileTypes: true });
    for (const entry of entries) {
      const child = path.join(absolute, entry.name);
      if (entry.isDirectory()) {
        if (SKIPPED_DIRECTORY_NAMES.has(entry.name)) continue;
        visit(child);
        continue;
      }
      if (!entry.isFile()) continue;
      const label = path.relative(repositoryRoot, child);
      const kind = surfaceKind(label);
      if (kind === null) continue;
      let text;
      try {
        text = fs.readFileSync(child, "utf8");
      } catch {
        continue;
      }
      if (collectDeclarationsFor(label, text).length > 0) {
        found[kind].add(label);
      }
    }
  };

  const walked = new Set();
  for (const root of [...SCAN_ROOTS, ...TEMPLATE_SCAN_ROOTS, ...SNIPPET_SCAN_ROOTS]) {
    const absolute = path.join(repositoryRoot, root);
    if (!fs.existsSync(absolute)) {
      throw new GateFailure(`scan root ${root} is missing; the coverage scope cannot be walked`);
    }
    if (walked.has(root)) continue;
    walked.add(root);
    visit(absolute);
  }

  return {
    cdk: [...found.cdk].sort(),
    template: [...found.template].sort(),
    snippet: [...found.snippet].sort(),
  };
}

function describeViolation(violation) {
  return `${violation.label}:${violation.line} ${violation.form} ${violation.detail}`;
}

// === self-test =============================================================

// [text, expected violation kinds] - driven through the real read path.
const CLASSIFIER_CASES = [
  {
    name: "supported-pinned-nodejs-runtime",
    text: "const fn = new NodejsFunction(this, 'Fn', {\n  runtime: lambda.Runtime.NODEJS_24_X,\n});\n",
    expected: [],
  },
  {
    name: "supported-floor-nodejs-runtime",
    text: "runtime: lambda.Runtime.NODEJS_22_X,\n",
    expected: [],
  },
  {
    name: "deprecated-nodejs-runtime",
    text: "runtime: lambda.Runtime.NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-nodejs-runtime-older-than-the-minimum",
    text: "runtime: lambda.Runtime.NODEJS_4_3,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-nodejs-literal",
    text: "runtime: lambda.Runtime.fromString('nodejs18.x'),\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-nodejs-literal",
    text: "runtime: lambda.Runtime.fromString('nodejs24.x'),\n",
    expected: [],
  },
  // --- the non-nodejs families TableTheory declares -------------------------
  {
    name: "supported-python-runtime",
    text: "runtime: lambda.Runtime.PYTHON_3_14,\n",
    expected: [],
  },
  {
    name: "deprecated-python-runtime",
    text: "runtime: lambda.Runtime.PYTHON_3_8,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-python-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('python3.9'),\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-python-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('python3.13'),\n",
    expected: [],
  },
  {
    name: "supported-provided-al2023-runtime",
    text: "runtime: lambda.Runtime.PROVIDED_AL2023,\n",
    expected: [],
  },
  {
    name: "deprecated-provided-al2-runtime",
    text: "runtime: lambda.Runtime.PROVIDED_AL2,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-provided-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('provided.al2'),\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-provided-al2023-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('provided.al2023'),\n",
    expected: [],
  },
  // A family the gate has no data for is a declaration that fails closed, not
  // one that is invisible. The modelled family with an unmodelled version
  // (`ruby3.2`) resolves to an identifier and fails as an unmodelled runtime;
  // the family outside the list (`rust1.0`) resolves to no runtime at all and
  // fails as an unmodelled literal. Before the literal rule was scoped to
  // `fromString` on a Runtime receiver the second case declared nothing at all -
  // the argument was dropped - so an unmodelled family was invisible rather than
  // judged.
  {
    name: "unmodelled-family-literal",
    text: "runtime: lambda.Runtime.fromString('ruby3.2'),\n",
    expected: ["unmodelled-runtime"],
  },
  {
    name: "unmodelled-family-literal-outside-the-family-list",
    text: "runtime: lambda.Runtime.fromString('rust1.0'),\n",
    expected: ["unmodelled-literal"],
  },
  // A runtime identifier needs a version component, and the deprecated set has
  // an entry without one. `provided` is a real AWS runtime that this gate has no
  // version to resolve, so the argument fails closed as an unmodelled literal
  // rather than being read as the deprecated `provided` identifier.
  {
    name: "runtime-literal-without-a-version",
    text: "runtime: lambda.Runtime.fromString('provided'),\n",
    expected: ["unmodelled-literal"],
  },
  // The other direction: a runtime-shaped string that is not a `fromString`
  // argument is not a declaration, so it cannot be judged in place of the
  // dynamic runtime beside it. Before the fix this surface was reported as
  // declaring the stray literal, which is a violation the surface never wrote.
  {
    name: "stray-runtime-literal-is-not-a-declaration",
    text:
      "const first = { runtime: lambda.Runtime.fromString(config.runtime) };\n" +
      "const releaseNote = 'nodejs18.x';\n" +
      "const second = { runtime: lambda.Runtime.NODEJS_24_X };\n",
    expected: [],
  },
  {
    name: "unmodelled-family-enum",
    text: "runtime: lambda.Runtime.RUBY_3_2,\n",
    expected: ["unmodelled-enum"],
  },
  // A runtime AWS lists as preview is not in the supported set, so it fails
  // closed until an editor admits it deliberately.
  {
    name: "preview-runtime-literal-fails-closed",
    text: "runtime: lambda.Runtime.fromString('nodejs26.x'),\n",
    expected: ["unmodelled-runtime"],
  },
  // --- property reads on a runtime are not a second declaration -------------
  {
    name: "bundling-image-read-is-not-a-second-declaration",
    text: "runtime: lambda.Runtime.PROVIDED_AL2023,\ncode: lambda.Code.fromAsset(dir, {\n  bundling: { image: lambda.Runtime.PROVIDED_AL2023.bundlingImage },\n}),\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
  {
    name: "bundling-image-read-does-not-hide-a-deprecated-declaration",
    text: "runtime: lambda.Runtime.NODEJS_18_X,\ncode: lambda.Code.fromAsset(dir, {\n  bundling: { image: lambda.Runtime.NODEJS_18_X.bundlingImage },\n}),\n",
    expected: ["deprecated"],
    expectedDeclarations: ["nodejs18.x"],
  },
  {
    name: "unpinned-moving-alias",
    text: "runtime: lambda.Runtime.NODEJS_LATEST,\n",
    expected: ["unpinned-alias"],
  },
  {
    name: "every-declaration-is-judged",
    text: "runtime: lambda.Runtime.NODEJS_24_X,\nruntime: lambda.Runtime.NODEJS_20_X,\nruntime: lambda.Runtime.PYTHON_3_9,\n",
    expected: ["deprecated", "deprecated"],
  },
  {
    name: "quoted-strings-that-are-not-runtimes",
    text: 'const first = { runtime: lambda.Runtime.NODEJS_24_X };\nconst a = "node_modules";\nconst b = "assets";\nconst c = "go.mod";\nconst d = "vite";\nconst e = "1.2.3";\nconst f = "node24";\n',
    expected: [],
  },
  {
    name: "deprecated-runtime-through-a-named-import",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\n\nnew lambda.Function(this, 'Fn', {\n  runtime: Runtime.NODEJS_20_X,\n});\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-a-named-import",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\nruntime: Runtime.NODEJS_24_X,\n",
    expected: [],
  },
  {
    name: "deprecated-python-runtime-through-a-named-import",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\nruntime: Runtime.PYTHON_3_8,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-runtime-through-an-aliased-namespace",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nruntime: R.NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-an-aliased-namespace",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nruntime: R.PROVIDED_AL2023,\n",
    expected: [],
  },
  {
    name: "deprecated-runtime-through-a-renamed-named-import",
    text: "import { Runtime as R } from 'aws-cdk-lib/aws-lambda';\nruntime: R.NODEJS_18_X,\n",
    expected: ["deprecated"],
  },
  // The three receiver idioms the alternation gained. Each of these declared
  // nothing before, in the classifier and in the coverage walk alike, so a
  // surface written this way passed the gate without ever being judged - and
  // each pair pins the idiom in both directions.
  {
    name: "deprecated-runtime-through-a-second-hop-alias",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nconst R2 = R;\nruntime: R2.NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-a-second-hop-alias",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nconst R2 = R;\nruntime: R2.NODEJS_24_X,\n",
    expected: [],
  },
  {
    name: "deprecated-runtime-through-a-quoted-index",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nruntime: lambda['Runtime'].NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-a-quoted-index",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nruntime: lambda['Runtime'].NODEJS_24_X,\n",
    expected: [],
  },
  {
    name: "deprecated-runtime-through-a-renamed-destructure",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst { Runtime: RT } = lambda;\nruntime: RT.NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-a-renamed-destructure",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst { Runtime: RT } = lambda;\nruntime: RT.NODEJS_24_X,\n",
    expected: [],
  },
  // A comment between `fromString(` and its argument must not hide the
  // declaration, in either comment form. The original hostile wrote the block
  // form in code position inside an otherwise-clean surface.
  {
    name: "deprecated-literal-behind-a-block-comment",
    text: "runtime: lambda.Runtime.fromString(/* pinned */ 'nodejs18.x'),\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-literal-behind-a-line-comment",
    text: "runtime: lambda.Runtime.fromString(// pinned\n  'nodejs18.x'),\n",
    expected: ["deprecated"],
  },
  {
    name: "unmodelled-literal-behind-a-block-comment",
    text: "runtime: lambda.Runtime.fromString(/* preview */ 'nodejs26.x'),\n",
    expected: ["unmodelled-runtime"],
  },
  // The ordinary shapes stay unaffected: a supported literal behind a comment, a
  // supported enum with a trailing comment, and a URL inside a string that must
  // not be read as a comment.
  {
    name: "supported-literal-behind-a-block-comment",
    text: "runtime: lambda.Runtime.fromString(/* keep */ 'nodejs24.x'),\n",
    expected: [],
  },
  {
    name: "supported-enum-with-a-trailing-line-comment",
    text: "runtime: lambda.Runtime.NODEJS_24_X, // current\n",
    expected: [],
  },
  {
    name: "url-string-before-a-supported-runtime",
    text: 'const endpoint = "https://runtime.example.com/path";\nruntime: lambda.Runtime.NODEJS_24_X,\n',
    expected: [],
  },
  // --- regex, division, and template expressions are syntax -----------------
  // A character class can contain the two characters a comment stripper treats
  // as a comment opener. Scanning syntax, not slices of text, keeps the
  // declaration beside the regex visible.
  {
    name: "regex-character-class-markers-do-not-hide-a-declaration",
    text: "const marker = /[//]/; const re = /[/*]/; const fn = { runtime: lambda.Runtime.NODEJS_18_X };\n",
    expected: ["deprecated"],
  },
  {
    name: "regex-character-class-markers-preserve-a-supported-runtime",
    text: "const marker = /[//]/; const re = /[/*]/; const fn = { runtime: lambda.Runtime.NODEJS_24_X };\n",
    expected: [],
  },
  // Ordinary division is not a comment opener and declares nothing by itself.
  {
    name: "division-before-a-declaration",
    text: "const ratio = total / count; const fn = { runtime: lambda.Runtime.NODEJS_20_X };\n",
    expected: ["deprecated"],
  },
  // A runtime expression inside a template interpolation is executable code.
  {
    name: "template-expression-is-scanned-as-syntax",
    text: "const note = `${lambda.Runtime.NODEJS_18_X}`; const fn = { runtime: lambda.Runtime.NODEJS_24_X };\n",
    expected: ["deprecated"],
  },
  // --- templates: SAM / CloudFormation runtime scalars ----------------------
  // The class the first version of this gate could not see: a SAM Globals runtime
  // is a YAML scalar, not a CDK enum, so no `runtime:` line was ever judged.
  {
    name: "deprecated-provided-al2-in-sam-globals",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: provided.al2\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-provided-al2023-in-sam-globals",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: provided.al2023\n",
    expected: [],
  },
  {
    name: "deprecated-nodejs-runtime-in-a-template",
    file: "template.yml",
    text: "Resources:\n  Function:\n    Type: AWS::Serverless::Function\n    Properties:\n      Runtime: nodejs18.x\n",
    expected: ["deprecated"],
  },
  {
    name: "quoted-deprecated-runtime-in-a-template",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: \"provided.al2\"\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-runtime-with-an-inline-comment",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: provided.al2  # pinned for the legacy lane\n",
    expected: ["deprecated"],
  },
  {
    name: "unmodelled-runtime-in-a-template",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: ruby3.2\n",
    expected: ["unmodelled-runtime"],
  },
  // A commented-out key is not a declaration, and the live key beside it is.
  {
    name: "commented-out-template-runtime-is-not-a-declaration",
    file: "template.yaml",
    text: "# Runtime: provided.al2\nGlobals:\n  Function:\n    Runtime: provided.al2023\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
  {
    name: "deprecated-runtime-in-cloudformation-json",
    file: "template.json",
    text: '{\n  "Resources": {\n    "Function": {\n      "Properties": {\n        "Runtime": "provided.al2"\n      }\n    }\n  }\n}\n',
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-in-cloudformation-json",
    file: "template.json",
    text:
      '{\n  "Resources": {\n    "Function": {\n      "Properties": {\n' +
      '        "Runtime": "provided.al2023"\n      }\n    }\n  }\n}\n',
    expected: [],
  },
  // Structural parsing sees flow YAML and compact JSON that never put the key
  // at the start of a line.
  {
    name: "flow-yaml-deprecated-runtime",
    file: "template.yaml",
    text: "{Globals: {Function: {Runtime: provided.al2}}}\n",
    expected: ["deprecated"],
  },
  {
    name: "compact-json-deprecated-runtime",
    file: "template.json",
    text: '{"Resources":{"Function":{"Properties":{"Runtime":"provided.al2"}}}}\n',
    expected: ["deprecated"],
  },
  {
    name: "compact-json-supported-runtime",
    file: "template.json",
    text: '{"Resources":{"Function":{"Properties":{"Runtime":"provided.al2023"}}}}\n',
    expected: [],
  },
  // The resource runtime is effective over the global. A supported global does
  // not hide a deprecated resource override, and a deprecated global that every
  // resource overrides is not itself deployed.
  {
    name: "supported-global-with-deprecated-resource-override",
    file: "template.yaml",
    text:
      "Globals:\n  Function:\n    Runtime: provided.al2023\n" +
      "Resources:\n  Fn:\n    Type: AWS::Serverless::Function\n    Properties:\n      Runtime: provided.al2\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-global-overridden-by-supported-resource",
    file: "template.yaml",
    text:
      "Globals:\n  Function:\n    Runtime: provided.al2\n" +
      "Resources:\n  Fn:\n    Type: AWS::Serverless::Function\n    Properties:\n      Runtime: provided.al2023\n",
    expected: [],
  },
  // A template that declares the same runtime twice counts one runtime, the same
  // de-duplication the CDK collector applies to a declaration and its property
  // read.
  {
    name: "template-repeated-runtime-counts-once",
    file: "template.yaml",
    text: "Resources:\n  A:\n    Properties:\n      Runtime: provided.al2023\n  B:\n    Properties:\n      Runtime: provided.al2023\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
  // --- deploy snippets: fenced `--runtime` flags and YAML blocks ------------
  {
    name: "deprecated-runtime-in-a-deploy-snippet",
    file: "README.md",
    text: "```bash\naws lambda create-function \\\n  --function-name demo \\\n  --runtime provided.al2 \\\n  --handler bootstrap\n```\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-in-a-deploy-snippet",
    file: "README.md",
    text: "```bash\naws lambda create-function \\\n  --function-name demo \\\n  --runtime provided.al2023 \\\n  --handler bootstrap\n```\n",
    expected: [],
  },
  {
    name: "deprecated-runtime-in-a-snippet-with-an-equals-sign",
    file: "README.md",
    text: "```bash\naws lambda update-function-configuration --runtime=provided.al2 --function-name demo\n```\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-runtime-in-a-yaml-block-inside-a-fence",
    file: "README.md",
    text: "~~~yaml\nGlobals:\n  Function:\n    Runtime: provided.al2\n~~~\n",
    expected: ["deprecated"],
  },
  // Prose is documentation, not a declaration: the same sentence outside a fence
  // names nothing, and the fenced flag beside it is what is judged.
  {
    name: "runtime-in-prose-is-not-a-declaration",
    file: "README.md",
    text: "Deploy with `--runtime provided.al2` if you must.\n\n```bash\naws lambda create-function --runtime provided.al2023 --handler bootstrap\n```\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
  {
    name: "commented-out-snippet-flag-is-not-a-declaration",
    file: "README.md",
    text: "```bash\n# --runtime provided.al2\naws lambda create-function --runtime provided.al2023 --handler bootstrap\n```\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
  // `--runtime-version` shares the `--runtime` prefix but is a different flag.
  {
    name: "runtime-version-flag-is-not-a-runtime-declaration",
    file: "README.md",
    text: "```bash\ntabletheory init --lang go --runtime-version 3.0.0\naws lambda create-function --runtime provided.al2023\n```\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
];

const FAIL_CLOSED_CASES = [
  {
    name: "surface-without-a-declaration",
    text: "export const nothing = 1;\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "surface-of-only-quoted-non-runtimes",
    text: 'const a = "node_modules";\nconst b = "1.2.3";\n',
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A source the parser rejects is not scanned as a recovered tree, even when
  // the recovered text still contains a supported runtime token.
  {
    name: "parse-error-fails-closed",
    text: "function ( {\nconst fn = { runtime: lambda.Runtime.NODEJS_24_X };\n",
    raw: true,
    expectFailureReason: "could not be parsed as TypeScript",
  },
  // Binding the namespace and then hiding the member is not a way to declare
  // nothing and still pass: the surface has no readable declaration, so it fails
  // closed on the same rule that catches an obfuscated surface.
  {
    name: "named-import-with-an-unreadable-member",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\nruntime: Runtime[legacyRuntimeName],\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "aliased-namespace-with-an-unreadable-member",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nruntime: R[process.env.TABLETHEORY_RUNTIME],\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A method call is not a member declaration: `R.fromString(runtimeName)` names
  // `fromString`, not a runtime, and the argument is not a literal, so the
  // surface ends up with no readable declaration and fails closed.
  {
    name: "aliased-namespace-calling-fromString-with-a-variable",
    text: "const R = lambda.Runtime;\nruntime: R.fromString(runtimeName),\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "dynamic-fromString-with-no-literal",
    text: "runtime: lambda.Runtime.fromString(process.env.LAMBDA_RUNTIME),\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // The false-declaration direction of the same rule. A dynamic `fromString`
  // declares nothing readable, and an unrelated runtime-shaped string must not
  // stand in for it. Before the literal rule was scoped, the stray literal WAS
  // the declaration, so this surface was judged as declaring a supported runtime
  // and passed.
  {
    name: "dynamic-fromString-rescued-by-a-stray-runtime-literal",
    text:
      "const first = { runtime: lambda.Runtime.fromString(config.runtime) };\n" +
      "const releaseNote = 'nodejs22.x';\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A declaration that is entirely commented out is not a declaration, in either
  // comment form: the surface is left with nothing readable and must fail closed.
  {
    name: "surface-with-only-a-line-commented-declaration",
    text: "// runtime: lambda.Runtime.NODEJS_20_X,\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "surface-with-only-a-block-commented-declaration",
    text: "/* runtime: lambda.Runtime.PYTHON_3_8, */\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A template whose only runtime is a deploy-time parameter declares nothing
  // readable, so it fails closed instead of passing on a guess about the value.
  {
    name: "template-with-only-a-parameterized-runtime",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: !Ref RuntimeParameter\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "template-with-only-an-interpolated-runtime",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: !Sub \"${RuntimeFamily}.al2023\"\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A supported global does not rescue a resource whose effective runtime is
  // ambiguous. The override is what deploys, so it fails closed.
  {
    name: "supported-global-with-dynamic-resource-override",
    file: "template.yaml",
    text:
      "Globals:\n  Function:\n    Runtime: provided.al2023\n" +
      "Resources:\n  Fn:\n    Type: AWS::Serverless::Function\n    Properties:\n      Runtime: !Ref RuntimeParameter\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "supported-global-with-json-ref-resource-override",
    file: "template.json",
    text:
      '{"Globals":{"Function":{"Runtime":"provided.al2023"}},' +
      '"Resources":{"Fn":{"Type":"AWS::Serverless::Function","Properties":{"Runtime":{"Ref":"RuntimeParameter"}}}}}\n',
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A malformed declared template is parse ambiguity, not an empty template.
  {
    name: "malformed-template-fails-closed",
    file: "template.yaml",
    text: "Globals:\n  Function:\n    Runtime: provided.al2023\nResources: [\n",
    expectFailureReason: "could not be parsed as YAML",
  },
  // A snippet that only names a different flag declares no runtime at all.
  {
    name: "snippet-with-only-a-runtime-version-flag",
    file: "README.md",
    text: "```bash\ntabletheory init --lang go --runtime-version 3.0.0\n```\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "snippet-with-only-a-commented-runtime",
    file: "README.md",
    text: "```bash\n# --runtime provided.al2\n```\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A runtime named in prose is documentation; a doc with only prose declares no
  // deployment at all.
  {
    name: "doc-with-only-a-prose-runtime",
    file: "README.md",
    text: "Deploy with `--runtime provided.al2` when you need it.\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
];

function selfTestViolationKinds(outcome) {
  return outcome.violations.map((violation) => violation.kind);
}

function parsesAsTypeScript(text) {
  const ts = typescript();
  const sourceFile = ts.createSourceFile("probe.ts", text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  return (sourceFile.parseDiagnostics ?? []).length === 0;
}

// Most classifier cases are object-property fragments (`runtime: ...`) rather
// than complete modules. They were always fed to a text scanner, so they were
// never required to parse. The AST scanner requires real syntax, so a fragment
// is wrapped as object-literal properties before it is written; a case that is
// already a complete module is left untouched. Mixed statement/property cases
// are written as complete modules instead.
function syntheticCaseSource(scenario) {
  if (scenario.raw) return scenario.text;
  if (parsesAsTypeScript(scenario.text)) return scenario.text;

  const lines = scenario.text.split("\n");
  const firstProperty = lines.findIndex((line) =>
    /^\s*(?:runtime|code|bundling|environment|timeout|memorySize)\s*:/.test(line),
  );
  if (firstProperty > 0) {
    const prelude = lines.slice(0, firstProperty).join("\n");
    const tail = lines.slice(firstProperty).join("\n");
    const wrapped = `${prelude}\nconst __probe = ({\n${tail}\n});\n`;
    if (parsesAsTypeScript(wrapped)) return wrapped;
  }

  const wrappedAll = `const __probe = ({\n${scenario.text}\n});\n`;
  if (parsesAsTypeScript(wrappedAll)) return wrappedAll;

  throw new GateFailure(
    `self-test case ${scenario.name} is not valid TypeScript after fragment normalization`,
  );
}

function writeSyntheticSurface(tempDir, scenario) {
  const file = path.join(tempDir, scenario.name, scenario.file ?? "surface.ts");
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const extension = path.extname(file).toLowerCase();
  const text =
    scenario.file === undefined || CDK_SURFACE_EXTENSIONS.has(extension)
      ? syntheticCaseSource(scenario)
      : scenario.text;
  fs.writeFileSync(file, text);
  return file;
}

// A declared-surface list is pinned against an independently restated copy, so
// narrowing the scope fails the self-test instead of quietly scanning less.
function scopePinFailures(actual, expected, description) {
  const matches =
    actual.length === expected.length && actual.every((entry, index) => entry === expected[index]);
  return {
    matches,
    failure: matches
      ? null
      : `${description} must be exactly ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`,
  };
}

function runSelfTest() {
  const failures = [];
  let classifierCases = 0;

  const scopeCases = 6;
  const scopePins = [
    {
      label: "scan scope",
      actual: SCANNED_SURFACES,
      expected: EXPECTED_SCANNED_SURFACES,
      description: "scanned surface set",
      summary: `surfaces ${SCANNED_SURFACES.length}`,
    },
    {
      label: "coverage scope",
      actual: SCAN_ROOTS,
      expected: EXPECTED_SCAN_ROOTS,
      description: "coverage scan roots",
      summary: `roots ${SCAN_ROOTS.length}`,
    },
    {
      label: "template scan scope",
      actual: SCANNED_TEMPLATE_SURFACES,
      expected: EXPECTED_SCANNED_TEMPLATE_SURFACES,
      description: "scanned template surface set",
      summary: `surfaces ${SCANNED_TEMPLATE_SURFACES.length}`,
    },
    {
      label: "template coverage scope",
      actual: TEMPLATE_SCAN_ROOTS,
      expected: EXPECTED_TEMPLATE_SCAN_ROOTS,
      description: "template coverage scan roots",
      summary: `roots ${TEMPLATE_SCAN_ROOTS.length}`,
    },
    {
      label: "snippet scan scope",
      actual: SCANNED_SNIPPET_SURFACES,
      expected: EXPECTED_SCANNED_SNIPPET_SURFACES,
      description: "scanned snippet surface set",
      summary: `surfaces ${SCANNED_SNIPPET_SURFACES.length}`,
    },
    {
      label: "snippet coverage scope",
      actual: SNIPPET_SCAN_ROOTS,
      expected: EXPECTED_SNIPPET_SCAN_ROOTS,
      description: "snippet coverage scan roots",
      summary: `roots ${SNIPPET_SCAN_ROOTS.length}`,
    },
  ];
  for (const pin of scopePins) {
    const { matches, failure } = scopePinFailures(pin.actual, pin.expected, pin.description);
    if (failure !== null) failures.push(failure);
    console.log(`  self-test ${pin.label}: ${matches ? pin.summary : "NOT THE EXPECTED SET"}`);
  }

  // The declared surface lists and the coverage walk must agree per class, or the
  // gate has an unmodelled surface of that class before it even starts scanning.
  try {
    const discovered = collectDeclarationSurfaces();
    for (const surfaceClass of [
      {
        kind: "cdk",
        roots: SCAN_ROOTS,
        declared: [...SCANNED_SURFACES].sort(),
        label: "coverage walk",
      },
      {
        kind: "template",
        roots: TEMPLATE_SCAN_ROOTS,
        declared: [...SCANNED_TEMPLATE_SURFACES].sort(),
        label: "template coverage walk",
      },
      {
        kind: "snippet",
        roots: SNIPPET_SCAN_ROOTS,
        declared: [...SCANNED_SNIPPET_SURFACES].sort(),
        label: "snippet coverage walk",
      },
    ]) {
      const found = discovered[surfaceClass.kind];
      const matches = found.join("\n") === surfaceClass.declared.join("\n");
      if (!matches) {
        failures.push(
          `coverage walk under ${JSON.stringify(surfaceClass.roots)} must find exactly the declared ` +
            `${surfaceClass.kind} surfaces; found ${JSON.stringify(found)}`,
        );
      }
      console.log(
        `  self-test ${surfaceClass.label}: ${matches ? `${found.length} surfaces` : "NOT THE DECLARED SET"}`,
      );
    }
  } catch (err) {
    failures.push(`coverage walk threw ${err.message}`);
    console.log("  self-test coverage walk: UNEXPECTED FAILURE");
  }

  // The two classification sets and the enum map cannot drift apart. Every enum
  // name the map can produce must land in exactly one set, so a runtime cannot
  // be named by the map without carrying a verdict. The reverse direction is
  // deliberately not required: `nodejs4.3-edge` is an AWS runtime CDK has no
  // enum member for, and it is still reachable as a literal.
  for (const [enumName, identifier] of LAMBDA_RUNTIME_ENUMS) {
    const deprecated = DEPRECATED_LAMBDA_RUNTIMES.includes(identifier);
    const supported = SUPPORTED_LAMBDA_RUNTIMES.includes(identifier);
    if (deprecated === supported) {
      failures.push(
        `enum ${enumName} names ${identifier}, which is ` +
          `${deprecated ? "both deprecated and supported" : "in neither DEPRECATED_LAMBDA_RUNTIMES nor SUPPORTED_LAMBDA_RUNTIMES"}`,
      );
    }
  }
  for (const identifier of DEPRECATED_LAMBDA_RUNTIMES) {
    if (SUPPORTED_LAMBDA_RUNTIMES.includes(identifier)) {
      failures.push(`identifier ${identifier} is in both the deprecated and the supported set`);
    }
  }

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "tabletheory-lambda-runtimes-"));
  try {
    for (const scenario of [...CLASSIFIER_CASES, ...FAIL_CLOSED_CASES]) {
      classifierCases += 1;
      const file = writeSyntheticSurface(tempDir, scenario);
      let outcome;
      try {
        outcome = scanSurface(file);
      } catch (err) {
        if (scenario.expectFailureReason === undefined) {
          failures.push(`synthetic surface ${scenario.name} threw ${err.message}`);
          console.log(`  self-test synthetic ${scenario.name}: UNEXPECTED FAILURE`);
          continue;
        }
        if (!(err instanceof GateFailure) || !err.message.includes(scenario.expectFailureReason)) {
          failures.push(
            `synthetic surface ${scenario.name} must fail closed with ` +
              `${JSON.stringify(scenario.expectFailureReason)}, got ${err.name}: ${err.message}`,
          );
          console.log(`  self-test synthetic ${scenario.name}: FAIL CLOSED FOR THE WRONG REASON`);
          continue;
        }
        console.log(
          `  self-test synthetic ${scenario.name}: FAIL CLOSED (${scenario.expectFailureReason})`,
        );
        continue;
      }
      if (scenario.expectFailureReason !== undefined) {
        failures.push(
          `synthetic surface ${scenario.name} must fail closed with ` +
            `${JSON.stringify(scenario.expectFailureReason)}, got ${JSON.stringify(selfTestViolationKinds(outcome))}`,
        );
        console.log(`  self-test synthetic ${scenario.name}: PASSED OPEN`);
        continue;
      }
      if (scenario.expectedDeclarations !== undefined) {
        const actualDeclarations = outcome.declarations.map(
          (declaration) => declaration.identifier ?? declaration.form,
        );
        const declarationsMatch =
          actualDeclarations.length === scenario.expectedDeclarations.length &&
          actualDeclarations.every((value, index) => value === scenario.expectedDeclarations[index]);
        if (!declarationsMatch) {
          failures.push(
            `synthetic surface ${scenario.name} expected declarations ` +
              `${JSON.stringify(scenario.expectedDeclarations)}, got ${JSON.stringify(actualDeclarations)}`,
          );
        }
      }
      const expected = scenario.expected;
      const actual = selfTestViolationKinds(outcome);
      const matched = actual.length === expected.length && actual.every((value, i) => value === expected[i]);
      if (!matched) {
        failures.push(
          `synthetic surface ${scenario.name} expected violations ${JSON.stringify(expected)}, ` +
            `got ${JSON.stringify(actual)}`,
        );
      }
      console.log(
        `  self-test synthetic ${scenario.name}: ${actual.length === 0 ? "PASS" : `FAIL ${actual.join("; ")}`}`,
      );
    }
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }

  const missingSurface = path.join(tempDir, "absent", "surface.ts");
  classifierCases += 1;
  try {
    scanSurface(missingSurface);
    failures.push("a missing scanned surface must fail closed, but the scan succeeded");
    console.log("  self-test missing surface: PASSED OPEN");
  } catch (err) {
    if (!(err instanceof GateFailure) || !err.message.includes("could not read scanned surface")) {
      failures.push(`missing scanned surface threw ${err.name}: ${err.message}`);
      console.log("  self-test missing surface: FAIL CLOSED FOR THE WRONG REASON");
    } else {
      console.log("  self-test missing surface: FAIL CLOSED (could not read scanned surface)");
    }
  }

  // The entry-point error boundary is in main(), so it can only be pinned by
  // running the entry point. This probe copies the checker into a scratch
  // repository whose scan root is absent, runs the bare path there, and asserts
  // the child reported this gate's own FAIL line: a coverage walk that cannot
  // run must never present itself as a passing scan, and an uncaught
  // GateFailure stack trace is not that FAIL line.
  //
  // The scratch repository gets no scan root at all, rather than an empty first
  // one followed by an absent second. That is the whole of the difference from
  // the FaceTheory checker this probe is ported from: SCAN_ROOTS here holds one
  // root, so that root is itself the absent one and the walk reaches it
  // immediately.
  const entryPointProbes = 1;
  try {
    const scratchRoot = fs.mkdtempSync(path.join(os.tmpdir(), "tabletheory-lambda-entry-point-"));
    try {
      const scratchScripts = path.join(scratchRoot, "scripts");
      fs.mkdirSync(scratchScripts, { recursive: true });
      fs.copyFileSync(
        fileURLToPath(import.meta.url),
        path.join(scratchScripts, "check-lambda-runtime-deprecations.mjs"),
      );
      const probe = spawnSync(
        process.execPath,
        [path.join(scratchScripts, "check-lambda-runtime-deprecations.mjs")],
        { encoding: "utf8" },
      );
      const output = `${probe.stdout ?? ""}${probe.stderr ?? ""}`;
      const reportedMissingRoot =
        /lambda-runtime-deprecations: FAIL \(scan root \S+ is missing/.test(output);
      const stackFrames = /\n\s+at /.test(output);
      if (probe.status !== 1 || !reportedMissingRoot || stackFrames) {
        failures.push(
          `an entry point whose coverage walk cannot run must report the gate FAIL line with no ` +
            `stack frames; got exit ${probe.status}, FAIL line ` +
            `${reportedMissingRoot ? "present" : "absent"}, stack frames ` +
            `${stackFrames ? "present" : "absent"}: ${output.trim()}`,
        );
        console.log("  self-test entry point without a coverage scan root: NOT A GATE FAILURE");
      } else {
        console.log(
          "  self-test entry point without a coverage scan root: FAIL CLOSED (gate FAIL line, no stack frames)",
        );
      }
    } finally {
      fs.rmSync(scratchRoot, { recursive: true, force: true });
    }
  } catch (err) {
    failures.push(`entry-point probe threw ${err.name}: ${err.message}`);
    console.log("  self-test entry point without a coverage scan root: UNEXPECTED FAILURE");
  }

  if (failures.length > 0) {
    for (const failure of failures) console.error(`  self-test: ${failure}`);
    throw new GateFailure(
      `self-test failed (${failures.length} failures across ${classifierCases} synthetic surfaces, ` +
        `${scopeCases} scope cases, and ${entryPointProbes} entry-point probe)`,
    );
  }

  return { classifierCases, scopeCases, entryPointProbes };
}

// === entry point ===========================================================

function main() {
  const args = process.argv.slice(2);
  const unsupported = args.filter((arg) => arg !== "--self-test");
  if (unsupported.length > 0) {
    fail(`unsupported arguments ${unsupported.join(" ")} (this gate takes no arguments)`);
  }
  const selfTest = args.includes("--self-test");
  // The self-test walks the real coverage scope, so a failure here is a gate
  // failure like any other and must report as a FAIL line rather than a stack
  // trace: an undeclared surface is exactly what it is meant to catch.
  let selfTestCounts = null;
  if (selfTest) {
    try {
      selfTestCounts = runSelfTest();
    } catch (err) {
      if (err instanceof GateFailure) fail(err.message);
      throw err;
    }
  }

  // A failure in the coverage walk (a missing scan root, an undeclared surface)
  // is a gate failure like any other and must report as a FAIL line rather than
  // a stack trace, so the walk and the scan share one error boundary.
  let outcomes;
  try {
    if (!selfTest) {
      // The coverage walk is what makes an unmodelled surface fail closed; it runs
      // on every real scan, not only under --self-test.
      const discovered = collectDeclarationSurfaces();
      const surfaceClasses = [
        { kind: "cdk", declared: SCANNED_SURFACES, list: "SCANNED_SURFACES" },
        { kind: "template", declared: SCANNED_TEMPLATE_SURFACES, list: "SCANNED_TEMPLATE_SURFACES" },
        { kind: "snippet", declared: SCANNED_SNIPPET_SURFACES, list: "SCANNED_SNIPPET_SURFACES" },
      ];
      for (const surfaceClass of surfaceClasses) {
        const undeclared = discovered[surfaceClass.kind].filter(
          (label) => !surfaceClass.declared.includes(label),
        );
        if (undeclared.length > 0) {
          fail(
            `unmodelled surface(s) declare a Lambda runtime but are not judged: ` +
              `${undeclared.join(", ")}; add each to ${surfaceClass.list} in ` +
              `scripts/check-lambda-runtime-deprecations.mjs - surfaces are never skipped`,
          );
        }
        for (const label of [...surfaceClass.declared].sort()) {
          if (!discovered[surfaceClass.kind].includes(label)) {
            fail(`declared surface ${label} declares no modelled Lambda runtime`);
          }
        }
      }
    }
    outcomes = [
      ...SCANNED_SURFACES,
      ...SCANNED_TEMPLATE_SURFACES,
      ...SCANNED_SNIPPET_SURFACES,
    ].map((label) => scanSurface(label));
  } catch (err) {
    if (err instanceof GateFailure) fail(err.message);
    throw err;
  }

  const violations = outcomes.flatMap((outcome) => outcome.violations);
  const declarationCount = outcomes.reduce((total, outcome) => total + outcome.declarationCount, 0);

  for (const outcome of outcomes) {
    console.log(`  ${outcome.label} declares ${outcome.declarationCount} Lambda runtime(s)`);
  }

  if (violations.length > 0) {
    for (const violation of violations) {
      console.error(`lambda-runtime-deprecations: ${describeViolation(violation)}`);
    }
    fail(
      `${violations.length} Lambda runtime declaration(s) are deprecated or unmodelled ` +
        `(${declarationCount} declarations across ${outcomes.length} surfaces)`,
    );
  }

  const selfTestSummary = selfTestCounts
    ? `self-test ${selfTestCounts.classifierCases} synthetic surfaces + ` +
      `${selfTestCounts.scopeCases} scope cases + ${selfTestCounts.entryPointProbes} entry-point probe; `
    : "";
  console.log(
    `lambda-runtime-deprecations: PASS (${selfTestSummary}surfaces ${outcomes.length} ` +
      `(cdk ${SCANNED_SURFACES.length}, template ${SCANNED_TEMPLATE_SURFACES.length}, ` +
      `snippet ${SCANNED_SNIPPET_SURFACES.length}); declarations ${declarationCount}; ` +
      `deprecated set ${DEPRECATED_LAMBDA_RUNTIMES.length}; ` +
      `supported set ${SUPPORTED_LAMBDA_RUNTIMES.length})`,
  );
}

const invokedDirectly =
  process.argv[1] !== undefined &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (invokedDirectly) main();

// The synthesized-template assertion reuses the same verdict sets, so a runtime
// added to this gate is judged the same way after `cdk synth`.
export {
  DEPRECATED_LAMBDA_RUNTIMES,
  SUPPORTED_LAMBDA_RUNTIMES,
  runtimeIdentifierFromLiteral,
};
